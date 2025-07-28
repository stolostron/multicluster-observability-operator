// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsvirtualization

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateUpdateVirtualizationPlacement creates the Placement resource for virtualization
func CreateUpdateVirtualizationPlacement(ctx context.Context, c client.Client, placementConfig clusterv1beta1.Placement) error {

	placement := &clusterv1beta1.Placement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PlacementName,
			Namespace: Namespace,
		},
	}
	key := types.NamespacedName{
		Namespace: Namespace,
		Name:      PlacementName,
	}

	// Declare name, namespace in common log context and use it later everywhere
	logCtx := []any{"Namespace:", placement.Namespace, ", Name:", placement.Name}

	err := c.Get(ctx, key, placement)

	if err != nil {
		if errors.IsNotFound(err) {
			// Resource not found, create it
			placement.Spec = placementConfig.Spec

			if err := c.Create(ctx, placement); err != nil {
				return fmt.Errorf("rs - failed to create placement: %w", err)
			}
			log.Info("rs - placement created successfully", logCtx...)
			return nil
		}
		return fmt.Errorf("rs - failed to fetch placement: %w", err)
	}

	// Update placement
	placement.Spec = placementConfig.Spec

	if err := c.Update(ctx, placement); err != nil {
		return fmt.Errorf("rs - failed to update placement: %w", err)
	}

	log.Info("rs - placement updated successfully", logCtx...)
	return nil
}
