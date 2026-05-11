// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsutility

import (
	"fmt"
	"strings"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ProfileConfig defines how a recommendation profile aggregates 5m metrics over 1d.
type ProfileConfig struct {
	Name    string
	AggExpr func(metric5m string) string
}

// RecommendationProfiles lists all supported recommendation aggregation profiles.
var RecommendationProfiles = []ProfileConfig{
	{Name: "Max OverAll", AggExpr: Build1dMaxAggregationExpr},
	{Name: "P99", AggExpr: BuildP99AggregationExpr},
	{Name: "P95", AggExpr: BuildP95AggregationExpr},
	{Name: "Avg", AggExpr: BuildAvgAggregationExpr},
}

// Build1dMaxAggregationExpr returns a max_over_time aggregation over 1d.
func Build1dMaxAggregationExpr(metric5m string) string {
	return fmt.Sprintf("max_over_time(%s[1d])", metric5m)
}

// BuildP99AggregationExpr returns a P99 quantile_over_time aggregation over 1d.
func BuildP99AggregationExpr(metric5m string) string {
	return fmt.Sprintf("quantile_over_time(0.99, %s[1d])", metric5m)
}

// BuildP95AggregationExpr returns a P95 quantile_over_time aggregation over 1d.
func BuildP95AggregationExpr(metric5m string) string {
	return fmt.Sprintf("quantile_over_time(0.95, %s[1d])", metric5m)
}

// BuildAvgAggregationExpr returns an avg_over_time aggregation over 1d.
func BuildAvgAggregationExpr(metric5m string) string {
	return fmt.Sprintf("avg_over_time(%s[1d])", metric5m)
}

// Build1dAggregationExpr is a backward-compatible alias for Build1dMaxAggregationExpr.
func Build1dAggregationExpr(metric5m string) string {
	return Build1dMaxAggregationExpr(metric5m)
}

// BuildProfiledRecommendationExpr computes profile.AggExpr(usageMetric) * (rp/100).
func BuildProfiledRecommendationExpr(usageMetric string, rp int, profile ProfileConfig) string {
	return fmt.Sprintf("%s * (%d/100)", profile.AggExpr(usageMetric), rp)
}

// BuildRecommendationExpr is a backward-compatible alias using the Max OverAll profile.
func BuildRecommendationExpr(metric5m string, rp int) string {
	return BuildProfiledRecommendationExpr(metric5m, rp, RecommendationProfiles[0])
}

// RuleWithProfile creates a monitoringv1.Rule with profile and aggregation labels.
func RuleWithProfile(record, expr, profileName string) monitoringv1.Rule {
	return monitoringv1.Rule{
		Record: record,
		Expr:   intstr.FromString(expr),
		Labels: map[string]string{
			"profile":     profileName,
			"aggregation": "1d",
		},
	}
}

// NormalizeRecommendationPercentage returns DefaultRecommendationPercentage when rp is 0.
func NormalizeRecommendationPercentage(rp int) int {
	if rp == 0 {
		return DefaultRecommendationPercentage
	}
	return rp
}

// BuildNamespaceFilter creates a namespace filter string for Prometheus queries
func BuildNamespaceFilter(nsConfig RSPrometheusRuleConfig) (string, error) {
	ns := nsConfig.NamespaceFilterCriteria
	if len(ns.InclusionCriteria) > 0 && len(ns.ExclusionCriteria) > 0 {
		return "", fmt.Errorf("only one of inclusion or exclusion criteria allowed for namespacefiltercriteria")
	}
	if len(ns.InclusionCriteria) > 0 {
		return fmt.Sprintf(`namespace=~"%s"`, strings.Join(ns.InclusionCriteria, "|")), nil
	}
	if len(ns.ExclusionCriteria) > 0 {
		return fmt.Sprintf(`namespace!~"%s"`, strings.Join(ns.ExclusionCriteria, "|")), nil
	}
	return `namespace!=""`, nil
}

// BuildLabelJoin creates a label join string for Prometheus queries
func BuildLabelJoin(labelFilters []RSLabelFilter) (string, error) {
	for _, l := range labelFilters {
		if l.LabelName != "label_env" {
			continue
		}
		if len(l.InclusionCriteria) > 0 && len(l.ExclusionCriteria) > 0 {
			return "", fmt.Errorf("only one of inclusion or exclusion allowed for label_env")
		}
		var selector string
		switch {
		case len(l.InclusionCriteria) > 0:
			selector = fmt.Sprintf(`kube_namespace_labels{label_env=~"%s"}`, strings.Join(l.InclusionCriteria, "|"))
		case len(l.ExclusionCriteria) > 0:
			selector = fmt.Sprintf(`kube_namespace_labels{label_env!~"%s"}`, strings.Join(l.ExclusionCriteria, "|"))
		default:
			continue
		}
		return fmt.Sprintf(`* on (namespace) group_left() (%s or kube_namespace_labels{label_env=""})`, selector), nil
	}
	return "", nil
}
