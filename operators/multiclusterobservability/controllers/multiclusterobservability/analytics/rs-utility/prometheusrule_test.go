// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsutility

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildNamespaceFilter_InclusionOnly(t *testing.T) {
	config := RSPrometheusRuleConfig{
		NamespaceFilterCriteria: struct {
			InclusionCriteria []string `yaml:"inclusionCriteria"`
			ExclusionCriteria []string `yaml:"exclusionCriteria"`
		}{
			InclusionCriteria: []string{"ns1", "ns2"},
		},
	}

	result, err := BuildNamespaceFilter(config)
	require.NoError(t, err)
	assert.Equal(t, `namespace=~"ns1|ns2"`, result)
}

func TestBuildNamespaceFilter_ExclusionOnly(t *testing.T) {
	config := RSPrometheusRuleConfig{
		NamespaceFilterCriteria: struct {
			InclusionCriteria []string `yaml:"inclusionCriteria"`
			ExclusionCriteria []string `yaml:"exclusionCriteria"`
		}{
			ExclusionCriteria: []string{"openshift.*", "kube.*"},
		},
	}

	result, err := BuildNamespaceFilter(config)
	require.NoError(t, err)
	assert.Equal(t, `namespace!~"openshift.*|kube.*"`, result)
}

func TestBuildNamespaceFilter_BothInclusionAndExclusion(t *testing.T) {
	config := RSPrometheusRuleConfig{
		NamespaceFilterCriteria: struct {
			InclusionCriteria []string `yaml:"inclusionCriteria"`
			ExclusionCriteria []string `yaml:"exclusionCriteria"`
		}{
			InclusionCriteria: []string{"ns1"},
			ExclusionCriteria: []string{"ns2"},
		},
	}

	_, err := BuildNamespaceFilter(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only one of inclusion or exclusion criteria allowed")
}

func TestBuildNamespaceFilter_NoFilters(t *testing.T) {
	config := RSPrometheusRuleConfig{}

	result, err := BuildNamespaceFilter(config)
	require.NoError(t, err)
	assert.Equal(t, `namespace!=""`, result)
}

func TestBuildLabelJoin_NoFilters(t *testing.T) {
	result, err := BuildLabelJoin([]RSLabelFilter{})
	require.NoError(t, err)
	assert.Equal(t, "", result)
}

func TestBuildLabelJoin_EnvLabelInclusion(t *testing.T) {
	filters := []RSLabelFilter{
		{
			LabelName:         "label_env",
			InclusionCriteria: []string{"prod", "staging"},
		},
	}

	result, err := BuildLabelJoin(filters)
	require.NoError(t, err)
	expected := `* on (namespace) group_left() (kube_namespace_labels{label_env=~"prod|staging"} or kube_namespace_labels{label_env=""})`
	assert.Equal(t, expected, result)
}

func TestBuildLabelJoin_EnvLabelExclusion(t *testing.T) {
	filters := []RSLabelFilter{
		{
			LabelName:         "label_env",
			ExclusionCriteria: []string{"dev", "test"},
		},
	}

	result, err := BuildLabelJoin(filters)
	require.NoError(t, err)
	expected := `* on (namespace) group_left() (kube_namespace_labels{label_env!~"dev|test"} or kube_namespace_labels{label_env=""})`
	assert.Equal(t, expected, result)
}

func TestBuildLabelJoin_EnvLabelBothInclusionAndExclusion(t *testing.T) {
	filters := []RSLabelFilter{
		{
			LabelName:         "label_env",
			InclusionCriteria: []string{"prod"},
			ExclusionCriteria: []string{"dev"},
		},
	}

	_, err := BuildLabelJoin(filters)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only one of inclusion or exclusion allowed for label_env")
}
