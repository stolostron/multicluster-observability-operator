// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package mcoa

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	cooSubscriptionName = "cluster-observability-operator"
	mcoaReleaseLabel    = "release"
	mcoaReleaseName     = "multicluster-observability-addon"
	managedByMCOA       = "mcoa"
	managedByExternal   = "external"

	CooInstalledClaimName = "coo-installed.observability.open-cluster-management.io"
	CooManagedByClaimName = "coo-managed-by.observability.open-cluster-management.io"
)

// WriteCOOStatus checks whether a COO Subscription exists on the spoke and
// writes the result as ClusterClaims. The OCM registration agent automatically
// syncs these to ManagedCluster.Status.ClusterClaims on the hub.
func WriteCOOStatus(ctx context.Context, c client.Client, log logr.Logger) error {
	installed, managedBy, err := getCOOSubscriptionStatus(ctx, c)
	if err != nil {
		return fmt.Errorf("failed to check COO subscription: %w", err)
	}

	if err := ensureClusterClaim(ctx, c, CooInstalledClaimName, installed); err != nil {
		return fmt.Errorf("failed to ensure ClusterClaim %s: %w", CooInstalledClaimName, err)
	}

	if managedBy == "" {
		managedBy = "none"
	}
	if err := ensureClusterClaim(ctx, c, CooManagedByClaimName, managedBy); err != nil {
		return fmt.Errorf("failed to ensure ClusterClaim %s: %w", CooManagedByClaimName, err)
	}

	log.V(1).Info("COO status ClusterClaims updated", "installed", installed, "managedBy", managedBy)
	return nil
}

func ensureClusterClaim(ctx context.Context, c client.Client, name, value string) error {
	claim := &clusterv1alpha1.ClusterClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, c, claim, func() error {
		claim.Spec.Value = value
		return nil
	})
	return err
}

func getCOOSubscriptionStatus(ctx context.Context, c client.Client) (installed string, managedBy string, err error) {
	subList := &unstructured.UnstructuredList{}
	subList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "operators.coreos.com",
		Version: "v1alpha1",
		Kind:    "Subscription",
	})

	if err := c.List(ctx, subList); err != nil {
		if errors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return "false", "", nil
		}
		return "", "", fmt.Errorf("failed to list subscriptions: %w", err)
	}

	for _, sub := range subList.Items {
		specName, _, _ := unstructured.NestedString(sub.Object, "spec", "name")
		if specName == cooSubscriptionName {
			managedBy := managedByExternal
			labels := sub.GetLabels()
			if labels != nil && labels[mcoaReleaseLabel] == mcoaReleaseName {
				managedBy = managedByMCOA
			}
			return "true", managedBy, nil
		}
	}

	return "false", "", nil
}
