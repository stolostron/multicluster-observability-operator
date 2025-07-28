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
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateVirtualizationPlacementBinding creates the PlacementBinding resource for virtualization
func CreateVirtualizationPlacementBinding(ctx context.Context, c client.Client) error {
	placementBinding := &policyv1.PlacementBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PlacementBindingName,
			Namespace: Namespace,
		},
	}

	// Declare name, namespace in common log context and use it later everywhere
	logCtx := []any{"Namespace:", placementBinding.Namespace, ", Name:", placementBinding.Name}

	// Fetch the PlacementBinding
	key := types.NamespacedName{
		Namespace: Namespace,
		Name:      PlacementBindingName,
	}

	err := c.Get(ctx, key, placementBinding)

	if err != nil {
		if errors.IsNotFound(err) {
			// Resource not found, create it
			placementBinding.PlacementRef = policyv1.PlacementSubject{
				Name:     PlacementName,
				Kind:     "Placement",
				APIGroup: "cluster.open-cluster-management.io",
			}

			placementBinding.Subjects = []policyv1.Subject{
				{
					Name:     PrometheusRulePolicyName,
					Kind:     "Policy",
					APIGroup: "policy.open-cluster-management.io",
				},
			}

			if err := c.Create(ctx, placementBinding); err != nil {
				return fmt.Errorf("rs - failed to create placementbinding: %w", err)
			}
			log.Info("rs - placementbinding created successfully", logCtx...)
			return nil
		}
		return fmt.Errorf("rs - failed to fetch placementbinding: %w", err)
	}

	log.V(1).Info("rs - placementbinding already exists, skipping creation", logCtx...)
	return nil
}
