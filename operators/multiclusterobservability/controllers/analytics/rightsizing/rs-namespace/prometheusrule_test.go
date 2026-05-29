// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsnamespace

import (
	"testing"

	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing/rs-utility"
	"github.com/stretchr/testify/assert"
)

func TestGeneratePrometheusRule_InclusionOnly(t *testing.T) {
	config := rsutility.RSNamespaceConfigMapData{
		PrometheusRuleConfig: rsutility.RSPrometheusRuleConfig{
			NamespaceFilterCriteria: struct {
				InclusionCriteria []string "yaml:\"inclusionCriteria\""
				ExclusionCriteria []string "yaml:\"exclusionCriteria\""
			}{
				InclusionCriteria: []string{"ns-a", "ns-b"},
			},
			RecommendationPercentage: 120,
			LabelFilterCriteria: []rsutility.RSLabelFilter{
				{
					LabelName:         "label_env",
					InclusionCriteria: []string{"prod"},
				},
			},
		},
	}

	rule, err := GeneratePrometheusRule(config)
	assert.NoError(t, err)
	assert.Equal(t, PrometheusRuleName, rule.Name)
	assert.Len(t, rule.Spec.Groups, 4)
	assert.Contains(t, rule.Spec.Groups[0].Rules[0].Expr.String(), `namespace=~"ns-a|ns-b"`)
	assert.Contains(t, rule.Spec.Groups[0].Rules[0].Expr.String(), `label_env=~"prod"`)
}

func TestGeneratePrometheusRule_ExclusionOnly(t *testing.T) {
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

	rule, err := GeneratePrometheusRule(config)
	assert.NoError(t, err)
	assert.Contains(t, rule.Spec.Groups[0].Rules[0].Expr.String(), `namespace!~"openshift.*"`)
}

func TestGeneratePrometheusRule_BothNamespaceInclusionAndExclusion(t *testing.T) {
	config := rsutility.RSNamespaceConfigMapData{
		PrometheusRuleConfig: rsutility.RSPrometheusRuleConfig{
			NamespaceFilterCriteria: struct {
				InclusionCriteria []string "yaml:\"inclusionCriteria\""
				ExclusionCriteria []string "yaml:\"exclusionCriteria\""
			}{
				InclusionCriteria: []string{"ns-a"},
				ExclusionCriteria: []string{"ns-b"},
			},
		},
	}

	_, err := GeneratePrometheusRule(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only one of inclusion or exclusion criteria allowed")
}

func TestGeneratePrometheusRule_LabelEnvInclusionAndExclusion(t *testing.T) {
	config := rsutility.RSNamespaceConfigMapData{
		PrometheusRuleConfig: rsutility.RSPrometheusRuleConfig{
			NamespaceFilterCriteria: struct {
				InclusionCriteria []string "yaml:\"inclusionCriteria\""
				ExclusionCriteria []string "yaml:\"exclusionCriteria\""
			}{
				InclusionCriteria: []string{"ns-a"},
			},
			LabelFilterCriteria: []rsutility.RSLabelFilter{
				{
					LabelName:         "label_env",
					InclusionCriteria: []string{"qa"},
					ExclusionCriteria: []string{"prod"},
				},
			},
		},
	}

	_, err := GeneratePrometheusRule(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only one of inclusion or exclusion allowed for label_env")
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

	rule, err := GeneratePrometheusRule(config)
	assert.NoError(t, err)
	assert.Len(t, rule.Spec.Groups, 4)

	namespace1d := rule.Spec.Groups[1]
	assert.Equal(t, "acm-right-sizing-namespace-1d.rules", namespace1d.Name)
	assert.Len(t, namespace1d.Rules, 8*len(rsutility.RecommendationProfiles))

	cluster1d := rule.Spec.Groups[3]
	assert.Equal(t, "acm-right-sizing-cluster-1d.rule", cluster1d.Name)
	assert.Len(t, cluster1d.Rules, 8*len(rsutility.RecommendationProfiles))

	profileSeen := map[string]bool{}
	for _, r := range namespace1d.Rules {
		profileSeen[r.Labels["profile"]] = true
	}
	for _, p := range rsutility.RecommendationProfiles {
		assert.True(t, profileSeen[p.Name], "missing profile %s in namespace 1d rules", p.Name)
	}

	for _, r := range namespace1d.Rules {
		if r.Record != "acm_rs:namespace:cpu_recommendation" {
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

	rule, err := GeneratePrometheusRule(config)
	assert.NoError(t, err)

	for _, r := range rule.Spec.Groups[1].Rules {
		if r.Record == "acm_rs:namespace:cpu_recommendation" {
			assert.Contains(t, r.Expr.String(), "(110/100)",
				"rp=0 should be normalized to DefaultRecommendationPercentage")
			return
		}
	}
	t.Fatal("cpu_recommendation rule not found")
}
