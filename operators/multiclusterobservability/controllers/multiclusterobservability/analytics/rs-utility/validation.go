// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsutility

import (
	"fmt"
	"regexp"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
)

// ValidationError represents a configuration validation error
type ValidationError struct {
	Component ComponentType
	Field     string
	Message   string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s component validation failed for %s: %s", e.Component, e.Field, e.Message)
}

// ValidationResult holds the results of configuration validation
type ValidationResult struct {
	Valid  bool
	Errors []ValidationError
}

// AddError adds a validation error to the result
func (vr *ValidationResult) AddError(component ComponentType, field, message string) {
	vr.Valid = false
	vr.Errors = append(vr.Errors, ValidationError{
		Component: component,
		Field:     field,
		Message:   message,
	})
}

// GetErrorSummary returns a summary of all validation errors
func (vr *ValidationResult) GetErrorSummary() string {
	if vr.Valid {
		return "Configuration is valid"
	}

	summary := fmt.Sprintf("Found %d validation errors:\n", len(vr.Errors))
	for i, err := range vr.Errors {
		summary += fmt.Sprintf("  %d. %s\n", i+1, err.Error())
	}
	return summary
}

// ValidateComponentConfiguration validates the configuration for a specific component
func ValidateComponentConfiguration(mco *mcov1beta2.MultiClusterObservability, componentType ComponentType) ValidationResult {
	result := ValidationResult{Valid: true}

	// Basic MCO validation
	if err := validateBasicMCO(mco); err != nil {
		result.AddError(componentType, "mco", err.Error())
		return result // Early return for critical errors
	}

	// Platform feature validation
	if !IsPlatformFeatureConfigured(mco) {
		// This is not an error, just means the feature is not configured
		return result
	}

	// Component-specific validation
	switch componentType {
	case ComponentTypeNamespace:
		validateNamespaceComponent(mco, &result)
	case ComponentTypeVirtualization:
		validateVirtualizationComponent(mco, &result)
	default:
		result.AddError(componentType, "componentType", "unknown component type")
	}

	return result
}

// ValidateAllComponents validates all right-sizing components
func ValidateAllComponents(mco *mcov1beta2.MultiClusterObservability) ValidationResult {
	result := ValidationResult{Valid: true}

	// Basic MCO validation
	if err := validateBasicMCO(mco); err != nil {
		result.AddError("", "mco", err.Error())
		return result
	}

	// Skip component validation if platform features not configured
	if !IsPlatformFeatureConfigured(mco) {
		return result
	}

	// Validate each component
	components := []ComponentType{ComponentTypeNamespace, ComponentTypeVirtualization}
	for _, component := range components {
		componentResult := ValidateComponentConfiguration(mco, component)
		if !componentResult.Valid {
			result.Valid = false
			result.Errors = append(result.Errors, componentResult.Errors...)
		}
	}

	// Cross-component validation
	validateCrossComponentConfiguration(mco, &result)

	return result
}

// validateBasicMCO performs basic MCO validation
func validateBasicMCO(mco *mcov1beta2.MultiClusterObservability) error {
	if mco == nil {
		return fmt.Errorf("MultiClusterObservability cannot be nil")
	}

	if mco.ObjectMeta.Name == "" {
		return fmt.Errorf("MultiClusterObservability must have a name")
	}

	return nil
}

// validateNamespaceComponent validates namespace right-sizing specific configuration
func validateNamespaceComponent(mco *mcov1beta2.MultiClusterObservability, result *ValidationResult) {
	nsConfig := mco.Spec.Capabilities.Platform.Analytics.NamespaceRightSizingRecommendation

	// Validate namespace binding
	if nsConfig.Enabled && nsConfig.NamespaceBinding != "" {
		if err := validateNamespaceFormat(nsConfig.NamespaceBinding); err != nil {
			result.AddError(ComponentTypeNamespace, "namespaceBinding", err.Error())
		}
	}

	// Additional namespace-specific validations can be added here
}

// validateVirtualizationComponent validates virtualization right-sizing specific configuration
func validateVirtualizationComponent(mco *mcov1beta2.MultiClusterObservability, result *ValidationResult) {
	virtConfig := mco.Spec.Capabilities.Platform.Analytics.VirtualizationRightSizingRecommendation

	// Validate namespace binding
	if virtConfig.Enabled && virtConfig.NamespaceBinding != "" {
		if err := validateNamespaceFormat(virtConfig.NamespaceBinding); err != nil {
			result.AddError(ComponentTypeVirtualization, "namespaceBinding", err.Error())
		}
	}

	// Additional virtualization-specific validations can be added here
}

// validateCrossComponentConfiguration validates configuration that affects multiple components
func validateCrossComponentConfiguration(mco *mcov1beta2.MultiClusterObservability, result *ValidationResult) {
	nsConfig := mco.Spec.Capabilities.Platform.Analytics.NamespaceRightSizingRecommendation
	virtConfig := mco.Spec.Capabilities.Platform.Analytics.VirtualizationRightSizingRecommendation

	// Check for conflicting namespace bindings
	if nsConfig.Enabled && virtConfig.Enabled &&
		nsConfig.NamespaceBinding != "" && virtConfig.NamespaceBinding != "" &&
		nsConfig.NamespaceBinding == virtConfig.NamespaceBinding {

		result.AddError("", "namespaceBinding",
			fmt.Sprintf("namespace and virtualization components cannot use the same namespace binding: %s",
				nsConfig.NamespaceBinding))
	}
}

// validateNamespaceFormat validates that a namespace name follows Kubernetes naming conventions
func validateNamespaceFormat(namespace string) error {
	if namespace == "" {
		return fmt.Errorf("namespace cannot be empty")
	}

	// Kubernetes namespace name validation
	// Must be a valid DNS-1123 label (RFC 1123)
	nsRegex := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	if !nsRegex.MatchString(namespace) {
		return fmt.Errorf("namespace '%s' is invalid: must be a valid DNS-1123 label", namespace)
	}

	// Length validation (Kubernetes limit is 63 characters)
	if len(namespace) > 63 {
		return fmt.Errorf("namespace '%s' is too long: maximum length is 63 characters", namespace)
	}

	// Reserved namespace validation
	reservedNamespaces := []string{
		"kube-system", "kube-public", "kube-node-lease", "default",
	}
	for _, reserved := range reservedNamespaces {
		if namespace == reserved {
			return fmt.Errorf("namespace '%s' is reserved and cannot be used", namespace)
		}
	}

	return nil
}

// ValidateConfigMapData validates the structure and content of ConfigMap data
func ValidateConfigMapData(configData RSNamespaceConfigMapData) error {
	// Validate recommendation percentage
	if configData.PrometheusRuleConfig.RecommendationPercentage < 100 ||
		configData.PrometheusRuleConfig.RecommendationPercentage > 200 {
		return fmt.Errorf("recommendation percentage must be between 100 and 200, got: %d",
			configData.PrometheusRuleConfig.RecommendationPercentage)
	}

	// Validate namespace filter criteria
	nsFilter := configData.PrometheusRuleConfig.NamespaceFilterCriteria
	if len(nsFilter.InclusionCriteria) > 0 && len(nsFilter.ExclusionCriteria) > 0 {
		return fmt.Errorf("cannot specify both inclusion and exclusion criteria for namespace filtering")
	}

	// Validate label filter criteria
	for i, labelFilter := range configData.PrometheusRuleConfig.LabelFilterCriteria {
		if labelFilter.LabelName == "" {
			return fmt.Errorf("label filter %d: label name cannot be empty", i)
		}
		if len(labelFilter.InclusionCriteria) > 0 && len(labelFilter.ExclusionCriteria) > 0 {
			return fmt.Errorf("label filter %d (%s): cannot specify both inclusion and exclusion criteria",
				i, labelFilter.LabelName)
		}
	}

	return nil
}
