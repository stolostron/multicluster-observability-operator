// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// createPlacementBinding creates the PlacementBinding resource
func createPlacementBinding(ctx context.Context, c client.Client) error {
	placementBinding := &policyv1.PlacementBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rsPlacementBindingName,
			Namespace: rsNamespace,
		},
	}

	// Declare name, namespace in common log context and use it later everywhere
	logCtx := []any{"Namespace:", placementBinding.Namespace, ", Name:", placementBinding.Name}

	// Fetch the PlacementBinding
	err := c.Get(ctx, types.NamespacedName{
		Namespace: placementBinding.Namespace,
		Name:      placementBinding.Name,
	}, placementBinding)

	if err != nil {
		if errors.IsNotFound(err) {
			placementBinding.PlacementRef = policyv1.PlacementSubject{
				Name:     rsPlacementName,
				APIGroup: "cluster.open-cluster-management.io",
				Kind:     "Placement",
			}
			placementBinding.Subjects = []policyv1.Subject{
				{
					Name:     rsPrometheusRulePolicyName,
					APIGroup: "policy.open-cluster-management.io",
					Kind:     "Policy",
				},
			}

			if err := c.Create(ctx, placementBinding); err != nil {
				return fmt.Errorf("rs - failed to create placementbinding: %w", err)
			}
			log.Info("rs - placementbinding created successfully", logCtx...)
		} else {
			return fmt.Errorf("rs - failed to fetch placementbinding: %w", err)
		}
	} else {
		log.V(1).Info("rs - placementbinding already exists, skipping creation", logCtx...)
	}

	return nil
}
