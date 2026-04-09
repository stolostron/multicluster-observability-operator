// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rightsizing

import (
	"context"
	"errors"
	"fmt"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	rsnamespace "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing/rs-namespace"
	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing/rs-utility"
	rsvirtualization "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing/rs-virtualization"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var log = logf.Log.WithName("analytics")

// CreateRightSizingComponent reconciles namespace and virtualization right-sizing resources based on the MCO spec.
func CreateRightSizingComponent(
	ctx context.Context,
	c client.Client,
	mco *mcov1beta2.MultiClusterObservability,
) error {
	log.V(1).Info("rs - inside create rs component")

	// Handle namespace right-sizing
	if err := rsnamespace.HandleRightSizing(ctx, c, mco); err != nil {
		return fmt.Errorf("failed to handle namespace right-sizing: %w", err)
	}

	// Handle virtualization right-sizing
	if err := rsvirtualization.HandleRightSizing(ctx, c, mco); err != nil {
		return fmt.Errorf("failed to handle virtualization right-sizing: %w", err)
	}

	log.Info("rs - create component task completed")
	return nil
}

// CleanupRightSizingResources cleans up all right-sizing resources (Policies, Placements, PlacementBindings, ConfigMaps).
// It uses a hybrid approach:
//  1. Label-based cleanup: discovers RS resources across all namespaces using the managed-by label.
//     This handles cases where NamespaceBinding was changed before deletion.
//  2. Name-based fallback: cleans up resources by well-known names in the spec namespace.
//     This handles upgrade scenarios where existing resources don't have labels yet.
func CleanupRightSizingResources(ctx context.Context, c client.Client, mco *mcov1beta2.MultiClusterObservability) error {
	log.Info("rs - cleaning up all right-sizing resources")

	var errs []error

	// Label-based cleanup: catches resources in any namespace
	if err := rsutility.CleanupRSResourcesByLabel(ctx, c); err != nil {
		errs = append(errs, err)
	}

	// Name-based fallback: catches old unlabeled resources (upgrade path)
	nsNamespace := getNamespaceBinding(mco, rsutility.ComponentTypeNamespace)
	virtNamespace := getNamespaceBinding(mco, rsutility.ComponentTypeVirtualization)
	if err := rsnamespace.CleanupRSNamespaceResources(ctx, c, nsNamespace, false); err != nil {
		errs = append(errs, err)
	}
	if err := rsvirtualization.CleanupRSVirtualizationResources(ctx, c, virtNamespace, false); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// getNamespaceBinding extracts the namespace binding from MCO spec for a given component type.
// Falls back to the default namespace if not configured.
func getNamespaceBinding(mco *mcov1beta2.MultiClusterObservability, componentType rsutility.ComponentType) string {
	_, binding, err := rsutility.GetComponentConfig(mco, componentType)
	if err != nil {
		log.V(1).Info("rs - falling back to default namespace", "component", componentType, "error", err)
		return rsutility.DefaultNamespace
	}
	if binding == "" {
		return rsutility.DefaultNamespace
	}
	return binding
}

// GetNamespaceRSConfigMapPredicateFunc returns predicate for namespace right-sizing ConfigMap
func GetNamespaceRSConfigMapPredicateFunc(ctx context.Context, c client.Client) predicate.Funcs {
	return rsnamespace.GetNamespaceRSConfigMapPredicateFunc(ctx, c)
}

// GetVirtualizationRSConfigMapPredicateFunc returns predicate for virtualization right-sizing ConfigMap
func GetVirtualizationRSConfigMapPredicateFunc(ctx context.Context, c client.Client) predicate.Funcs {
	return rsvirtualization.GetVirtualizationRSConfigMapPredicateFunc(ctx, c)
}
