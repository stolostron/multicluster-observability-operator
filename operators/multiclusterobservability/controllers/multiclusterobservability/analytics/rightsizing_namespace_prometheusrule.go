package analytics

import (
	"fmt"
	"strings"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func generatePrometheusRule(configData RSNamespaceConfigMapData) (monitoringv1.PrometheusRule, error) {
	ns := configData.PrometheusRuleConfig.NamespaceFilterCriteria
	recommendationPercentage := configData.PrometheusRuleConfig.RecommendationPercentage

	// Enforce only one of inclusion/exclusion for namespaces
	if len(ns.InclusionCriteria) > 0 && len(ns.ExclusionCriteria) > 0 {
		return monitoringv1.PrometheusRule{}, fmt.Errorf("only one of inclusion or exclusion allowed for namespaceFilterCriteria")
	}

	// Build namespace filter
	var nsFilter string
	if len(ns.InclusionCriteria) > 0 {
		nsFilter = fmt.Sprintf(`namespace=~"%s"`, strings.Join(ns.InclusionCriteria, "|"))
	} else if len(ns.ExclusionCriteria) > 0 {
		nsFilter = fmt.Sprintf(`namespace!~"%s"`, strings.Join(ns.ExclusionCriteria, "|"))
	} else {
		nsFilter = `namespace!=""`
	}

	// Build label_env filter only if label filter is provided
	var labelJoin string
	for _, l := range configData.PrometheusRuleConfig.LabelFilterCriteria {
		if l.LabelName != "label_env" {
			continue
		}
		if len(l.InclusionCriteria) > 0 && len(l.ExclusionCriteria) > 0 {
			return monitoringv1.PrometheusRule{}, fmt.Errorf("only one of inclusion or exclusion allowed for label_env")
		}
		if len(l.InclusionCriteria) > 0 {
			selector := fmt.Sprintf(`kube_namespace_labels{label_env=~"%s"}`, strings.Join(l.InclusionCriteria, "|"))
			labelJoin = fmt.Sprintf(`* on (namespace) group_left() (%s or kube_namespace_labels{label_env=""})`, selector)
		} else if len(l.ExclusionCriteria) > 0 {
			selector := fmt.Sprintf(`kube_namespace_labels{label_env!~"%s"}`, strings.Join(l.ExclusionCriteria, "|"))
			labelJoin = fmt.Sprintf(`* on (namespace) group_left() (%s or kube_namespace_labels{label_env=""})`, selector)
		}
		break
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
	ruleWithLabels := func(record, expr string) monitoringv1.Rule {
		return monitoringv1.Rule{
			Record: record,
			Expr:   intstr.FromString(expr),
			Labels: map[string]string{
				"profile":     "Max OverAll",
				"aggregation": "1d",
			},
		}
	}

	// Group: namespace 5m
	nsRules5m := []monitoringv1.Rule{
		rule("acm_rs:namespace:cpu_request_hard:5m", fmt.Sprintf(`max_over_time(sum(kube_resourcequota{resource=~"requests.cpu", type="hard", %s}) by (namespace)[5m:])`, nsFilter)),
		rule("acm_rs:namespace:cpu_request:5m", fmt.Sprintf(`max_over_time(sum(kube_pod_container_resource_requests{%s, resource="cpu", container!=""}) by (namespace)[5m:])`, nsFilter)),
		rule("acm_rs:namespace:cpu_usage:5m", fmt.Sprintf(`max_over_time(sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{%s, container!=""}) by (namespace)[5m:])`, nsFilter)),
		rule("acm_rs:namespace:memory_request_hard:5m", fmt.Sprintf(`max_over_time(sum(kube_resourcequota{resource=~"requests.memory", type="hard", %s}) by (namespace)[5m:])`, nsFilter)),
		rule("acm_rs:namespace:memory_request:5m", fmt.Sprintf(`max_over_time(sum(kube_pod_container_resource_requests{%s, resource="memory", container!=""}) by (namespace)[5m:])`, nsFilter)),
		rule("acm_rs:namespace:memory_usage:5m", fmt.Sprintf(`max_over_time(sum(container_memory_working_set_bytes{%s, container!=""}) by (namespace)[5m:])`, nsFilter)),
	}

	// Group: namespace 1d
	nsRules1d := []monitoringv1.Rule{
		ruleWithLabels("acm_rs:namespace:cpu_request_hard", `max_over_time(acm_rs:namespace:cpu_request_hard:5m[1d])`),
		ruleWithLabels("acm_rs:namespace:cpu_request", `max_over_time(acm_rs:namespace:cpu_request:5m[1d])`),
		ruleWithLabels("acm_rs:namespace:cpu_usage", `max_over_time(acm_rs:namespace:cpu_usage:5m[1d])`),
		ruleWithLabels("acm_rs:namespace:cpu_recommendation", fmt.Sprintf(`max_over_time(acm_rs:namespace:cpu_usage{profile="Max OverAll"}[1d]) * (1 + (%d/100))`, recommendationPercentage)),
		ruleWithLabels("acm_rs:namespace:memory_request_hard", `max_over_time(acm_rs:namespace:memory_request_hard:5m[1d])`),
		ruleWithLabels("acm_rs:namespace:memory_request", `max_over_time(acm_rs:namespace:memory_request:5m[1d])`),
		ruleWithLabels("acm_rs:namespace:memory_usage", `max_over_time(acm_rs:namespace:memory_usage:5m[1d])`),
		ruleWithLabels("acm_rs:namespace:memory_recommendation", fmt.Sprintf(`max_over_time(acm_rs:namespace:memory_usage{profile="Max OverAll"}[1d]) * (1 + (%d/100))`, recommendationPercentage)),
	}

	// Group: cluster 5m
	clusterRules5m := []monitoringv1.Rule{
		rule("acm_rs:cluster:cpu_request_hard:5m", fmt.Sprintf(`max_over_time(sum(kube_resourcequota{resource=~"requests.cpu", type="hard", %s}) by (cluster)[5m:])`, nsFilter)),
		rule("acm_rs:cluster:cpu_request:5m", fmt.Sprintf(`max_over_time(sum(kube_pod_container_resource_requests{%s, resource="cpu", container!=""}) by (cluster)[5m:])`, nsFilter)),
		rule("acm_rs:cluster:cpu_usage:5m", fmt.Sprintf(`max_over_time(sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{%s, container!=""}) by (cluster)[5m:])`, nsFilter)),
		rule("acm_rs:cluster:memory_request_hard:5m", fmt.Sprintf(`max_over_time(sum(kube_resourcequota{resource=~"requests.memory", type="hard", %s}) by (cluster)[5m:])`, nsFilter)),
		rule("acm_rs:cluster:memory_request:5m", fmt.Sprintf(`max_over_time(sum(kube_pod_container_resource_requests{%s, resource="memory", container!=""}) by (cluster)[5m:])`, nsFilter)),
		rule("acm_rs:cluster:memory_usage:5m", fmt.Sprintf(`max_over_time(sum(container_memory_working_set_bytes{%s, container!=""}) by (cluster)[5m:])`, nsFilter)),
	}

	// Group: cluster 1d
	clusterRules1d := []monitoringv1.Rule{
		ruleWithLabels("acm_rs:cluster:cpu_request_hard", `max_over_time(acm_rs:cluster:cpu_request_hard:5m[1d])`),
		ruleWithLabels("acm_rs:cluster:cpu_request", `max_over_time(acm_rs:cluster:cpu_request:5m[1d])`),
		ruleWithLabels("acm_rs:cluster:cpu_usage", `max_over_time(acm_rs:cluster:cpu_usage:5m[1d])`),
		ruleWithLabels("acm_rs:cluster:cpu_recommendation", fmt.Sprintf(`max_over_time(acm_rs:cluster:cpu_usage{profile="Max OverAll"}[1d]) * (1 + (%d/100))`, recommendationPercentage)),
		ruleWithLabels("acm_rs:cluster:memory_request_hard", `max_over_time(acm_rs:cluster:memory_request_hard:5m[1d])`),
		ruleWithLabels("acm_rs:cluster:memory_request", `max_over_time(acm_rs:cluster:memory_request:5m[1d])`),
		ruleWithLabels("acm_rs:cluster:memory_usage", `max_over_time(acm_rs:cluster:memory_usage:5m[1d])`),
		ruleWithLabels("acm_rs:cluster:memory_recommendation", fmt.Sprintf(`max_over_time(acm_rs:cluster:memory_usage{profile="Max OverAll"}[1d]) * (1 + (%d/100))`, recommendationPercentage)),
	}

	return monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rsPrometheusRuleName,
			Namespace: rsMonitoringNamespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "PrometheusRule",
			APIVersion: "monitoring.coreos.com/v1",
		},
		Spec: monitoringv1.PrometheusRuleSpec{
			Groups: []monitoringv1.RuleGroup{
				{Name: "acm-right-sizing-namespace-5m.rule", Interval: &duration5m, Rules: nsRules5m},
				{Name: "acm-right-sizing-namespace-1d.rules", Interval: &duration1d, Rules: nsRules1d},
				{Name: "acm-right-sizing-cluster-5m.rule", Interval: &duration5m, Rules: clusterRules5m},
				{Name: "acm-right-sizing-cluster-1d.rule", Interval: &duration1d, Rules: clusterRules1d},
			},
		},
	}, nil
}
