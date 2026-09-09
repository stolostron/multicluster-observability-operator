// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package mcoa

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	cooSubscriptionName   = "cluster-observability-operator"
	cooStatusConfigMap    = "coo-status"
	cooStatusInstalledKey = "installed"
	cooStatusManagedByKey = "managedBy"
	mcoaReleaseLabel      = "release"
	mcoaReleaseName       = "multicluster-observability-addon"
	managedByMCOA         = "mcoa"
	managedByExternal     = "external"
)

// WriteCOOStatus checks whether a COO Subscription exists on the spoke (in any namespace)
// and writes the result to a coo-status ConfigMap. MCOA on the hub reads this ConfigMap
// via ManifestWork feedback to decide whether to install COO.
func WriteCOOStatus(ctx context.Context, c client.Client, namespace string, log logr.Logger) error {
	status, err := getCOOSubscriptionStatus(ctx, c)
	if err != nil {
		return fmt.Errorf("failed to check COO subscription: %w", err)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cooStatusConfigMap,
			Namespace: namespace,
		},
	}

	existing := &corev1.ConfigMap{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(cm), existing); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get coo-status ConfigMap: %w", err)
		}
		cm.Data = status
		if err := c.Create(ctx, cm); err != nil {
			return fmt.Errorf("failed to create coo-status ConfigMap: %w", err)
		}
		log.Info("Created coo-status ConfigMap", "data", status)
		return nil
	}

	if needsUpdate(existing.Data, status) {
		existing.Data = status
		if err := c.Update(ctx, existing); err != nil {
			return fmt.Errorf("failed to update coo-status ConfigMap: %w", err)
		}
		log.Info("Updated coo-status ConfigMap", "data", status)
	}

	return nil
}

func needsUpdate(current, desired map[string]string) bool {
	if len(current) != len(desired) {
		return true
	}
	for k, v := range desired {
		if current[k] != v {
			return true
		}
	}
	return false
}

func getCOOSubscriptionStatus(ctx context.Context, c client.Client) (map[string]string, error) {
	subList := &unstructured.UnstructuredList{}
	subList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "operators.coreos.com",
		Version: "v1alpha1",
		Kind:    "Subscription",
	})

	if err := c.List(ctx, subList); err != nil {
		if errors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return map[string]string{
				cooStatusInstalledKey: "false",
				cooStatusManagedByKey: "",
			}, nil
		}
		return nil, fmt.Errorf("failed to list subscriptions: %w", err)
	}

	for _, sub := range subList.Items {
		specName, _, _ := unstructured.NestedString(sub.Object, "spec", "name")
		if specName == cooSubscriptionName {
			managedBy := managedByExternal
			labels := sub.GetLabels()
			if labels != nil && labels[mcoaReleaseLabel] == mcoaReleaseName {
				managedBy = managedByMCOA
			}
			return map[string]string{
				cooStatusInstalledKey: "true",
				cooStatusManagedByKey: managedBy,
			}, nil
		}
	}

	return map[string]string{
		cooStatusInstalledKey: "false",
		cooStatusManagedByKey: "",
	}, nil
}
