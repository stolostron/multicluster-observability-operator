// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsutility

import (
	"context"
	"errors"
	"fmt"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ComponentType represents the type of right-sizing component
type ComponentType string

const (
	ComponentTypeNamespace      ComponentType = "namespace"
	ComponentTypeVirtualization ComponentType = "virtualization"

	// RSManagedByLabel is applied to all RS resources for label-based discovery during cleanup.
	RSManagedByLabel = "observability.open-cluster-management.io/managed-by"
	RSManagedByValue = "analytics-rightsizing"
)

// RSLabels returns the standard labels applied to all right-sizing resources.
func RSLabels() map[string]string {
	return map[string]string{RSManagedByLabel: RSManagedByValue}
}

// ComponentConfig holds configuration for a right-sizing component
type ComponentConfig struct {
	ComponentType            ComponentType
	ConfigMapName            string
	PlacementName            string
	PlacementBindingName     string
	PrometheusRulePolicyName string
	DefaultNamespace         string
	GetDefaultConfigFunc     func() map[string]string
	ApplyChangesFunc         func(context.Context, client.Client, RSNamespaceConfigMapData) error
}

// ComponentState holds the runtime state for a component
type ComponentState struct {
	Namespace string
	Enabled   bool
}

// GetComponentConfig extracts the configuration for a specific component type from MCO
func GetComponentConfig(mco *mcov1beta2.MultiClusterObservability, componentType ComponentType) (bool, string, error) {
	if !IsPlatformFeatureConfigured(mco) {
		return false, "", nil
	}

	switch componentType {
	case ComponentTypeNamespace:
		enabled := mco.Spec.Capabilities.Platform.Analytics.NamespaceRightSizingRecommendation.Enabled
		binding := mco.Spec.Capabilities.Platform.Analytics.NamespaceRightSizingRecommendation.NamespaceBinding
		return enabled, binding, nil
	case ComponentTypeVirtualization:
		enabled := mco.Spec.Capabilities.Platform.Analytics.VirtualizationRightSizingRecommendation.Enabled
		binding := mco.Spec.Capabilities.Platform.Analytics.VirtualizationRightSizingRecommendation.NamespaceBinding
		return enabled, binding, nil
	default:
		return false, "", fmt.Errorf("unknown component type: %s", componentType)
	}
}

// HandleComponentRightSizing handles the right-sizing functionality for any component type
func HandleComponentRightSizing(
	ctx context.Context,
	c client.Client,
	mco *mcov1beta2.MultiClusterObservability,
	componentConfig ComponentConfig,
	state *ComponentState,
) error {
	log.V(1).Info("rs - handling right-sizing", "component", componentConfig.ComponentType)

	// Get right-sizing configuration
	isEnabled, newBinding, err := GetComponentConfig(mco, componentConfig.ComponentType)
	if err != nil {
		return fmt.Errorf("rs - failed to get %s right-sizing config: %w", componentConfig.ComponentType, err)
	}

	// Set to default namespace if not given
	if newBinding == "" {
		newBinding = componentConfig.DefaultNamespace
	}

	// Check if right-sizing feature enabled or not
	// If disabled then cleanup related resources
	if !isEnabled {
		log.V(1).Info("rs - feature not enabled", "component", componentConfig.ComponentType)
		if err := CleanupComponentResources(ctx, c, componentConfig, state.Namespace, false); err != nil {
			return fmt.Errorf("rs - failed to cleanup %s resources: %w", componentConfig.ComponentType, err)
		}
		state.Namespace = newBinding
		state.Enabled = false
		return nil
	}

	// Set the flag if namespaceBindingUpdated
	namespaceBindingUpdated := state.Namespace != newBinding && state.Enabled

	// Set enabled flag which will be used for checking namespaceBindingUpdated condition next time
	state.Enabled = true

	// Retrieve the existing namespace and update the new namespace
	existingNamespace := state.Namespace
	state.Namespace = newBinding

	// Creating configmap with default values
	if err := EnsureRSConfigMapExists(ctx, c, componentConfig.ConfigMapName, componentConfig.GetDefaultConfigFunc); err != nil {
		return err
	}

	if namespaceBindingUpdated {
		// Clean up resources except config map to update NamespaceBinding
		if err := CleanupComponentResources(ctx, c, componentConfig, existingNamespace, true); err != nil {
			return fmt.Errorf("rs - failed to cleanup %s resources after namespace binding update: %w", componentConfig.ComponentType, err)
		}

		// Get configmap
		cm := &corev1.ConfigMap{}
		if err := c.Get(ctx, client.ObjectKey{Name: componentConfig.ConfigMapName, Namespace: config.GetDefaultNamespace()}, cm); err != nil {
			return fmt.Errorf("rs - failed to get existing configmap: %w", err)
		}

		// Get configmap data into specified structure
		configData, err := GetRSConfigData(cm)
		if err != nil {
			return fmt.Errorf("rs - failed to extract config data: %w", err)
		}

		// If NamespaceBinding has been updated apply the Policy Placement Placementbinding again
		if err := componentConfig.ApplyChangesFunc(ctx, c, configData); err != nil {
			return fmt.Errorf("rs - failed to apply configmap changes: %w", err)
		}
	}

	log.Info("rs - create component task completed", "component", componentConfig.ComponentType)
	return nil
}

// CleanupComponentResources cleans up the resources created for any component type
func CleanupComponentResources(
	ctx context.Context,
	c client.Client,
	componentConfig ComponentConfig,
	namespace string,
	bindingUpdated bool,
) error {
	log.V(1).Info("rs - cleaning up resources if exist", "component", componentConfig.ComponentType)

	var resourcesToDelete []client.Object
	commonResources := []client.Object{
		&policyv1.PlacementBinding{ObjectMeta: metav1.ObjectMeta{Name: componentConfig.PlacementBindingName, Namespace: namespace}},
		&clusterv1beta1.Placement{ObjectMeta: metav1.ObjectMeta{Name: componentConfig.PlacementName, Namespace: namespace}},
		&policyv1.Policy{ObjectMeta: metav1.ObjectMeta{Name: componentConfig.PrometheusRulePolicyName, Namespace: namespace}},
	}

	if bindingUpdated {
		// If NamespaceBinding has been updated delete only common resources
		resourcesToDelete = commonResources
	} else {
		resourcesToDelete = append(resourcesToDelete, commonResources...)
		resourcesToDelete = append(resourcesToDelete,
			&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: componentConfig.ConfigMapName, Namespace: config.GetDefaultNamespace()}},
		)
	}

	// Delete related resources, collecting errors so all deletes are attempted
	var errs []error
	for _, resource := range resourcesToDelete {
		err := c.Delete(ctx, resource)
		if err != nil && !apierrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
			log.Error(err, "rs - failed to delete resource", "name", resource.GetName(), "namespace", resource.GetNamespace(), "component", componentConfig.ComponentType)
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	log.Info("rs - cleanup success", "component", componentConfig.ComponentType)
	return nil
}

// CleanupRSResourcesByLabel deletes all right-sizing resources across namespaces using the managed-by label.
// This catches resources that may have been created in a different namespace due to NamespaceBinding changes.
func CleanupRSResourcesByLabel(ctx context.Context, c client.Client) error {
	log.Info("rs - label-based cleanup of all right-sizing resources")
	labelSelector := client.MatchingLabels{RSManagedByLabel: RSManagedByValue}

	var errs []error

	// Clean up Policies
	policyList := &policyv1.PolicyList{}
	if err := c.List(ctx, policyList, labelSelector); err != nil {
		if !meta.IsNoMatchError(err) {
			errs = append(errs, fmt.Errorf("rs - failed to list policies: %w", err))
		}
	} else {
		for i := range policyList.Items {
			if err := c.Delete(ctx, &policyList.Items[i]); err != nil && !apierrors.IsNotFound(err) {
				errs = append(errs, err)
			}
		}
	}

	// Clean up PlacementBindings
	pbList := &policyv1.PlacementBindingList{}
	if err := c.List(ctx, pbList, labelSelector); err != nil {
		if !meta.IsNoMatchError(err) {
			errs = append(errs, fmt.Errorf("rs - failed to list placementbindings: %w", err))
		}
	} else {
		for i := range pbList.Items {
			if err := c.Delete(ctx, &pbList.Items[i]); err != nil && !apierrors.IsNotFound(err) {
				errs = append(errs, err)
			}
		}
	}

	// Clean up Placements
	placementList := &clusterv1beta1.PlacementList{}
	if err := c.List(ctx, placementList, labelSelector); err != nil {
		if !meta.IsNoMatchError(err) {
			errs = append(errs, fmt.Errorf("rs - failed to list placements: %w", err))
		}
	} else {
		for i := range placementList.Items {
			if err := c.Delete(ctx, &placementList.Items[i]); err != nil && !apierrors.IsNotFound(err) {
				errs = append(errs, err)
			}
		}
	}

	// Clean up ConfigMaps
	cmList := &corev1.ConfigMapList{}
	if err := c.List(ctx, cmList, labelSelector); err != nil {
		if !meta.IsNoMatchError(err) {
			errs = append(errs, fmt.Errorf("rs - failed to list configmaps: %w", err))
		}
	} else {
		for i := range cmList.Items {
			if err := c.Delete(ctx, &cmList.Items[i]); err != nil && !apierrors.IsNotFound(err) {
				errs = append(errs, err)
			}
		}
	}

	return errors.Join(errs...)
}
