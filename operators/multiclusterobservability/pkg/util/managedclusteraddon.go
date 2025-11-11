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
		//nolint:staticcheck
		desiredStatus.AddOnConfiguration = addonv1alpha1.ConfigCoordinates{
			CRDName: "observabilityaddons.observability.open-cluster-management.io",
			CRName:  "observability-addon",
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

// UpdateManagedClusterAddOnSpecHash updates the ManagedClusterAddOn status spec hash for a given AddOnDeploymentConfig.
// Since this operator doesn't use addon-framework's WithConfigGVRs, we manually manage both spec.configs and status.configReferences.
// Errors are returned to let the controller retry the reconcile.
func UpdateManagedClusterAddOnSpecHash(ctx context.Context, c client.Client, namespace string, addonConfig *addonv1alpha1.AddOnDeploymentConfig) error {
	if addonConfig == nil {
		return nil
	}

	// Compute hash using the shared utility
	specHash, err := CalculateAddOnDeploymentConfigSpecHash(addonConfig)
	if err != nil {
		return fmt.Errorf("failed to calculate spec hash: %w", err)
	}
	if specHash == "" {
		return nil
	}

	// Get the ManagedClusterAddOn
	mca := &addonv1alpha1.ManagedClusterAddOn{}
	if err := c.Get(ctx, types.NamespacedName{Name: config.ManagedClusterAddonName, Namespace: namespace}, mca); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("managedclusteraddon %s/%s not found - ensure it exists before updating spec hash", namespace, config.ManagedClusterAddonName)
		}
		return fmt.Errorf("failed to get managedclusteraddon %s/%s: %w", namespace, config.ManagedClusterAddonName, err)
	}

	// Ensure status.configReferences exists and has an entry for AddOnDeploymentConfig
	// This mimics what addon-framework's addonconfig controller does
	found := false
	for i, cfgRef := range mca.Status.ConfigReferences {
		if cfgRef.ConfigGroupResource.Group == AddonGroup && cfgRef.ConfigGroupResource.Resource == AddonDeploymentConfigResource {
			if mca.Status.ConfigReferences[i].DesiredConfig == nil {
				mca.Status.ConfigReferences[i].DesiredConfig = &addonv1alpha1.ConfigSpecHash{}
			}
			mca.Status.ConfigReferences[i].DesiredConfig.SpecHash = specHash
			// Also update the config referent to match the actual config being used
			mca.Status.ConfigReferences[i].ConfigReferent = addonv1alpha1.ConfigReferent{
				Name:      addonConfig.Name,
				Namespace: addonConfig.Namespace,
			}
			found = true
			break
		}
	}

	// If no matching ConfigReference exists, create one
	// This happens when the MCA doesn't have spec.configs set (using default config)
	if !found {
		newConfigRef := addonv1alpha1.ConfigReference{
			ConfigGroupResource: addonv1alpha1.ConfigGroupResource{
				Group:    AddonGroup,
				Resource: AddonDeploymentConfigResource,
			},
			ConfigReferent: addonv1alpha1.ConfigReferent{
				Name:      addonConfig.Name,
				Namespace: addonConfig.Namespace,
			},
			DesiredConfig: &addonv1alpha1.ConfigSpecHash{
				SpecHash: specHash,
			},
		}
		mca.Status.ConfigReferences = append(mca.Status.ConfigReferences, newConfigRef)
		log.Info("Created new ConfigReference in ManagedClusterAddOn status", "namespace", namespace)
	}

	if err := c.Status().Update(ctx, mca); err != nil {
		return fmt.Errorf("failed to update managedclusteraddon status: %w", err)
	}
	log.Info("Updated ManagedClusterAddOn status with spec hash", "namespace", namespace, "hash", specHash, "config", addonConfig.Namespace+"/"+addonConfig.Name)
	return nil
}
