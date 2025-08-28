// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

// Package metricquery is responsible for modifying incoming Prometheus queries to enforce multicluster
// Role-Based Access Control (RBAC). It inspects the user's permissions and injects the appropriate
// `cluster` and `namespace` label matchers into the PromQL query before it is sent to the upstream
// Observatorium API. This ensures that users can only access metrics from the clusters and namespaces
// they are authorized to see.
package metricquery

import (
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/cache"
	proxyconfig "github.com/stolostron/multicluster-observability-operator/proxy/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/informer"
	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/rewrite"
	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/util"
	"k8s.io/klog"
)

// AccessReviewer defines an interface for checking a user's access to metrics on managed clusters.
type AccessReviewer interface {
	// GetMetricsAccess returns a map where the keys are managed clusters and the values are slices of allowed namespaces for the user.
	GetMetricsAccess(token string, extraArgs ...string) (map[string][]string, error)
}

// Modifier holds the necessary components to modify a metrics query based on user permissions.
type Modifier struct {
	Req            *http.Request
	ReqURL         string
	AccessReviewer AccessReviewer
	UPI            *cache.UserProjectInfo
	MCI            informer.ManagedClusterInformable
}

// Modify inspects the incoming HTTP request, determines the user's access rights,
// and rewrites the PromQL query parameters (`query` and `match[]`) to enforce RBAC.
// If the user has access to all clusters and namespaces, the query is not modified.
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

	userMetricsAccess, err := mqm.getUserMetricsACLs(userName, token)
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

		modifiedQueryValues, err := rewriteQueryValues(queryValues, userMetricsAccess)
		if err != nil {
			return err
		}

		rawQuery = modifiedQueryValues.Encode()
		mqm.Req.Body = io.NopCloser(strings.NewReader(rawQuery))
		mqm.Req.Header.Set("Content-Length", fmt.Sprint(len([]rune(rawQuery))))
		mqm.Req.ContentLength = int64(len([]rune(rawQuery)))
	} else {
		queryValues := mqm.Req.URL.Query()
		if len(queryValues) == 0 {
			klog.V(1).Info("no query values found in URL, skipping rewrite")
			return nil
		}

		modifiedQueryValues, err := rewriteQueryValues(queryValues, userMetricsAccess)
		if err != nil {
			return err
		}

		mqm.Req.URL.RawQuery = modifiedQueryValues.Encode()
		rawQuery = mqm.Req.URL.RawQuery
	}

	klog.V(1).Info("modified URL is:")
	klog.V(1).Infof("URL is: %s", mqm.Req.URL)
	klog.V(1).Infof("URL path is: %v", mqm.Req.URL.Path)
	klog.V(1).Infof("URL RawQuery is: %v", rawQuery)
	return nil
}

func (mqm *Modifier) getUserMetricsACLs(userName string, token string) (map[string][]string, error) {
	// get all metricsaccess ACLs for the user
	// i.e every  metrics/<ns> on managedcluster CR defined for the user
	// in the returned map -  key is managedcluster name , value is namespaces accesible on that cluster
	metricsAccess, err := mqm.AccessReviewer.GetMetricsAccess(token)
	if err != nil {
		return nil, fmt.Errorf("failed to get Metrics Access from Access Reviewer: %w", err)
	}

	klog.V(1).Infof("user <%v>  metrics-access: %v", userName, metricsAccess)
	managedClusterNames := mqm.MCI.GetAllManagedClusterNames()

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
	projectList, ok := mqm.UPI.GetUserProjectList(token)
	if !ok {
		projectList, err = util.FetchUserProjectList(token, mqm.ReqURL)
		if err != nil {
			// if we cannot fetch project list, we will just assume the user has no project access.
			// The query will be modified based on the metrics access list only.
			klog.Errorf("failed to fetch user project list: %v", err)
			projectList = []string{}
		}
		mqm.UPI.UpdateUserProject(userName, token, projectList)
		klog.V(1).Infof("projectList from api server = %v", projectList)
	}
	klog.V(1).Infof("cluster list: %v", managedClusterNames)
	klog.V(1).Infof("user <%s> project list: %v", userName, projectList)

	clusterList := filterProjectsToManagedClusters(projectList, managedClusterNames)

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

// filterProjectsToManagedClusters filters a list of projects to only include those that are also managed clusters.
func filterProjectsToManagedClusters(projectList []string, managedClusterNames map[string]string) []string {
	clusterList := []string{}
	for _, projectName := range projectList {
		if clusterName, ok := managedClusterNames[projectName]; ok {
			clusterList = append(clusterList, clusterName)
		}
	}

	return clusterList
}

// rewriteQueryValues extracts the `query` and `match[]` parameters from a url.Values object,
// rewrites them to enforce RBAC, and returns the modified url.Values.
func rewriteQueryValues(queryValues url.Values, userMetricsAccess map[string][]string) (url.Values, error) {
	if originalQuery := queryValues.Get("query"); originalQuery != "" {
		modifiedQuery, err := rewriteQuery(originalQuery, userMetricsAccess)
		if err != nil {
			return nil, fmt.Errorf("failed to rewrite 'query' parameter: %w", err)
		}
		queryValues.Set("query", modifiedQuery)
	}

	if originalMatches, ok := queryValues["match[]"]; ok {
		modifiedMatches := make([]string, 0, len(originalMatches))
		for _, originalMatch := range originalMatches {
			modifiedMatch, err := rewriteQuery(originalMatch, userMetricsAccess)
			if err != nil {
				return nil, fmt.Errorf("failed to rewrite 'match[]' parameter: %w", err)
			}
			modifiedMatches = append(modifiedMatches, modifiedMatch)
		}
		queryValues["match[]"] = modifiedMatches
	}
	return queryValues, nil
}

// rewriteQuery is the core logic that injects `cluster` and `namespace` label matchers
// into a PromQL query based on the user's permissions.
func rewriteQuery(originalQuery string, userMetricsAccess map[string][]string) (string, error) {
	klog.V(2).Infof("REWRITE QUERY: originalQuery is: %v\n", originalQuery)
	if originalQuery == "" {
		return "", nil
	}

	clusterList := slices.Sorted(maps.Keys(userMetricsAccess))
	label := getLabel(originalQuery)
	modifiedQuery, err := rewrite.InjectLabels(originalQuery, label, clusterList)
	if err != nil {
		return "", err
	}

	klog.V(2).Infof("REWRITE QUERY Modified Query after injecting %v: %v\n", label, modifiedQuery)

	if !strings.Contains(originalQuery, proxyconfig.GetACMManagedClusterLabelNamesMetricName()) {
		filter := NewNamespaceFilter(userMetricsAccess)
		modifiedQuery, err = filter.AddNamespaceFilters(originalQuery, modifiedQuery)
		if err != nil {
			return "", err
		}
	}

	return modifiedQuery, nil
}

// getLabel determines which label to use for injecting cluster names.
// For the `acm_managed_cluster_labels` metric, the cluster name is held in the `name` label.
// For all other metrics, the standard `cluster` label is used. This function accounts for that difference.
func getLabel(originalQuery string) string {
	if strings.Contains(originalQuery, proxyconfig.GetACMManagedClusterLabelNamesMetricName()) {
		return "name"
	}
	return "cluster"
}

// canAccessAll checks if a user has permission to access all namespaces ("*") in all managed clusters.
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
