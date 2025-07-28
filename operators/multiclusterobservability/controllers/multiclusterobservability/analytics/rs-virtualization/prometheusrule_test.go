// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsvirtualization

import (
	"testing"

	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability/analytics/rs-utility"
	"github.com/stretchr/testify/assert"
)

func TestGeneratePrometheusRule_InclusionOnly(t *testing.T) {
	config := rsutility.RSNamespaceConfigMapData{
		PrometheusRuleConfig: rsutility.RSPrometheusRuleConfig{
			NamespaceFilterCriteria: struct {
				InclusionCriteria []string `yaml:"inclusionCriteria"`
				ExclusionCriteria []string `yaml:"exclusionCriteria"`
			}{
				InclusionCriteria: []string{"vm1", "vm2"},
				ExclusionCriteria: []string{},
			},
			LabelFilterCriteria:      []rsutility.RSLabelFilter{},
			RecommendationPercentage: 110,
		},
	}

	rule, err := GeneratePrometheusRule(config)
	assert.NoError(t, err)
	assert.NotEmpty(t, rule.Spec.Groups)

	// Verify the rule name and namespace
	assert.Equal(t, PrometheusRuleName, rule.Name)
	assert.Equal(t, MonitoringNamespace, rule.Namespace)

	// Verify we have 4 rule groups
	assert.Len(t, rule.Spec.Groups, 4)

	// Verify the rule group names
	expectedGroups := []string{
		"acm-vm-right-sizing-namespace-5m.rule",
		"acm-vm-right-sizing-namespace-1d.rules",
		"acm-vm-right-sizing-cluster-5m.rule",
		"acm-vm-right-sizing-cluster-1d.rule",
	}

	for i, expectedName := range expectedGroups {
		assert.Equal(t, expectedName, rule.Spec.Groups[i].Name)
	}
}

func TestGeneratePrometheusRule_ExclusionOnly(t *testing.T) {
	config := rsutility.RSNamespaceConfigMapData{
		PrometheusRuleConfig: rsutility.RSPrometheusRuleConfig{
			NamespaceFilterCriteria: struct {
				InclusionCriteria []string `yaml:"inclusionCriteria"`
				ExclusionCriteria []string `yaml:"exclusionCriteria"`
			}{
				InclusionCriteria: []string{},
				ExclusionCriteria: []string{"openshift.*"},
			},
			LabelFilterCriteria:      []rsutility.RSLabelFilter{},
			RecommendationPercentage: 120,
		},
	}

	rule, err := GeneratePrometheusRule(config)
	assert.NoError(t, err)
	assert.NotEmpty(t, rule.Spec.Groups)
	assert.Len(t, rule.Spec.Groups, 4)
}

func TestGeneratePrometheusRule_BothInclusionAndExclusion(t *testing.T) {
	config := rsutility.RSNamespaceConfigMapData{
		PrometheusRuleConfig: rsutility.RSPrometheusRuleConfig{
			NamespaceFilterCriteria: struct {
				InclusionCriteria []string `yaml:"inclusionCriteria"`
				ExclusionCriteria []string `yaml:"exclusionCriteria"`
			}{
				InclusionCriteria: []string{"vm1"},
				ExclusionCriteria: []string{"openshift.*"},
			},
			LabelFilterCriteria:      []rsutility.RSLabelFilter{},
			RecommendationPercentage: 110,
		},
	}

	_, err := GeneratePrometheusRule(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only one of inclusion or exclusion criteria allowed for namespacefiltercriteria")
}

func TestGeneratePrometheusRule_WithLabelFilter(t *testing.T) {
	config := rsutility.RSNamespaceConfigMapData{
		PrometheusRuleConfig: rsutility.RSPrometheusRuleConfig{
			NamespaceFilterCriteria: struct {
				InclusionCriteria []string `yaml:"inclusionCriteria"`
				ExclusionCriteria []string `yaml:"exclusionCriteria"`
			}{
				InclusionCriteria: []string{},
				ExclusionCriteria: []string{},
			},
			LabelFilterCriteria: []rsutility.RSLabelFilter{
				{
					LabelName:         "label_env",
					InclusionCriteria: []string{"prod", "staging"},
					ExclusionCriteria: []string{},
				},
			},
			RecommendationPercentage: 110,
		},
	}

	rule, err := GeneratePrometheusRule(config)
	assert.NoError(t, err)
	assert.NotEmpty(t, rule.Spec.Groups)
	assert.Len(t, rule.Spec.Groups, 4)
}

func TestBuildNamespaceFilter_InclusionOnly(t *testing.T) {
	nsConfig := rsutility.RSPrometheusRuleConfig{
		NamespaceFilterCriteria: struct {
			InclusionCriteria []string `yaml:"inclusionCriteria"`
			ExclusionCriteria []string `yaml:"exclusionCriteria"`
		}{
			InclusionCriteria: []string{"namespace1", "namespace2"},
			ExclusionCriteria: []string{},
		},
	}

	filter, err := buildNamespaceFilter(nsConfig)
	assert.NoError(t, err)
	assert.Contains(t, filter, "namespace1|namespace2")
	assert.Contains(t, filter, "namespace=~")
}

func TestBuildNamespaceFilter_ExclusionOnly(t *testing.T) {
	nsConfig := rsutility.RSPrometheusRuleConfig{
		NamespaceFilterCriteria: struct {
			InclusionCriteria []string `yaml:"inclusionCriteria"`
			ExclusionCriteria []string `yaml:"exclusionCriteria"`
		}{
			InclusionCriteria: []string{},
			ExclusionCriteria: []string{"openshift.*"},
		},
	}

	filter, err := buildNamespaceFilter(nsConfig)
	assert.NoError(t, err)
	assert.Contains(t, filter, "openshift.*")
	assert.Contains(t, filter, "namespace!~")
}

func TestBuildNamespaceFilter_BothInclusionAndExclusion(t *testing.T) {
	nsConfig := rsutility.RSPrometheusRuleConfig{
		NamespaceFilterCriteria: struct {
			InclusionCriteria []string `yaml:"inclusionCriteria"`
			ExclusionCriteria []string `yaml:"exclusionCriteria"`
		}{
			InclusionCriteria: []string{"namespace1"},
			ExclusionCriteria: []string{"openshift.*"},
		},
	}

	_, err := buildNamespaceFilter(nsConfig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only one of inclusion or exclusion criteria allowed for namespacefiltercriteria")
}

func TestBuildNamespaceFilter_NoFilters(t *testing.T) {
	nsConfig := rsutility.RSPrometheusRuleConfig{
		NamespaceFilterCriteria: struct {
			InclusionCriteria []string `yaml:"inclusionCriteria"`
			ExclusionCriteria []string `yaml:"exclusionCriteria"`
		}{
			InclusionCriteria: []string{},
			ExclusionCriteria: []string{},
		},
	}

	filter, err := buildNamespaceFilter(nsConfig)
	assert.NoError(t, err)
	assert.Equal(t, `namespace!=""`, filter)
}

func TestBuildLabelJoin_NoFilters(t *testing.T) {
	labelFilters := []rsutility.RSLabelFilter{}

	filter, err := buildLabelJoin(labelFilters)
	assert.NoError(t, err)
	assert.Empty(t, filter)
}

func TestBuildLabelJoin_NonEnvLabel(t *testing.T) {
	labelFilters := []rsutility.RSLabelFilter{
		{
			LabelName:         "label_app",
			InclusionCriteria: []string{"app1"},
			ExclusionCriteria: []string{},
		},
	}

	filter, err := buildLabelJoin(labelFilters)
	assert.NoError(t, err)
	assert.Empty(t, filter)
}

func TestBuildLabelJoin_EnvLabelInclusion(t *testing.T) {
	labelFilters := []rsutility.RSLabelFilter{
		{
			LabelName:         "label_env",
			InclusionCriteria: []string{"prod", "staging"},
			ExclusionCriteria: []string{},
		},
	}

	filter, err := buildLabelJoin(labelFilters)
	assert.NoError(t, err)
	assert.Contains(t, filter, "kube_namespace_labels{label_env=~\"prod|staging\"}")
	assert.Contains(t, filter, "* on (namespace) group_left()")
}

func TestBuildLabelJoin_EnvLabelExclusion(t *testing.T) {
	labelFilters := []rsutility.RSLabelFilter{
		{
			LabelName:         "label_env",
			InclusionCriteria: []string{},
			ExclusionCriteria: []string{"test"},
		},
	}

	filter, err := buildLabelJoin(labelFilters)
	assert.NoError(t, err)
	assert.Contains(t, filter, "kube_namespace_labels{label_env!~\"test\"}")
	assert.Contains(t, filter, "* on (namespace) group_left()")
}

func TestBuildLabelJoin_EnvLabelBothInclusionAndExclusion(t *testing.T) {
	labelFilters := []rsutility.RSLabelFilter{
		{
			LabelName:         "label_env",
			InclusionCriteria: []string{"prod"},
			ExclusionCriteria: []string{"test"},
		},
	}

	_, err := buildLabelJoin(labelFilters)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only one of inclusion or exclusion allowed for label_env")
}
