// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsutility

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
)

func newValidMCO() *mcov1beta2.MultiClusterObservability {
	return &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{Name: "test-observability"},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			Capabilities: &mcov1beta2.CapabilitiesSpec{
				Platform: &mcov1beta2.PlatformCapabilitiesSpec{
					Analytics: mcov1beta2.PlatformAnalyticsSpec{
						NamespaceRightSizingRecommendation: mcov1beta2.PlatformRightSizingRecommendationSpec{
							Enabled:          true,
							NamespaceBinding: "valid-namespace",
						},
						VirtualizationRightSizingRecommendation: mcov1beta2.PlatformRightSizingRecommendationSpec{
							Enabled:          true,
							NamespaceBinding: "valid-virt-namespace",
						},
					},
				},
			},
		},
	}
}

func TestValidateComponentConfiguration_ValidNamespace(t *testing.T) {
	mco := newValidMCO()

	result := ValidateComponentConfiguration(mco, ComponentTypeNamespace)
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)
}

func TestValidateComponentConfiguration_ValidVirtualization(t *testing.T) {
	mco := newValidMCO()

	result := ValidateComponentConfiguration(mco, ComponentTypeVirtualization)
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)
}

func TestValidateComponentConfiguration_NilMCO(t *testing.T) {
	result := ValidateComponentConfiguration(nil, ComponentTypeNamespace)
	assert.False(t, result.Valid)
	assert.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Error(), "MultiClusterObservability cannot be nil")
}

func TestValidateComponentConfiguration_EmptyName(t *testing.T) {
	mco := newValidMCO()
	mco.ObjectMeta.Name = ""

	result := ValidateComponentConfiguration(mco, ComponentTypeNamespace)
	assert.False(t, result.Valid)
	assert.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Error(), "MultiClusterObservability must have a name")
}

func TestValidateComponentConfiguration_UnknownComponentType(t *testing.T) {
	mco := newValidMCO()

	result := ValidateComponentConfiguration(mco, ComponentType("unknown"))
	assert.False(t, result.Valid)
	assert.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Error(), "unknown component type")
}

func TestValidateComponentConfiguration_PlatformNotConfigured(t *testing.T) {
	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{Name: "test-observability"},
		Spec:       mcov1beta2.MultiClusterObservabilitySpec{},
	}

	result := ValidateComponentConfiguration(mco, ComponentTypeNamespace)
	assert.True(t, result.Valid) // Not an error, just not configured
	assert.Empty(t, result.Errors)
}

func TestValidateComponentConfiguration_InvalidNamespaceFormat(t *testing.T) {
	mco := newValidMCO()
	mco.Spec.Capabilities.Platform.Analytics.NamespaceRightSizingRecommendation.NamespaceBinding = "Invalid_Namespace!"

	result := ValidateComponentConfiguration(mco, ComponentTypeNamespace)
	assert.False(t, result.Valid)
	assert.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Error(), "must be a valid DNS-1123 label")
}

func TestValidateAllComponents_Valid(t *testing.T) {
	mco := newValidMCO()

	result := ValidateAllComponents(mco)
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)
}

func TestValidateAllComponents_ConflictingNamespaceBindings(t *testing.T) {
	mco := newValidMCO()
	// Set both components to use the same namespace
	mco.Spec.Capabilities.Platform.Analytics.NamespaceRightSizingRecommendation.NamespaceBinding = "same-namespace"
	mco.Spec.Capabilities.Platform.Analytics.VirtualizationRightSizingRecommendation.NamespaceBinding = "same-namespace"

	result := ValidateAllComponents(mco)
	assert.False(t, result.Valid)
	assert.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Error(), "cannot use the same namespace binding")
}

func TestValidateNamespaceFormat_Valid(t *testing.T) {
	validNamespaces := []string{
		"valid-namespace",
		"test123",
		"a",
		"namespace-with-dashes",
		"ns123-test",
	}

	for _, ns := range validNamespaces {
		err := validateNamespaceFormat(ns)
		assert.NoError(t, err, "Expected %s to be valid", ns)
	}
}

func TestValidateNamespaceFormat_Invalid(t *testing.T) {
	invalidCases := []struct {
		namespace string
		reason    string
	}{
		{"", "empty namespace"},
		{"Invalid_Namespace", "contains underscore"},
		{"namespace!", "contains special character"},
		{"UPPERCASE", "contains uppercase"},
		{"-start-with-dash", "starts with dash"},
		{"end-with-dash-", "ends with dash"},
		{"kube-system", "reserved namespace"},
		{"default", "reserved namespace"},
		{string(make([]byte, 64)), "too long"},
	}

	for _, tc := range invalidCases {
		err := validateNamespaceFormat(tc.namespace)
		assert.Error(t, err, "Expected %s to be invalid (%s)", tc.namespace, tc.reason)
	}
}

func TestValidateConfigMapData_Valid(t *testing.T) {
	configData := RSNamespaceConfigMapData{
		PrometheusRuleConfig: RSPrometheusRuleConfig{
			NamespaceFilterCriteria: struct {
				InclusionCriteria []string `yaml:"inclusionCriteria"`
				ExclusionCriteria []string `yaml:"exclusionCriteria"`
			}{
				ExclusionCriteria: []string{"openshift.*"},
			},
			LabelFilterCriteria: []RSLabelFilter{
				{
					LabelName:         "label_env",
					InclusionCriteria: []string{"prod", "staging"},
				},
			},
			RecommendationPercentage: 110,
		},
	}

	err := ValidateConfigMapData(configData)
	assert.NoError(t, err)
}

func TestValidateConfigMapData_InvalidRecommendationPercentage(t *testing.T) {
	configData := RSNamespaceConfigMapData{
		PrometheusRuleConfig: RSPrometheusRuleConfig{
			RecommendationPercentage: 50, // Too low
		},
	}

	err := ValidateConfigMapData(configData)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "recommendation percentage must be between 100 and 200")

	configData.PrometheusRuleConfig.RecommendationPercentage = 250 // Too high
	err = ValidateConfigMapData(configData)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "recommendation percentage must be between 100 and 200")
}

func TestValidateConfigMapData_BothInclusionAndExclusion(t *testing.T) {
	configData := RSNamespaceConfigMapData{
		PrometheusRuleConfig: RSPrometheusRuleConfig{
			NamespaceFilterCriteria: struct {
				InclusionCriteria []string `yaml:"inclusionCriteria"`
				ExclusionCriteria []string `yaml:"exclusionCriteria"`
			}{
				InclusionCriteria: []string{"include"},
				ExclusionCriteria: []string{"exclude"},
			},
			RecommendationPercentage: 110,
		},
	}

	err := ValidateConfigMapData(configData)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot specify both inclusion and exclusion criteria")
}

func TestValidateConfigMapData_InvalidLabelFilter(t *testing.T) {
	configData := RSNamespaceConfigMapData{
		PrometheusRuleConfig: RSPrometheusRuleConfig{
			LabelFilterCriteria: []RSLabelFilter{
				{
					LabelName: "", // Empty label name
				},
			},
			RecommendationPercentage: 110,
		},
	}

	err := ValidateConfigMapData(configData)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "label name cannot be empty")

	// Test both inclusion and exclusion criteria
	configData.PrometheusRuleConfig.LabelFilterCriteria[0] = RSLabelFilter{
		LabelName:         "test_label",
		InclusionCriteria: []string{"include"},
		ExclusionCriteria: []string{"exclude"},
	}

	err = ValidateConfigMapData(configData)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot specify both inclusion and exclusion criteria")
}

func TestValidationResult_GetErrorSummary(t *testing.T) {
	// Test valid result
	validResult := ValidationResult{Valid: true}
	summary := validResult.GetErrorSummary()
	assert.Equal(t, "Configuration is valid", summary)

	// Test invalid result with errors
	invalidResult := ValidationResult{Valid: false}
	invalidResult.AddError(ComponentTypeNamespace, "field1", "error message 1")
	invalidResult.AddError(ComponentTypeVirtualization, "field2", "error message 2")

	summary = invalidResult.GetErrorSummary()
	assert.Contains(t, summary, "Found 2 validation errors")
	assert.Contains(t, summary, "namespace component validation failed")
	assert.Contains(t, summary, "virtualization component validation failed")
}

func TestValidationError_Error(t *testing.T) {
	err := ValidationError{
		Component: ComponentTypeNamespace,
		Field:     "namespaceBinding",
		Message:   "invalid format",
	}

	errorStr := err.Error()
	assert.Equal(t, "namespace component validation failed for namespaceBinding: invalid format", errorStr)
}
