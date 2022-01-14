// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package rewrite

import (
	"regexp"
	"strings"

	"github.com/prometheus-community/prom-label-proxy/injectproxy"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"k8s.io/klog"
)

const (
	placeholderMetrics = "acm_metrics_placeholder"
)

// InjectLabels is used to inject addtional label filters into original query
func InjectLabels(query string, label string, values []string) (string, error) {

	reg := regexp.MustCompile(`([{|,][ ]*)(` + label + `[ ]*)(=|!=|=~|!~)([ ]*"[^"]+")`)
	query = reg.ReplaceAllString(query, "$1 "+placeholderMetrics+" $3$4")

	expr, err := parser.ParseExpr(query)
	if err != nil {
		klog.Errorf("Failed to parse the query %s: %v", query, err)
		return "", err
	}

	matchType := labels.MatchRegexp
	if len(values) == 1 {
		matchType = labels.MatchEqual
	}
	err = injectproxy.NewEnforcer([]*labels.Matcher{
		{
			Name:  label,
			Type:  matchType,
			Value: strings.Join(values[:], "|"),
		},
	}...).EnforceNode(expr)
	if err != nil {
		klog.Errorf("Failed to inject the label filters: %v", err)
		return "", err
	}

	query = strings.Replace(expr.String(), placeholderMetrics, label, -1)
	klog.Infof("Query string after filter inject: %s", query)

	return query, nil
}
