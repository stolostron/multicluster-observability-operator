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
// It reads the namespace from the MCO spec to avoid relying on in-memory state which may be stale after operator restart.
func CleanupRightSizingResources(ctx context.Context, c client.Client, mco *mcov1beta2.MultiClusterObservability) error {
	log.Info("rs - cleaning up all right-sizing resources")

	nsNamespace := getNamespaceBinding(mco, rsutility.ComponentTypeNamespace)
	virtNamespace := getNamespaceBinding(mco, rsutility.ComponentTypeVirtualization)

	var errs []error
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
	if err != nil || binding == "" {
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
