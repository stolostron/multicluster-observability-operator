// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsutility

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecommendationProfiles_AllPresent(t *testing.T) {
	expectedNames := []string{"Max OverAll", "P99", "P95", "Avg"}
	require.Len(t, RecommendationProfiles, len(expectedNames))
	for i, name := range expectedNames {
		assert.Equal(t, name, RecommendationProfiles[i].Name)
		assert.NotNil(t, RecommendationProfiles[i].AggExpr)
	}
}

func TestAggregationExprs(t *testing.T) {
	metric := "acm_rs:namespace:cpu_usage:5m"
	tests := []struct {
		name     string
		fn       func(string) string
		expected string
	}{
		{"Max", Build1dMaxAggregationExpr, "max_over_time(acm_rs:namespace:cpu_usage:5m[1d])"},
		{"P99", BuildP99AggregationExpr, "quantile_over_time(0.99, acm_rs:namespace:cpu_usage:5m[1d])"},
		{"P95", BuildP95AggregationExpr, "quantile_over_time(0.95, acm_rs:namespace:cpu_usage:5m[1d])"},
		{"Avg", BuildAvgAggregationExpr, "avg_over_time(acm_rs:namespace:cpu_usage:5m[1d])"},
		{"Alias", Build1dAggregationExpr, "max_over_time(acm_rs:namespace:cpu_usage:5m[1d])"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.fn(metric))
		})
	}
}

func TestBuildProfiledRecommendationExpr(t *testing.T) {
	metric := "acm_rs:namespace:cpu_usage:5m"
	tests := []struct {
		name     string
		profile  ProfileConfig
		expected string
	}{
		{"Max OverAll", RecommendationProfiles[0], "max_over_time(acm_rs:namespace:cpu_usage:5m[1d]) * (110/100)"},
		{"P99", RecommendationProfiles[1], "quantile_over_time(0.99, acm_rs:namespace:cpu_usage:5m[1d]) * (110/100)"},
		{"P95", RecommendationProfiles[2], "quantile_over_time(0.95, acm_rs:namespace:cpu_usage:5m[1d]) * (110/100)"},
		{"Avg", RecommendationProfiles[3], "avg_over_time(acm_rs:namespace:cpu_usage:5m[1d]) * (110/100)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, BuildProfiledRecommendationExpr(metric, 110, tt.profile))
		})
	}
}

func TestBuildRecommendationExpr_BackwardCompatible(t *testing.T) {
	result := BuildRecommendationExpr("acm_rs:namespace:cpu_usage:5m", 110)
	expected := BuildProfiledRecommendationExpr("acm_rs:namespace:cpu_usage:5m", 110, RecommendationProfiles[0])
	assert.Equal(t, expected, result)
}

func TestRuleWithProfile(t *testing.T) {
	rule := RuleWithProfile("acm_rs:namespace:cpu_usage", "max_over_time(acm_rs:namespace:cpu_usage:5m[1d])", "P95")
	assert.Equal(t, "acm_rs:namespace:cpu_usage", rule.Record)
	assert.Equal(t, "max_over_time(acm_rs:namespace:cpu_usage:5m[1d])", rule.Expr.String())
	assert.Equal(t, "P95", rule.Labels["profile"])
	assert.Equal(t, "1d", rule.Labels["aggregation"])
}

func TestNormalizeRecommendationPercentage(t *testing.T) {
	assert.Equal(t, DefaultRecommendationPercentage, NormalizeRecommendationPercentage(0))
	assert.Equal(t, 120, NormalizeRecommendationPercentage(120))
	assert.Equal(t, 90, NormalizeRecommendationPercentage(90))
}

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
