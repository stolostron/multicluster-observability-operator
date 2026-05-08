// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsgpu

import (
	"testing"

	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing/rs-utility"
	"github.com/stretchr/testify/assert"
)

func TestGeneratePrometheusRule_IncludesNamespaceGPU(t *testing.T) {
	config := rsutility.RSNamespaceConfigMapData{
		PrometheusRuleConfig: rsutility.RSPrometheusRuleConfig{
			NamespaceFilterCriteria: struct {
				InclusionCriteria []string "yaml:\"inclusionCriteria\""
				ExclusionCriteria []string "yaml:\"exclusionCriteria\""
			}{
				InclusionCriteria: []string{"ns-a"},
			},
			RecommendationPercentage: 110,
		},
	}

	rule, err := GeneratePrometheusRuleWithMapping(config, true)
	assert.NoError(t, err)
	assert.Equal(t, PrometheusRuleName, rule.Name)
	assert.Equal(t, "k8s", rule.Labels["prometheus"])
	assert.Equal(t, "alert-rules", rule.Labels["role"])
	assert.Len(t, rule.Spec.Groups, 6)
	// First rule group should include namespace GPU request expression.
	assert.Contains(t, rule.Spec.Groups[0].Rules[0].Expr.String(), `resource=~"nvidia.com/gpu|amd.com/gpu"`)
}

func TestGeneratePrometheusRule_IncludesClusterGPUMetrics(t *testing.T) {
	config := rsutility.RSNamespaceConfigMapData{
		PrometheusRuleConfig: rsutility.RSPrometheusRuleConfig{
			NamespaceFilterCriteria: struct {
				InclusionCriteria []string "yaml:\"inclusionCriteria\""
				ExclusionCriteria []string "yaml:\"exclusionCriteria\""
			}{
				InclusionCriteria: []string{"ns-a"},
			},
			RecommendationPercentage: 110,
		},
	}

	rule, err := GeneratePrometheusRuleWithMapping(config, true)
	assert.NoError(t, err)

	// Collect all cluster-level 5m rule names.
	cluster5mRules := map[string]bool{}
	for _, rg := range rule.Spec.Groups {
		if rg.Name == "acm-right-sizing-gpu-cluster-5m.rules" {
			for _, r := range rg.Rules {
				cluster5mRules[r.Record] = true
			}
		}
	}
	for _, expected := range []string{
		"acm_rs:cluster:gpu_request:5m",
		"acm_rs:cluster:gpu_usage:5m",
		"acm_rs:cluster:gpu_memory_used:5m",
		"acm_rs:cluster:gpu_memory_total:5m",
		"acm_rs:cluster:gpu_power_usage_watts:5m",
		"acm_rs:cluster:gpu_temperature_celsius:5m",
		"acm_rs:cluster:gpu_sm_clock_hertz:5m",
		"acm_rs:cluster:gpu_memory_clock_hertz:5m",
	} {
		assert.True(t, cluster5mRules[expected], "missing cluster 5m rule: %s", expected)
	}

	// Collect all cluster-level 1d rule names.
	cluster1dRules := map[string]bool{}
	for _, rg := range rule.Spec.Groups {
		if rg.Name == "acm-right-sizing-gpu-cluster-1d.rules" {
			for _, r := range rg.Rules {
				cluster1dRules[r.Record] = true
			}
		}
	}
	for _, expected := range []string{
		"acm_rs:cluster:gpu_request",
		"acm_rs:cluster:gpu_usage",
		"acm_rs:cluster:gpu_recommendation",
		"acm_rs:cluster:gpu_memory_used",
		"acm_rs:cluster:gpu_memory_recommendation",
		"acm_rs:cluster:gpu_memory_total",
		"acm_rs:cluster:gpu_power_usage_watts",
		"acm_rs:cluster:gpu_temperature_celsius",
		"acm_rs:cluster:gpu_sm_clock_hertz",
		"acm_rs:cluster:gpu_memory_clock_hertz",
	} {
		assert.True(t, cluster1dRules[expected], "missing cluster 1d rule: %s", expected)
	}
}

func TestGeneratePrometheusRule_IncludesGPUTypeRules(t *testing.T) {
	config := rsutility.RSNamespaceConfigMapData{
		PrometheusRuleConfig: rsutility.RSPrometheusRuleConfig{
			NamespaceFilterCriteria: struct {
				InclusionCriteria []string "yaml:\"inclusionCriteria\""
				ExclusionCriteria []string "yaml:\"exclusionCriteria\""
			}{
				InclusionCriteria: []string{"ns-a"},
			},
			RecommendationPercentage: 110,
		},
	}

	rule, err := GeneratePrometheusRuleWithMapping(config, true)
	assert.NoError(t, err)

	// Verify namespace gpu_type 5m rule exists.
	ns5mRules := map[string]bool{}
	for _, rg := range rule.Spec.Groups {
		if rg.Name == "acm-right-sizing-gpu-namespace-5m.rules" {
			for _, r := range rg.Rules {
				ns5mRules[r.Record] = true
			}
		}
	}
	assert.True(t, ns5mRules["acm_rs:namespace:gpu_type:5m"], "missing namespace 5m rule: acm_rs:namespace:gpu_type:5m")

	// Verify namespace gpu_type 1d rule exists.
	ns1dRules := map[string]bool{}
	for _, rg := range rule.Spec.Groups {
		if rg.Name == "acm-right-sizing-gpu-namespace-1d.rules" {
			for _, r := range rg.Rules {
				ns1dRules[r.Record] = true
			}
		}
	}
	assert.True(t, ns1dRules["acm_rs:namespace:gpu_type"], "missing namespace 1d rule: acm_rs:namespace:gpu_type")

	// Verify the 5m rule expression preserves the resource label.
	for _, rg := range rule.Spec.Groups {
		if rg.Name == "acm-right-sizing-gpu-namespace-5m.rules" {
			for _, r := range rg.Rules {
				if r.Record == "acm_rs:namespace:gpu_type:5m" {
					assert.Contains(t, r.Expr.String(), "resource")
					assert.Contains(t, r.Expr.String(), `nvidia.com/gpu|amd.com/gpu`)
				}
			}
		}
	}
}

func TestGeneratePrometheusRule_MemoryTotalUsesDCGMMetrics(t *testing.T) {
	config := rsutility.RSNamespaceConfigMapData{
		PrometheusRuleConfig: rsutility.RSPrometheusRuleConfig{
			NamespaceFilterCriteria: struct {
				InclusionCriteria []string "yaml:\"inclusionCriteria\""
				ExclusionCriteria []string "yaml:\"exclusionCriteria\""
			}{
				InclusionCriteria: []string{"ns-a"},
			},
			RecommendationPercentage: 110,
		},
	}

	rule, err := GeneratePrometheusRuleWithMapping(config, true)
	assert.NoError(t, err)

	memoryTotalRules := map[string]string{}
	for _, rg := range rule.Spec.Groups {
		for _, r := range rg.Rules {
			if r.Record == "acm_rs:namespace:gpu_memory_total:5m" ||
				r.Record == "acm_rs:pod:gpu_memory_total:5m" ||
				r.Record == "acm_rs:workload:gpu_memory_total:5m" ||
				r.Record == "acm_rs:cluster:gpu_memory_total:5m" {
				memoryTotalRules[r.Record] = r.Expr.String()
			}
		}
	}

	assert.Len(t, memoryTotalRules, 4, "expected 4 gpu_memory_total 5m rules")
	for name, expr := range memoryTotalRules {
		assert.Contains(t, expr, "DCGM_FI_DEV_FB_USED", "rule %s should use DCGM_FI_DEV_FB_USED", name)
		assert.Contains(t, expr, "DCGM_FI_DEV_FB_FREE", "rule %s should use DCGM_FI_DEV_FB_FREE", name)
		assert.NotContains(t, expr, "accelerator_memory_total_bytes",
			"rule %s should not reference non-existent accelerator_memory_total_bytes", name)
	}
}

func TestGeneratePrometheusRule_NoOrphanedGPUUtilizationRule(t *testing.T) {
	config := rsutility.RSNamespaceConfigMapData{
		PrometheusRuleConfig: rsutility.RSPrometheusRuleConfig{
			NamespaceFilterCriteria: struct {
				InclusionCriteria []string "yaml:\"inclusionCriteria\""
				ExclusionCriteria []string "yaml:\"exclusionCriteria\""
			}{
				InclusionCriteria: []string{"ns-a"},
			},
			RecommendationPercentage: 110,
		},
	}

	rule, err := GeneratePrometheusRuleWithMapping(config, true)
	assert.NoError(t, err)

	for _, rg := range rule.Spec.Groups {
		for _, r := range rg.Rules {
			assert.NotEqual(t, "acm_rs:namespace:gpu_utilization:5m", r.Record,
				"orphaned gpu_utilization:5m rule should not be present (no 1d rollup, not in allowlist)")
		}
	}
}

func TestGeneratePrometheusRule_AllDashboardMetricsProduced(t *testing.T) {
	config := rsutility.RSNamespaceConfigMapData{
		PrometheusRuleConfig: rsutility.RSPrometheusRuleConfig{
			NamespaceFilterCriteria: struct {
				InclusionCriteria []string "yaml:\"inclusionCriteria\""
				ExclusionCriteria []string "yaml:\"exclusionCriteria\""
			}{
				InclusionCriteria: []string{"ns-a"},
			},
			RecommendationPercentage: 110,
		},
	}

	rule, err := GeneratePrometheusRuleWithMapping(config, true)
	assert.NoError(t, err)

	produced := map[string]bool{}
	for _, rg := range rule.Spec.Groups {
		for _, r := range rg.Rules {
			produced[r.Record] = true
		}
	}

	dashboardMetrics := []string{
		"acm_rs:cluster:gpu_request", "acm_rs:cluster:gpu_usage", "acm_rs:cluster:gpu_recommendation",
		"acm_rs:cluster:gpu_memory_used", "acm_rs:cluster:gpu_memory_total", "acm_rs:cluster:gpu_memory_recommendation",
		"acm_rs:namespace:gpu_request", "acm_rs:namespace:gpu_usage", "acm_rs:namespace:gpu_recommendation",
		"acm_rs:namespace:gpu_memory_used", "acm_rs:namespace:gpu_memory_total", "acm_rs:namespace:gpu_memory_recommendation",
		"acm_rs:namespace:gpu_power_usage_watts", "acm_rs:namespace:gpu_temperature_celsius",
		"acm_rs:namespace:gpu_sm_clock_hertz", "acm_rs:namespace:gpu_memory_clock_hertz", "acm_rs:namespace:gpu_type",
		"acm_rs:workload:gpu_request", "acm_rs:workload:gpu_usage", "acm_rs:workload:gpu_recommendation",
		"acm_rs:workload:gpu_memory_used", "acm_rs:workload:gpu_memory_total", "acm_rs:workload:gpu_memory_recommendation",
		"acm_rs:pod:gpu_request", "acm_rs:pod:gpu_usage", "acm_rs:pod:gpu_recommendation",
		"acm_rs:pod:gpu_memory_used", "acm_rs:pod:gpu_memory_total", "acm_rs:pod:gpu_memory_recommendation",
	}

	for _, metric := range dashboardMetrics {
		assert.True(t, produced[metric], "dashboard metric %q has no corresponding recording rule", metric)
	}
}

func TestGeneratePrometheusRule_IncludesWorkloadMappingForBatchControllers(t *testing.T) {
	config := rsutility.RSNamespaceConfigMapData{
		PrometheusRuleConfig: rsutility.RSPrometheusRuleConfig{
			NamespaceFilterCriteria: struct {
				InclusionCriteria []string "yaml:\"inclusionCriteria\""
				ExclusionCriteria []string "yaml:\"exclusionCriteria\""
			}{
				InclusionCriteria: []string{"ns-a"},
			},
			RecommendationPercentage: 110,
		},
	}

	rule, err := GeneratePrometheusRuleWithMapping(config, true)
	assert.NoError(t, err)

	// Workload+pod mapping is generated in the first workload/pod 5m rule group.
	var mappingExpr string
	for _, rg := range rule.Spec.Groups {
		for _, r := range rg.Rules {
			if r.Record == "acm_rs:pod_workload:relabel:5m" {
				mappingExpr = r.Expr.String()
			}
		}
	}
	assert.NotEmpty(t, mappingExpr)
	assert.Contains(t, mappingExpr, `owner_kind="Job"`)
	assert.Contains(t, mappingExpr, `kube_job_owner`)
	assert.Contains(t, mappingExpr, `owner_kind="CronJob"`)
	assert.Contains(t, mappingExpr, `"workload_type", "ReplicaSet"`)
}
