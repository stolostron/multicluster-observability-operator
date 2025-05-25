// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// createUpdatePlacement creates the Placement resource
func createUpdatePlacement(ctx context.Context, c client.Client, placementConfig clusterv1beta1.Placement) error {

	placement := &clusterv1beta1.Placement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rsPlacementName,
			Namespace: rsNamespace,
		},
	}
	key := types.NamespacedName{
		Namespace: rsNamespace,
		Name:      rsPlacementName,
	}

	// Declare name, namespace in common log context and use it later everywhere
	logCtx := []any{"Namespace:", placement.Namespace, ", Name:", placement.Name}

	err := c.Get(ctx, key, placement)
	if err != nil {
		if errors.IsNotFound(err) {
			placement.Spec = placementConfig.Spec

			// Create placement
			if err := c.Create(ctx, placement); err != nil {
				log.Error(err, "RS - Failed to create Placement", logCtx...)
				return err
			}

			log.Info("RS - Placement created successfully", logCtx...)
			return nil
		} else {
			log.Error(err, "RS - Unable to fetch Placement", logCtx...)
			return err
		}
	}

	// Update existing placement
	placement.Spec = placementConfig.Spec
	if err := c.Update(ctx, placement); err != nil {
		log.Error(err, "RS - Failed to update Placement", logCtx...)
		return err
	}

	log.Info("RS - Placement updated successfully", logCtx...)
	return nil
}
