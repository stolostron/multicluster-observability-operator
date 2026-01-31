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
			_, err := createManagedClusterAddOn(ctx, c, namespace, labelKey, labelValue)
			if err != nil {
				return nil, fmt.Errorf("failed to create managedclusteraddon for namespace %s: %w", namespace, err)
			}

			// Initialize status for newly created MCA
			addon, err := updateManagedClusterAddOnStatus(ctx, c, namespace)
			if err != nil {
				return nil, fmt.Errorf("failed to initialize the managedClusterAddon status: %w", err)
			}

			log.Info("Successfully created and initialized ManagedClusterAddOn.", "name", addon.Name, "namespace", addon.Namespace)
			return addon, nil
		}
		return nil, fmt.Errorf("failed to get the managedClusterAddon: %w", err)
	}

	// MCA already exists - check if ConfigReferences need initialization.
	// This handles the case where CMA's defaultConfig was added after MCA creation.
	// We check first to avoid race conditions: calling updateManagedClusterAddOnStatus unconditionally
	// would cause MCO to fetch MCA while addon-framework is updating its status, creating conflicts.
	needsConfigInit, err := needsConfigReferencesInitialization(ctx, c, managedClusterAddon)
	if err != nil {
		return nil, fmt.Errorf("failed to check if config initialization needed: %w", err)
	}

	if needsConfigInit {
		// Initialize ConfigReferences - CMA now has defaultConfig but MCA doesn't have ConfigReferences
		if managedClusterAddon, err = updateManagedClusterAddOnStatus(ctx, c, namespace); err != nil {
			return nil, fmt.Errorf("failed to initialize ConfigReferences in managedClusterAddon status: %w", err)
		}
		log.Info("Initialized ConfigReferences for existing ManagedClusterAddOn", "name", managedClusterAddon.Name, "namespace", managedClusterAddon.Namespace)
	}

	// Return the MCA - addon-framework manages status updates after initialization
	return managedClusterAddon, nil
}

// needsConfigReferencesInitialization checks if MCA's ConfigReferences need initialization or update.
// Returns true if:
//  1. MCA has no Spec.Configs (should inherit from CMA defaultConfig) AND
//  2. CMA has a defaultConfig for AddonDeploymentConfig AND
//  3. Either:
//     a. MCA's ConfigReferences don't have an entry for AddonDeploymentConfig (needs initialization)
//     b. OR the existing ConfigReferent doesn't match CMA's defaultConfig (needs update)
func needsConfigReferencesInitialization(ctx context.Context, c client.Client, mca *addonv1alpha1.ManagedClusterAddOn) (bool, error) {
	// If MCA has explicit Spec.Configs, it's not using CMA defaultConfig
	if len(mca.Spec.Configs) > 0 {
		return false, nil
	}

	// Get CMA to check for defaultConfig
	cma := &addonv1alpha1.ClusterManagementAddOn{}
	if err := c.Get(ctx, types.NamespacedName{Name: ObservabilityController}, cma); err != nil {
		if apierrors.IsNotFound(err) {
			// CMA doesn't exist, no initialization needed
			return false, nil
		}
		return false, fmt.Errorf("failed to get ClusterManagementAddOn: %w", err)
	}

	// Find CMA's defaultConfig for AddonDeploymentConfig
	var cmaDefaultConfig *addonv1alpha1.ConfigReferent
	for _, supportedConfig := range cma.Spec.SupportedConfigs {
		if supportedConfig.Group == AddonGroup &&
			supportedConfig.Resource == AddonDeploymentConfigResource &&
			supportedConfig.DefaultConfig != nil {
			cmaDefaultConfig = supportedConfig.DefaultConfig
			break
		}
	}

	// If CMA has no defaultConfig, no initialization needed
	if cmaDefaultConfig == nil {
		return false, nil
	}

	// Check if MCA has ConfigReferences for AddonDeploymentConfig
	for _, configRef := range mca.Status.ConfigReferences {
		if configRef.Group == AddonGroup &&
			configRef.Resource == AddonDeploymentConfigResource {
			// ConfigReferences entry exists - check if ConfigReferent matches CMA's defaultConfig
			if configRef.DesiredConfig == nil {
				// DesiredConfig is nil, needs initialization
				log.Info("MCA ConfigReferences DesiredConfig is nil, needs initialization",
					"name", mca.Name, "namespace", mca.Namespace)
				return true, nil
			}
			// Check if ConfigReferent matches
			if configRef.DesiredConfig.Name != cmaDefaultConfig.Name ||
				configRef.DesiredConfig.Namespace != cmaDefaultConfig.Namespace {
				// ConfigReferent doesn't match CMA's defaultConfig - needs update
				log.Info("MCA ConfigReferences ConfigReferent doesn't match CMA defaultConfig, needs update",
					"name", mca.Name, "namespace", mca.Namespace,
					"currentConfigReferent", configRef.DesiredConfig.Name+"/"+configRef.DesiredConfig.Namespace,
					"cmaDefaultConfig", cmaDefaultConfig.Name+"/"+cmaDefaultConfig.Namespace)
				return true, nil
			}
			// ConfigReferent matches - no update needed
			return false, nil
		}
	}

	// No ConfigReferences entry exists - needs initialization
	log.Info("MCA has no ConfigReferences entry for AddonDeploymentConfig, needs initialization",
		"name", mca.Name, "namespace", mca.Namespace)
	return true, nil
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

		desiredStatus.AddOnMeta = addonv1alpha1.AddOnMeta{
			DisplayName: "Observability Controller",
			Description: "Manages Observability components.",
		}

		// Initialize ConfigReferences if the MCA should inherit from CMA defaultConfig
		// This is required for the addon-framework to populate specHash
		if len(existingManagedClusterAddon.Spec.Configs) == 0 {
			// Check if CMA exists before trying to initialize
			cma := &addonv1alpha1.ClusterManagementAddOn{}
			if err := c.Get(ctx, types.NamespacedName{Name: ObservabilityController}, cma); err == nil {
				// CMA exists, initialize MCA ConfigReferences from CMA defaultConfig
				initializeConfigReferencesFromCMA(cma, desiredStatus)
			} else if !apierrors.IsNotFound(err) {
				// Real error (not just NotFound), should retry
				return fmt.Errorf("failed to get ClusterManagementAddOn: %w", err)
			}
			// If NotFound, just skip initialization - nothing to inherit from
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

// initializeConfigReferencesFromCMA initializes/updates MCA status.ConfigReferences based on CMA's defaultConfig.
// This is needed because the addon-framework only updates specHash and lastObservedGeneration fields.
// The addon-framework does NOT update ConfigReferent fields - that's the addon implementation's responsibility.
// This function:
// - Creates new ConfigReferences entries if they don't exist
// - Updates ConfigReferent fields when CMA's defaultConfig changes (name/namespace)
// - Never modifies SpecHash or LastObservedGeneration (owned by addon-framework)
// The caller is responsible for checking if CMA exists before calling this function.
func initializeConfigReferencesFromCMA(cma *addonv1alpha1.ClusterManagementAddOn, mcaStatus *addonv1alpha1.ManagedClusterAddOnStatus) {
	// Find the defaultConfig for addondeploymentconfigs
	for _, supportedConfig := range cma.Spec.SupportedConfigs {
		if supportedConfig.Group == AddonGroup &&
			supportedConfig.Resource == AddonDeploymentConfigResource &&
			supportedConfig.DefaultConfig != nil {
			// Check if ConfigReferences already has this entry
			found := false
			for i := range mcaStatus.ConfigReferences {
				if mcaStatus.ConfigReferences[i].Group == AddonGroup &&
					mcaStatus.ConfigReferences[i].Resource == AddonDeploymentConfigResource {
					found = true
					// Update ConfigReferent fields only if they changed
					if mcaStatus.ConfigReferences[i].Name != supportedConfig.DefaultConfig.Name ||
						mcaStatus.ConfigReferences[i].Namespace != supportedConfig.DefaultConfig.Namespace {
						log.Info("Updating top-level ConfigReferent",
							"oldConfigReferent", mcaStatus.ConfigReferences[i].Name+"/"+mcaStatus.ConfigReferences[i].Namespace,
							"newConfigReferent", supportedConfig.DefaultConfig.Name+"/"+supportedConfig.DefaultConfig.Namespace)
						mcaStatus.ConfigReferences[i].Name = supportedConfig.DefaultConfig.Name
						mcaStatus.ConfigReferences[i].Namespace = supportedConfig.DefaultConfig.Namespace
					}
					if mcaStatus.ConfigReferences[i].DesiredConfig == nil {
						log.Info("DesiredConfig is nil, initializing it")
						mcaStatus.ConfigReferences[i].DesiredConfig = &addonv1alpha1.ConfigSpecHash{}
					}
					// Update DesiredConfig.ConfigReferent only if it changed
					// This tells addon-framework which config to calculate SpecHash from
					if mcaStatus.ConfigReferences[i].DesiredConfig.Name != supportedConfig.DefaultConfig.Name ||
						mcaStatus.ConfigReferences[i].DesiredConfig.Namespace != supportedConfig.DefaultConfig.Namespace {
						log.Info("Updating DesiredConfig.ConfigReferent",
							"oldDesiredConfigReferent", mcaStatus.ConfigReferences[i].DesiredConfig.Name+"/"+mcaStatus.ConfigReferences[i].DesiredConfig.Namespace,
							"newDesiredConfigReferent", supportedConfig.DefaultConfig.Name+"/"+supportedConfig.DefaultConfig.Namespace,
							"existingSpecHash", mcaStatus.ConfigReferences[i].DesiredConfig.SpecHash)
						mcaStatus.ConfigReferences[i].DesiredConfig.ConfigReferent = *supportedConfig.DefaultConfig
					}
					// Note: SpecHash and LastObservedGeneration are managed by addon-framework
					break
				}
			}

			// If not found, create a new entry
			if !found {
				newConfigRef := addonv1alpha1.ConfigReference{
					ConfigGroupResource: supportedConfig.ConfigGroupResource,
					ConfigReferent:      *supportedConfig.DefaultConfig,
					DesiredConfig: &addonv1alpha1.ConfigSpecHash{
						ConfigReferent: *supportedConfig.DefaultConfig,
						// SpecHash will be filled by addon-framework addonconfig controller
					},
					// LastObservedGeneration will be filled by addon-framework addonconfig controller
				}
				mcaStatus.ConfigReferences = append(mcaStatus.ConfigReferences, newConfigRef)
				log.Info("Initialized ConfigReferences from CMA defaultConfig",
					"config", supportedConfig.DefaultConfig.Name,
					"namespace", supportedConfig.DefaultConfig.Namespace)
			}
		}
	}
}
