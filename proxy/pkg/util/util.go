// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	projectv1 "github.com/openshift/api/project/v1"
	userv1 "github.com/openshift/api/user/v1"
	proxyconfig "github.com/stolostron/multicluster-observability-operator/proxy/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/informer"
	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/rewrite"
	"k8s.io/klog"

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
	clusterMatchRegExp = regexp.MustCompile(`([{|,][ ]*)cluster(=|!=|=~|!~)([ ]*)"([^"]+)"`) // Corrected regex to properly escape the double quote within the string literal.
)

func NewAccessReviewer(kConfig *rest.Config) (AccessReviewer, error) {
	return rbac.NewAccessReviewer(kConfig, nil)
}

// MetricsQueryParamsModifier holds the parameters for modifying metrics query parameters.
type MetricsQueryParamsModifier struct {
	Req            *http.Request
	ReqURL         string
	AccessReviewer AccessReviewer
	UPI            *UserProjectInfo
	MCI            informer.ManagedClusterInformable
}

// Modify will modify request url params for query metrics.
func (mqm *MetricsQueryParamsModifier) Modify() {
	userName := mqm.Req.Header.Get("X-Forwarded-User")
	klog.V(1).Infof("user is %v", userName)
	klog.V(1).Infof("URL is: %s", mqm.Req.URL)
	klog.V(1).Infof("URL path is: %v", mqm.Req.URL.Path)
	klog.V(1).Infof("URL RawQuery is: %v", mqm.Req.URL.RawQuery)
	token := mqm.Req.Header.Get("X-Forwarded-Access-Token")
	if token == "" {
		klog.Errorf("failed to get token from http header")
	}

	userMetricsAccess, err := getUserMetricsACLs(userName, token, mqm.ReqURL, mqm.AccessReviewer, mqm.UPI, mqm.MCI.GetAllManagedClusterNames())
	if err != nil {
		klog.Errorf("Failed to determine user's metrics access: %v", err)
		return
	}

	klog.Infof("user <%v> have metrics access to : %v", userName, userMetricsAccess)

	allAccess := canAccessAll(userMetricsAccess, mqm.MCI.GetAllManagedClusterNames())
	if allAccess {
		klog.Infof("user <%v> have access to all clusters and all namespaces", userName)
		return
	}

	var rawQuery string
	if mqm.Req.Method == "POST" {
		body, _ := io.ReadAll(mqm.Req.Body)
		_ = mqm.Req.Body.Close()
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
		mqm.Req.Body = io.NopCloser(strings.NewReader(rawQuery))
		mqm.Req.Header.Set("Content-Length", fmt.Sprint(len([]rune(rawQuery))))
		mqm.Req.ContentLength = int64(len([]rune(rawQuery)))
	} else {
		queryValues := mqm.Req.URL.Query()
		if len(queryValues) == 0 {
			return
		}
		queryValues = rewriteQuery(queryValues, userMetricsAccess, "query")
		queryValues = rewriteQuery(queryValues, userMetricsAccess, "match[]")
		mqm.Req.URL.RawQuery = queryValues.Encode()
		rawQuery = mqm.Req.URL.RawQuery
	}

	klog.V(1).Info("modified URL is:")
	klog.V(1).Infof("URL is: %s", mqm.Req.URL)
	klog.V(1).Infof("URL path is: %v", mqm.Req.URL.Path)
	klog.V(1).Infof("URL RawQuery is: %v", rawQuery)
}

func sendHTTPRequestWithClient(client *http.Client, url string, verb string, token string) (*http.Response, error) {
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
	return client.Do(req)
}

func FetchUserProjectList(token string, url string) []string {
	return FetchUserProjectListWithClient(http.DefaultClient, token, url)
}

func FetchUserProjectListWithClient(client *http.Client, token string, url string) []string {
	resp, err := sendHTTPRequestWithClient(client, url, "GET", token)
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
	return GetUserNameWithClient(http.DefaultClient, token, url)
}

func GetUserNameWithClient(client *http.Client, token string, url string) string {
	resp, err := sendHTTPRequestWithClient(client, url, "GET", token)
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

func getUserClusterList(projectList []string, managedClusterNames map[string]string) []string {
	clusterList := []string{}

	for _, projectName := range projectList {
		clusterName, ok := managedClusterNames[projectName]
		if ok {
			clusterList = append(clusterList, clusterName)
		}
	}

	return clusterList
}

func getUserMetricsACLs(userName string, token string, reqUrl string, accessReviewer AccessReviewer, upi *UserProjectInfo, managedClusterNames map[string]string) (map[string][]string, error) {

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

		for mcName := range managedClusterNames {
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
	projectList, ok := upi.GetUserProjectList(token)
	if !ok {
		projectList = FetchUserProjectList(token, reqUrl)
		upi.UpdateUserProject(userName, token, projectList)
		klog.V(1).Infof("projectList from api server = %v", projectList)
	}
	klog.V(1).Infof("cluster list: %v", managedClusterNames)
	klog.V(1).Infof("user <%s> project list: %v", userName, projectList)

	clusterList := getUserClusterList(projectList, managedClusterNames)
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

// canAccessAll check user have permission to access all clusters
func canAccessAll(clusterNamespaces map[string][]string, managedClusterNames map[string]string) bool {
	if len(managedClusterNames) == 0 && len(clusterNamespaces) == 0 {
		return false
	}

	for _, clusterName := range managedClusterNames {
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

var healthCheckFilePath = "/tmp/health"

func writeError(msg string) {
	f, err := os.OpenFile(healthCheckFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		klog.Errorf("failed to create file for probe: %v", err)
	}

	_, err = f.Write([]byte(msg))
	if err != nil {
		klog.Errorf("failed to write error message to probe file: %v", err)
	}

	_ = f.Close()
}
