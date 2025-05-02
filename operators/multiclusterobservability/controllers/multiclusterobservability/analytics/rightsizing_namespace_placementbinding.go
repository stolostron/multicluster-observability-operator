// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics

import (
	"context"

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
	err := c.Get(ctx, types.NamespacedName{
		Namespace: placementBinding.Namespace,
		Name:      placementBinding.Name,
	}, placementBinding)

	if err != nil && errors.IsNotFound(err) {

		log.Info("RS - PlacementBinding not found, creating a new one",
			" Name:", placementBinding.Name,
			" Namespace:", placementBinding.Namespace,
		)
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "RS - Unable to fetch PlacementBinding")
			return err
		}

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

		if err = c.Create(ctx, placementBinding); err != nil {
			log.Error(err, "Failed to create Placement")
			return err
		}
		log.Info("RS - PlacementBinding created", "PlacementBinding", rsPlacementBindingName)
	}
	log.Info("RS - PlacementBindingCreation completed")
	return nil
}
