// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsvirtualization

import (
	"fmt"
	"strings"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability/analytics/rs-utility"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// function helps to build PrometheusRule based on configdata
func GeneratePrometheusRule(configData rsutility.RSNamespaceConfigMapData) (monitoringv1.PrometheusRule, error) {
	nsFilter, err := buildNamespaceFilter(configData.PrometheusRuleConfig)
	if err != nil {
		return monitoringv1.PrometheusRule{}, err
	}

	labelJoin, err := buildLabelJoin(configData.PrometheusRuleConfig.LabelFilterCriteria)
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

	return monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PrometheusRuleName,
			Namespace: MonitoringNamespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "PrometheusRule",
			APIVersion: "monitoring.coreos.com/v1",
		},
		Spec: monitoringv1.PrometheusRuleSpec{
			Groups: []monitoringv1.RuleGroup{
				{
					Name:     "acm-vm-right-sizing-namespace-5m.rule",
					Interval: &duration5m,
					Rules:    buildNamespaceRules5m(nsFilter, rule),
				},
				{
					Name:     "acm-vm-right-sizing-namespace-1d.rules",
					Interval: &duration1d,
					Rules:    buildNamespaceRules1d(configData, ruleWithLabels),
				},
				{
					Name:     "acm-vm-right-sizing-cluster-5m.rule",
					Interval: &duration5m,
					Rules:    buildClusterRules5m(nsFilter, rule),
				},
				{
					Name:     "acm-vm-right-sizing-cluster-1d.rule",
					Interval: &duration1d,
					Rules:    buildClusterRules1d(configData, ruleWithLabels),
				},
			},
		},
	}, nil
}

func buildNamespaceFilter(nsConfig rsutility.RSPrometheusRuleConfig) (string, error) {
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

func buildLabelJoin(labelFilters []rsutility.RSLabelFilter) (string, error) {
	for _, l := range labelFilters {
		if l.LabelName != "label_env" {
			continue
		}
		if len(l.InclusionCriteria) > 0 && len(l.ExclusionCriteria) > 0 {
			return "", fmt.Errorf("only one of inclusion or exclusion allowed for label_env")
		}
		var selector string
		if len(l.InclusionCriteria) > 0 {
			selector = fmt.Sprintf(`kube_namespace_labels{label_env=~"%s"}`, strings.Join(l.InclusionCriteria, "|"))
		} else if len(l.ExclusionCriteria) > 0 {
			selector = fmt.Sprintf(`kube_namespace_labels{label_env!~"%s"}`, strings.Join(l.ExclusionCriteria, "|"))
		} else {
			continue
		}
		return fmt.Sprintf(`* on (namespace) group_left() (%s or kube_namespace_labels{label_env=""})`, selector), nil
	}
	return "", nil
}

func buildNamespaceRules5m(
	nsFilter string,
	rule func(string, string) monitoringv1.Rule,
) []monitoringv1.Rule {
	return []monitoringv1.Rule{
		rule(
			"acm_rs_vm:namespace:cpu_request:5m",
			fmt.Sprintf(
				`max_over_time(sum(kubevirt_vm_resource_requests{resource="cpu", type="hard", %s}) by (namespace)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs_vm:namespace:memory_request:5m",
			fmt.Sprintf(
				`max_over_time(sum(kubevirt_vm_resource_requests{`+
					`%s, resource='memory', container!=""}) by (namespace)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs_vm:namespace:cpu_usage:5m",
			fmt.Sprintf(
				`max_over_time(sum(kubevirt_vmi_cpu_usage_seconds_total{`+
					`%s, container!=""}) by (namespace)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs_vm:namespace:memory_usage:5m",
			fmt.Sprintf(
				`max_over_time(sum(kubevirt_vmi_memory_available_bytes{resource=~"requests.memory", %s}) by (namespace)[5m:])`,
				nsFilter,
			),
		),
	}
}

func buildNamespaceRules1d(
	configData rsutility.RSNamespaceConfigMapData,
	ruleWithLabels func(string, string) monitoringv1.Rule,
) []monitoringv1.Rule {
	rp := configData.PrometheusRuleConfig.RecommendationPercentage
	return []monitoringv1.Rule{
		ruleWithLabels("acm_rs_vm:namespace:cpu_request", `max_over_time(acm_rs_vm:namespace:cpu_request:5m[1d])`),
		ruleWithLabels("acm_rs_vm:namespace:cpu_usage", `max_over_time(acm_rs_vm:namespace:cpu_usage:5m[1d])`),
		ruleWithLabels("acm_rs_vm:namespace:memory_request", `max_over_time(acm_rs_vm:namespace:memory_request:5m[1d])`),
		ruleWithLabels("acm_rs_vm:namespace:memory_usage", `max_over_time(acm_rs_vm:namespace:memory_usage:5m[1d])`),
		ruleWithLabels(
			"acm_rs_vm:namespace:cpu_recommendation",
			fmt.Sprintf(
				`max_over_time(acm_rs_vm:namespace:cpu_usage{profile="Max OverAll"}[1d]) * (%d/100)`,
				rp,
			),
		),
		ruleWithLabels(
			"acm_rs_vm:namespace:cpu_recommendation",
			fmt.Sprintf(
				`max_over_time(acm_rs_vm:namespace:memory_usage{profile="Max OverAll"}[1d]) * (%d/100)`,
				rp,
			),
		),
	}
}

func buildClusterRules5m(
	nsFilter string,
	rule func(string, string) monitoringv1.Rule,
) []monitoringv1.Rule {
	return []monitoringv1.Rule{
		rule(
			"acm_vm_rs:cluster:cpu_request:5m",
			fmt.Sprintf(
				`max_over_time(sum(kubevirt_vm_resource_requests{`+
					`%s, resource="cpu", container!=""}) by (cluster)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_vm_rs:cluster:cpu_usage:5m",
			fmt.Sprintf(
				`max_over_time(sum(kubevirt_vmi_cpu_usage_seconds_total{`+
					`%s, container!=""}) by (cluster)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_vm_rs:cluster:memory_request:5m",
			fmt.Sprintf(
				`max_over_time(sum(kubevirt_vm_resource_requests{`+
					`%s, resource="memory", container!=""}) by (cluster)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_vm_rs:cluster:memory_usage:5m",
			fmt.Sprintf(
				`max_over_time(sum(kubevirt_vmi_memory_available_bytes{`+
					`%s, container!=""}) by (cluster)[5m:])`,
				nsFilter,
			),
		),
	}
}

func buildClusterRules1d(
	configData rsutility.RSNamespaceConfigMapData,
	ruleWithLabels func(string, string) monitoringv1.Rule,
) []monitoringv1.Rule {
	rp := configData.PrometheusRuleConfig.RecommendationPercentage
	return []monitoringv1.Rule{
		ruleWithLabels("acm_vm_rs:cluster:cpu_request", `max_over_time(acm_vm_rs:cluster:cpu_request:5m[1d])`),
		ruleWithLabels("acm_vm_rs:cluster:cpu_usage", `max_over_time(acm_vm_rs:cluster:cpu_usage:5m[1d])`),
		ruleWithLabels(
			"acm_vm_rs:cluster:cpu_recommendation",
			fmt.Sprintf(
				`max_over_time(acm_vm_rs:cluster:cpu_usage{profile="Max OverAll"}[1d]) * (%d/100)`,
				rp,
			),
		),
		ruleWithLabels("acm_vm_rs:cluster:memory_request", `max_over_time(acm_vm_rs:cluster:memory_request:5m[1d])`),
		ruleWithLabels("acm_vm_rs:cluster:memory_usage", `max_over_time(acm_vm_rs:cluster:memory_usage:5m[1d])`),
		ruleWithLabels(
			"acm_vm_rs:cluster:memory_recommendation",
			fmt.Sprintf(
				`max_over_time(acm_vm_rs:cluster:memory_usage{profile="Max OverAll"}[1d]) * (%d/100)`,
				rp,
			),
		),
	}
}
