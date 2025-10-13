// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package metricquery

import (
	"fmt"
	"maps"
	"regexp"
	"slices"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/rewrite"
	"k8s.io/klog/v2"
)

// NamespaceFilter analyzes a PromQL query against a user's permissions to determine
// the correct RBAC filters to apply.
type NamespaceFilter struct {
	userMetricsAccess map[string][]string
}

// NewNamespaceFilter creates a new NamespaceFilter.
func NewNamespaceFilter(userMetricsAccess map[string][]string) *NamespaceFilter {
	return &NamespaceFilter{userMetricsAccess: userMetricsAccess}
}

// AddNamespaceFilters is responsible for adding the `namespace` label selector to the query.
func (nf *NamespaceFilter) AddNamespaceFilters(originalQuery, modifiedQuery string) (string, error) {
	queryClusters, err := nf.getClustersInQuery(originalQuery)
	if err != nil {
		return "", err
	}
	commonNsAcrossQueryClusters := nf.getCommonNamespacesAcrossClusters(queryClusters)
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

// getCommonNamespacesAcrossClusters calculates the intersection of accessible namespaces for a given set of clusters.
// This is used to determine the correct `namespace` label selectors to inject into the query.
// If the user has access to all namespaces in all the specified clusters, it returns ["*"]
func (nf *NamespaceFilter) getCommonNamespacesAcrossClusters(clusters []string) []string {
	klog.V(2).Infof("common namespaces across clusters: %v  in metrics access map: %v", clusters, nf.userMetricsAccess)

	//make a smaller map of metricsaccess for just the requested clusters
	reqClustersMetricsAccess := make(map[string][]string)
	if len(clusters) == 0 {
		reqClustersMetricsAccess = nf.userMetricsAccess
	} else {
		for _, cluster := range clusters {
			reqClustersMetricsAccess[cluster] = nf.userMetricsAccess[cluster]
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
			continue
		}
		for _, namespace := range namespaces {
			if count, ok := namespaceCounts[namespace]; ok {
				namespaceCounts[namespace] = count + 1
			} else {
				namespaceCounts[namespace] = 1
			}
		}
	}

	klog.V(2).Infof("allAccessCount  : %v", allAccessCount)
	klog.V(2).Infof("Namespaces Count  : %v", namespaceCounts)

	if allAccessCount == len(reqClustersMetricsAccess) {
		return []string{"*"}
	}

	commonNamespaces := []string{}
	for ns, count := range namespaceCounts {
		if (count + allAccessCount) == len(reqClustersMetricsAccess) {
			commonNamespaces = append(commonNamespaces, ns)
		}
	}

	klog.V(2).Infof("common namespaces across clusters: %v   : %v", clusters, commonNamespaces)
	slices.Sort(commonNamespaces)
	return commonNamespaces
}

// clusterVisitor is a promql.Visitor that walks the PromQL AST to extract `cluster` label matchers.
type clusterVisitor struct {
	userMetricsAccess map[string][]string
	queryClusters     map[string]struct{}
}

// Visit implements the promql.Visitor interface, inspecting vector selectors for "cluster" labels.
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

// getClustersInQuery parses a PromQL query and uses a clusterVisitor to determine which clusters
// the user is explicitly targeting. If no `cluster` label is found, it returns an empty slice,
// implying that all accessible clusters are being targeted.
func (nf *NamespaceFilter) getClustersInQuery(query string) ([]string, error) {
	if query == "" {
		return []string{}, nil
	}

	expr, err := parser.ParseExpr(query)
	if err != nil {
		return nil, fmt.Errorf("failed to parse promql query <%s>: %w", query, err)
	}

	// Collect all unique cluster names found in the query's label matchers.
	visitor := &clusterVisitor{
		userMetricsAccess: nf.userMetricsAccess,
		queryClusters:     make(map[string]struct{}),
	}

	// Walk the AST
	err = parser.Walk(visitor, expr, nil)
	if err != nil {
		return nil, fmt.Errorf("error walking promql ast: %w", err)
	}

	return slices.Collect(maps.Keys(visitor.queryClusters)), nil
}
