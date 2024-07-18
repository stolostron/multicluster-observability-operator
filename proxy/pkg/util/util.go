// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-co-op/gocron"
	projectv1 "github.com/openshift/api/project/v1"
	userv1 "github.com/openshift/api/user/v1"
	proxyconfig "github.com/stolostron/multicluster-observability-operator/proxy/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/rewrite"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	"k8s.io/kubectl/pkg/util/slice"
	clusterclientset "open-cluster-management.io/api/client/cluster/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/rbac-api-utils/pkg/rbac"
	"golang.org/x/exp/slices"
	"k8s.io/client-go/rest"
)

const (
	managedClusterAPIPath = "/apis/cluster.open-cluster-management.io/v1/managedclusters"
	caPath                = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
)

// AccessReviewer defines an interface for the GetMetricsAccess method.
type AccessReviewer interface {
	GetMetricsAccess(token string, extraArgs ...string) (map[string][]string, error)
}

var (
	allManagedClusterNames      map[string]string
	allManagedClusterLabelNames map[string]bool
	accessReviewer              AccessReviewer

	managedLabelList   = proxyconfig.GetManagedClusterLabelList()
	syncLabelList      = proxyconfig.GetSyncLabelList()
	clusterMatchRegExp = regexp.MustCompile(`([{|,][ ]*)cluster(=|!=|=~|!~)([ ]*)"([^"]+)"`)
)

// resources for the gocron scheduler.
var (
	resyncTag = "managed-cluster-label-allowlist-resync"
	scheduler *gocron.Scheduler
)

// func GetAccessReviewer() *rbac.AccessReviewer {
func GetAccessReviewer() AccessReviewer {
	return accessReviewer
}

// GetAllManagedClusterNames returns all managed cluster names.
func GetAllManagedClusterNames() map[string]string {
	return allManagedClusterNames
}

// GetAllManagedClusterLabelNames returns all managed cluster labels.
func GetAllManagedClusterLabelNames() map[string]bool {
	return allManagedClusterLabelNames
}

// InitAllManagedClusterNames initializes all managed cluster names map.
func InitAllManagedClusterNames() {
	allManagedClusterNames = map[string]string{}
}

// InitAllManagedClusterLabelNames initializes all managed cluster labels map.
func InitAllManagedClusterLabelNames() {
	allManagedClusterLabelNames = map[string]bool{}
}

func InitScheduler() {
	scheduler = gocron.NewScheduler(time.UTC)
}

func InitAccessReviewer(kConfig *rest.Config) (err error) {
	accessReviewer, err = rbac.NewAccessReviewer(kConfig, nil)
	return err
}

// shouldUpdateManagedClusterLabelNames determine whether the managedcluster label names map should be updated.
func shouldUpdateManagedClusterLabelNames(clusterLabels map[string]string,
	managedLabelList *proxyconfig.ManagedClusterLabelList) bool {
	updateRequired := false

	for key := range clusterLabels {
		if !slice.ContainsString(managedLabelList.LabelList, key, nil) {
			managedLabelList.LabelList = append(managedLabelList.LabelList, key)
			updateRequired = true
		}
	}

	klog.Infof("managedcluster label names update required: %v", updateRequired)
	return updateRequired
}

// addManagedClusterLabelNames set key to enable within the managedcluster label names map.
func addManagedClusterLabelNames(managedLabelList *proxyconfig.ManagedClusterLabelList) {
	for _, key := range managedLabelList.LabelList {
		if _, ok := allManagedClusterLabelNames[key]; !ok {
			klog.Infof("added managedcluster label: %s", key)
			allManagedClusterLabelNames[key] = true

		} else if slice.ContainsString(managedLabelList.IgnoreList, key, nil) {
			klog.V(2).Infof("managedcluster label <%s> set to ignore, remove label from ignore list to enable.", key)

		} else if isEnabled := allManagedClusterLabelNames[key]; !isEnabled {
			klog.Infof("enabled managedcluster label: %s", key)
			allManagedClusterLabelNames[key] = true
		}
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
	syncLabelList.RegexLabelList = managedLabelList.RegexLabelList
}

// ignoreManagedClusterLabelNames set key to ignore within the managedcluster label names map.
func ignoreManagedClusterLabelNames(managedLabelList *proxyconfig.ManagedClusterLabelList) {
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
	syncLabelList.RegexLabelList = managedLabelList.RegexLabelList
}

// updateAllManagedClusterLabelNames updates all managed cluster label names status within the map.
func updateAllManagedClusterLabelNames(managedLabelList *proxyconfig.ManagedClusterLabelList) {
	if managedLabelList.LabelList != nil {
		addManagedClusterLabelNames(managedLabelList)
	} else {
		klog.Infof("managed label list is empty")
	}

	if managedLabelList.IgnoreList != nil {
		ignoreManagedClusterLabelNames(managedLabelList)
	} else {
		klog.Infof("managed ignore list is empty")
	}
}

// ModifyMetricsQueryParams will modify request url params for query metrics.
func ModifyMetricsQueryParams(req *http.Request, reqUrl string, accessReviewer AccessReviewer) {

	userName := req.Header.Get("X-Forwarded-User")
	klog.V(1).Infof("user is %v", userName)
	klog.V(1).Infof("URL is: %s", req.URL)
	klog.V(1).Infof("URL path is: %v", req.URL.Path)
	klog.V(1).Infof("URL RawQuery is: %v", req.URL.RawQuery)
	token := req.Header.Get("X-Forwarded-Access-Token")
	if token == "" {
		klog.Errorf("failed to get token from http header")
	}

	userMetricsAccess, err := GetUserMetricsACLs(userName, token, reqUrl, accessReviewer)
	if err != nil {
		klog.Errorf("Failed to determine user's metrics access: %v", err)
		return
	}

	klog.Infof("user <%v> have metrics access to : %v", userName, userMetricsAccess)

	allAccess := CanAccessAll(userMetricsAccess)
	if allAccess {
		klog.Infof("user <%v> have access to all clusters and all namespaces", userName)
		return
	}

	var rawQuery string
	if req.Method == "POST" {
		body, _ := io.ReadAll(req.Body)
		_ = req.Body.Close()
		queryValues, err := url.ParseQuery(string(body))
		if err != nil {
			klog.Errorf("Failed to parse request body: %v", err)
			return
		}
		if len(queryValues) == 0 {
			return
		}
		queryValues = rewriteQuery(queryValues, userMetricsAccess, "query")
		queryValues = rewriteQuery(queryValues, userMetricsAccess, "match[]")
		rawQuery = queryValues.Encode()
		req.Body = io.NopCloser(strings.NewReader(rawQuery))
		req.Header.Set("Content-Length", fmt.Sprint(len([]rune(rawQuery))))
		req.ContentLength = int64(len([]rune(rawQuery)))
	} else {
		queryValues := req.URL.Query()
		if len(queryValues) == 0 {
			return
		}
		queryValues = rewriteQuery(queryValues, userMetricsAccess, "query")
		queryValues = rewriteQuery(queryValues, userMetricsAccess, "match[]")
		req.URL.RawQuery = queryValues.Encode()
		rawQuery = req.URL.RawQuery
	}

	klog.V(1).Info("modified URL is:")
	klog.V(1).Infof("URL is: %s", req.URL)
	klog.V(1).Infof("URL path is: %v", req.URL.Path)
	klog.V(1).Infof("URL RawQuery is: %v", rawQuery)
}

// GetManagedClusterEventHandler return event handler functions for managed cluster watch events.
func GetManagedClusterEventHandler() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			clusterName := obj.(*clusterv1.ManagedCluster).Name
			klog.Infof("added a managedcluster: %s \n", obj.(*clusterv1.ManagedCluster).Name)
			allManagedClusterNames[clusterName] = clusterName
			CleanExpiredProjectInfo(1)

			clusterLabels := obj.(*clusterv1.ManagedCluster).Labels
			if ok := shouldUpdateManagedClusterLabelNames(clusterLabels, managedLabelList); ok {
				addManagedClusterLabelNames(managedLabelList)
			}
		},

		DeleteFunc: func(obj interface{}) {
			clusterName := obj.(*clusterv1.ManagedCluster).Name
			klog.Infof("deleted a managedcluster: %s \n", obj.(*clusterv1.ManagedCluster).Name)
			delete(allManagedClusterNames, clusterName)
			CleanExpiredProjectInfo(1)
		},

		UpdateFunc: func(oldObj, newObj interface{}) {
			clusterName := newObj.(*clusterv1.ManagedCluster).Name
			klog.Infof("changed a managedcluster: %s \n", newObj.(*clusterv1.ManagedCluster).Name)
			allManagedClusterNames[clusterName] = clusterName

			clusterLabels := newObj.(*clusterv1.ManagedCluster).Labels
			if ok := shouldUpdateManagedClusterLabelNames(clusterLabels, managedLabelList); ok {
				addManagedClusterLabelNames(managedLabelList)
			}
		},
	}
}

// WatchManagedCluster will watch and save managedcluster when create/update/delete managedcluster.
func WatchManagedCluster(clusterClient clusterclientset.Interface, kubeClient kubernetes.Interface) {
	InitAllManagedClusterNames()
	InitAllManagedClusterLabelNames()
	watchlist := cache.NewListWatchFromClient(
		clusterClient.ClusterV1().RESTClient(),
		"managedclusters",
		v1.NamespaceAll,
		fields.Everything(),
	)

	_, controller := cache.NewInformer(watchlist, &clusterv1.ManagedCluster{}, time.Second*0,
		GetManagedClusterEventHandler())

	stop := make(chan struct{})
	go controller.Run(stop)
	for {
		time.Sleep(time.Second * 30)
		klog.V(1).Infof("found %v clusters", len(allManagedClusterNames))
	}
}

func ScheduleManagedClusterLabelAllowlistResync(kubeClient kubernetes.Interface) {
	if scheduler == nil {
		InitScheduler()
	}

	_, err := scheduler.Tag(resyncTag).Every(30).Second().Do(resyncManagedClusterLabelAllowList, kubeClient)
	if err != nil {
		klog.Errorf("failed to schedule job for managedcluster allowlist resync: %v", err)
	}

	klog.Info("starting scheduler for managedcluster allowlist resync")
	scheduler.StartAsync()
}

func StopScheduleManagedClusterLabelAllowlistResync() {
	klog.Info("stopping scheduler for managedcluster allowlist resync")
	scheduler.Stop()

	if ok := scheduler.IsRunning(); !ok {
		InitScheduler()
	}
}

// GetManagedClusterLabelAllowListEventHandler return event handler for managedcluster label allow list watch event.
func GetManagedClusterLabelAllowListEventHandler(kubeClient kubernetes.Interface) cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if obj.(*v1.ConfigMap).Name == proxyconfig.GetManagedClusterLabelAllowListConfigMapName() {
				klog.Infof("added configmap: %s", proxyconfig.GetManagedClusterLabelAllowListConfigMapName())

				if ok := scheduler != nil; ok {
					if ok := scheduler.IsRunning(); !ok {
						go ScheduleManagedClusterLabelAllowlistResync(kubeClient)
					}
				}
			}
		},

		DeleteFunc: func(obj interface{}) {
			if obj.(*v1.ConfigMap).Name == proxyconfig.GetManagedClusterLabelAllowListConfigMapName() {
				klog.Warningf("deleted configmap: %s", proxyconfig.GetManagedClusterLabelAllowListConfigMapName())
				StopScheduleManagedClusterLabelAllowlistResync()
			}
		},

		UpdateFunc: func(oldObj, newObj interface{}) {
			if newObj.(*v1.ConfigMap).Name == proxyconfig.GetManagedClusterLabelAllowListConfigMapName() {
				klog.Infof("updated configmap: %s", proxyconfig.GetManagedClusterLabelAllowListConfigMapName())

				_ = unmarshalDataToManagedClusterLabelList(newObj.(*v1.ConfigMap).Data,
					proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(), syncLabelList)

				sortManagedLabelList(managedLabelList)
				sortManagedLabelList(syncLabelList)

				if ok := reflect.DeepEqual(syncLabelList, managedLabelList); !ok {
					managedLabelList.IgnoreList = syncLabelList.IgnoreList
					*syncLabelList = *managedLabelList
				}

				updateAllManagedClusterLabelNames(managedLabelList)
			}
		},
	}
}

// WatchManagedClusterLabelAllowList will watch and save managedcluster label allowlist configmap
// when create/update/delete.
func WatchManagedClusterLabelAllowList(kubeClient kubernetes.Interface) {
	watchlist := cache.NewListWatchFromClient(kubeClient.CoreV1().RESTClient(), "configmaps",
		proxyconfig.ManagedClusterLabelAllowListNamespace, fields.Everything())

	_, controller := cache.NewInformer(watchlist, &v1.ConfigMap{}, time.Second*0,
		GetManagedClusterLabelAllowListEventHandler(kubeClient))

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
	caCert, err := os.ReadFile(filepath.Clean(caPath))
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

	defer func() {
		err := resp.Body.Close()
		if err != nil {
			klog.Errorf("failed to close response body: %v", err)
		}
	}()

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

	user := userv1.User{}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			klog.Errorf("failed to close response body: %v", err)
		}
	}()

	err = json.NewDecoder(resp.Body).Decode(&user)
	if err != nil {
		klog.Errorf("failed to decode response json body: %v", err)
		return ""
	}

	return user.Name
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

func GetUserMetricsACLs(userName string, token string, reqUrl string, accessReviewer AccessReviewer) (map[string][]string, error) {

	klog.Infof("Getting metrics access for user : %v", userName)

	// get all metricsaccess ACLs for the user
	// i.e every  metrics/<ns> on managedcluster CR defined for the user
	// in the returned map -  key is managedcluster name , value is namespaces accesible on that cluster
	metricsAccess, arerr := accessReviewer.GetMetricsAccess(token)
	if arerr != nil {
		klog.Errorf("Failed to get Metrics Access from Access Reviewer: %v", arerr)
		return nil, arerr
	}

	klog.Infof("user <%v>  metrics-access: %v", userName, metricsAccess)

	//if metrics access contains a key  "*" then the corresponding
	// value i.e acls  apply to all managedclusters
	if allClusterAcls, found := metricsAccess["*"]; found {
		for mcName := range allManagedClusterNames {
			if clusterAcls, ok := metricsAccess[mcName]; ok {
				for _, allClusterAclItem := range allClusterAcls {
					if !slices.Contains(clusterAcls, allClusterAclItem) {
						clusterAcls = append(clusterAcls, allClusterAclItem)
					}
				}
				metricsAccess[mcName] = clusterAcls
			} else {
				metricsAccess[mcName] = allClusterAcls
			}
		}
		delete(metricsAccess, "*")
	}

	klog.Infof("user <%v>  metrics-access after processing for AllCluster acls: %v", userName, metricsAccess)

	// for backward compatibility, support the existing access control mechanism
	// i.e access to managedcluster project\namespace means access to all namespaces on that managedcluster

	//get all managedcluster project/namespace user has access to
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

	clusterList := getUserClusterList(projectList)
	klog.Infof("user <%v> have access to these clusters: %v", userName, clusterList)

	// combine the  user project access list to the metrics access list
	// project access by default give access to all namespaces ( i.e * ) unless
	// scoped down to specific namespaces through metrics/<ns>  acl definition
	for _, cluster := range clusterList {
		if _, found := metricsAccess[cluster]; found {
			continue
		}

		// metricAcls not found for this cluster,
		// so add * for backward compatibility
		// as user has access to the managedcluster project
		metricsAccess[cluster] = []string{"*"}
	}
	klog.Infof("user <%v>  metrics-access after merging with cluster access list %v", userName, metricsAccess)

	return metricsAccess, nil
}

// CanAccessAll check user have permission to access all clusters
func CanAccessAll(clusterNamespaces map[string][]string) bool {
	if len(allManagedClusterNames) == 0 && len(clusterNamespaces) == 0 {
		return false
	}

	for _, clusterName := range allManagedClusterNames {

		namespaces, contains := clusterNamespaces[clusterName]

		//does not have access to the cluster
		if !contains {
			return false
		}

		//does not have access to all the namespaces on the cluster
		if len(namespaces) != 0 && !slices.Contains(namespaces, "*") {
			return false
		}
	}

	return true
}

func getCommonNamespacesAcrossClusters(clusters []string, metricsAccess map[string][]string) []string {

	klog.Infof("common namespaces across clusters: %v  in metrics access map: %v", clusters, metricsAccess)

	//make a smaller map of metricsaccess for just the requested clusters
	reqClustersMetricsAccess := make(map[string][]string)
	if len(clusters) == 0 {
		reqClustersMetricsAccess = metricsAccess
	} else {
		for _, cluster := range clusters {
			reqClustersMetricsAccess[cluster] = metricsAccess[cluster]
		}
	}
	klog.Infof("requested clusters metrics access map: %v", reqClustersMetricsAccess)

	//make a count of how many times each unique namespace occurs across clusters
	//if the count of a ns == le(clusters), it means it exists across all clusters

	namespaceCounts := make(map[string]int)
	allAccessCount := 0

	for _, namespaces := range reqClustersMetricsAccess {

		if len(namespaces) == 0 || slices.Contains(namespaces, "*") {
			allAccessCount++
		} else {
			for _, namespace := range namespaces {
				count, ok := namespaceCounts[namespace]
				if !ok {
					namespaceCounts[namespace] = 1
				} else {
					namespaceCounts[namespace] = count + 1
				}
			}
		}
	}

	klog.Infof("allAccessCount  : %v", allAccessCount)
	klog.Infof("Namespaces Count  : %v", namespaceCounts)

	commonNamespaces := []string{}

	if allAccessCount == len(reqClustersMetricsAccess) {
		commonNamespaces = append(commonNamespaces, "*")
	} else {
		for ns, count := range namespaceCounts {
			if (count + allAccessCount) == len(reqClustersMetricsAccess) {
				commonNamespaces = append(commonNamespaces, ns)
			}
		}
	}

	klog.Infof("common namespaces across clusters: %v   : %v", clusters, commonNamespaces)
	return commonNamespaces
}

// read clusters in the user's query
func getClustersInQuery(queryValues url.Values, key string, userMetricsAccess map[string][]string) []string {

	// parse the promql query and return the queries  set of cluster
	// so that better namespace filtering can be done

	// for query  --  namespace:container_memory_usage_bytes:sum{cluster=~"devcluster1"}
	// result should be -- "devcluster1"

	// stubbing to return "empty" i.e all clusters

	query := queryValues.Get(key)
	matches := clusterMatchRegExp.FindStringSubmatch(query)
	if len(matches) == 0 {
		return []string{}
	}
	operator := matches[2]
	expr := matches[4]
	clusters := make([]string, 0, len(userMetricsAccess))
	for cluster := range userMetricsAccess {
		clusters = append(clusters, cluster)
	}
	queryClusters := make([]string, 0, len(clusters))

	var reg *regexp.Regexp
	switch operator {
	case "=":
		return []string{expr}
	case "!=":
		for _, cluster := range clusters {
			if cluster != expr {
				queryClusters = append(queryClusters, cluster)
			}
		}
		return queryClusters
	case "=~":
		reg = regexp.MustCompile(expr)
		for _, cluster := range clusters {
			if reg.MatchString(cluster) {
				queryClusters = append(queryClusters, cluster)
			}
		}
		return queryClusters
	case "!~":
		reg = regexp.MustCompile(expr)
		for _, cluster := range clusters {
			if !reg.MatchString(cluster) {
				queryClusters = append(queryClusters, cluster)
			}
		}
		return queryClusters
	}
	return []string{}
}

func rewriteQuery(queryValues url.Values, userMetricsAccess map[string][]string, key string) url.Values {
	klog.Infof("REWRITE QUERY: queryValues: %v, userMetricsAccess: %v, key: %v\n", queryValues, userMetricsAccess, key)
	originalQuery := queryValues.Get(key)
	klog.Infof("REWRITE QUERY: key is: %v, originalQuery is: %v\n", key, originalQuery)
	if len(originalQuery) == 0 {
		return queryValues
	}

	clusterList := getClusterList(userMetricsAccess)
	label := getLabel(originalQuery)
	modifiedQuery, err := rewrite.InjectLabels(originalQuery, label, clusterList)
	if err != nil {
		return queryValues
	}

	klog.Infof("REWRITE QUERY Modified Query after injecting %v: %v\n", label, modifiedQuery)

	if !strings.Contains(originalQuery, proxyconfig.GetACMManagedClusterLabelNamesMetricName()) {
		modifiedQuery, err = injectNamespaces(queryValues, key, userMetricsAccess, modifiedQuery)
		if err != nil {
			return queryValues
		}
	}

	queryValues.Del(key)
	queryValues.Add(key, modifiedQuery)
	return queryValues
}

func getClusterList(userMetricsAccess map[string][]string) []string {
	clusterList := make([]string, 0, len(userMetricsAccess))
	for clusterName := range userMetricsAccess {
		clusterList = append(clusterList, clusterName)
	}
	slices.Sort(clusterList)
	return clusterList
}

func getLabel(originalQuery string) string {
	if strings.Contains(originalQuery, proxyconfig.GetACMManagedClusterLabelNamesMetricName()) {
		return "name"
	}
	return "cluster"
}

func injectNamespaces(queryValues url.Values, key string, userMetricsAccess map[string][]string, modifiedQuery string) (string, error) {
	queryClusters := getClustersInQuery(queryValues, key, userMetricsAccess)
	commonNsAcrossQueryClusters := getCommonNamespacesAcrossClusters(queryClusters, userMetricsAccess)
	allNamespaceAccess := len(commonNsAcrossQueryClusters) == 1 && commonNsAcrossQueryClusters[0] == "*"

	klog.Infof("REWRITE QUERY Modified Query hasAccess to All namespaces: \n %v", allNamespaceAccess)
	if !allNamespaceAccess {
		modifiedQuery2, err := rewrite.InjectLabels(modifiedQuery, "namespace", commonNsAcrossQueryClusters)
		if err != nil {
			return modifiedQuery, err
		}
		klog.Infof("REWRITE QUERY Modified Query after injecting namespaces:  \n %v", modifiedQuery2)
		return modifiedQuery2, nil
	}
	return modifiedQuery, nil
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

// resyncManagedClusterLabelAllowList resync the managedcluster Label allowlist configmap data.
func resyncManagedClusterLabelAllowList(kubeClient kubernetes.Interface) error {
	found, err := proxyconfig.GetManagedClusterLabelAllowListConfigmap(kubeClient,
		proxyconfig.ManagedClusterLabelAllowListNamespace)

	if err != nil {
		return err
	}

	err = unmarshalDataToManagedClusterLabelList(found.Data,
		proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(), syncLabelList)

	if err != nil {
		return err
	}

	sortManagedLabelList(managedLabelList)
	sortManagedLabelList(syncLabelList)

	syncIgnoreList := []string{}
	syncUpdate := false

	for _, label := range syncLabelList.IgnoreList {
		if slice.ContainsString(proxyconfig.GetRequiredLabelList(), label, nil) {
			klog.Infof("detected required managedcluster label in ignorelist. resetting label: %s", label)
		} else {
			syncIgnoreList = append(syncIgnoreList, label)
			syncUpdate = true
		}
	}

	sort.Strings(syncIgnoreList)
	if syncUpdate {
		syncLabelList.IgnoreList = syncIgnoreList
	}

	if ok := reflect.DeepEqual(syncLabelList, managedLabelList); !ok {
		klog.Infof("resyncing required for managedcluster label allowlist: %v",
			proxyconfig.GetManagedClusterLabelAllowListConfigMapName())

		managedLabelList.IgnoreList = syncLabelList.IgnoreList
		ignoreManagedClusterLabelNames(managedLabelList)

		*syncLabelList = *managedLabelList
		_ = marshalLabelListToConfigMap(found,
			proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(), syncLabelList)

		_, err := kubeClient.CoreV1().ConfigMaps(proxyconfig.ManagedClusterLabelAllowListNamespace).Update(
			context.TODO(),
			found,
			metav1.UpdateOptions{},
		)
		if err != nil {
			klog.Errorf("failed to update managedcluster label allowlist configmap: %v", err)
			return err
		}
	}

	return nil
}

// marshalLabelListToConfigMap marshal managedcluster label list data to configmap data key.
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

// unmarshalDataToManagedClusterLabelList unmarshal managedcluster label allowlist.
func unmarshalDataToManagedClusterLabelList(data map[string]string, key string,
	managedLabelList *proxyconfig.ManagedClusterLabelList) error {
	err := yaml.Unmarshal([]byte(data[key]), managedLabelList)

	if err != nil {
		klog.Errorf("failed to unmarshal configmap <%s> data to the managedLabelList: %v", key, err)
		return err
	}

	return nil
}

func sortManagedLabelList(managedLabelList *proxyconfig.ManagedClusterLabelList) {
	if managedLabelList != nil {
		sort.Strings(managedLabelList.IgnoreList)
		sort.Strings(managedLabelList.LabelList)
		sort.Strings(managedLabelList.RegexLabelList)
	} else {
		klog.Infof("managedLabelList is empty: %v", managedLabelList)
	}
}
