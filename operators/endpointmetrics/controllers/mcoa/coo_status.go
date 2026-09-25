// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package mcoa

import (
	"context"
	"fmt"

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

	CooStatusClaimName = "coo.observability.open-cluster-management.io"

	CooStatusNotInstalled = "not-installed"
	CooStatusMCOA         = "mcoa"
	CooStatusExternal     = "external"
)

// WriteCOOStatus checks whether a COO Subscription exists on the spoke and
// writes the result as a single ClusterClaim. The OCM registration agent
// automatically syncs it to ManagedCluster.Status.ClusterClaims on the hub.
// On non-OCP clusters (OLMAvailable=false), this is a no-op.
func (r *MCOAAgentReconciler) WriteCOOStatus(ctx context.Context) error {
	if !r.OLMAvailable {
		return nil
	}

	status, err := getCOOSubscriptionStatus(ctx, r.Client)
	if err != nil {
		return fmt.Errorf("failed to check COO subscription: %w", err)
	}

	claim := &clusterv1alpha1.ClusterClaim{
		ObjectMeta: metav1.ObjectMeta{Name: CooStatusClaimName},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, claim, func() error {
		claim.Spec.Value = status
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to ensure ClusterClaim %s: %w", CooStatusClaimName, err)
	}

	r.Log.V(1).Info("COO status ClusterClaim updated", "status", status)
	return nil
}

func getCOOSubscriptionStatus(ctx context.Context, c client.Client) (string, error) {
	subList := &unstructured.UnstructuredList{}
	subList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "operators.coreos.com",
		Version: "v1alpha1",
		Kind:    "Subscription",
	})

	if err := c.List(ctx, subList); err != nil {
		if errors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return CooStatusNotInstalled, nil
		}
		return "", fmt.Errorf("failed to list subscriptions: %w", err)
	}

	allMCOA := true
	found := false
	for _, sub := range subList.Items {
		specName, _, _ := unstructured.NestedString(sub.Object, "spec", "name")
		if specName != cooSubscriptionName {
			continue
		}
		found = true
		labels := sub.GetLabels()
		if labels == nil || labels[mcoaReleaseLabel] != mcoaReleaseName {
			allMCOA = false
		}
	}

	if !found {
		return CooStatusNotInstalled, nil
	}
	if allMCOA {
		return CooStatusMCOA, nil
	}
	return CooStatusExternal, nil
}
