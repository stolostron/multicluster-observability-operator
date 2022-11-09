// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"

	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"

	"k8s.io/klog"
	"k8s.io/kubectl/pkg/util/slice"

	projectv1 "github.com/openshift/api/project/v1"
	userv1 "github.com/openshift/api/user/v1"
	proxyconfig "github.com/stolostron/multicluster-observability-operator/proxy/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/rewrite"

	"gopkg.in/yaml.v2"

	clusterclientset "open-cluster-management.io/api/client/cluster/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

const (
	managedClusterAPIPath = "/apis/cluster.open-cluster-management.io/v1/managedclusters"
	caPath                = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
)

var (
	allManagedClusterNames      map[string]string
	allManagedClusterLabelNames map[string]bool

	managedLabelListMutex = &sync.Mutex{}
)

// GetAllManagedClusterNames returns all managed cluster names
func GetAllManagedClusterNames() map[string]string {
	return allManagedClusterNames
}

// GetAllManagedClusterLabelNames returns all managed cluster labels
func GetAllManagedClusterLabelNames() map[string]bool {
	return allManagedClusterLabelNames
}

// InitAllManagedClusterNames initializes all managed cluster names map
func InitAllManagedClusterNames() {
	allManagedClusterNames = map[string]string{}
}

// InitAllManagedClusterLabelNames initializes all managed cluster labels map
func InitAllManagedClusterLabelNames() {
	allManagedClusterLabelNames = map[string]bool{}
}

// ModifyMetricsQueryParams will modify request url params for query metrics
func ModifyMetricsQueryParams(req *http.Request, reqUrl string) {
	userName := req.Header.Get("X-Forwarded-User")
	klog.V(1).Infof("user is %v", userName)
	klog.V(1).Infof("URL is: %s", req.URL)
	klog.V(1).Infof("URL path is: %v", req.URL.Path)
	klog.V(1).Infof("URL RawQuery is: %v", req.URL.RawQuery)
	token := req.Header.Get("X-Forwarded-Access-Token")
	if token == "" {
		klog.Errorf("failed to get token from http header")
	}

	projectList, ok := GetUserProjectList(token)
	klog.V(1).Infof("projectList from local mem cache = %v, ok = %v", projectList, ok)
	if !ok {
		projectList = FetchUserProjectList(token, reqUrl)
		up := NewUserProject(userName, token, projectList)
		UpdateUserProject(up)
		klog.V(1).Infof("projectList from api server = %v", projectList)
	}

	klog.V(1).Infof("cluster list: %v", allManagedClusterNames)
	klog.V(1).Infof("user <%s> project list: %v", userName, projectList)
	if canAccessAllClusters(projectList) {
		klog.Infof("user <%v> have access to all clusters", userName)
		return
	}

	clusterList := getUserClusterList(projectList)
	klog.Infof("user <%v> have access to these clusters: %v", userName, clusterList)

	var rawQuery string
	if req.Method == "POST" {
		body, _ := ioutil.ReadAll(req.Body)
		_ = req.Body.Close()
		queryValues, err := url.ParseQuery(string(body))
		if err != nil {
			klog.Errorf("Failed to parse request body: %v", err)
			return
		}
		if len(queryValues) == 0 {
			return
		}
		queryValues = rewriteQuery(queryValues, clusterList, "query")
		queryValues = rewriteQuery(queryValues, clusterList, "match[]")
		rawQuery = queryValues.Encode()
		req.Body = ioutil.NopCloser(strings.NewReader(rawQuery))
		req.Header.Set("Content-Length", fmt.Sprint(len([]rune(rawQuery))))
		req.ContentLength = int64(len([]rune(rawQuery)))
	} else {
		queryValues := req.URL.Query()
		if len(queryValues) == 0 {
			return
		}
		queryValues = rewriteQuery(queryValues, clusterList, "query")
		queryValues = rewriteQuery(queryValues, clusterList, "match[]")
		req.URL.RawQuery = queryValues.Encode()
		rawQuery = req.URL.RawQuery
	}

	klog.V(1).Info("modified URL is:")
	klog.V(1).Infof("URL is: %s", req.URL)
	klog.V(1).Infof("URL path is: %v", req.URL.Path)
	klog.V(1).Infof("URL RawQuery is: %v", rawQuery)
	return
}

// GetManagedClusterLabelAllowListConfigmap returns the managedcluster label allowlist configmap
func GetManagedClusterLabelAllowListConfigmap(CoreV1Interface corev1.CoreV1Interface, namespace string) (*v1.ConfigMap,
	error) {
	configmap, err := CoreV1Interface.ConfigMaps(namespace).Get(
		context.TODO(),
		proxyconfig.GetManagedClusterLabelAllowListConfigMapName(),
		metav1.GetOptions{},
	)
	if err != nil {
		klog.Errorf("failed to get managedcluster label allowlist configmap: %v", err)
		return nil, err
	}
	return configmap, nil
}

// ModifyManagedClusterLabelAllowListConfigMapData modifies the data for managedcluster label allowlist
func ModifyManagedClusterLabelAllowListConfigMapData(cm *v1.ConfigMap, clusterLabels map[string]string) error {
	var clusterLabelList = &proxyconfig.ManagedClusterLabelList{}

	err := yaml.Unmarshal(
		[]byte(cm.Data[proxyconfig.GetManagedClusterLabelAllowListConfigMapKey()]),
		clusterLabelList,
	)
	if err != nil {
		klog.Errorf("failed to unmarshal configmap <%s> data to the clusterLabelList: %v",
			proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(), err)
		return err
	}

	for key := range clusterLabels {
		if !slice.ContainsString(clusterLabelList.BlackList, key, nil) &&
			!slice.ContainsString(clusterLabelList.LabelList, key, nil) {
			clusterLabelList.LabelList = append(clusterLabelList.LabelList, key)
			klog.Infof("added label <%s> to managedcluster label allowlist", key)
		}
	}

	data, err := yaml.Marshal(clusterLabelList)
	if err != nil {
		klog.Errorf("failed to marshal data to clusterLabelList: %v", err)
		return err
	}
	cm.Data = map[string]string{proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(): string(data)}
	return nil
}

// UpdateClusterLabelsStatus updates the managed cluster labels status for the label allowlist
func UpdateClusterLabelsStatus(managedLabelList *proxyconfig.ManagedClusterLabelList) {
	for _, label := range managedLabelList.LabelList {
		if _, ok := allManagedClusterLabelNames[label]; !ok {
			klog.V(1).Infof("added managedcluster label: %s", label)
		} else if isEnabled := allManagedClusterLabelNames[label]; !isEnabled {
			klog.Infof("enabled managedcluster label: %s", label)
		}
		allManagedClusterLabelNames[label] = true
	}

	for _, label := range managedLabelList.BlackList {
		if _, ok := allManagedClusterLabelNames[label]; !ok {
			klog.V(1).Infof("blacklisted managedcluster label: %s", label)
		} else if isEnabled := allManagedClusterLabelNames[label]; isEnabled {
			klog.Infof("disabled managedcluster label: %s", label)
		}
		allManagedClusterLabelNames[label] = false
	}

	managedLabelList.RegexLabelList = []string{}
	regex := regexp.MustCompile(`[^\w]+`)

	for label, isEnabled := range allManagedClusterLabelNames {
		if isEnabled {
			managedLabelList.RegexLabelList = append(
				managedLabelList.RegexLabelList,
				regex.ReplaceAllString(label, "_"),
			)
		}
	}
}

func UpdateManagedClusterLabelAllowListConfigMap(
	CoreV1Interface corev1.CoreV1Interface,
	namespace string,
	cm *v1.ConfigMap,
) error {
	_, err := CoreV1Interface.ConfigMaps(namespace).Update(
		context.TODO(),
		cm,
		metav1.UpdateOptions{},
	)
	if err != nil {
		klog.Errorf("failed to update managedcluster label allowlist: %v", err)
		return err
	}
	return nil
}

// WatchManagedCluster will watch and save managedcluster when create/update/delete managedcluster
func WatchManagedCluster(clusterClient clusterclientset.Interface, kubeClient kubernetes.Interface) {
	InitAllManagedClusterNames()
	watchlist := cache.NewListWatchFromClient(
		clusterClient.ClusterV1().RESTClient(),
		"managedclusters",
		v1.NamespaceAll,
		fields.Everything(),
	)

	found, _ := GetManagedClusterLabelAllowListConfigmap(kubeClient.CoreV1(), "open-cluster-management-observability")
	_, controller := cache.NewInformer(
		watchlist,
		&clusterv1.ManagedCluster{},
		time.Second*0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				clusterName := obj.(*clusterv1.ManagedCluster).Name
				klog.Infof("added a managedcluster: %s \n", obj.(*clusterv1.ManagedCluster).Name)
				allManagedClusterNames[clusterName] = clusterName

				managedLabelListMutex.Lock()
				ModifyManagedClusterLabelAllowListConfigMapData(found, obj.(*clusterv1.ManagedCluster).Labels)
				UpdateManagedClusterLabelAllowListConfigMap(
					kubeClient.CoreV1(),
					"open-cluster-management-observability",
					found,
				)
			},

			DeleteFunc: func(obj interface{}) {
				clusterName := obj.(*clusterv1.ManagedCluster).Name
				klog.Infof("deleted a managedcluster: %s \n", obj.(*clusterv1.ManagedCluster).Name)
				delete(allManagedClusterNames, clusterName)
			},

			UpdateFunc: func(oldObj, newObj interface{}) {
				clusterName := newObj.(*clusterv1.ManagedCluster).Name
				klog.Infof("changed a managedcluster: %s \n", newObj.(*clusterv1.ManagedCluster).Name)
				allManagedClusterNames[clusterName] = clusterName

				ModifyManagedClusterLabelAllowListConfigMapData(found, newObj.(*clusterv1.ManagedCluster).Labels)
				UpdateManagedClusterLabelAllowListConfigMap(
					kubeClient.CoreV1(),
					"open-cluster-management-observability",
					found,
				)
			},
		},
	)

	stop := make(chan struct{})
	go controller.Run(stop)
	for {
		time.Sleep(time.Second * 30)
		klog.V(1).Infof("found %v clusters", len(allManagedClusterNames))
	}
}

// WatchManagedClusterLabelAllowList will watch and save managedcluster label allowlist configmap
// when create/update/delete
func WatchManagedClusterLabelAllowList(kubeClient kubernetes.Interface) {
	managedLabelList := proxyconfig.GetManagedClusterLabelList()

	watchlist := cache.NewListWatchFromClient(kubeClient.CoreV1().RESTClient(),
		"configmaps",
		"open-cluster-management-observability",
		fields.Everything(),
	)
	_, controller := cache.NewInformer(
		watchlist,
		&v1.ConfigMap{},
		time.Second*0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				if obj.(*v1.ConfigMap).Name == proxyconfig.GetManagedClusterLabelAllowListConfigMapName() {
					klog.Infof("added configmap: %s", proxyconfig.GetManagedClusterLabelAllowListConfigMapName())
					InitAllManagedClusterLabelNames()

					err := yaml.Unmarshal(
						[]byte(obj.(*v1.ConfigMap).Data[proxyconfig.GetManagedClusterLabelAllowListConfigMapKey()]),
						managedLabelList,
					)
					if err != nil {
						klog.Errorf("failed to unmarshal configmap <%s> data to the managedLabelList: %v",
							proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(), err)
					}
					UpdateClusterLabelsStatus(managedLabelList)
				}
			},

			DeleteFunc: func(obj interface{}) {
				if obj.(*v1.ConfigMap).Name == proxyconfig.GetManagedClusterLabelAllowListConfigMapName() {
					klog.Warningf("deleted configmap: %s", proxyconfig.GetManagedClusterLabelAllowListConfigMapName())
				}
			},

			UpdateFunc: func(oldObj, newObj interface{}) {
				if oldObj.(*v1.ConfigMap).Name == proxyconfig.GetManagedClusterLabelAllowListConfigMapName() &&
					newObj.(*v1.ConfigMap).Name == proxyconfig.GetManagedClusterLabelAllowListConfigMapName() {
					klog.Infof("updated configmap: %s", proxyconfig.GetManagedClusterLabelAllowListConfigMapName())

					err := yaml.Unmarshal(
						[]byte(newObj.(*v1.ConfigMap).Data[proxyconfig.GetManagedClusterLabelAllowListConfigMapKey()]),
						managedLabelList,
					)
					if err != nil {
						klog.Errorf("failed to unmarshal configmap <%s> data to the managedLabelList: %v",
							proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(), err)
					}

					for label := range allManagedClusterLabelNames {
						if !slice.ContainsString(managedLabelList.LabelList, label, nil) &&
							!slice.ContainsString(managedLabelList.BlackList, label, nil) {
							klog.Infof("removed managedcluster label: %s", label)
							delete(allManagedClusterLabelNames, label)
						}
					}
					UpdateClusterLabelsStatus(managedLabelList)
				}
			},
		},
	)

	stop := make(chan struct{})
	go controller.Run(stop)
	for {
		time.Sleep(time.Second * 30)
		klog.V(1).Infof("found %v labels", len(allManagedClusterLabelNames))
	}
}

func sendHTTPRequest(url string, verb string, token string) (*http.Response, error) {
	req, err := http.NewRequest(verb, url, nil)
	if err != nil {
		klog.Errorf("failed to new http request: %v", err)
		return nil, err
	}

	if len(token) == 0 {
		transport := &http.Transport{}
		defaultClient := &http.Client{Transport: transport}
		return defaultClient.Do(req)
	}

	if !strings.HasPrefix(token, "Bearer ") {
		token = "Bearer " + token
	}
	req.Header.Set("Authorization", token)
	caCert, err := ioutil.ReadFile(filepath.Clean(caPath))
	if err != nil {
		klog.Error("failed to load root ca cert file")
		return nil, err
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:    caCertPool,
			MinVersion: tls.VersionTLS12,
		},
		MaxIdleConns:    100,
		IdleConnTimeout: 60 * time.Second,
	}

	client := http.Client{Transport: tr}
	return client.Do(req)
}

func FetchUserProjectList(token string, url string) []string {
	resp, err := sendHTTPRequest(url, "GET", token)
	if err != nil {
		klog.Errorf("failed to send http request: %v", err)
		/*
			This is adhoc step to make sure that if this error happens,
			we can automatically restart the POD using liveness probe which checks for this file.
			Once the real cause is determined and fixed, we will remove this.
		*/
		writeError(fmt.Sprintf("failed to send http request: %v", err))
		return []string{}
	}
	defer resp.Body.Close()

	var projects projectv1.ProjectList
	err = json.NewDecoder(resp.Body).Decode(&projects)
	if err != nil {
		klog.Errorf("failed to decode response json body: %v", err)
		return []string{}
	}

	projectList := make([]string, len(projects.Items))
	for idx, p := range projects.Items {
		projectList[idx] = p.Name
	}

	return projectList
}

func GetUserName(token string, url string) string {
	resp, err := sendHTTPRequest(url, "GET", token)
	if err != nil {
		klog.Errorf("failed to send http request: %v", err)
		writeError(fmt.Sprintf("failed to send http request: %v", err))
		return ""
	}
	defer resp.Body.Close()

	user := userv1.User{}
	err = json.NewDecoder(resp.Body).Decode(&user)
	if err != nil {
		klog.Errorf("failed to decode response json body: %v", err)
		return ""
	}

	return user.Name
}

// Contains is used to check whether a list contains string s
func Contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// canAccessAllClusters check user have permission to access all clusters
func canAccessAllClusters(projectList []string) bool {
	if len(allManagedClusterNames) == 0 && len(projectList) == 0 {
		return false
	}

	for name := range allManagedClusterNames {
		if !Contains(projectList, name) {
			return false
		}
	}

	return true
}

func getUserClusterList(projectList []string) []string {
	clusterList := []string{}
	if len(projectList) == 0 {
		return clusterList
	}

	for _, projectName := range projectList {
		clusterName, ok := allManagedClusterNames[projectName]
		if ok {
			clusterList = append(clusterList, clusterName)
		}
	}

	return clusterList
}

func rewriteQuery(queryValues url.Values, clusterList []string, key string) url.Values {
	originalQuery := queryValues.Get(key)
	if len(originalQuery) == 0 {
		return queryValues
	}

	modifiedQuery, err := rewrite.InjectLabels(originalQuery, "cluster", clusterList)
	if err != nil {
		return queryValues
	}

	queryValues.Del(key)
	queryValues.Add(key, modifiedQuery)
	return queryValues
}

func writeError(msg string) {
	f, err := os.OpenFile("/tmp/health", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		klog.Errorf("failed to create file for probe: %v", err)
	}

	_, err = f.Write([]byte(msg))
	if err != nil {
		klog.Errorf("failed to write error message to probe file: %v", err)
	}

	_ = f.Close()
}
