// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	"fmt"
	"os"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	ManagedClusterAddonName = "observability-controller" // #nosec G101 -- Not a hardcoded credential.
)

var (
	log            = logf.Log.WithName("util")
	spokeNameSpace = os.Getenv("SPOKE_NAMESPACE")
)

func CreateManagedClusterAddonCR(ctx context.Context, c client.Client, namespace, labelKey, labelValue string) (*addonv1alpha1.ManagedClusterAddOn, error) {
	// check if managedClusterAddon exists
	managedClusterAddon := &addonv1alpha1.ManagedClusterAddOn{}
	objectKey := types.NamespacedName{
		Name:      ManagedClusterAddonName,
		Namespace: namespace,
	}
	err := c.Get(ctx, objectKey, managedClusterAddon)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get the managedClusterAddon: %w", err)
	}
	if err == nil {
		return managedClusterAddon, nil
	}

	// AddOn doesn't exist, create it
	newManagedClusterAddon := &addonv1alpha1.ManagedClusterAddOn{
		TypeMeta: metav1.TypeMeta{
			APIVersion: addonv1alpha1.SchemeGroupVersion.String(),
			Kind:       "ManagedClusterAddOn",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ManagedClusterAddonName,
			Namespace: namespace,
			Labels: map[string]string{
				labelKey: labelValue,
			},
		},
		Spec: addonv1alpha1.ManagedClusterAddOnSpec{
			InstallNamespace: spokeNameSpace,
		},
	}
	if err := c.Create(ctx, newManagedClusterAddon); err != nil {
		return nil, fmt.Errorf("failed to create managedclusteraddon for namespace %s: %w", namespace, err)
	}

	// Update its status
	if managedClusterAddon, err = updateManagedClusterAddOnStatus(ctx, c, namespace); err != nil {
		return nil, fmt.Errorf("failed to update newly created managedClusterAddonStatus: %w", err)
	}

	log.Info("ManagedClusterAddOn is created/updated successfully", "name", ManagedClusterAddonName, "namespace", namespace)
	return managedClusterAddon, nil
}

func updateManagedClusterAddOnStatus(ctx context.Context, c client.Client, namespace string) (*addonv1alpha1.ManagedClusterAddOn, error) {
	managedClusterAddon := &addonv1alpha1.ManagedClusterAddOn{}
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// wait until the cache gets the newly created managedClusterAddon
		errPoll := wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
			objectKey := types.NamespacedName{
				Name:      ManagedClusterAddonName,
				Namespace: namespace,
			}
			if err := c.Get(ctx, objectKey, managedClusterAddon); err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil // Not found yet, continue polling
				}
				return false, err
			}
			return true, nil
		})
		if errPoll != nil {
			return fmt.Errorf("failed to get the created managedclusteraddon: %w", errPoll)
		}

		// got the created managedclusteraddon just now, updating its status
		//nolint:staticcheck
		managedClusterAddon.Status.AddOnConfiguration = addonv1alpha1.ConfigCoordinates{
			CRDName: "observabilityaddons.observability.open-cluster-management.io",
			CRName:  "observability-addon",
		}
		managedClusterAddon.Status.AddOnMeta = addonv1alpha1.AddOnMeta{
			DisplayName: "Observability Controller",
			Description: "Manages Observability components.",
		}
		if len(managedClusterAddon.Status.Conditions) > 0 {
			managedClusterAddon.Status.Conditions = append(managedClusterAddon.Status.Conditions, metav1.Condition{
				Type:               "Progressing",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.NewTime(time.Now()),
				Reason:             "ManifestWorkCreated",
				Message:            "Addon Installing",
			})
		} else {
			managedClusterAddon.Status.Conditions = []metav1.Condition{
				{
					Type:               "Progressing",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.NewTime(time.Now()),
					Reason:             "ManifestWorkCreated",
					Message:            "Addon Installing",
				},
			}
		}

		// update status for the created managedclusteraddon
		if err := c.Status().Update(ctx, managedClusterAddon); err != nil {
			return fmt.Errorf("failed to update status for managedclusteraddon: %w", err)
		}

		return nil
	})

	return managedClusterAddon, retryErr
}
