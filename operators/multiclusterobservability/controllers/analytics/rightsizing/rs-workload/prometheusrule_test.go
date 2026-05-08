// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsworkload

import (
	"testing"

	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing/rs-utility"
	"github.com/stretchr/testify/assert"
)

func TestGeneratePrometheusRule_IncludesMappingRule(t *testing.T) {
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

	rule, err := GeneratePrometheusRuleWithFeatures(config, true, true)
	assert.NoError(t, err)
	assert.Equal(t, PrometheusRuleName, rule.Name)
	assert.Equal(t, "k8s", rule.Labels["prometheus"])
	assert.Equal(t, "alert-rules", rule.Labels["role"])
	assert.Len(t, rule.Spec.Groups, 2)
	assert.Equal(t, "acm-right-sizing-workload-5m.rules", rule.Spec.Groups[0].Name)
	assert.Equal(t, "acm_rs:pod_workload:relabel:5m", rule.Spec.Groups[0].Rules[0].Record)
	expr := rule.Spec.Groups[0].Rules[0].Expr.String()
	assert.Contains(t, expr, `namespace=~"ns-a"`)
	assert.Contains(t, expr, `owner_kind="Job"`)
	assert.Contains(t, expr, `kube_job_owner`)
	assert.Contains(t, expr, `owner_kind="CronJob"`)
	assert.Contains(t, expr, `"workload_type", "ReplicaSet"`)
}

func TestGeneratePrometheusRule_IncludesLimitRules(t *testing.T) {
	config := rsutility.RSNamespaceConfigMapData{
		PrometheusRuleConfig: rsutility.RSPrometheusRuleConfig{
			NamespaceFilterCriteria: struct {
				InclusionCriteria []string "yaml:\"inclusionCriteria\""
				ExclusionCriteria []string "yaml:\"exclusionCriteria\""
			}{
				ExclusionCriteria: []string{"openshift.*"},
			},
			RecommendationPercentage: 110,
		},
	}

	rule, err := GeneratePrometheusRuleWithFeatures(config, true, true)
	assert.NoError(t, err)

	recordNames5m := make(map[string]string)
	for _, r := range rule.Spec.Groups[0].Rules {
		recordNames5m[r.Record] = r.Expr.String()
	}
	recordNames1d := make(map[string]string)
	for _, r := range rule.Spec.Groups[1].Rules {
		recordNames1d[r.Record] = r.Expr.String()
	}

	for _, name := range []string{
		"acm_rs:pod:cpu_limit:5m",
		"acm_rs:pod:memory_limit:5m",
		"acm_rs:workload:cpu_limit:5m",
		"acm_rs:workload:memory_limit:5m",
	} {
		expr, ok := recordNames5m[name]
		assert.True(t, ok, "5m rule %q must be present", name)
		assert.Contains(t, expr, "kube_pod_container_resource_limits", "5m rule %q must use limits metric", name)
	}

	for _, name := range []string{
		"acm_rs:pod:cpu_limit",
		"acm_rs:pod:memory_limit",
		"acm_rs:workload:cpu_limit",
		"acm_rs:workload:memory_limit",
	} {
		expr, ok := recordNames1d[name]
		assert.True(t, ok, "1d rule %q must be present", name)
		assert.Contains(t, expr, name+":5m", "1d rule %q must reference its 5m counterpart", name)
	}
}

func TestGeneratePrometheusRule_PodsOnlyIncludesLimitRules(t *testing.T) {
	config := rsutility.RSNamespaceConfigMapData{
		PrometheusRuleConfig: rsutility.RSPrometheusRuleConfig{
			NamespaceFilterCriteria: struct {
				InclusionCriteria []string "yaml:\"inclusionCriteria\""
				ExclusionCriteria []string "yaml:\"exclusionCriteria\""
			}{
				ExclusionCriteria: []string{"openshift.*"},
			},
			RecommendationPercentage: 110,
		},
	}

	rule, err := GeneratePrometheusRuleWithFeatures(config, false, true)
	assert.NoError(t, err)

	recordNames5m := make(map[string]bool)
	for _, r := range rule.Spec.Groups[0].Rules {
		recordNames5m[r.Record] = true
	}
	recordNames1d := make(map[string]bool)
	for _, r := range rule.Spec.Groups[1].Rules {
		recordNames1d[r.Record] = true
	}

	assert.True(t, recordNames5m["acm_rs:pod:cpu_limit:5m"])
	assert.True(t, recordNames5m["acm_rs:pod:memory_limit:5m"])
	assert.True(t, recordNames1d["acm_rs:pod:cpu_limit"])
	assert.True(t, recordNames1d["acm_rs:pod:memory_limit"])

	assert.False(t, recordNames5m["acm_rs:workload:cpu_limit:5m"])
	assert.False(t, recordNames5m["acm_rs:workload:memory_limit:5m"])
	assert.False(t, recordNames1d["acm_rs:workload:cpu_limit"])
	assert.False(t, recordNames1d["acm_rs:workload:memory_limit"])
}
