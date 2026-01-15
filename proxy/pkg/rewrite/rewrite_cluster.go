// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rewrite

import (
	"strings"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	proxyconfig "github.com/stolostron/multicluster-observability-operator/proxy/pkg/config"
	"k8s.io/klog/v2"
)

// clusterLabelInjector is a visitor that walks the PromQL AST. It intelligently injects
// a label matcher for cluster filtering into every vector and matrix selector it encounters.
type clusterLabelInjector struct {
	clusterValues []string
}

// Visit implements the parser.Visitor interface. It inspects each node in the AST.
func (li *clusterLabelInjector) Visit(node parser.Node, _ []parser.Node) (parser.Visitor, error) {
	var selector *parser.VectorSelector
	switch n := node.(type) {
	case *parser.VectorSelector:
		selector = n
	case *parser.MatrixSelector:
		// MatrixSelectors embed VectorSelectors, so we operate on the underlying VectorSelector.
		if vs, ok := n.VectorSelector.(*parser.VectorSelector); ok {
			selector = vs
		}
	}

	if selector != nil {
		// The label used to identify a cluster's name depends on the metric.
		// For `acm_managed_cluster_labels`, the identifier is the `name` label.
		// For all other metrics, the standard `cluster` label is used.
		labelName := "cluster"
		if selector.Name == proxyconfig.ACMManagedClusterLabelNamesMetricName {
			labelName = "name"
		}

		matchType := labels.MatchRegexp
		if len(li.clusterValues) == 1 {
			matchType = labels.MatchEqual
		}

		matcher := &labels.Matcher{
			Name:  labelName,
			Type:  matchType,
			Value: strings.Join(li.clusterValues, "|"),
		}
		selector.LabelMatchers = append(selector.LabelMatchers, matcher)
	}

	return li, nil
}

// InjectClusterLabels injects cluster-based RBAC filters into a PromQL query.
// It parses the query into an AST and walks it, applying the correct cluster filtering
// label to each metric selector.
//
// This function is aware of a special case:
//   - For the `acm_managed_cluster_labels` metric, it injects a `name` label matcher.
//   - For all other metrics, it injects a standard `cluster` label matcher.
//
// This ensures that complex queries with binary operators joining different metrics
// are filtered correctly.
func InjectClusterLabels(query string, clusterValues []string) (string, error) {
	if len(clusterValues) == 0 {
		// If there are no values, create a matcher that matches nothing.
		// This is more robust than returning an error or the original query.
		clusterValues = []string{""}
	}

	expr, err := parser.ParseExpr(query)
	if err != nil {
		klog.Errorf("Failed to parse the query %s: %v", query, err)
		return "", err
	}

	visitor := &clusterLabelInjector{clusterValues: clusterValues}
	if err := parser.Walk(visitor, expr, nil); err != nil {
		klog.Errorf("Failed to walk the PromQL AST: %v", err)
		return "", err
	}

	finalQuery := expr.String()
	klog.Infof("Query string after filter inject: %s", finalQuery)

	return finalQuery, nil
}
