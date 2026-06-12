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
	addonframeworkutils "open-cluster-management.io/addon-framework/pkg/utils"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	addonv1beta1 "open-cluster-management.io/api/addon/v1beta1"
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
		} else {
			return nil, fmt.Errorf("failed to get the managedClusterAddon: %w", err)
		}
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
//     c. OR the existing SpecHash is empty or stale (needs refresh)
func needsConfigReferencesInitialization(ctx context.Context, c client.Client, mca *addonv1alpha1.ManagedClusterAddOn) (bool, error) {
	// If MCA has explicit Spec.Configs, it's not using CMA defaultConfig
	if len(mca.Spec.Configs) > 0 {
		return false, nil
	}

	// Get CMA to check for defaultConfig
	cma := &addonv1beta1.ClusterManagementAddOn{}
	if err := c.Get(ctx, types.NamespacedName{Name: ObservabilityController}, cma); err != nil {
		if apierrors.IsNotFound(err) {
			// CMA doesn't exist, no initialization needed
			return false, nil
		}
		return false, fmt.Errorf("failed to get ClusterManagementAddOn: %w", err)
	}

	// Find CMA's defaultConfig for AddonDeploymentConfig
	var cmaDefaultConfig *addonv1beta1.ConfigReferent
	for _, config := range cma.Spec.DefaultConfigs {
		if config.Group == AddonGroup &&
			config.Resource == AddonDeploymentConfigResource {
			cmaDefaultConfig = &config.ConfigReferent
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
				log.Info("MCA ConfigReferences ConfigReferent doesn't match CMA defaultConfig, needs update",
					"name", mca.Name, "namespace", mca.Namespace,
					"currentConfigReferent", configRef.DesiredConfig.Name+"/"+configRef.DesiredConfig.Namespace,
					"cmaDefaultConfig", cmaDefaultConfig.Name+"/"+cmaDefaultConfig.Namespace)
				return true, nil
			}
			// ConfigReferent matches but SpecHash is empty — the addon-framework
			// only populates SpecHash for configs in Spec.Configs, not inherited defaults.
			if configRef.DesiredConfig.SpecHash == "" {
				log.Info("MCA ConfigReferences SpecHash is empty, needs initialization",
					"name", mca.Name, "namespace", mca.Namespace)
				return true, nil
			}
			currentSpecHash, err := computeADCSpecHash(ctx, c, cmaDefaultConfig.Namespace, cmaDefaultConfig.Name)
			if err != nil {
				return false, fmt.Errorf("failed to compute AddOnDeploymentConfig spec hash: %w", err)
			}
			if configRef.DesiredConfig.SpecHash != currentSpecHash {
				log.Info("MCA ConfigReferences SpecHash is stale, needs refresh",
					"name", mca.Name, "namespace", mca.Namespace,
					"storedSpecHash", configRef.DesiredConfig.SpecHash,
					"currentSpecHash", currentSpecHash)
				return true, nil
			}
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
			APIVersion: addonv1alpha1.GroupVersion.String(),
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
			cma := &addonv1beta1.ClusterManagementAddOn{}
			if err := c.Get(ctx, types.NamespacedName{Name: ObservabilityController}, cma); err == nil {
				if err := initializeConfigReferencesFromCMA(ctx, c, cma, desiredStatus); err != nil {
					return fmt.Errorf("failed to initialize ConfigReferences from CMA: %w", err)
				}
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
// This is needed because the addon-framework's addon-config-controller only populates SpecHash
// for configs explicitly listed in MCA.Spec.Configs. When configs are inherited from CMA's
// defaultConfig (no Spec.Configs), the framework never fills the SpecHash, causing the
// addon-registration-controller to fail with "deployment config desired spec hash is empty".
//
// This function:
// - Creates new ConfigReferences entries if they don't exist
// - Updates ConfigReferent fields when CMA's defaultConfig changes (name/namespace)
// - Computes and sets SpecHash by fetching the AddOnDeploymentConfig
func initializeConfigReferencesFromCMA(
	ctx context.Context,
	c client.Client,
	cma *addonv1beta1.ClusterManagementAddOn,
	mcaStatus *addonv1alpha1.ManagedClusterAddOnStatus,
) error {
	for _, supportedConfig := range cma.Spec.DefaultConfigs {
		if supportedConfig.Group != AddonGroup ||
			supportedConfig.Resource != AddonDeploymentConfigResource {
			continue
		}

		configReferentv1 := addonv1alpha1.ConfigReferent{
			Name:      supportedConfig.Name,
			Namespace: supportedConfig.Namespace,
		}

		cgrv1 := addonv1alpha1.ConfigGroupResource{
			Group:    supportedConfig.Group,
			Resource: supportedConfig.Resource,
		}

		specHash, err := computeADCSpecHash(ctx, c, supportedConfig.Namespace, supportedConfig.Name)
		if err != nil {
			return fmt.Errorf("failed to compute AddOnDeploymentConfig spec hash: %w", err)
		}

		found := false
		for i := range mcaStatus.ConfigReferences {
			if mcaStatus.ConfigReferences[i].Group != AddonGroup ||
				mcaStatus.ConfigReferences[i].Resource != AddonDeploymentConfigResource {
				continue
			}
			found = true

			if mcaStatus.ConfigReferences[i].Name != supportedConfig.Name ||
				mcaStatus.ConfigReferences[i].Namespace != supportedConfig.Namespace {
				log.Info("Updating top-level ConfigReferent",
					"oldConfigReferent", mcaStatus.ConfigReferences[i].Name+"/"+mcaStatus.ConfigReferences[i].Namespace,
					"newConfigReferent", supportedConfig.Name+"/"+supportedConfig.Namespace)
				mcaStatus.ConfigReferences[i].Name = supportedConfig.Name
				mcaStatus.ConfigReferences[i].Namespace = supportedConfig.Namespace
			}
			if mcaStatus.ConfigReferences[i].DesiredConfig == nil {
				log.Info("DesiredConfig is nil, initializing it")
				mcaStatus.ConfigReferences[i].DesiredConfig = &addonv1alpha1.ConfigSpecHash{}
			}

			if mcaStatus.ConfigReferences[i].DesiredConfig.Name != supportedConfig.Name ||
				mcaStatus.ConfigReferences[i].DesiredConfig.Namespace != supportedConfig.Namespace {
				log.Info("Updating DesiredConfig.ConfigReferent",
					"oldDesiredConfigReferent", mcaStatus.ConfigReferences[i].DesiredConfig.Name+"/"+mcaStatus.ConfigReferences[i].DesiredConfig.Namespace,
					"newDesiredConfigReferent", supportedConfig.Name+"/"+supportedConfig.Namespace,
					"existingSpecHash", mcaStatus.ConfigReferences[i].DesiredConfig.SpecHash)
				mcaStatus.ConfigReferences[i].DesiredConfig.ConfigReferent = configReferentv1
			}

			if mcaStatus.ConfigReferences[i].DesiredConfig.SpecHash != specHash {
				log.Info("Updating DesiredConfig.SpecHash",
					"oldSpecHash", mcaStatus.ConfigReferences[i].DesiredConfig.SpecHash,
					"newSpecHash", specHash)
				mcaStatus.ConfigReferences[i].DesiredConfig.SpecHash = specHash
			}
			break
		}

		if !found {
			newConfigRef := addonv1alpha1.ConfigReference{
				ConfigGroupResource: cgrv1,
				ConfigReferent:      configReferentv1,
				DesiredConfig: &addonv1alpha1.ConfigSpecHash{
					ConfigReferent: configReferentv1,
					SpecHash:       specHash,
				},
			}
			mcaStatus.ConfigReferences = append(mcaStatus.ConfigReferences, newConfigRef)
			log.Info("Initialized ConfigReferences from CMA defaultConfig",
				"config", supportedConfig.Name,
				"namespace", supportedConfig.Namespace,
				"specHash", specHash)
		}
	}
	return nil
}

// computeADCSpecHash fetches the AddOnDeploymentConfig and computes its spec hash using
// the same algorithm as the addon-framework to ensure consistency.
func computeADCSpecHash(ctx context.Context, c client.Client, namespace, name string) (string, error) {
	adc := &addonv1beta1.AddOnDeploymentConfig{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, adc); err != nil {
		return "", fmt.Errorf("failed to get AddOnDeploymentConfig %s/%s: %w", namespace, name, err)
	}
	return addonframeworkutils.GetAddOnDeploymentConfigSpecHash(adc)
}
