// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"

	"k8s.io/client-go/kubernetes"
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

	managedLabelList = proxyconfig.GetManagedClusterLabelList()
	syncLabelList    = &proxyconfig.ManagedClusterLabelList{}
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

// GetManagedClusterEventHandler return event handler functions for managed cluster watch events
func GetManagedClusterEventHandler() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			clusterName := obj.(*clusterv1.ManagedCluster).Name
			klog.Infof("added a managedcluster: %s \n", obj.(*clusterv1.ManagedCluster).Name)
			allManagedClusterNames[clusterName] = clusterName

			if ok := shouldUpdateAllManagedClusterLabelNames(
				obj.(*clusterv1.ManagedCluster).Labels, managedLabelList); ok {
				updateAllManagedClusterLabelNames(managedLabelList)
			}
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

			if ok := shouldUpdateAllManagedClusterLabelNames(
				newObj.(*clusterv1.ManagedCluster).Labels, managedLabelList); ok {
				updateAllManagedClusterLabelNames(managedLabelList)
			}
		},
	}
}

// WatchManagedCluster will watch and save managedcluster when create/update/delete managedcluster
func WatchManagedCluster(clusterClient clusterclientset.Interface, kubeClient kubernetes.Interface) {
	InitAllManagedClusterNames()
	InitAllManagedClusterLabelNames()
	watchlist := cache.NewListWatchFromClient(
		clusterClient.ClusterV1().RESTClient(),
		"managedclusters",
		v1.NamespaceAll,
		fields.Everything(),
	)

	found, _ := proxyconfig.GetManagedClusterLabelAllowListConfigmap(kubeClient.CoreV1(),
		proxyconfig.ManagedClusterLabelAllowListNamespace)

	_ = unmarshalDataToManagedClusterLabelList(found.Data, proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(),
		managedLabelList)

	updateAllManagedClusterLabelNames(managedLabelList)
	_, controller := cache.NewInformer(watchlist, &clusterv1.ManagedCluster{}, time.Second*0,
		GetManagedClusterEventHandler())

	stop := make(chan struct{})
	go controller.Run(stop)
	for {
		time.Sleep(time.Second * 30)
		klog.V(1).Infof("found %v clusters", len(allManagedClusterNames))
	}
}

// GetManagedClusterLabelAllowListEventHandler return event handler for managedcluster label allow list watch event
func GetManagedClusterLabelAllowListEventHandler() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if obj.(*v1.ConfigMap).Name == proxyconfig.GetManagedClusterLabelAllowListConfigMapName() {
				klog.Infof("added configmap: %s", proxyconfig.GetManagedClusterLabelAllowListConfigMapName())

				_ = unmarshalDataToManagedClusterLabelList(obj.(*v1.ConfigMap).Data,
					proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(), syncLabelList)

				if ok := reflect.DeepEqual(syncLabelList, managedLabelList); !ok {
					klog.Infof("resyncing required for managedcluster label allowlist: %v",
						proxyconfig.GetManagedClusterLabelAllowListConfigMapName())

					// Sync the managedLabelList ignore list to the ignore list within the configmap.
					managedLabelList.IgnoreList = syncLabelList.IgnoreList

					// Resync the configmap data to the labelList stored in memory.
					syncLabelList = managedLabelList

					_ = marshalLabelListToConfigMap(obj,
						proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(), syncLabelList)

					klog.Info("TODO: Update the managedcluster label allowlist configmap")
					if syncLabelList == managedLabelList {
						klog.Infof("resync completed")
					}
				}
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

				_ = unmarshalDataToManagedClusterLabelList(newObj.(*v1.ConfigMap).Data,
					proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(), syncLabelList)

				if ok := reflect.DeepEqual(syncLabelList, managedLabelList); !ok {
					klog.Infof("resyncing required for managedcluster label allowlist: %v",
						proxyconfig.GetManagedClusterLabelAllowListConfigMapName())

					managedLabelList.IgnoreList = syncLabelList.IgnoreList
					syncLabelList = managedLabelList

					_ = marshalLabelListToConfigMap(newObj,
						proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(), syncLabelList)

					klog.Info("TODO: Update the managedcluster label allowlist configmap")
					if syncLabelList == managedLabelList {
						klog.Infof("resync completed")
					}
				}
			}
		},
	}
}

// WatchManagedClusterLabelAllowList will watch and save managedcluster label allowlist configmap
// when create/update/delete
func WatchManagedClusterLabelAllowList(kubeClient kubernetes.Interface) {
	watchlist := cache.NewListWatchFromClient(kubeClient.CoreV1().RESTClient(), "configmaps",
		proxyconfig.ManagedClusterLabelAllowListNamespace, fields.Everything())

	_, controller := cache.NewInformer(watchlist, &v1.ConfigMap{}, time.Second*0,
		GetManagedClusterLabelAllowListEventHandler())

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

// marshalLabelListToConfigMap marshal managedcluster label list data to configmap data key
func marshalLabelListToConfigMap(obj interface{}, key string,
	managedLabelList *proxyconfig.ManagedClusterLabelList) error {
	data, err := yaml.Marshal(managedLabelList)

	if err != nil {
		klog.Errorf("failed to marshal managedLabelList data: %v", err)
		return err
	}
	obj.(*v1.ConfigMap).Data[key] = string(data)
	return nil
}

// shouldUpdateAllManagedClusterLabelNames determines whether the cluster label names map should be updated
func shouldUpdateAllManagedClusterLabelNames(clusterLabels map[string]string,
	managedLabelList *proxyconfig.ManagedClusterLabelList) bool {
	updateRequired := false

	for key := range clusterLabels {
		if !slice.ContainsString(managedLabelList.LabelList, key, nil) &&
			!slice.ContainsString(managedLabelList.IgnoreList, key, nil) {
			managedLabelList.LabelList = append(managedLabelList.LabelList, key)
			updateRequired = true
		}
	}

	klog.Infof("managedcluster label update required: %v", updateRequired)
	return updateRequired
}

// updateAllManagedClusterLabelNames updates the managedcluster label names status
func updateAllManagedClusterLabelNames(managedLabelList *proxyconfig.ManagedClusterLabelList) {
	for _, key := range managedLabelList.LabelList {
		if _, ok := allManagedClusterLabelNames[key]; !ok {
			klog.Infof("added managedcluster label: %s", key)
			allManagedClusterLabelNames[key] = true

		} else if slice.ContainsString(managedLabelList.IgnoreList, key, nil) {
			klog.Warningf("managedcluster label <%s> set to ignore, remove label from ignore list to enable.", key)

		} else if isEnabled := allManagedClusterLabelNames[key]; !isEnabled {
			klog.Infof("enabled managedcluster label: %s", key)
			allManagedClusterLabelNames[key] = true
		}
	}

	for _, key := range managedLabelList.IgnoreList {
		if _, ok := allManagedClusterLabelNames[key]; !ok {
			klog.Infof("ignoring managedcluster label: %s", key)

		} else if isEnabled := allManagedClusterLabelNames[key]; isEnabled {
			klog.Infof("disabled managedcluster label: %s", key)
		}

		allManagedClusterLabelNames[key] = false
	}

	managedLabelList.RegexLabelList = []string{}
	regex := regexp.MustCompile(`[^\w]+`)

	for key, isEnabled := range allManagedClusterLabelNames {
		if isEnabled {
			managedLabelList.RegexLabelList = append(
				managedLabelList.RegexLabelList,
				regex.ReplaceAllString(key, "_"),
			)
		}
	}
}

// unmarshalDataToManagedClusterLabelList unmarshal managedcluster label allowlist
func unmarshalDataToManagedClusterLabelList(data map[string]string, key string,
	labelList *proxyconfig.ManagedClusterLabelList) error {
	err := yaml.Unmarshal([]byte(data[key]), labelList)

	if err != nil {
		klog.Errorf("failed to unmarshal configmap <%s> data to the managedLabelList: %v", key, err)
		return err
	}

	return nil
}
