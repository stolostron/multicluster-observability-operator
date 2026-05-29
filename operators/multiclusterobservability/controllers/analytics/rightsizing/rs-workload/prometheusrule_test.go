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
		if _, exists := recordNames1d[r.Record]; !exists {
			recordNames1d[r.Record] = r.Expr.String()
		}
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

func TestGeneratePrometheusRule_AllProfilesGenerated(t *testing.T) {
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

	rules1d := rule.Spec.Groups[1]
	assert.Equal(t, "acm-right-sizing-workload-1d.rules", rules1d.Name)
	// 8 pod metrics + 8 workload metrics = 16 per profile × 4 profiles = 64
	assert.Len(t, rules1d.Rules, 16*len(rsutility.RecommendationProfiles))

	profileSeen := map[string]bool{}
	for _, r := range rules1d.Rules {
		profileSeen[r.Labels["profile"]] = true
	}
	for _, p := range rsutility.RecommendationProfiles {
		assert.True(t, profileSeen[p.Name], "missing profile %s in workload 1d rules", p.Name)
	}

	for _, r := range rules1d.Rules {
		if r.Record != "acm_rs:workload:cpu_recommendation" {
			continue
		}
		switch r.Labels["profile"] {
		case "Max OverAll":
			assert.Contains(t, r.Expr.String(), "max_over_time(")
		case "P99":
			assert.Contains(t, r.Expr.String(), "quantile_over_time(0.99,")
		case "P95":
			assert.Contains(t, r.Expr.String(), "quantile_over_time(0.95,")
		case "Avg":
			assert.Contains(t, r.Expr.String(), "avg_over_time(")
		}
		assert.Contains(t, r.Expr.String(), "(110/100)")
	}
}

func TestGeneratePrometheusRule_RPZeroNormalized(t *testing.T) {
	config := rsutility.RSNamespaceConfigMapData{
		PrometheusRuleConfig: rsutility.RSPrometheusRuleConfig{
			NamespaceFilterCriteria: struct {
				InclusionCriteria []string "yaml:\"inclusionCriteria\""
				ExclusionCriteria []string "yaml:\"exclusionCriteria\""
			}{
				ExclusionCriteria: []string{"openshift.*"},
			},
			RecommendationPercentage: 0,
		},
	}

	rule, err := GeneratePrometheusRuleWithFeatures(config, true, true)
	assert.NoError(t, err)

	for _, r := range rule.Spec.Groups[1].Rules {
		if r.Record == "acm_rs:workload:cpu_recommendation" {
			assert.Contains(t, r.Expr.String(), "(110/100)")
			return
		}
	}
	t.Fatal("workload cpu_recommendation rule not found")
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
	recordNames1d := map[string]bool{}
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
