// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rewrite

import (
	"strings"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"k8s.io/klog/v2"
)

// labelInjector is a visitor that walks the PromQL AST and appends a label matcher
// into all vector and matrix selectors.
type labelInjector struct {
	matcher *labels.Matcher
}

// Visit implements the parser.Visitor interface.
func (li *labelInjector) Visit(node parser.Node, path []parser.Node) (parser.Visitor, error) {
	switch n := node.(type) {
	case *parser.VectorSelector:
		n.LabelMatchers = append(n.LabelMatchers, li.matcher)
	case *parser.MatrixSelector:
		// MatrixSelectors embed VectorSelectors, so we modify the underlying VectorSelector.
		if vs, ok := n.VectorSelector.(*parser.VectorSelector); ok {
			vs.LabelMatchers = append(vs.LabelMatchers, li.matcher)
		}
	}
	return li, nil
}

// InjectLabels is used to inject additional label filters into an original PromQL query
// by walking its AST and appending a new matcher to every selector.
func InjectLabels(query string, label string, values []string) (string, error) {
	if len(values) == 0 {
		// If there are no values, create a matcher that matches nothing.
		// This is more robust than returning an error or the original query.
		values = []string{""}
	}

	expr, err := parser.ParseExpr(query)
	if err != nil {
		klog.Errorf("Failed to parse the query %s: %v", query, err)
		return "", err
	}

	matchType := labels.MatchRegexp
	if len(values) == 1 {
		matchType = labels.MatchEqual
	}

	matcher := &labels.Matcher{
		Name:  label,
		Type:  matchType,
		Value: strings.Join(values, "|"),
	}

	visitor := &labelInjector{matcher: matcher}
	if err := parser.Walk(visitor, expr, nil); err != nil {
		klog.Errorf("Failed to walk the PromQL AST: %v", err)
		return "", err
	}

	finalQuery := expr.String()
	klog.Infof("Query string after filter inject: %s", finalQuery)

	return finalQuery, nil
}
