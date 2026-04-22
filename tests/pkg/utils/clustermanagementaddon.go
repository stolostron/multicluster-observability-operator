// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
)

const (
	MCOA_CLUSTER_MANAGEMENT_ADDON_NAME = "multicluster-observability-addon"
)

var (
	clusterManagementAddonGVR = schema.GroupVersionResource{
		Group:    "addon.open-cluster-management.io",
		Version:  "v1alpha1",
		Resource: "clustermanagementaddons",
	}
	ScrapeConfigGVR = schema.GroupVersionResource{
		Group:    "monitoring.rhobs",
		Version:  "v1alpha1",
		Resource: "scrapeconfigs",
	}
)

func AddConfigToPlacementInClusterManagementAddon(
	ctx context.Context,
	opt TestOptions,
	name string,
	placementName string,
	configGVR schema.GroupVersionResource,
	configName string,
	configNamespace string,
) error {
	clientDynamic := GetKubeClientDynamic(opt, true)
	backoffConfig := wait.Backoff{
		Steps:    11,
		Duration: 10 * time.Millisecond,
		Factor:   2.0,
		Jitter:   0.1,
		// Cap:      10 * time.Second,
	}
	retryErr := retry.RetryOnConflict(backoffConfig, func() error {
		cma, err := clientDynamic.Resource(clusterManagementAddonGVR).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get ClusterManagementAddon %s: %w", name, err)
		}

		installStrategy, found, err := unstructured.NestedMap(cma.Object, "spec", "installStrategy")
		if err != nil || !found {
			return fmt.Errorf("failed to get installStrategy from ClusterManagementAddon or it's not found: %w", err)
		}

		placements, found, err := unstructured.NestedSlice(installStrategy, "placements")
		if err != nil || !found {
			return fmt.Errorf("failed to get placements from installStrategy or it's not found: %w", err)
		}

		for i, p := range placements {
			placement, ok := p.(map[string]any)
			if !ok {
				continue
			}
			if placement["name"] == placementName {
				configs, found, err := unstructured.NestedSlice(placement, "configs")
				if err != nil {
					return fmt.Errorf("failed to get configs from placement %s: %w", placementName, err)
				}
				if !found {
					configs = []any{}
				}

				newConfig := map[string]any{
					"group":    configGVR.Group,
					"resource": configGVR.Resource,
					"name":     configName,
				}
				if configNamespace != "" {
					newConfig["namespace"] = configNamespace
				}

				configs = append(configs, newConfig)
				if err := unstructured.SetNestedSlice(placement, configs, "configs"); err != nil {
					return fmt.Errorf("failed to set configs in placement %s: %w", placementName, err)
				}
				placements[i] = placement
				break
			}
		}

		if err := unstructured.SetNestedSlice(installStrategy, placements, "placements"); err != nil {
			return fmt.Errorf("failed to set placements in installStrategy: %w", err)
		}
		if err := unstructured.SetNestedMap(cma.Object, installStrategy, "spec", "installStrategy"); err != nil {
			return fmt.Errorf("failed to set installStrategy in ClusterManagementAddon: %w", err)
		}

		_, err = clientDynamic.Resource(clusterManagementAddonGVR).Update(ctx, cma, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update ClusterManagementAddon: %w", err)
		}

		return nil
	})
	if retryErr != nil {
		return retryErr
	}

	// When a config is added to the CMA, the OCM hub controller fully replaces
	// MCA.Status.ConfigReferences with a new list that includes the added config but has every
	// DesiredConfig.SpecHash set to "". The addonconfig-controller then repopulates all hashes
	// asynchronously. Until it does, any controller reading the AddOnDeploymentConfig hash
	// (addon-registration-controller, addon-deploy-controller) errors with "deployment config
	// desired spec hash is empty" and keeps requeuing, preventing config propagation to spokes.
	// Waiting for the newly added config's hash to be populated confirms that the
	// addonconfig-controller completed its pass and all hashes — including AddOnDeploymentConfig
	// — are populated again.
	return waitForConfigHashPopulated(ctx, opt, name, configGVR, configName, configNamespace)
}

const (
	addonConfigHashPollInterval = 2 * time.Second
	addonConfigHashPollTimeout  = 2 * time.Minute
)

// waitForConfigHashPopulated polls until the given config appears with a non-empty
// desiredConfig.specHash in the status.configReferences of every ManagedClusterAddon for the
// given addon. This signals that the addonconfig-controller has fully processed the CMA update
// and all spec hashes in MCA.Status.ConfigReferences have been repopulated.
func waitForConfigHashPopulated(ctx context.Context, opt TestOptions, addonName string, configGVR schema.GroupVersionResource, configName, configNamespace string) error {
	clientDynamic := GetKubeClientDynamic(opt, true)
	mcaGVR := NewMCOManagedClusterAddonsGVR()

	managedClusters, err := GetAvailableManagedClusters(opt)
	if err != nil {
		return fmt.Errorf("failed to get available managed clusters: %w", err)
	}
	if len(managedClusters) == 0 {
		return fmt.Errorf("no managed clusters found")
	}

	return wait.PollUntilContextTimeout(ctx, addonConfigHashPollInterval, addonConfigHashPollTimeout, true, func(ctx context.Context) (bool, error) {
		for _, cluster := range managedClusters {
			mca, err := clientDynamic.Resource(mcaGVR).
				Namespace(cluster.Name).
				Get(ctx, addonName, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) ||
					apierrors.IsTimeout(err) ||
					apierrors.IsServerTimeout(err) ||
					apierrors.IsTooManyRequests(err) {
					return false, nil // transient, retry
				}
				return false, fmt.Errorf("failed to get ManagedClusterAddon %s/%s: %w", cluster.Name, addonName, err)
			}

			configRefs, found, err := unstructured.NestedSlice(mca.Object, "status", "configReferences")
			if err != nil {
				return false, fmt.Errorf("failed to read status.configReferences from ManagedClusterAddon %s/%s: %w", cluster.Name, addonName, err)
			}
			if !found {
				klog.V(1).Infof("status.configReferences not found in ManagedClusterAddon %s/%s, retrying", cluster.Name, addonName)
				return false, nil // status not yet written, retry
			}

			newConfigFound := false
			for _, ref := range configRefs {
				refMap, ok := ref.(map[string]any)
				if !ok {
					continue
				}

				desiredConfig, found, _ := unstructured.NestedMap(refMap, "desiredConfig")
				if !found {
					klog.V(1).Infof("desiredConfig not found in a configReference of ManagedClusterAddon %s/%s, skipping ref", cluster.Name, addonName)
					continue
				}

				group, _, _ := unstructured.NestedString(refMap, "group")
				resource, _, _ := unstructured.NestedString(refMap, "resource")
				name, _, _ := unstructured.NestedString(desiredConfig, "name")
				namespace, _, _ := unstructured.NestedString(desiredConfig, "namespace")

				if group != configGVR.Group || resource != configGVR.Resource || name != configName || namespace != configNamespace {
					continue
				}

				specHash, _, _ := unstructured.NestedString(desiredConfig, "specHash")
				if specHash == "" {
					return false, nil // target config hash still empty, retry
				}
				newConfigFound = true
				break
			}
			if !newConfigFound {
				return false, nil // newly added config not yet in MCA status, retry
			}
		}

		return true, nil
	})
}

func RemoveConfigFromPlacementInClusterManagementAddon(
	ctx context.Context,
	opt TestOptions,
	name string,
	placementName string,
	configGVR schema.GroupVersionResource,
	configName string,
	configNamespace string,
) error {
	clientDynamic := GetKubeClientDynamic(opt, true)
	backoffConfig := wait.Backoff{
		Steps:    11,
		Duration: 10 * time.Millisecond,
		Factor:   2.0,
		Jitter:   0.1,
		// Cap:      500 * time.Millisecond,
	}
	return retry.RetryOnConflict(backoffConfig, func() error {
		cma, err := clientDynamic.Resource(clusterManagementAddonGVR).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get ClusterManagementAddon %s: %w", name, err)
		}

		installStrategy, found, err := unstructured.NestedMap(cma.Object, "spec", "installStrategy")
		if err != nil || !found {
			return fmt.Errorf("failed to get installStrategy from ClusterManagementAddon or it's not found: %w", err)
		}

		placements, found, err := unstructured.NestedSlice(installStrategy, "placements")
		if err != nil || !found {
			return fmt.Errorf("failed to get placements from installStrategy or it's not found: %w", err)
		}

		for i, p := range placements {
			placement, ok := p.(map[string]any)
			if !ok {
				continue
			}
			if placement["name"] == placementName {
				configs, found, err := unstructured.NestedSlice(placement, "configs")
				if err != nil || !found {
					return fmt.Errorf("failed to get configs from placement %s or it's not found: %w", placementName, err)
				}

				newConfigs := []any{}
				for _, config := range configs {
					configMap, ok := config.(map[string]any)
					if !ok {
						continue
					}
					if configMap["group"] != configGVR.Group ||
						configMap["resource"] != configGVR.Resource ||
						configMap["name"] != configName ||
						configMap["namespace"] != configNamespace {
						newConfigs = append(newConfigs, config)
					}
				}

				if err := unstructured.SetNestedSlice(placement, newConfigs, "configs"); err != nil {
					return fmt.Errorf("failed to set configs in placement %s: %w", placementName, err)
				}
				placements[i] = placement
				break
			}
		}

		if err := unstructured.SetNestedSlice(installStrategy, placements, "placements"); err != nil {
			return fmt.Errorf("failed to set placements in installStrategy: %w", err)
		}
		if err := unstructured.SetNestedMap(cma.Object, installStrategy, "spec", "installStrategy"); err != nil {
			return fmt.Errorf("failed to set installStrategy in ClusterManagementAddon: %w", err)
		}

		_, err = clientDynamic.Resource(clusterManagementAddonGVR).Update(ctx, cma, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update ClusterManagementAddon: %w", err)
		}

		return nil
	})
}
