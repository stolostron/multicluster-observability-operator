// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
)

// retryOnConflict provides exponential backoff retry logic for update operations
// that may encounter HTTP 409 Conflict errors due to concurrent modifications.
// This is specifically for test utilities to handle race conditions during e2e testing.
func retryOnConflict(fn func() error) error {
	backoff := wait.Backoff{
		Steps:    5,                      // retry up to 5 times
		Duration: 100 * time.Millisecond, // initial delay
		Factor:   2.0,                    // exponential factor (100ms, 200ms, 400ms, 800ms, 1600ms)
		Jitter:   0.1,                    // add 10% jitter to avoid thundering herd
	}

	return wait.ExponentialBackoff(backoff, func() (bool, error) {
		err := fn()
		if err == nil {
			return true, nil // success
		}

		// Retry on conflict errors (HTTP 409)
		if errors.IsConflict(err) {
			klog.V(2).Infof("Conflict error encountered, retrying: %v", err)
			return false, nil // retry
		}

		// For other errors, fail immediately
		return false, err
	})
}

func CheckOBAStatus(opt TestOptions, namespace string) error {
	dynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	// Step 1: Check ManagedClusterAddOn for "Available" condition (set by addon-framework)
	mcaObj, err := dynClient.Resource(NewMCOManagedClusterAddonsGVR()).
		Namespace(namespace).
		Get(context.TODO(), "observability-controller", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get ManagedClusterAddOn: %w", err)
	}

	mca := &addonv1alpha1.ManagedClusterAddOn{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(mcaObj.Object, mca)
	if err != nil {
		return fmt.Errorf("failed to convert unstructured to ManagedClusterAddOn: %w", err)
	}

	if !meta.IsStatusConditionTrue(mca.Status.Conditions, "Available") {
		return fmt.Errorf("ManagedClusterAddOn observability-controller is not available in %s, conditions: %+v", namespace, mca.Status.Conditions)
	}

	// Step 2: Check ObservabilityAddon for "MetricsCollector" condition (set by MCOA status controller)
	obaObj, err := dynClient.Resource(NewMCOAddonGVR()).
		Namespace(namespace).
		Get(context.TODO(), "observability-addon", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get ObservabilityAddon: %w", err)
	}

	// ObservabilityAddon uses custom StatusCondition type, but FromUnstructured handles it
	oba := &addonv1alpha1.ManagedClusterAddOn{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(obaObj.Object, oba)
	if err != nil {
		return fmt.Errorf("failed to convert unstructured to ObservabilityAddon: %w", err)
	}

	if !meta.IsStatusConditionTrue(oba.Status.Conditions, "MetricsCollector") {
		return fmt.Errorf("ObservabilityAddon MetricsCollector is not ready for managed cluster %q, conditions: %+v", namespace, oba.Status.Conditions)
	}

	return nil
}

func CheckOBADeleted(opt TestOptions, cluster ClustersInfo) error {
	klog.V(1).Infof("Checking observability-addon deleted for managed cluster %s", cluster.Name)
	dynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	_, err := dynClient.Resource(NewMCOAddonGVR()).Namespace(cluster.Name).Get(context.TODO(), "observability-addon", metav1.GetOptions{})
	if err == nil {
		klog.Errorf("observability-addon still exists for managed cluster %s", cluster.Name)
		return fmt.Errorf("observability-addon still exists for managed cluster %s", cluster.Name)
	}
	if !errors.IsNotFound(err) {
		klog.Errorf("failed to get observability-addon for managed cluster %s: %v", cluster.Name, err)
		return fmt.Errorf("failed to get observability-addon for managed cluster %s: %w", cluster.Name, err)
	}
	return nil
}

func CheckAllOBAsEnabled(opt TestOptions) error {
	clusters, err := ListManagedClusters(opt)
	if err != nil {
		return err
	}
	klog.V(1).Infof("Check OBA status for managedclusters: %v", clusters)

	for _, cluster := range clusters {
		// skip the check for local-cluster
		if cluster.IsLocalCluster {
			klog.V(1).Infof("Skip OBA status for managedcluster: %v", cluster.Name)
			continue
		}
		err = CheckOBAStatus(opt, cluster.Name)
		if err != nil {
			klog.V(1).Infof("Error checking OBA status for cluster %q: %v", cluster.Name, err)
			return err
		}
	}
	return nil
}

func CheckAllOBAsDeleted(opt TestOptions) error {
	clusters, err := ListManagedClusters(opt)
	if err != nil {
		return err
	}
	for _, cluster := range clusters {
		// skip the check for local-cluster
		if cluster.Name == "local-cluster" {
			klog.V(1).Infof("Skip OBA status for managedcluster: %v", cluster)
			continue
		}
		err = CheckOBADeleted(opt, cluster)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetManagedClusterAddOnSpecHash retrieves the specHash from ManagedClusterAddOn status.
func GetManagedClusterAddOnSpecHash(opt TestOptions, namespace string) (string, error) {
	dynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	mca, err := dynClient.Resource(NewMCOManagedClusterAddonsGVR()).
		Namespace(namespace).
		Get(context.TODO(), "observability-controller", metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get managedclusteraddon: %w", err)
	}

	addon := &addonv1alpha1.ManagedClusterAddOn{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(mca.Object, addon)
	if err != nil {
		return "", fmt.Errorf("failed to convert unstructured to addon: %w", err)
	}

	for _, configRef := range addon.Status.ConfigReferences {
		if configRef.Group == "addon.open-cluster-management.io" &&
			configRef.Resource == "addondeploymentconfigs" {
			if configRef.DesiredConfig != nil {
				return configRef.DesiredConfig.SpecHash, nil
			}
		}
	}

	return "", fmt.Errorf("AddOnDeploymentConfig specHash not found in ManagedClusterAddOn status")
}

// GetClusterManagementAddOnSpecHash retrieves the specHash from ClusterManagementAddOn status.
func GetClusterManagementAddOnSpecHash(opt TestOptions) (string, error) {
	dynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	cma, err := dynClient.Resource(NewMCOClusterManagementAddonsGVR()).
		Get(context.TODO(), "observability-controller", metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get clustermanagementaddon: %w", err)
	}

	addon := &addonv1alpha1.ClusterManagementAddOn{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(cma.Object, addon)
	if err != nil {
		return "", fmt.Errorf("failed to convert unstructured to addon: %w", err)
	}

	for _, configRef := range addon.Status.DefaultConfigReferences {
		if configRef.Group == "addon.open-cluster-management.io" &&
			configRef.Resource == "addondeploymentconfigs" {
			if configRef.DesiredConfig != nil {
				return configRef.DesiredConfig.SpecHash, nil
			}
		}
	}

	return "", fmt.Errorf("AddOnDeploymentConfig specHash not found in ClusterManagementAddOn status")
}

// ValidateAddOnDeploymentConfigSpecHash validates specHash is set in both CMA and MCA.
func ValidateAddOnDeploymentConfigSpecHash(opt TestOptions, namespace string) error {
	cmaSpecHash, err := GetClusterManagementAddOnSpecHash(opt)
	if err != nil {
		return fmt.Errorf("failed to get CMA specHash: %w", err)
	}

	if cmaSpecHash == "" {
		return fmt.Errorf("CMA specHash is empty")
	}

	mcaSpecHash, err := GetManagedClusterAddOnSpecHash(opt, namespace)
	if err != nil {
		return fmt.Errorf("failed to get MCA specHash for cluster %s: %w", namespace, err)
	}

	if mcaSpecHash == "" {
		return fmt.Errorf("MCA specHash is empty for cluster %s", namespace)
	}

	klog.V(1).Infof("SpecHash validation successful - CMA: %s, MCA (%s): %s", cmaSpecHash, namespace, mcaSpecHash)
	return nil
}

// GetAddOnDeploymentConfig retrieves an AddOnDeploymentConfig.
func GetAddOnDeploymentConfig(
	opt TestOptions,
	name, namespace string,
) (map[string]any, error) {
	dynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	gvr := NewMCOAddOnDeploymentConfigGVR()
	config, err := dynClient.Resource(gvr).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get AddOnDeploymentConfig: %w", err)
	}

	return config.Object, nil
}

// UpdateAddOnDeploymentConfig updates an AddOnDeploymentConfig via callback.
func UpdateAddOnDeploymentConfig(
	opt TestOptions,
	name, namespace string,
	updateFn func(map[string]any),
) error {
	dynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	gvr := NewMCOAddOnDeploymentConfigGVR()
	config, err := dynClient.Resource(gvr).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get AddOnDeploymentConfig: %w", err)
	}

	updateFn(config.Object)

	_, err = dynClient.Resource(gvr).Namespace(namespace).Update(context.TODO(), config, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update AddOnDeploymentConfig: %w", err)
	}

	return nil
}

// ValidateSpecHashChanged validates specHash changed from original.
func ValidateSpecHashChanged(
	opt TestOptions,
	namespace, originalCMAHash, originalMCAHash string,
) error {
	newCMAHash, err := GetClusterManagementAddOnSpecHash(opt)
	if err != nil {
		return fmt.Errorf("failed to get new CMA specHash: %w", err)
	}

	if newCMAHash == originalCMAHash {
		return fmt.Errorf("CMA specHash unchanged: %s", newCMAHash)
	}

	newMCAHash, err := GetManagedClusterAddOnSpecHash(opt, namespace)
	if err != nil {
		return fmt.Errorf("failed to get new MCA specHash: %w", err)
	}

	if newMCAHash == originalMCAHash {
		return fmt.Errorf("MCA specHash unchanged: %s", newMCAHash)
	}

	klog.V(1).Infof("SpecHash changed - CMA: %s -> %s, MCA: %s -> %s", originalCMAHash, newCMAHash, originalMCAHash, newMCAHash)
	return nil
}

// ValidateSpecHashDifferent validates CMA and MCA have different specHash.
func ValidateSpecHashDifferent(opt TestOptions, namespace string) error {
	cmaHash, err := GetClusterManagementAddOnSpecHash(opt)
	if err != nil {
		return fmt.Errorf("failed to get CMA specHash: %w", err)
	}

	mcaHash, err := GetManagedClusterAddOnSpecHash(opt, namespace)
	if err != nil {
		return fmt.Errorf("failed to get MCA specHash: %w", err)
	}

	if cmaHash == mcaHash {
		return fmt.Errorf("CMA and MCA specHash are identical: %s (expected different values for per-cluster override)", cmaHash)
	}

	klog.V(1).Infof("SpecHash correctly different - CMA: %s, MCA: %s", cmaHash, mcaHash)
	return nil
}

// CreateClusterSpecificAddOnDeploymentConfig creates a per-cluster AddOnDeploymentConfig.
func CreateClusterSpecificAddOnDeploymentConfig(
	opt TestOptions,
	name, namespace string,
	config map[string]any,
) error {
	dynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	gvr := NewMCOAddOnDeploymentConfigGVR()
	configObj := &unstructured.Unstructured{
		Object: config,
	}
	configObj.SetAPIVersion("addon.open-cluster-management.io/v1alpha1")
	configObj.SetKind("AddOnDeploymentConfig")
	configObj.SetName(name)
	configObj.SetNamespace(namespace)

	_, err := dynClient.Resource(gvr).Namespace(namespace).Create(context.TODO(), configObj, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create AddOnDeploymentConfig: %w", err)
	}

	klog.V(1).Infof("Created cluster-specific AddOnDeploymentConfig %s/%s", namespace, name)
	return nil
}

// DeleteAddOnDeploymentConfig deletes an AddOnDeploymentConfig.
func DeleteAddOnDeploymentConfig(opt TestOptions, name, namespace string) error {
	dynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	gvr := NewMCOAddOnDeploymentConfigGVR()
	err := dynClient.Resource(gvr).Namespace(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete AddOnDeploymentConfig: %w", err)
	}

	klog.V(1).Infof("Deleted AddOnDeploymentConfig %s/%s", namespace, name)
	return nil
}

// UpdateManagedClusterAddOnConfig updates ManagedClusterAddOn spec.configs.
// Uses exponential backoff retry to handle concurrent modification conflicts.
func UpdateManagedClusterAddOnConfig(
	opt TestOptions,
	namespace, configName, configNamespace string,
) error {
	dynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	gvr := NewMCOManagedClusterAddonsGVR()

	return retryOnConflict(func() error {
		mca, err := dynClient.Resource(gvr).Namespace(namespace).Get(context.TODO(), "observability-controller", metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get ManagedClusterAddOn: %w", err)
		}

		specInterface, exists := mca.Object["spec"]
		if !exists || specInterface == nil {
			return fmt.Errorf("ManagedClusterAddOn spec not found")
		}

		spec, ok := specInterface.(map[string]any)
		if !ok {
			return fmt.Errorf("ManagedClusterAddOn spec is not a map")
		}

		spec["configs"] = []any{
			map[string]any{
				"group":     "addon.open-cluster-management.io",
				"resource":  "addondeploymentconfigs",
				"name":      configName,
				"namespace": configNamespace,
			},
		}

		_, err = dynClient.Resource(gvr).Namespace(namespace).Update(context.TODO(), mca, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update ManagedClusterAddOn spec: %w", err)
		}

		klog.V(1).Infof("Updated ManagedClusterAddOn %s to use config %s/%s (framework will create status)", namespace, configNamespace, configName)
		return nil
	})
}

// UpdateClusterManagementAddOnDefaultConfig updates CMA spec.supportedConfigs[].defaultConfig.
// Uses exponential backoff retry to handle concurrent modification conflicts.
func UpdateClusterManagementAddOnDefaultConfig(
	opt TestOptions,
	configName, configNamespace string,
) error {
	dynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	gvr := NewMCOClusterManagementAddonsGVR()

	return retryOnConflict(func() error {
		cma, err := dynClient.Resource(gvr).Get(context.TODO(), "observability-controller", metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get ClusterManagementAddOn: %w", err)
		}

		specInterface, exists := cma.Object["spec"]
		if !exists || specInterface == nil {
			return fmt.Errorf("ClusterManagementAddOn spec not found")
		}

		spec, ok := specInterface.(map[string]any)
		if !ok {
			return fmt.Errorf("ClusterManagementAddOn spec is not a map")
		}

		supportedConfigsInterface, exists := spec["supportedConfigs"]
		if !exists || supportedConfigsInterface == nil {
			return fmt.Errorf("supportedConfigs not found in ClusterManagementAddOn spec")
		}

		supportedConfigs, ok := supportedConfigsInterface.([]any)
		if !ok {
			return fmt.Errorf("supportedConfigs is not an array")
		}

		// Find the addondeploymentconfigs entry and update its defaultConfig
		for i, cfg := range supportedConfigs {
			cfgMap, ok := cfg.(map[string]any)
			if !ok {
				continue
			}

			group, hasGroup := cfgMap["group"]
			resource, hasResource := cfgMap["resource"]

			if hasGroup && hasResource &&
				group == "addon.open-cluster-management.io" &&
				resource == "addondeploymentconfigs" {
				supportedConfigs[i].(map[string]any)["defaultConfig"] = map[string]any{
					"name":      configName,
					"namespace": configNamespace,
				}
				break
			}
		}

		_, err = dynClient.Resource(gvr).Update(context.TODO(), cma, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update ClusterManagementAddOn spec: %w", err)
		}

		klog.V(1).Infof("Updated ClusterManagementAddOn defaultConfig to %s/%s (framework will create status)", configNamespace, configName)
		return nil
	})
}

// ClearClusterManagementAddOnDefaultConfig clears CMA spec.supportedConfigs[].defaultConfig.
// Uses exponential backoff retry to handle concurrent modification conflicts.
func ClearClusterManagementAddOnDefaultConfig(opt TestOptions) error {
	dynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	gvr := NewMCOClusterManagementAddonsGVR()

	return retryOnConflict(func() error {
		cma, err := dynClient.Resource(gvr).Get(context.TODO(), "observability-controller", metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get ClusterManagementAddOn: %w", err)
		}

		specInterface, exists := cma.Object["spec"]
		if !exists || specInterface == nil {
			return fmt.Errorf("ClusterManagementAddOn spec not found")
		}

		spec, ok := specInterface.(map[string]any)
		if !ok {
			return fmt.Errorf("ClusterManagementAddOn spec is not a map")
		}

		supportedConfigsInterface, exists := spec["supportedConfigs"]
		if !exists || supportedConfigsInterface == nil {
			return fmt.Errorf("supportedConfigs not found in ClusterManagementAddOn spec")
		}

		supportedConfigs, ok := supportedConfigsInterface.([]any)
		if !ok {
			return fmt.Errorf("supportedConfigs is not an array")
		}

		// Find the addondeploymentconfigs entry and remove its defaultConfig
		for i, cfg := range supportedConfigs {
			cfgMap, ok := cfg.(map[string]any)
			if !ok {
				continue
			}

			group, hasGroup := cfgMap["group"]
			resource, hasResource := cfgMap["resource"]

			if hasGroup && hasResource &&
				group == "addon.open-cluster-management.io" &&
				resource == "addondeploymentconfigs" {
				delete(supportedConfigs[i].(map[string]any), "defaultConfig")
				break
			}
		}

		_, err = dynClient.Resource(gvr).Update(context.TODO(), cma, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update ClusterManagementAddOn spec: %w", err)
		}

		klog.V(1).Info("Cleared ClusterManagementAddOn defaultConfig")
		return nil
	})
}

// ClearManagedClusterAddOnConfig clears ManagedClusterAddOn spec.configs.
// Uses exponential backoff retry to handle concurrent modification conflicts.
func ClearManagedClusterAddOnConfig(opt TestOptions, namespace string) error {
	dynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	gvr := NewMCOManagedClusterAddonsGVR()

	return retryOnConflict(func() error {
		mca, err := dynClient.Resource(gvr).Namespace(namespace).Get(context.TODO(), "observability-controller", metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get ManagedClusterAddOn: %w", err)
		}

		specInterface, exists := mca.Object["spec"]
		if !exists || specInterface == nil {
			return fmt.Errorf("ManagedClusterAddOn spec not found")
		}

		spec, ok := specInterface.(map[string]any)
		if !ok {
			return fmt.Errorf("ManagedClusterAddOn spec is not a map")
		}

		delete(spec, "configs")

		_, err = dynClient.Resource(gvr).Namespace(namespace).Update(context.TODO(), mca, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update ManagedClusterAddOn spec: %w", err)
		}

		klog.V(1).Infof("Cleared ManagedClusterAddOn %s configs", namespace)
		return nil
	})
}
