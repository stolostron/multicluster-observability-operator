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

	err := c.Get(ctx, key, placement)
	if errors.IsNotFound(err) {
		log.Info("RS - Placement not found, creating a new one",
			" Name:", placement.Name,
			" Namespace:", placement.Namespace,
		)

		placement.Spec = placementConfig.Spec

		if err := c.Create(ctx, placement); err != nil {
			log.Error(err, "Failed to create Placement")
			return err
		}

		log.Info("RS - Placement created", "Placement", placement.Name)
		return nil
	}

	if err != nil {
		log.Error(err, "RS - Unable to fetch Placement")
		return err
	}

	placement.Spec = placementConfig.Spec

	if err := c.Update(ctx, placement); err != nil {
		log.Error(err, "Failed to update Placement")
		return err
	}

	log.Info("RS - Placement updated ", "Name:", placement.Name)
	return nil
}
