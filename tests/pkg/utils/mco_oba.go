// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"crypto/md5" // #nosec G401 G501 - Not used for cryptographic purposes
	"encoding/hex"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/yaml"
)

const (
	OBMAddonEnabledMessage             = "Cluster metrics sent successfully"
	ObservabilityController            = "observability-controller"
	AddonGroup                         = "addon.open-cluster-management.io"
	AddonDeploymentConfigResource      = "addondeploymentconfigs"
	DefaultConfigReferencesStatusField = "defaultConfigReferences"
	DesiredConfigField                 = "desiredConfig"
	SpecHashField                      = "specHash"
)

func CheckOBAStatus(opt TestOptions, namespace, status string) error {
	dynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	oba, err := dynClient.Resource(NewMCOAddonGVR()).
		Namespace(namespace).
		Get(context.TODO(), "observability-addon", metav1.GetOptions{})
	if err != nil {
		return err
	}

	addon := &addonv1alpha1.ManagedClusterAddOn{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(oba.Object, addon)
	if err != nil {
		return fmt.Errorf("failed to convert unstructured to addon: %w", err)
	}

	if meta.IsStatusConditionTrue(addon.Status.Conditions, "Available") {
		return nil
	}

	return fmt.Errorf("observability-addon is not ready for managed cluster %q, conditions: %+v", namespace, addon.Status.Conditions)
}

func CheckOBADeleted(opt TestOptions, cluster ClustersInfo) error {
	dynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	_, err := dynClient.Resource(NewMCOAddonGVR()).Namespace(cluster.Name).Get(context.TODO(), "observability-addon", metav1.GetOptions{})
	if err == nil || !errors.IsNotFound(err) {
		return fmt.Errorf("observability-addon is not properly deleted for managed cluster %s", cluster.Name)
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
		if cluster.isLocalCluster {
			klog.V(1).Infof("Skip OBA status for managedcluster: %v", cluster.Name)
			continue
		}
		err = CheckOBAStatus(opt, cluster.Name, "")
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

// GetClusterManagementAddOn retrieves the observability-controller ClusterManagementAddOn
func GetClusterManagementAddOn(opt TestOptions) (*unstructured.Unstructured, error) {
	dynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	cma, err := dynClient.Resource(NewMCOClusterManagementAddonsGVR()).
		Get(context.TODO(), ObservabilityController, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get clustermanagementaddon %s: %w", ObservabilityController, err)
	}

	return cma, nil
}

// GetAddOnDeploymentConfigForCluster retrieves the AddOnDeploymentConfig for a managed cluster
func GetAddOnDeploymentConfigForCluster(opt TestOptions, clusterName string) (*unstructured.Unstructured, error) {
	dynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	// First get the ManagedClusterAddOn to find which AddOnDeploymentConfig it references
	mca, err := dynClient.Resource(NewMCOManagedClusterAddonsGVR()).
		Namespace(clusterName).
		Get(context.TODO(), ObservabilityController, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get managedclusteraddon for cluster %s: %w", clusterName, err)
	}

	// Look for AddOnDeploymentConfig in spec.configs
	spec, found, err := unstructured.NestedMap(mca.Object, "spec")
	if err != nil || !found {
		return nil, fmt.Errorf("failed to get spec from managedclusteraddon: %w", err)
	}

	configs, found, err := unstructured.NestedSlice(spec, "configs")
	if err != nil || !found {
		// No configs specified, will use default
		klog.V(1).Infof("No configs found in managedclusteraddon for cluster %s", clusterName)
		return nil, nil
	}

	// Find the AddOnDeploymentConfig reference
	for _, config := range configs {
		configMap, ok := config.(map[string]interface{})
		if !ok {
			continue
		}

		cgr, found, err := unstructured.NestedMap(configMap, "configGroupResource")
		if err != nil || !found {
			continue
		}

		group, _, _ := unstructured.NestedString(cgr, "group")
		resource, _, _ := unstructured.NestedString(cgr, "resource")

		if group == AddonGroup && resource == AddonDeploymentConfigResource {
			// Found the AddOnDeploymentConfig reference
			referent, found, err := unstructured.NestedMap(configMap, "configReferent")
			if err != nil || !found {
				continue
			}

			name, _, _ := unstructured.NestedString(referent, "name")
			namespace, _, _ := unstructured.NestedString(referent, "namespace")

			if name == "" {
				continue
			}

			// Get the AddOnDeploymentConfig
			addonConfig, err := dynClient.Resource(NewAddOnDeploymentConfigGVR()).
				Namespace(namespace).
				Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return nil, fmt.Errorf("failed to get addondeploymentconfig %s/%s: %w", namespace, name, err)
			}

			return addonConfig, nil
		}
	}

	return nil, fmt.Errorf("no AddOnDeploymentConfig reference found in managedclusteraddon for cluster %s", clusterName)
}

// CalculateSpecHashFromAddOnDeploymentConfig calculates the spec hash for an AddOnDeploymentConfig
func CalculateSpecHashFromAddOnDeploymentConfig(addonConfig *unstructured.Unstructured) (string, error) {
	if addonConfig == nil {
		return "", nil
	}

	// Get the spec portion
	spec, found, err := unstructured.NestedMap(addonConfig.Object, "spec")
	if err != nil {
		return "", fmt.Errorf("failed to get spec from addondeploymentconfig: %w", err)
	}
	if !found {
		return "", nil
	}

	// Hash the spec
	hasher := md5.New() // #nosec G401 G501 - Not used for cryptographic purposes
	specData, err := yaml.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("failed to marshal addon config spec: %w", err)
	}

	hasher.Write(specData)
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// GetSpecHashFromClusterManagementAddOn retrieves the spec hash from ClusterManagementAddOn status
func GetSpecHashFromClusterManagementAddOn(cma *unstructured.Unstructured) (string, error) {
	status, found, err := unstructured.NestedMap(cma.Object, "status")
	if err != nil || !found {
		return "", fmt.Errorf("failed to get status from clustermanagementaddon: %w", err)
	}

	defaultConfigRefs, found, err := unstructured.NestedSlice(status, DefaultConfigReferencesStatusField)
	if err != nil || !found {
		return "", fmt.Errorf("no defaultConfigReferences found in status")
	}

	// Find the AddOnDeploymentConfig entry
	for _, ref := range defaultConfigRefs {
		refMap, ok := ref.(map[string]interface{})
		if !ok {
			continue
		}

		cgr, found, err := unstructured.NestedMap(refMap, "configGroupResource")
		if err != nil || !found {
			continue
		}

		group, _, _ := unstructured.NestedString(cgr, "group")
		resource, _, _ := unstructured.NestedString(cgr, "resource")

		if group == AddonGroup && resource == AddonDeploymentConfigResource {
			// Found the AddOnDeploymentConfig entry
			desiredConfig, found, err := unstructured.NestedMap(refMap, DesiredConfigField)
			if err != nil || !found {
				return "", fmt.Errorf("no desiredConfig found for AddOnDeploymentConfig")
			}

			specHash, found, err := unstructured.NestedString(desiredConfig, SpecHashField)
			if err != nil || !found {
				return "", fmt.Errorf("no specHash found in desiredConfig")
			}

			return specHash, nil
		}
	}

	return "", fmt.Errorf("no AddOnDeploymentConfig entry found in defaultConfigReferences")
}

// ValidateSpecHashInClusterManagementAddOn validates that the spec hash in ClusterManagementAddOn
// matches the calculated hash from the AddOnDeploymentConfig
func ValidateSpecHashInClusterManagementAddOn(opt TestOptions) error {
	// Get the ClusterManagementAddOn
	cma, err := GetClusterManagementAddOn(opt)
	if err != nil {
		return err
	}

	// Get the spec hash from the ClusterManagementAddOn
	actualHash, err := GetSpecHashFromClusterManagementAddOn(cma)
	if err != nil {
		return fmt.Errorf("failed to get spec hash from ClusterManagementAddOn: %w", err)
	}

	if actualHash == "" {
		return fmt.Errorf("spec hash is empty in ClusterManagementAddOn")
	}

	klog.V(1).Infof("Spec hash in ClusterManagementAddOn: %s", actualHash)
	return nil
}

// ValidateSpecHashMatchesConfig validates that the spec hash in ClusterManagementAddOn
// matches a given AddOnDeploymentConfig
func ValidateSpecHashMatchesConfig(opt TestOptions, addonConfig *unstructured.Unstructured) error {
	// Get the ClusterManagementAddOn
	cma, err := GetClusterManagementAddOn(opt)
	if err != nil {
		return err
	}

	// Get the spec hash from the ClusterManagementAddOn
	actualHash, err := GetSpecHashFromClusterManagementAddOn(cma)
	if err != nil {
		return fmt.Errorf("failed to get spec hash from ClusterManagementAddOn: %w", err)
	}

	// Calculate the expected hash from the AddOnDeploymentConfig
	expectedHash, err := CalculateSpecHashFromAddOnDeploymentConfig(addonConfig)
	if err != nil {
		return fmt.Errorf("failed to calculate spec hash from AddOnDeploymentConfig: %w", err)
	}

	if actualHash != expectedHash {
		return fmt.Errorf("spec hash mismatch: ClusterManagementAddOn has %s, expected %s from AddOnDeploymentConfig", actualHash, expectedHash)
	}

	klog.V(1).Infof("Spec hash validation successful: %s", actualHash)
	return nil
}

// GetSpecHashFromManagedClusterAddOn retrieves the spec hash from ManagedClusterAddOn status
func GetSpecHashFromManagedClusterAddOn(opt TestOptions, clusterName string) (string, error) {
	dynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	mca, err := dynClient.Resource(NewMCOManagedClusterAddonsGVR()).
		Namespace(clusterName).
		Get(context.TODO(), ObservabilityController, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get managedclusteraddon for cluster %s: %w", clusterName, err)
	}

	status, found, err := unstructured.NestedMap(mca.Object, "status")
	if err != nil || !found {
		return "", fmt.Errorf("failed to get status from managedclusteraddon: %w", err)
	}

	configRefs, found, err := unstructured.NestedSlice(status, "configReferences")
	if err != nil || !found {
		return "", fmt.Errorf("no configReferences found in status")
	}

	// Find the AddOnDeploymentConfig entry
	for _, ref := range configRefs {
		refMap, ok := ref.(map[string]interface{})
		if !ok {
			continue
		}

		cgr, found, err := unstructured.NestedMap(refMap, "configGroupResource")
		if err != nil || !found {
			continue
		}

		group, _, _ := unstructured.NestedString(cgr, "group")
		resource, _, _ := unstructured.NestedString(cgr, "resource")

		if group == AddonGroup && resource == AddonDeploymentConfigResource {
			// Found the AddOnDeploymentConfig entry
			desiredConfig, found, err := unstructured.NestedMap(refMap, "desiredConfig")
			if err != nil || !found {
				return "", fmt.Errorf("no desiredConfig found for AddOnDeploymentConfig")
			}

			specHash, found, err := unstructured.NestedString(desiredConfig, "specHash")
			if err != nil || !found {
				return "", fmt.Errorf("no specHash found in desiredConfig")
			}

			return specHash, nil
		}
	}

	return "", fmt.Errorf("no AddOnDeploymentConfig entry found in configReferences")
}

// ValidateSpecHashInManagedClusterAddOn validates that the spec hash is set in ManagedClusterAddOn
func ValidateSpecHashInManagedClusterAddOn(opt TestOptions, clusterName string) error {
	specHash, err := GetSpecHashFromManagedClusterAddOn(opt, clusterName)
	if err != nil {
		return fmt.Errorf("failed to get spec hash from ManagedClusterAddOn: %w", err)
	}

	if specHash == "" {
		return fmt.Errorf("spec hash is empty in ManagedClusterAddOn for cluster %s", clusterName)
	}

	klog.V(1).Infof("Spec hash in ManagedClusterAddOn for cluster %s: %s", clusterName, specHash)
	return nil
}
