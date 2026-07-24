// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsutility

import (
	"context"
	"errors"
	"fmt"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
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

	// Legacy resource names from pre-GA Policy-based installations.
	// Used by CleanupLegacyPolicyResourcesByName for unlabeled 2.16 resources.
	legacyNSPolicyName           = "rs-prom-rules-policy"
	legacyNSPlacementName        = "rs-placement"
	legacyNSPlacementBindingName = "rs-policyset-binding"

	legacyVirtPolicyName           = "rs-virt-prom-rules-policy"
	legacyVirtPlacementName        = "rs-virt-placement"
	legacyVirtPlacementBindingName = "rs-virt-policyset-binding"
)

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

// CleanupRSResourcesByLabel deletes all right-sizing resources across namespaces using the managed-by label.
// This catches resources that may have been created in a different namespace due to NamespaceBinding changes.
// ConfigMaps are included — use this only in full deletion (MCO CR deleted).
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
				errs = append(errs, fmt.Errorf("rs - failed to delete Policy %s/%s: %w",
					policyList.Items[i].Namespace, policyList.Items[i].Name, err))
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
				errs = append(errs, fmt.Errorf("rs - failed to delete PlacementBinding %s/%s: %w",
					pbList.Items[i].Namespace, pbList.Items[i].Name, err))
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
				errs = append(errs, fmt.Errorf("rs - failed to delete Placement %s/%s: %w",
					placementList.Items[i].Namespace, placementList.Items[i].Name, err))
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
				errs = append(errs, fmt.Errorf("rs - failed to delete ConfigMap %s/%s: %w",
					cmList.Items[i].Namespace, cmList.Items[i].Name, err))
			}
		}
	}

	return errors.Join(errs...)
}

// CleanupLegacyPolicyResourcesByLabel deletes labeled Policy, PlacementBinding, and Placement
// resources created by pre-GA (Policy-based) installations. ConfigMaps are NOT deleted because
// MCOA reuses them for per-cluster configuration.
func CleanupLegacyPolicyResourcesByLabel(ctx context.Context, c client.Client) error {
	log.Info("rs - label-based cleanup of legacy Policy resources (preserving ConfigMaps)")
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
				errs = append(errs, fmt.Errorf("rs - failed to delete legacy Policy %s/%s: %w",
					policyList.Items[i].Namespace, policyList.Items[i].Name, err))
			} else if err == nil {
				log.Info("rs - deleted legacy Policy", "name", policyList.Items[i].Name, "namespace", policyList.Items[i].Namespace)
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
				errs = append(errs, fmt.Errorf("rs - failed to delete legacy PlacementBinding %s/%s: %w",
					pbList.Items[i].Namespace, pbList.Items[i].Name, err))
			} else if err == nil {
				log.Info("rs - deleted legacy PlacementBinding", "name", pbList.Items[i].Name, "namespace", pbList.Items[i].Namespace)
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
				errs = append(errs, fmt.Errorf("rs - failed to delete legacy Placement %s/%s: %w",
					placementList.Items[i].Namespace, placementList.Items[i].Name, err))
			} else if err == nil {
				log.Info("rs - deleted legacy Placement", "name", placementList.Items[i].Name, "namespace", placementList.Items[i].Namespace)
			}
		}
	}

	return errors.Join(errs...)
}

// CleanupLegacyPolicyResourcesByName deletes Policy, PlacementBinding, and Placement resources
// by their well-known pre-GA names. This handles 2.16 resources that may not have RS labels.
// ConfigMaps are NOT deleted. NotFound and CRD-missing errors are silently ignored.
func CleanupLegacyPolicyResourcesByName(ctx context.Context, c client.Client, nsNamespace, virtNamespace string) error {
	log.Info("rs - name-based cleanup of legacy Policy resources", "nsNamespace", nsNamespace, "virtNamespace", virtNamespace)

	resources := []client.Object{
		&policyv1.Policy{ObjectMeta: metav1.ObjectMeta{Name: legacyNSPolicyName, Namespace: nsNamespace}},
		&policyv1.PlacementBinding{ObjectMeta: metav1.ObjectMeta{Name: legacyNSPlacementBindingName, Namespace: nsNamespace}},
		&clusterv1beta1.Placement{ObjectMeta: metav1.ObjectMeta{Name: legacyNSPlacementName, Namespace: nsNamespace}},
		&policyv1.Policy{ObjectMeta: metav1.ObjectMeta{Name: legacyVirtPolicyName, Namespace: virtNamespace}},
		&policyv1.PlacementBinding{ObjectMeta: metav1.ObjectMeta{Name: legacyVirtPlacementBindingName, Namespace: virtNamespace}},
		&clusterv1beta1.Placement{ObjectMeta: metav1.ObjectMeta{Name: legacyVirtPlacementName, Namespace: virtNamespace}},
	}

	var errs []error
	for _, obj := range resources {
		if err := c.Delete(ctx, obj); err != nil {
			if !apierrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
				errs = append(errs, fmt.Errorf("rs - failed to delete %s/%s: %w",
					obj.GetNamespace(), obj.GetName(), err))
			}
		} else {
			log.Info("rs - deleted legacy resource by name", "name", obj.GetName(), "namespace", obj.GetNamespace())
		}
	}
	return errors.Join(errs...)
}
