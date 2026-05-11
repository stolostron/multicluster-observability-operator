// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsworkload

import (
	"fmt"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing/rs-utility"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func GeneratePrometheusRule(configData rsutility.RSNamespaceConfigMapData) (monitoringv1.PrometheusRule, error) {
	return GeneratePrometheusRuleWithFeatures(configData, true, true)
}

func GeneratePrometheusRuleWithFeatures(
	configData rsutility.RSNamespaceConfigMapData,
	workloadsEnabled bool,
	podsEnabled bool,
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
	duration1d := monitoringv1.Duration("1h")

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

	groups := []monitoringv1.RuleGroup{
		{
			Name:     "acm-right-sizing-workload-5m.rules",
			Interval: &duration5m,
			Rules:    buildWorkloadRules5m(nsFilter, rule, workloadsEnabled, podsEnabled),
		},
		{
			Name:     "acm-right-sizing-workload-1d.rules",
			Interval: &duration1d,
			Rules:    buildWorkloadRules1d(configData, workloadsEnabled, podsEnabled),
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

func buildWorkloadRules5m(
	nsFilter string,
	rule func(string, string) monitoringv1.Rule,
	workloadsEnabled bool,
	podsEnabled bool,
) []monitoringv1.Rule {
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

	rules := []monitoringv1.Rule{
		rule("acm_rs:pod_workload:relabel:5m", podWorkloadRelabelExpr),
	}

	if podsEnabled {
		rules = append(rules,
			rule(
				"acm_rs:pod:cpu_request:5m",
				fmt.Sprintf(
					`max_over_time(sum by (namespace, pod, workload, workload_type) (
					  kube_pod_container_resource_requests{%s, resource="cpu", container!=""}
					  * on (namespace, pod) group_left(workload, workload_type)
					    acm_rs:pod_workload:relabel:5m
					)[5m:])`,
					nsFilter,
				),
			),
			rule(
				"acm_rs:pod:cpu_limit:5m",
				fmt.Sprintf(
					`max_over_time(sum by (namespace, pod, workload, workload_type) (
					  kube_pod_container_resource_limits{%s, resource="cpu", container!=""}
					  * on (namespace, pod) group_left(workload, workload_type)
					    acm_rs:pod_workload:relabel:5m
					)[5m:])`,
					nsFilter,
				),
			),
			rule(
				"acm_rs:pod:cpu_usage:5m",
				fmt.Sprintf(
					`max_over_time(sum by (namespace, pod, workload, workload_type) (
					  node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{%s, container!=""}
					  * on (namespace, pod) group_left(workload, workload_type)
					    acm_rs:pod_workload:relabel:5m
					)[5m:])`,
					nsFilter,
				),
			),
			rule(
				"acm_rs:pod:memory_request:5m",
				fmt.Sprintf(
					`max_over_time(sum by (namespace, pod, workload, workload_type) (
					  kube_pod_container_resource_requests{%s, resource="memory", container!=""}
					  * on (namespace, pod) group_left(workload, workload_type)
					    acm_rs:pod_workload:relabel:5m
					)[5m:])`,
					nsFilter,
				),
			),
			rule(
				"acm_rs:pod:memory_limit:5m",
				fmt.Sprintf(
					`max_over_time(sum by (namespace, pod, workload, workload_type) (
					  kube_pod_container_resource_limits{%s, resource="memory", container!=""}
					  * on (namespace, pod) group_left(workload, workload_type)
					    acm_rs:pod_workload:relabel:5m
					)[5m:])`,
					nsFilter,
				),
			),
			rule(
				"acm_rs:pod:memory_usage:5m",
				fmt.Sprintf(
					`max_over_time(sum by (namespace, pod, workload, workload_type) (
					  container_memory_working_set_bytes{%s, container!=""}
					  * on (namespace, pod) group_left(workload, workload_type)
					    acm_rs:pod_workload:relabel:5m
					)[5m:])`,
					nsFilter,
				),
			),
		)
	}

	if workloadsEnabled {
		rules = append(rules,
			rule(
				"acm_rs:workload:cpu_request:5m",
				fmt.Sprintf(
					`max_over_time(sum by (namespace, workload, workload_type) (
					  kube_pod_container_resource_requests{%s, resource="cpu", container!=""}
					  * on (namespace, pod) group_left(workload, workload_type)
					    acm_rs:pod_workload:relabel:5m
					)[5m:])`,
					nsFilter,
				),
			),
			rule(
				"acm_rs:workload:cpu_limit:5m",
				fmt.Sprintf(
					`max_over_time(sum by (namespace, workload, workload_type) (
					  kube_pod_container_resource_limits{%s, resource="cpu", container!=""}
					  * on (namespace, pod) group_left(workload, workload_type)
					    acm_rs:pod_workload:relabel:5m
					)[5m:])`,
					nsFilter,
				),
			),
			rule(
				"acm_rs:workload:cpu_usage:5m",
				fmt.Sprintf(
					`max_over_time(sum by (namespace, workload, workload_type) (
					  node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{%s, container!=""}
					  * on (namespace, pod) group_left(workload, workload_type)
					    acm_rs:pod_workload:relabel:5m
					)[5m:])`,
					nsFilter,
				),
			),
			rule(
				"acm_rs:workload:memory_request:5m",
				fmt.Sprintf(
					`max_over_time(sum by (namespace, workload, workload_type) (
					  kube_pod_container_resource_requests{%s, resource="memory", container!=""}
					  * on (namespace, pod) group_left(workload, workload_type)
					    acm_rs:pod_workload:relabel:5m
					)[5m:])`,
					nsFilter,
				),
			),
			rule(
				"acm_rs:workload:memory_limit:5m",
				fmt.Sprintf(
					`max_over_time(sum by (namespace, workload, workload_type) (
					  kube_pod_container_resource_limits{%s, resource="memory", container!=""}
					  * on (namespace, pod) group_left(workload, workload_type)
					    acm_rs:pod_workload:relabel:5m
					)[5m:])`,
					nsFilter,
				),
			),
			rule(
				"acm_rs:workload:memory_usage:5m",
				fmt.Sprintf(
					`max_over_time(sum by (namespace, workload, workload_type) (
					  container_memory_working_set_bytes{%s, container!=""}
					  * on (namespace, pod) group_left(workload, workload_type)
					    acm_rs:pod_workload:relabel:5m
					)[5m:])`,
					nsFilter,
				),
			),
		)
	}

	return rules
}

func buildWorkloadRules1d(
	configData rsutility.RSNamespaceConfigMapData,
	workloadsEnabled bool,
	podsEnabled bool,
) []monitoringv1.Rule {
	rp := rsutility.NormalizeRecommendationPercentage(configData.PrometheusRuleConfig.RecommendationPercentage)
	const podMetrics, workloadMetrics = 8, 8
	capacity := 0
	if podsEnabled {
		capacity += podMetrics
	}
	if workloadsEnabled {
		capacity += workloadMetrics
	}
	rules := make([]monitoringv1.Rule, 0, capacity*len(rsutility.RecommendationProfiles))

	for _, profile := range rsutility.RecommendationProfiles {
		if podsEnabled {
			rules = append(rules,
				rsutility.RuleWithProfile("acm_rs:pod:cpu_request", profile.AggExpr("acm_rs:pod:cpu_request:5m"), profile.Name),
				rsutility.RuleWithProfile("acm_rs:pod:cpu_limit", profile.AggExpr("acm_rs:pod:cpu_limit:5m"), profile.Name),
				rsutility.RuleWithProfile("acm_rs:pod:cpu_usage", profile.AggExpr("acm_rs:pod:cpu_usage:5m"), profile.Name),
				rsutility.RuleWithProfile("acm_rs:pod:cpu_recommendation",
					rsutility.BuildProfiledRecommendationExpr("acm_rs:pod:cpu_usage:5m", rp, profile), profile.Name),
				rsutility.RuleWithProfile("acm_rs:pod:memory_request", profile.AggExpr("acm_rs:pod:memory_request:5m"), profile.Name),
				rsutility.RuleWithProfile("acm_rs:pod:memory_limit", profile.AggExpr("acm_rs:pod:memory_limit:5m"), profile.Name),
				rsutility.RuleWithProfile("acm_rs:pod:memory_usage", profile.AggExpr("acm_rs:pod:memory_usage:5m"), profile.Name),
				rsutility.RuleWithProfile("acm_rs:pod:memory_recommendation",
					rsutility.BuildProfiledRecommendationExpr("acm_rs:pod:memory_usage:5m", rp, profile), profile.Name),
			)
		}

		if workloadsEnabled {
			rules = append(rules,
				rsutility.RuleWithProfile("acm_rs:workload:cpu_request", profile.AggExpr("acm_rs:workload:cpu_request:5m"), profile.Name),
				rsutility.RuleWithProfile("acm_rs:workload:cpu_limit", profile.AggExpr("acm_rs:workload:cpu_limit:5m"), profile.Name),
				rsutility.RuleWithProfile("acm_rs:workload:cpu_usage", profile.AggExpr("acm_rs:workload:cpu_usage:5m"), profile.Name),
				rsutility.RuleWithProfile("acm_rs:workload:cpu_recommendation",
					rsutility.BuildProfiledRecommendationExpr("acm_rs:workload:cpu_usage:5m", rp, profile), profile.Name),
				rsutility.RuleWithProfile("acm_rs:workload:memory_request", profile.AggExpr("acm_rs:workload:memory_request:5m"), profile.Name),
				rsutility.RuleWithProfile("acm_rs:workload:memory_limit", profile.AggExpr("acm_rs:workload:memory_limit:5m"), profile.Name),
				rsutility.RuleWithProfile("acm_rs:workload:memory_usage", profile.AggExpr("acm_rs:workload:memory_usage:5m"), profile.Name),
				rsutility.RuleWithProfile("acm_rs:workload:memory_recommendation",
					rsutility.BuildProfiledRecommendationExpr("acm_rs:workload:memory_usage:5m", rp, profile), profile.Name),
			)
		}
	}

	return rules
}
