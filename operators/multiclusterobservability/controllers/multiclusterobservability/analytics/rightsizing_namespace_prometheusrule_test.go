// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGeneratePrometheusRule_InclusionOnly(t *testing.T) {
	config := RSNamespaceConfigMapData{
		PrometheusRuleConfig: RSPrometheusRuleConfig{
			NamespaceFilterCriteria: struct {
				InclusionCriteria []string "yaml:\"inclusionCriteria\""
				ExclusionCriteria []string "yaml:\"exclusionCriteria\""
			}{
				InclusionCriteria: []string{"ns-a", "ns-b"},
			},
			RecommendationPercentage: 120,
			LabelFilterCriteria: []RSLabelFilter{
				{
					LabelName:         "label_env",
					InclusionCriteria: []string{"prod"},
				},
			},
		},
	}

	rule, err := generatePrometheusRule(config)
	assert.NoError(t, err)
	assert.Equal(t, rsPrometheusRuleName, rule.Name)
	assert.Len(t, rule.Spec.Groups, 4)
	assert.Contains(t, rule.Spec.Groups[0].Rules[0].Expr.String(), `namespace=~"ns-a|ns-b"`)
	assert.Contains(t, rule.Spec.Groups[0].Rules[0].Expr.String(), `label_env=~"prod"`)
}

func TestGeneratePrometheusRule_ExclusionOnly(t *testing.T) {
	config := RSNamespaceConfigMapData{
		PrometheusRuleConfig: RSPrometheusRuleConfig{
			NamespaceFilterCriteria: struct {
				InclusionCriteria []string "yaml:\"inclusionCriteria\""
				ExclusionCriteria []string "yaml:\"exclusionCriteria\""
			}{
				ExclusionCriteria: []string{"openshift.*"},
			},
			RecommendationPercentage: 110,
		},
	}

	rule, err := generatePrometheusRule(config)
	assert.NoError(t, err)
	assert.Contains(t, rule.Spec.Groups[0].Rules[0].Expr.String(), `namespace!~"openshift.*"`)
}

func TestGeneratePrometheusRule_BothNamespaceInclusionAndExclusion(t *testing.T) {
	config := RSNamespaceConfigMapData{
		PrometheusRuleConfig: RSPrometheusRuleConfig{
			NamespaceFilterCriteria: struct {
				InclusionCriteria []string "yaml:\"inclusionCriteria\""
				ExclusionCriteria []string "yaml:\"exclusionCriteria\""
			}{
				InclusionCriteria: []string{"ns-a"},
				ExclusionCriteria: []string{"ns-b"},
			},
		},
	}

	_, err := generatePrometheusRule(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only one of inclusion or exclusion criteria allowed for namespaceFilterCriteria")
}

func TestGeneratePrometheusRule_LabelEnvInclusionAndExclusion(t *testing.T) {
	config := RSNamespaceConfigMapData{
		PrometheusRuleConfig: RSPrometheusRuleConfig{
			NamespaceFilterCriteria: struct {
				InclusionCriteria []string "yaml:\"inclusionCriteria\""
				ExclusionCriteria []string "yaml:\"exclusionCriteria\""
			}{
				InclusionCriteria: []string{"ns-a"},
			},
			LabelFilterCriteria: []RSLabelFilter{
				{
					LabelName:         "label_env",
					InclusionCriteria: []string{"qa"},
					ExclusionCriteria: []string{"prod"},
				},
			},
		},
	}

	_, err := generatePrometheusRule(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only one of inclusion or exclusion allowed for label_env")
}
