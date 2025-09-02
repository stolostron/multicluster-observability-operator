// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsnamespace

import (
	"testing"

	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability/analytics/rs-utility"
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
