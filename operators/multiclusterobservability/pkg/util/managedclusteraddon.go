// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	progressingConditionType = "Progressing"
	availableConditionType   = "Available"
	degradedConditionType    = "Degraded"
)

var (
	log            = logf.Log.WithName("util")
	spokeNameSpace = os.Getenv("SPOKE_NAMESPACE")
)

func CreateManagedClusterAddonCR(ctx context.Context, c client.Client, namespace, labelKey, labelValue string) (*addonv1alpha1.ManagedClusterAddOn, error) {
	// check if managedClusterAddon exists
	managedClusterAddon := &addonv1alpha1.ManagedClusterAddOn{}
	objectKey := types.NamespacedName{
		Name:      config.ManagedClusterAddonName,
		Namespace: namespace,
	}
	err := c.Get(ctx, objectKey, managedClusterAddon)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// AddOn doesn't exist, create it
			addon, err := createManagedClusterAddOn(ctx, c, namespace, labelKey, labelValue)
			if err != nil {
				return nil, fmt.Errorf("failed to create managedclusteraddon for namespace %s: %w", namespace, err)
			}

			log.Info("Successfully created ManagedClusterAddOn.", "name", addon.Name, "namespace", addon.Namespace)
		} else {
			return nil, fmt.Errorf("failed to get the managedClusterAddon: %w", err)
		}
	}

	// Update its status
	if managedClusterAddon, err = updateManagedClusterAddOnStatus(ctx, c, namespace); err != nil {
		return nil, fmt.Errorf("failed to update the managedClusterAddon status: %w", err)
	}

	return managedClusterAddon, nil
}

func createManagedClusterAddOn(ctx context.Context, c client.Client, namespace, labelKey, labelValue string) (*addonv1alpha1.ManagedClusterAddOn, error) {
	newManagedClusterAddon := &addonv1alpha1.ManagedClusterAddOn{
		TypeMeta: metav1.TypeMeta{
			APIVersion: addonv1alpha1.SchemeGroupVersion.String(),
			Kind:       "ManagedClusterAddOn",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.ManagedClusterAddonName,
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
		return nil, err
	}

	return newManagedClusterAddon, nil
}

func updateManagedClusterAddOnStatus(ctx context.Context, c client.Client, namespace string) (*addonv1alpha1.ManagedClusterAddOn, error) {
	existingManagedClusterAddon := &addonv1alpha1.ManagedClusterAddOn{}
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		objectKey := types.NamespacedName{
			Name:      config.ManagedClusterAddonName,
			Namespace: namespace,
		}
		if err := c.Get(ctx, objectKey, existingManagedClusterAddon); err != nil {
			return fmt.Errorf("failed to get the managedclusteraddon for namespace %s: %w", namespace, err)
		}

		desiredStatus := existingManagedClusterAddon.Status.DeepCopy()

		// Ensure that the progressing condition exists. If not, add it as it may have just been created.
		isAvailable := meta.IsStatusConditionTrue(desiredStatus.Conditions, availableConditionType)
		isDegraded := meta.IsStatusConditionTrue(desiredStatus.Conditions, degradedConditionType)
		if meta.FindStatusCondition(desiredStatus.Conditions, progressingConditionType) == nil && !isAvailable && !isDegraded {
			newCondition := metav1.Condition{
				Type:               progressingConditionType,
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.NewTime(time.Now()),
				Reason:             "ManifestWorkCreated",
				Message:            "Addon Installing",
			}
			desiredStatus.Conditions = append(desiredStatus.Conditions, newCondition)
		}

		// got the created managedclusteraddon just now, updating its status

		desiredStatus.ConfigReferences = []addonv1alpha1.ConfigReference{
			{
				ConfigGroupResource: addonv1alpha1.ConfigGroupResource{
					Group:    "observability.open-cluster-management.io",
					Resource: "observabilityaddons",
				},
				ConfigReferent: addonv1alpha1.ConfigReferent{
					Namespace: namespace,
					Name:      "observability-addon",
				},
			},
		}
		desiredStatus.AddOnMeta = addonv1alpha1.AddOnMeta{
			DisplayName: "Observability Controller",
			Description: "Manages Observability components.",
		}

		if equality.Semantic.DeepEqual(existingManagedClusterAddon.Status, desiredStatus) {
			// nothing to do
			return nil
		}
		existingManagedClusterAddon.Status = *desiredStatus

		// update status for the created managedclusteraddon
		if err := c.Status().Update(ctx, existingManagedClusterAddon); err != nil {
			return fmt.Errorf("failed to update status for managedclusteraddon: %w", err)
		}

		log.Info("Successfully updated the ManagedClsuterAddOn status", "name", existingManagedClusterAddon.Name, "namespace", existingManagedClusterAddon.Namespace)
		return nil
	})

	if retryErr != nil {
		return nil, retryErr
	}

	return existingManagedClusterAddon, nil
}
