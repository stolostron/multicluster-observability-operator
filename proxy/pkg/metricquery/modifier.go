// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package metricquery

import (
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/cache"
	proxyconfig "github.com/stolostron/multicluster-observability-operator/proxy/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/informer"
	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/rewrite"
	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/util"
	"k8s.io/klog"
)

// AccessReviewer defines an interface for the GetMetricsAccess method.
type AccessReviewer interface {
	// GetMetricsAccess returns a map where the keys are managed clusters and the values are slices of allowed namespaces for the user.
	GetMetricsAccess(token string, extraArgs ...string) (map[string][]string, error)
}

// Modifier holds the parameters for modifying metrics query parameters.
type Modifier struct {
	Req            *http.Request
	ReqURL         string
	AccessReviewer AccessReviewer
	UPI            *cache.UserProjectInfo
	MCI            informer.ManagedClusterInformable
}

// Modify will modify request url params for query metrics.
func (mqm *Modifier) Modify() error {
	userName := mqm.Req.Header.Get("X-Forwarded-User")
	klog.V(1).Infof("user is %v", userName)
	klog.V(1).Infof("URL is: %s", mqm.Req.URL)
	klog.V(1).Infof("URL path is: %v", mqm.Req.URL.Path)
	klog.V(1).Infof("URL RawQuery is: %v", mqm.Req.URL.RawQuery)
	token := mqm.Req.Header.Get("X-Forwarded-Access-Token")
	if token == "" {
		return fmt.Errorf("failed to get token from http header")
	}

	userMetricsAccess, err := getUserMetricsACLs(userName, token, mqm.ReqURL, mqm.AccessReviewer, mqm.UPI, mqm.MCI.GetAllManagedClusterNames())
	if err != nil {
		return fmt.Errorf("failed to determine user's metrics access: %w", err)
	}

	klog.V(1).Infof("user <%v> have metrics access to : %v", userName, userMetricsAccess)

	allAccess := canAccessAll(userMetricsAccess, mqm.MCI.GetAllManagedClusterNames())
	if allAccess {
		klog.V(1).Infof("user <%v> have access to all clusters and all namespaces", userName)
		return nil
	}

	var rawQuery string
	if mqm.Req.Method == "POST" {
		body, _ := io.ReadAll(mqm.Req.Body)
		_ = mqm.Req.Body.Close()
		queryValues, err := url.ParseQuery(string(body))
		if err != nil {
			return fmt.Errorf("failed to parse request body: %w", err)
		}
		if len(queryValues) == 0 {
			klog.V(1).Info("no query values found in POST body, skipping rewrite")
			return nil
		}
		queryValues, err = rewriteQuery(queryValues, userMetricsAccess, "query")
		if err != nil {
			return fmt.Errorf("failed to rewrite 'query' parameter: %w", err)
		}
		queryValues, err = rewriteQuery(queryValues, userMetricsAccess, "match[]")
		if err != nil {
			return fmt.Errorf("failed to rewrite 'match[]' parameter: %w", err)
		}
		rawQuery = queryValues.Encode()
		mqm.Req.Body = io.NopCloser(strings.NewReader(rawQuery))
		mqm.Req.Header.Set("Content-Length", fmt.Sprint(len([]rune(rawQuery))))
		mqm.Req.ContentLength = int64(len([]rune(rawQuery)))
	} else {
		queryValues := mqm.Req.URL.Query()
		if len(queryValues) == 0 {
			klog.V(1).Info("no query values found in URL, skipping rewrite")
			return nil
		}
		queryValues, err = rewriteQuery(queryValues, userMetricsAccess, "query")
		if err != nil {
			return fmt.Errorf("failed to rewrite 'query' parameter: %w", err)
		}
		queryValues, err = rewriteQuery(queryValues, userMetricsAccess, "match[]")
		if err != nil {
			return fmt.Errorf("failed to rewrite 'match[]' parameter: %w", err)
		}
		mqm.Req.URL.RawQuery = queryValues.Encode()
		rawQuery = mqm.Req.URL.RawQuery
	}

	klog.V(1).Info("modified URL is:")
	klog.V(1).Infof("URL is: %s", mqm.Req.URL)
	klog.V(1).Infof("URL path is: %v", mqm.Req.URL.Path)
	klog.V(1).Infof("URL RawQuery is: %v", rawQuery)
	return nil
}

func getUserMetricsACLs(userName string, token string, reqUrl string, accessReviewer AccessReviewer, upi *cache.UserProjectInfo, managedClusterNames map[string]string) (map[string][]string, error) {
	// get all metricsaccess ACLs for the user
	// i.e every  metrics/<ns> on managedcluster CR defined for the user
	// in the returned map -  key is managedcluster name , value is namespaces accesible on that cluster
	metricsAccess, arerr := accessReviewer.GetMetricsAccess(token)
	if arerr != nil {
		return nil, fmt.Errorf("failed to get Metrics Access from Access Reviewer: %w", arerr)
	}

	klog.V(1).Infof("user <%v>  metrics-access: %v", userName, metricsAccess)

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

	// for backward compatibility, support the existing access control mechanism
	// i.e access to managedcluster project\namespace means access to all namespaces on that managedcluster

	//get all managedcluster project/namespace user has access to
	projectList, ok := upi.GetUserProjectList(token)
	if !ok {
		var err error
		projectList, err = util.FetchUserProjectList(token, reqUrl)
		if err != nil {
			// if we cannot fetch project list, we will just assume the user has no project access.
			// The query will be modified based on the metrics access list only.
			klog.Errorf("failed to fetch user project list: %v", err)
			projectList = []string{}
		}
		upi.UpdateUserProject(userName, token, projectList)
		klog.V(1).Infof("projectList from api server = %v", projectList)
	}
	klog.V(1).Infof("cluster list: %v", managedClusterNames)
	klog.V(1).Infof("user <%s> project list: %v", userName, projectList)

	clusterList := getUserClusterList(projectList, managedClusterNames)

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
	klog.V(2).Infof("common namespaces across clusters: %v  in metrics access map: %v", clusters, metricsAccess)

	//make a smaller map of metricsaccess for just the requested clusters
	reqClustersMetricsAccess := make(map[string][]string)
	if len(clusters) == 0 {
		reqClustersMetricsAccess = metricsAccess
	} else {
		for _, cluster := range clusters {
			reqClustersMetricsAccess[cluster] = metricsAccess[cluster]
		}
	}
	klog.V(2).Infof("requested clusters metrics access map: %v", reqClustersMetricsAccess)

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

	klog.V(2).Infof("allAccessCount  : %v", allAccessCount)
	klog.V(2).Infof("Namespaces Count  : %v", namespaceCounts)

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

	klog.V(2).Infof("common namespaces across clusters: %v   : %v", clusters, commonNamespaces)
	return commonNamespaces
}

// clusterVisitor is a promql.Visitor that extracts the cluster names from a PromQL query.
type clusterVisitor struct {
	userMetricsAccess map[string][]string
	queryClusters     map[string]struct{}
}

// Visit implements the promql.Visitor interface.
func (v *clusterVisitor) Visit(node parser.Node, path []parser.Node) (parser.Visitor, error) {
	if vs, ok := node.(*parser.VectorSelector); ok {
		for _, matcher := range vs.LabelMatchers {
			if matcher.Name == "cluster" {
				// We found a cluster matcher. Now we need to apply it to the list of clusters
				// the user has access to.
				accessibleClusters := make([]string, 0, len(v.userMetricsAccess))
				for cluster := range v.userMetricsAccess {
					accessibleClusters = append(accessibleClusters, cluster)
				}

				// This is the pattern from the query (e.g., "dev-.*" or "prod-us-east-1")
				pattern := matcher.Value

				var matchedClusters []string
				switch matcher.Type {
				case labels.MatchEqual:
					// Simple equality check
					if _, exists := v.userMetricsAccess[pattern]; exists {
						matchedClusters = []string{pattern}
					}
				case labels.MatchNotEqual:
					for _, cluster := range accessibleClusters {
						if cluster != pattern {
							matchedClusters = append(matchedClusters, cluster)
						}
					}
				case labels.MatchRegexp:
					reg, err := regexp.Compile(pattern)
					if err != nil {
						// Invalid regex in query, skip this matcher
						continue
					}
					for _, cluster := range accessibleClusters {
						if reg.MatchString(cluster) {
							matchedClusters = append(matchedClusters, cluster)
						}
					}
				case labels.MatchNotRegexp:
					reg, err := regexp.Compile(pattern)
					if err != nil {
						// Invalid regex in query, skip this matcher
						continue
					}
					for _, cluster := range accessibleClusters {
						if !reg.MatchString(cluster) {
							matchedClusters = append(matchedClusters, cluster)
						}
					}
				}

				for _, c := range matchedClusters {
					v.queryClusters[c] = struct{}{}
				}
			}
		}
	}
	return v, nil
}

// read clusters in the user's query
func getClustersInQuery(queryValues url.Values, key string, userMetricsAccess map[string][]string) ([]string, error) {
	query := queryValues.Get(key)
	if query == "" {
		return []string{}, nil
	}

	expr, err := parser.ParseExpr(query)
	if err != nil {
		return nil, fmt.Errorf("failed to parse promql query <%s>: %w", query, err)
	}

	// Collect all unique cluster names found in the query's label matchers.
	visitor := &clusterVisitor{
		userMetricsAccess: userMetricsAccess,
		queryClusters:     make(map[string]struct{}),
	}

	// Walk the AST
	err = parser.Walk(visitor, expr, nil)
	if err != nil {
		return nil, fmt.Errorf("error walking promql ast: %w", err)
	}

	return slices.Collect(maps.Keys(visitor.queryClusters)), nil
}

func rewriteQuery(queryValues url.Values, userMetricsAccess map[string][]string, key string) (url.Values, error) {
	klog.V(2).Infof("REWRITE QUERY: queryValues: %v, userMetricsAccess: %v, key: %v\n", queryValues, userMetricsAccess, key)
	originalQuery := queryValues.Get(key)
	klog.V(2).Infof("REWRITE QUERY: key is: %v, originalQuery is: %v\n", key, originalQuery)
	if len(originalQuery) == 0 {
		return queryValues, nil
	}

	clusterList := getClusterList(userMetricsAccess)
	label := getLabel(originalQuery)
	modifiedQuery, err := rewrite.InjectLabels(originalQuery, label, clusterList)
	if err != nil {
		return nil, err
	}

	klog.V(2).Infof("REWRITE QUERY Modified Query after injecting %v: %v\n", label, modifiedQuery)

	if !strings.Contains(originalQuery, proxyconfig.GetACMManagedClusterLabelNamesMetricName()) {
		modifiedQuery, err = injectNamespaces(queryValues, key, userMetricsAccess, modifiedQuery)
		if err != nil {
			return nil, err
		}
	}

	queryValues.Del(key)
	queryValues.Add(key, modifiedQuery)
	return queryValues, nil
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
	queryClusters, err := getClustersInQuery(queryValues, key, userMetricsAccess)
	if err != nil {
		return "", err
	}
	commonNsAcrossQueryClusters := getCommonNamespacesAcrossClusters(queryClusters, userMetricsAccess)
	allNamespaceAccess := len(commonNsAcrossQueryClusters) == 1 && commonNsAcrossQueryClusters[0] == "*"

	klog.V(2).Infof("REWRITE QUERY Modified Query hasAccess to All namespaces: \n %v", allNamespaceAccess)
	if !allNamespaceAccess {
		modifiedQuery2, err := rewrite.InjectLabels(modifiedQuery, "namespace", commonNsAcrossQueryClusters)
		if err != nil {
			return modifiedQuery, err
		}
		klog.V(2).Infof("REWRITE QUERY Modified Query after injecting namespaces:  \n %v", modifiedQuery2)
		return modifiedQuery2, nil
	}
	return modifiedQuery, nil
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
