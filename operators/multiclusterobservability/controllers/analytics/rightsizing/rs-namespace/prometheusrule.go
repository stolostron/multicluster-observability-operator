// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsnamespace

import (
	"fmt"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing/rs-utility"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// function helps to build PrometheusRule based on configdata
func GeneratePrometheusRule(configData rsutility.RSNamespaceConfigMapData) (monitoringv1.PrometheusRule, error) {
	nsFilter, err := rsutility.BuildNamespaceFilter(configData.PrometheusRuleConfig)
	if err != nil {
		return monitoringv1.PrometheusRule{}, err
	}

	labelJoin, err := rsutility.BuildLabelJoin(configData.PrometheusRuleConfig.LabelFilterCriteria)
	if err != nil {
		return monitoringv1.PrometheusRule{}, err
	}

	// Define durations
	duration5m := monitoringv1.Duration("5m")
	duration1d := monitoringv1.Duration("15m")

	// Helper for rules
	rule := func(record, metricExpr string) monitoringv1.Rule {
		expr := metricExpr
		if labelJoin != "" {
			expr = fmt.Sprintf("%s %s", metricExpr, labelJoin)
		}
		return monitoringv1.Rule{
			Record: record,
			Expr:   intstr.FromString(expr),
		}
	}

	return monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PrometheusRuleName,
			Namespace: rsutility.MonitoringNamespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "PrometheusRule",
			APIVersion: "monitoring.coreos.com/v1",
		},
		Spec: monitoringv1.PrometheusRuleSpec{
			Groups: []monitoringv1.RuleGroup{
				{
					Name:     "acm-right-sizing-namespace-5m.rule",
					Interval: &duration5m,
					Rules:    buildNamespaceRules5m(nsFilter, rule),
				},
				{
					Name:     "acm-right-sizing-namespace-1d.rules",
					Interval: &duration1d,
					Rules:    buildNamespaceRules1d(configData),
				},
				{
					Name:     "acm-right-sizing-cluster-5m.rule",
					Interval: &duration5m,
					Rules:    buildClusterRules5m(nsFilter, rule),
				},
				{
					Name:     "acm-right-sizing-cluster-1d.rule",
					Interval: &duration1d,
					Rules:    buildClusterRules1d(configData),
				},
			},
		},
	}, nil
}

func buildNamespaceRules5m(
	nsFilter string,
	rule func(string, string) monitoringv1.Rule,
) []monitoringv1.Rule {
	return []monitoringv1.Rule{
		rule(
			"acm_rs:namespace:cpu_request_hard:5m",
			fmt.Sprintf(
				`max_over_time(sum(kube_resourcequota{resource=~"requests.cpu", type="hard", %s}) by (namespace)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:namespace:cpu_request:5m",
			fmt.Sprintf(
				`max_over_time(sum(kube_pod_container_resource_requests{`+
					`%s, resource="cpu", container!=""}) by (namespace)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:namespace:cpu_usage:5m",
			fmt.Sprintf(
				`max_over_time(sum(node_namespace_pod_container:`+
					`container_cpu_usage_seconds_total:sum_irate{`+
					`%s, container!=""}) by (namespace)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:namespace:memory_request_hard:5m",
			fmt.Sprintf(
				`max_over_time(sum(kube_resourcequota{resource=~"requests.memory", type="hard", %s}) by (namespace)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:namespace:memory_request:5m",
			fmt.Sprintf(
				`max_over_time(sum(kube_pod_container_resource_requests{`+
					`%s, resource="memory", container!=""}) by (namespace)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:namespace:memory_usage:5m",
			fmt.Sprintf(
				`max_over_time(sum(container_memory_working_set_bytes{`+
					`%s, container!=""}) by (namespace)[5m:])`,
				nsFilter,
			),
		),
	}
}

func buildNamespaceRules1d(configData rsutility.RSNamespaceConfigMapData) []monitoringv1.Rule {
	rp := rsutility.NormalizeRecommendationPercentage(configData.PrometheusRuleConfig.RecommendationPercentage)
	const metricsPerProfile = 8
	rules := make([]monitoringv1.Rule, 0, metricsPerProfile*len(rsutility.RecommendationProfiles))
	for _, profile := range rsutility.RecommendationProfiles {
		rules = append(rules,
			rsutility.RuleWithProfile("acm_rs:namespace:cpu_request_hard", profile.AggExpr("acm_rs:namespace:cpu_request_hard:5m"), profile.Name),
			rsutility.RuleWithProfile("acm_rs:namespace:cpu_request", profile.AggExpr("acm_rs:namespace:cpu_request:5m"), profile.Name),
			rsutility.RuleWithProfile("acm_rs:namespace:cpu_usage", profile.AggExpr("acm_rs:namespace:cpu_usage:5m"), profile.Name),
			rsutility.RuleWithProfile("acm_rs:namespace:cpu_recommendation",
				rsutility.BuildProfiledRecommendationExpr("acm_rs:namespace:cpu_usage:5m", rp, profile), profile.Name),
			rsutility.RuleWithProfile("acm_rs:namespace:memory_request_hard", profile.AggExpr("acm_rs:namespace:memory_request_hard:5m"), profile.Name),
			rsutility.RuleWithProfile("acm_rs:namespace:memory_request", profile.AggExpr("acm_rs:namespace:memory_request:5m"), profile.Name),
			rsutility.RuleWithProfile("acm_rs:namespace:memory_usage", profile.AggExpr("acm_rs:namespace:memory_usage:5m"), profile.Name),
			rsutility.RuleWithProfile("acm_rs:namespace:memory_recommendation",
				rsutility.BuildProfiledRecommendationExpr("acm_rs:namespace:memory_usage:5m", rp, profile), profile.Name),
		)
	}
	return rules
}

func buildClusterRules5m(
	nsFilter string,
	rule func(string, string) monitoringv1.Rule,
) []monitoringv1.Rule {
	return []monitoringv1.Rule{
		rule(
			"acm_rs:cluster:cpu_request_hard:5m",
			fmt.Sprintf(
				`max_over_time(sum(kube_resourcequota{resource=~"requests.cpu", type="hard", %s}) by (cluster)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:cluster:cpu_request:5m",
			fmt.Sprintf(
				`max_over_time(sum(kube_pod_container_resource_requests{`+
					`%s, resource="cpu", container!=""}) by (cluster)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:cluster:cpu_usage:5m",
			fmt.Sprintf(
				`max_over_time(sum(node_namespace_pod_container:`+
					`container_cpu_usage_seconds_total:sum_irate{`+
					`%s, container!=""}) by (cluster)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:cluster:memory_request_hard:5m",
			fmt.Sprintf(
				`max_over_time(sum(kube_resourcequota{resource=~"requests.memory", type="hard", %s}) by (cluster)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:cluster:memory_request:5m",
			fmt.Sprintf(
				`max_over_time(sum(kube_pod_container_resource_requests{`+
					`%s, resource="memory", container!=""}) by (cluster)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:cluster:memory_usage:5m",
			fmt.Sprintf(
				`max_over_time(sum(container_memory_working_set_bytes{`+
					`%s, container!=""}) by (cluster)[5m:])`,
				nsFilter,
			),
		),
	}
}

func buildClusterRules1d(configData rsutility.RSNamespaceConfigMapData) []monitoringv1.Rule {
	rp := rsutility.NormalizeRecommendationPercentage(configData.PrometheusRuleConfig.RecommendationPercentage)
	const metricsPerProfile = 8
	rules := make([]monitoringv1.Rule, 0, metricsPerProfile*len(rsutility.RecommendationProfiles))
	for _, profile := range rsutility.RecommendationProfiles {
		rules = append(rules,
			rsutility.RuleWithProfile("acm_rs:cluster:cpu_request_hard", profile.AggExpr("acm_rs:cluster:cpu_request_hard:5m"), profile.Name),
			rsutility.RuleWithProfile("acm_rs:cluster:cpu_request", profile.AggExpr("acm_rs:cluster:cpu_request:5m"), profile.Name),
			rsutility.RuleWithProfile("acm_rs:cluster:cpu_usage", profile.AggExpr("acm_rs:cluster:cpu_usage:5m"), profile.Name),
			rsutility.RuleWithProfile("acm_rs:cluster:cpu_recommendation",
				rsutility.BuildProfiledRecommendationExpr("acm_rs:cluster:cpu_usage:5m", rp, profile), profile.Name),
			rsutility.RuleWithProfile("acm_rs:cluster:memory_request_hard", profile.AggExpr("acm_rs:cluster:memory_request_hard:5m"), profile.Name),
			rsutility.RuleWithProfile("acm_rs:cluster:memory_request", profile.AggExpr("acm_rs:cluster:memory_request:5m"), profile.Name),
			rsutility.RuleWithProfile("acm_rs:cluster:memory_usage", profile.AggExpr("acm_rs:cluster:memory_usage:5m"), profile.Name),
			rsutility.RuleWithProfile("acm_rs:cluster:memory_recommendation",
				rsutility.BuildProfiledRecommendationExpr("acm_rs:cluster:memory_usage:5m", rp, profile), profile.Name),
		)
	}
	return rules
}
