// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsgpu

import (
	"fmt"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing/rs-utility"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func GeneratePrometheusRule(configData rsutility.RSNamespaceConfigMapData) (monitoringv1.PrometheusRule, error) {
	return GeneratePrometheusRuleWithMapping(configData, true)
}

func GeneratePrometheusRuleWithMapping(
	configData rsutility.RSNamespaceConfigMapData,
	includePodWorkloadMapping bool,
) (monitoringv1.PrometheusRule, error) {
	nsFilter, err := rsutility.BuildNamespaceFilter(configData.PrometheusRuleConfig)
	if err != nil {
		return monitoringv1.PrometheusRule{}, err
	}
	labelJoin, err := rsutility.BuildLabelJoin(configData.PrometheusRuleConfig.LabelFilterCriteria)
	if err != nil {
		return monitoringv1.PrometheusRule{}, err
	}

	duration5m := monitoringv1.Duration("5m")
	duration1h := monitoringv1.Duration("1h")

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

	groups := []monitoringv1.RuleGroup{
		{
			Name:     "acm-right-sizing-gpu-namespace-5m.rules",
			Interval: &duration5m,
			Rules:    buildGPUNamespaceRules5m(nsFilter, rule),
		},
		{
			Name:     "acm-right-sizing-gpu-workload-5m.rules",
			Interval: &duration5m,
			Rules:    buildGPUWorkloadPodRules5m(nsFilter, rule, includePodWorkloadMapping),
		},
		{
			Name:     "acm-right-sizing-gpu-namespace-1d.rules",
			Interval: &duration1h,
			Rules:    buildGPUNamespaceRules1d(configData, ruleWithLabels),
		},
		{
			Name:     "acm-right-sizing-gpu-workload-1d.rules",
			Interval: &duration1h,
			Rules:    buildGPUWorkloadPodRules1d(configData, ruleWithLabels),
		},
		{
			Name:     "acm-right-sizing-gpu-cluster-5m.rules",
			Interval: &duration5m,
			Rules:    buildGPUClusterRules5m(nsFilter, rule),
		},
		{
			Name:     "acm-right-sizing-gpu-cluster-1d.rules",
			Interval: &duration1h,
			Rules:    buildGPUClusterRules1d(configData, ruleWithLabels),
		},
	}

	return monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PrometheusRuleName,
			Namespace: rsutility.MonitoringNamespace,
			Labels:    rsutility.PrometheusK8sRuleLabels,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "PrometheusRule",
			APIVersion: "monitoring.coreos.com/v1",
		},
		Spec: monitoringv1.PrometheusRuleSpec{
			Groups: groups,
		},
	}, nil
}

func buildGPUNamespaceRules5m(
	nsFilter string,
	rule func(string, string) monitoringv1.Rule,
) []monitoringv1.Rule {
	return []monitoringv1.Rule{
		rule(
			"acm_rs:namespace:gpu_request:5m",
			fmt.Sprintf(
				`max_over_time(sum by (namespace) (kube_pod_container_resource_requests{%s, resource=~"nvidia.com/gpu|amd.com/gpu", container!=""})[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:namespace:gpu_usage:5m",
			fmt.Sprintf(
				`max_over_time(sum by (namespace) (accelerator_gpu_utilization{%s})[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:namespace:gpu_memory_used:5m",
			fmt.Sprintf(
				`max_over_time(sum by (namespace) (accelerator_memory_used_bytes{%s})[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:namespace:gpu_memory_total:5m",
			fmt.Sprintf(
				`max_over_time(sum by (namespace) ((DCGM_FI_DEV_FB_USED{%s} + DCGM_FI_DEV_FB_FREE{%s}))[5m:])`,
				nsFilter, nsFilter,
			),
		),
		rule(
			"acm_rs:namespace:gpu_power_usage_watts:5m",
			fmt.Sprintf(
				`max_over_time(sum by (namespace) (accelerator_power_usage_watts{%s})[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:namespace:gpu_temperature_celsius:5m",
			fmt.Sprintf(
				`max_over_time(max by (namespace) (accelerator_temperature_celsius{%s})[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:namespace:gpu_sm_clock_hertz:5m",
			fmt.Sprintf(
				`max_over_time(max by (namespace) (accelerator_sm_clock_hertz{%s})[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:namespace:gpu_memory_clock_hertz:5m",
			fmt.Sprintf(
				`max_over_time(max by (namespace) (accelerator_memory_clock_hertz{%s})[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:namespace:gpu_type:5m",
			fmt.Sprintf(
				`max by (namespace, resource) (kube_pod_container_resource_requests{%s, resource=~"nvidia.com/gpu|amd.com/gpu", container!=""})`,
				nsFilter,
			),
		),
	}
}

func buildGPUWorkloadPodRules5m(
	nsFilter string,
	rule func(string, string) monitoringv1.Rule,
	includeMapping bool,
) []monitoringv1.Rule {
	rules := []monitoringv1.Rule{}

	if includeMapping {
		podWorkloadRelabelExpr := fmt.Sprintf(
			`(
			  max by (namespace, pod, workload, workload_type) (
			    label_replace(
			      label_replace(
			        kube_pod_owner{%s, owner_kind=~"StatefulSet|DaemonSet"},
			        "workload", "$1", "owner_name", "(.*)"
			      ),
			      "workload_type", "$1", "owner_kind", "(.*)"
			    )
			  )
			)
			or
			(
			  max by (namespace, pod, workload, workload_type) (
			    label_replace(
			      label_replace(
			        (
			          label_replace(
			            kube_pod_owner{%s, owner_kind="ReplicaSet"},
			            "replicaset", "$1", "owner_name", "(.*)"
			          )
			          * on (namespace, replicaset) group_left(owner_name)
			            topk by (namespace, replicaset) (
			              1,
			              max by (namespace, replicaset, owner_name) (
			                kube_replicaset_owner{%s, owner_kind="Deployment"}
			              )
			            )
			        ),
			        "workload", "$1", "owner_name", "(.*)"
			      ),
			      "workload_type", "Deployment", "workload", ".*"
			    )
			  )
			)
			or
			(
			  max by (namespace, pod, workload, workload_type) (
			    label_replace(
			      label_replace(
			        (
			          label_replace(
			            kube_pod_owner{%s, owner_kind="ReplicaSet"},
			            "replicaset", "$1", "owner_name", "(.*)"
			          )
			          unless on (namespace, replicaset)
			            kube_replicaset_owner{%s, owner_kind="Deployment"}
			        ),
			        "workload", "$1", "replicaset", "(.*)"
			      ),
			      "workload_type", "ReplicaSet", "workload", ".*"
			    )
			  )
			)
			or
			(
			  max by (namespace, pod, workload, workload_type) (
			    label_replace(
			      label_replace(
			        (
			          label_replace(
			            kube_pod_owner{%s, owner_kind="Job"},
			            "job_name", "$1", "owner_name", "(.*)"
			          )
			          * on (namespace, job_name) group_left(owner_name)
			            max by (namespace, job_name, owner_name) (
			              kube_job_owner{%s, owner_kind="CronJob"}
			            )
			        ),
			        "workload", "$1", "owner_name", "(.*)"
			      ),
			      "workload_type", "CronJob", "workload", ".*"
			    )
			  )
			)
			or
			(
			  max by (namespace, pod, workload, workload_type) (
			    label_replace(
			      label_replace(
			        (
			          kube_pod_owner{%s, owner_kind="Job"}
			          unless on (namespace, owner_name)
			            max by (namespace, owner_name) (
			              label_replace(
			                kube_job_owner{%s, owner_kind="CronJob"},
			                "owner_name", "$1", "job_name", "(.*)"
			              )
			            )
			        ),
			        "workload", "$1", "owner_name", "(.*)"
			      ),
			      "workload_type", "Job", "workload", ".*"
			    )
			  )
			)`,
			nsFilter, nsFilter, nsFilter, nsFilter, nsFilter, nsFilter, nsFilter, nsFilter, nsFilter,
		)
		rules = append(rules, rule("acm_rs:pod_workload:relabel:5m", podWorkloadRelabelExpr))
	}

	// Pod GPU series
	rules = append(rules,
		rule(
			"acm_rs:pod:gpu_request:5m",
			fmt.Sprintf(
				`max_over_time(sum by (namespace, pod, workload, workload_type) (
				  kube_pod_container_resource_requests{%s, resource=~"nvidia.com/gpu|amd.com/gpu", container!=""}
				  * on (namespace, pod) group_left(workload, workload_type)
				    acm_rs:pod_workload:relabel:5m
				)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:pod:gpu_usage:5m",
			fmt.Sprintf(
				`max_over_time(sum by (namespace, pod, workload, workload_type) (
				  accelerator_gpu_utilization{%s}
				  * on (namespace, pod) group_left(workload, workload_type)
				    acm_rs:pod_workload:relabel:5m
				)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:pod:gpu_memory_used:5m",
			fmt.Sprintf(
				`max_over_time(sum by (namespace, pod, workload, workload_type) (
				  accelerator_memory_used_bytes{%s}
				  * on (namespace, pod) group_left(workload, workload_type)
				    acm_rs:pod_workload:relabel:5m
				)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:pod:gpu_memory_total:5m",
			fmt.Sprintf(
				`max_over_time(sum by (namespace, pod, workload, workload_type) (
				  (DCGM_FI_DEV_FB_USED{%s} + DCGM_FI_DEV_FB_FREE{%s})
				  * on (namespace, pod) group_left(workload, workload_type)
				    acm_rs:pod_workload:relabel:5m
				)[5m:])`,
				nsFilter, nsFilter,
			),
		),
		rule(
			"acm_rs:pod:gpu_power_usage_watts:5m",
			fmt.Sprintf(
				`max_over_time(sum by (namespace, pod, workload, workload_type) (
				  accelerator_power_usage_watts{%s}
				  * on (namespace, pod) group_left(workload, workload_type)
				    acm_rs:pod_workload:relabel:5m
				)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:pod:gpu_temperature_celsius:5m",
			fmt.Sprintf(
				`max_over_time(max by (namespace, pod, workload, workload_type) (
				  accelerator_temperature_celsius{%s}
				  * on (namespace, pod) group_left(workload, workload_type)
				    acm_rs:pod_workload:relabel:5m
				)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:pod:gpu_sm_clock_hertz:5m",
			fmt.Sprintf(
				`max_over_time(max by (namespace, pod, workload, workload_type) (
				  accelerator_sm_clock_hertz{%s}
				  * on (namespace, pod) group_left(workload, workload_type)
				    acm_rs:pod_workload:relabel:5m
				)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:pod:gpu_memory_clock_hertz:5m",
			fmt.Sprintf(
				`max_over_time(max by (namespace, pod, workload, workload_type) (
				  accelerator_memory_clock_hertz{%s}
				  * on (namespace, pod) group_left(workload, workload_type)
				    acm_rs:pod_workload:relabel:5m
				)[5m:])`,
				nsFilter,
			),
		),
	)

	// Workload GPU series
	rules = append(rules,
		rule(
			"acm_rs:workload:gpu_request:5m",
			fmt.Sprintf(
				`max_over_time(sum by (namespace, workload, workload_type) (
				  kube_pod_container_resource_requests{%s, resource=~"nvidia.com/gpu|amd.com/gpu", container!=""}
				  * on (namespace, pod) group_left(workload, workload_type)
				    acm_rs:pod_workload:relabel:5m
				)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:workload:gpu_usage:5m",
			fmt.Sprintf(
				`max_over_time(sum by (namespace, workload, workload_type) (
				  accelerator_gpu_utilization{%s}
				  * on (namespace, pod) group_left(workload, workload_type)
				    acm_rs:pod_workload:relabel:5m
				)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:workload:gpu_memory_used:5m",
			fmt.Sprintf(
				`max_over_time(sum by (namespace, workload, workload_type) (
				  accelerator_memory_used_bytes{%s}
				  * on (namespace, pod) group_left(workload, workload_type)
				    acm_rs:pod_workload:relabel:5m
				)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:workload:gpu_memory_total:5m",
			fmt.Sprintf(
				`max_over_time(sum by (namespace, workload, workload_type) (
				  (DCGM_FI_DEV_FB_USED{%s} + DCGM_FI_DEV_FB_FREE{%s})
				  * on (namespace, pod) group_left(workload, workload_type)
				    acm_rs:pod_workload:relabel:5m
				)[5m:])`,
				nsFilter, nsFilter,
			),
		),
		rule(
			"acm_rs:workload:gpu_power_usage_watts:5m",
			fmt.Sprintf(
				`max_over_time(sum by (namespace, workload, workload_type) (
				  accelerator_power_usage_watts{%s}
				  * on (namespace, pod) group_left(workload, workload_type)
				    acm_rs:pod_workload:relabel:5m
				)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:workload:gpu_temperature_celsius:5m",
			fmt.Sprintf(
				`max_over_time(max by (namespace, workload, workload_type) (
				  accelerator_temperature_celsius{%s}
				  * on (namespace, pod) group_left(workload, workload_type)
				    acm_rs:pod_workload:relabel:5m
				)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:workload:gpu_sm_clock_hertz:5m",
			fmt.Sprintf(
				`max_over_time(max by (namespace, workload, workload_type) (
				  accelerator_sm_clock_hertz{%s}
				  * on (namespace, pod) group_left(workload, workload_type)
				    acm_rs:pod_workload:relabel:5m
				)[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:workload:gpu_memory_clock_hertz:5m",
			fmt.Sprintf(
				`max_over_time(max by (namespace, workload, workload_type) (
				  accelerator_memory_clock_hertz{%s}
				  * on (namespace, pod) group_left(workload, workload_type)
				    acm_rs:pod_workload:relabel:5m
				)[5m:])`,
				nsFilter,
			),
		),
	)

	return rules
}

func buildGPUNamespaceRules1d(
	configData rsutility.RSNamespaceConfigMapData,
	ruleWithLabels func(string, string) monitoringv1.Rule,
) []monitoringv1.Rule {
	rp := configData.PrometheusRuleConfig.RecommendationPercentage
	return []monitoringv1.Rule{
		ruleWithLabels("acm_rs:namespace:gpu_request", `max_over_time(acm_rs:namespace:gpu_request:5m[1d])`),
		ruleWithLabels("acm_rs:namespace:gpu_usage", `max_over_time(acm_rs:namespace:gpu_usage:5m[1d])`),
		ruleWithLabels(
			"acm_rs:namespace:gpu_recommendation",
			fmt.Sprintf(`max_over_time(acm_rs:namespace:gpu_usage:5m[1d]) * (%d/100)`, rp),
		),
		ruleWithLabels("acm_rs:namespace:gpu_memory_used", `max_over_time(acm_rs:namespace:gpu_memory_used:5m[1d])`),
		ruleWithLabels(
			"acm_rs:namespace:gpu_memory_recommendation",
			fmt.Sprintf(`max_over_time(acm_rs:namespace:gpu_memory_used:5m[1d]) * (%d/100)`, rp),
		),
		ruleWithLabels("acm_rs:namespace:gpu_memory_total", `max_over_time(acm_rs:namespace:gpu_memory_total:5m[1d])`),
		ruleWithLabels("acm_rs:namespace:gpu_power_usage_watts", `max_over_time(acm_rs:namespace:gpu_power_usage_watts:5m[1d])`),
		ruleWithLabels("acm_rs:namespace:gpu_temperature_celsius", `max_over_time(acm_rs:namespace:gpu_temperature_celsius:5m[1d])`),
		ruleWithLabels("acm_rs:namespace:gpu_sm_clock_hertz", `max_over_time(acm_rs:namespace:gpu_sm_clock_hertz:5m[1d])`),
		ruleWithLabels("acm_rs:namespace:gpu_memory_clock_hertz", `max_over_time(acm_rs:namespace:gpu_memory_clock_hertz:5m[1d])`),
		ruleWithLabels("acm_rs:namespace:gpu_type", `max_over_time(acm_rs:namespace:gpu_type:5m[1d])`),
	}
}

func buildGPUWorkloadPodRules1d(
	configData rsutility.RSNamespaceConfigMapData,
	ruleWithLabels func(string, string) monitoringv1.Rule,
) []monitoringv1.Rule {
	rp := configData.PrometheusRuleConfig.RecommendationPercentage
	return []monitoringv1.Rule{
		// pod
		ruleWithLabels("acm_rs:pod:gpu_request", `max_over_time(acm_rs:pod:gpu_request:5m[1d])`),
		ruleWithLabels("acm_rs:pod:gpu_usage", `max_over_time(acm_rs:pod:gpu_usage:5m[1d])`),
		ruleWithLabels(
			"acm_rs:pod:gpu_recommendation",
			fmt.Sprintf(`max_over_time(acm_rs:pod:gpu_usage:5m[1d]) * (%d/100)`, rp),
		),
		ruleWithLabels("acm_rs:pod:gpu_memory_used", `max_over_time(acm_rs:pod:gpu_memory_used:5m[1d])`),
		ruleWithLabels(
			"acm_rs:pod:gpu_memory_recommendation",
			fmt.Sprintf(`max_over_time(acm_rs:pod:gpu_memory_used:5m[1d]) * (%d/100)`, rp),
		),
		ruleWithLabels("acm_rs:pod:gpu_memory_total", `max_over_time(acm_rs:pod:gpu_memory_total:5m[1d])`),
		ruleWithLabels("acm_rs:pod:gpu_power_usage_watts", `max_over_time(acm_rs:pod:gpu_power_usage_watts:5m[1d])`),
		ruleWithLabels("acm_rs:pod:gpu_temperature_celsius", `max_over_time(acm_rs:pod:gpu_temperature_celsius:5m[1d])`),
		ruleWithLabels("acm_rs:pod:gpu_sm_clock_hertz", `max_over_time(acm_rs:pod:gpu_sm_clock_hertz:5m[1d])`),
		ruleWithLabels("acm_rs:pod:gpu_memory_clock_hertz", `max_over_time(acm_rs:pod:gpu_memory_clock_hertz:5m[1d])`),
		// workload
		ruleWithLabels("acm_rs:workload:gpu_request", `max_over_time(acm_rs:workload:gpu_request:5m[1d])`),
		ruleWithLabels("acm_rs:workload:gpu_usage", `max_over_time(acm_rs:workload:gpu_usage:5m[1d])`),
		ruleWithLabels(
			"acm_rs:workload:gpu_recommendation",
			fmt.Sprintf(`max_over_time(acm_rs:workload:gpu_usage:5m[1d]) * (%d/100)`, rp),
		),
		ruleWithLabels("acm_rs:workload:gpu_memory_used", `max_over_time(acm_rs:workload:gpu_memory_used:5m[1d])`),
		ruleWithLabels(
			"acm_rs:workload:gpu_memory_recommendation",
			fmt.Sprintf(`max_over_time(acm_rs:workload:gpu_memory_used:5m[1d]) * (%d/100)`, rp),
		),
		ruleWithLabels("acm_rs:workload:gpu_memory_total", `max_over_time(acm_rs:workload:gpu_memory_total:5m[1d])`),
		ruleWithLabels("acm_rs:workload:gpu_power_usage_watts", `max_over_time(acm_rs:workload:gpu_power_usage_watts:5m[1d])`),
		ruleWithLabels("acm_rs:workload:gpu_temperature_celsius", `max_over_time(acm_rs:workload:gpu_temperature_celsius:5m[1d])`),
		ruleWithLabels("acm_rs:workload:gpu_sm_clock_hertz", `max_over_time(acm_rs:workload:gpu_sm_clock_hertz:5m[1d])`),
		ruleWithLabels("acm_rs:workload:gpu_memory_clock_hertz", `max_over_time(acm_rs:workload:gpu_memory_clock_hertz:5m[1d])`),
	}
}

func buildGPUClusterRules5m(
	nsFilter string,
	rule func(string, string) monitoringv1.Rule,
) []monitoringv1.Rule {
	return []monitoringv1.Rule{
		rule(
			"acm_rs:cluster:gpu_request:5m",
			fmt.Sprintf(
				`max_over_time(sum by (cluster) (kube_pod_container_resource_requests{%s, resource=~"nvidia.com/gpu|amd.com/gpu", container!=""})[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:cluster:gpu_usage:5m",
			fmt.Sprintf(
				`max_over_time(sum by (cluster) (accelerator_gpu_utilization{%s})[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:cluster:gpu_memory_used:5m",
			fmt.Sprintf(
				`max_over_time(sum by (cluster) (accelerator_memory_used_bytes{%s})[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:cluster:gpu_memory_total:5m",
			fmt.Sprintf(
				`max_over_time(sum by (cluster) ((DCGM_FI_DEV_FB_USED{%s} + DCGM_FI_DEV_FB_FREE{%s}))[5m:])`,
				nsFilter, nsFilter,
			),
		),
		rule(
			"acm_rs:cluster:gpu_power_usage_watts:5m",
			fmt.Sprintf(
				`max_over_time(sum by (cluster) (accelerator_power_usage_watts{%s})[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:cluster:gpu_temperature_celsius:5m",
			fmt.Sprintf(
				`max_over_time(max by (cluster) (accelerator_temperature_celsius{%s})[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:cluster:gpu_sm_clock_hertz:5m",
			fmt.Sprintf(
				`max_over_time(max by (cluster) (accelerator_sm_clock_hertz{%s})[5m:])`,
				nsFilter,
			),
		),
		rule(
			"acm_rs:cluster:gpu_memory_clock_hertz:5m",
			fmt.Sprintf(
				`max_over_time(max by (cluster) (accelerator_memory_clock_hertz{%s})[5m:])`,
				nsFilter,
			),
		),
	}
}

func buildGPUClusterRules1d(
	configData rsutility.RSNamespaceConfigMapData,
	ruleWithLabels func(string, string) monitoringv1.Rule,
) []monitoringv1.Rule {
	rp := configData.PrometheusRuleConfig.RecommendationPercentage
	return []monitoringv1.Rule{
		ruleWithLabels("acm_rs:cluster:gpu_request", `max_over_time(acm_rs:cluster:gpu_request:5m[1d])`),
		ruleWithLabels("acm_rs:cluster:gpu_usage", `max_over_time(acm_rs:cluster:gpu_usage:5m[1d])`),
		ruleWithLabels(
			"acm_rs:cluster:gpu_recommendation",
			fmt.Sprintf(`max_over_time(acm_rs:cluster:gpu_usage:5m[1d]) * (%d/100)`, rp),
		),
		ruleWithLabels("acm_rs:cluster:gpu_memory_used", `max_over_time(acm_rs:cluster:gpu_memory_used:5m[1d])`),
		ruleWithLabels(
			"acm_rs:cluster:gpu_memory_recommendation",
			fmt.Sprintf(`max_over_time(acm_rs:cluster:gpu_memory_used:5m[1d]) * (%d/100)`, rp),
		),
		ruleWithLabels("acm_rs:cluster:gpu_memory_total", `max_over_time(acm_rs:cluster:gpu_memory_total:5m[1d])`),
		ruleWithLabels("acm_rs:cluster:gpu_power_usage_watts", `max_over_time(acm_rs:cluster:gpu_power_usage_watts:5m[1d])`),
		ruleWithLabels("acm_rs:cluster:gpu_temperature_celsius", `max_over_time(acm_rs:cluster:gpu_temperature_celsius:5m[1d])`),
		ruleWithLabels("acm_rs:cluster:gpu_sm_clock_hertz", `max_over_time(acm_rs:cluster:gpu_sm_clock_hertz:5m[1d])`),
		ruleWithLabels("acm_rs:cluster:gpu_memory_clock_hertz", `max_over_time(acm_rs:cluster:gpu_memory_clock_hertz:5m[1d])`),
	}
}
