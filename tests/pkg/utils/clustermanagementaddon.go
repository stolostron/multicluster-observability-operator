// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
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
	opt TestOptions,
	name string,
	placementName string,
	configGVR schema.GroupVersionResource,
	configName string,
	configNamespace string,
) error {
	clientDynamic := GetKubeClientDynamic(opt, true)
	backoffConfig := wait.Backoff{
		Steps:    10,
		Duration: 10 * time.Millisecond,
		Factor:   3.0,
		Jitter:   0.1,
		Cap:      500 * time.Millisecond,
	}
	return retry.RetryOnConflict(backoffConfig, func() error {
		cma, err := clientDynamic.Resource(clusterManagementAddonGVR).Get(context.TODO(), name, metav1.GetOptions{})
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
			placement, ok := p.(map[string]interface{})
			if !ok {
				continue
			}
			if placement["name"] == placementName {
				configs, found, err := unstructured.NestedSlice(placement, "configs")
				if err != nil {
					return fmt.Errorf("failed to get configs from placement %s: %w", placementName, err)
				}
				if !found {
					configs = []interface{}{}
				}

				newConfig := map[string]interface{}{
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

		_, err = clientDynamic.Resource(clusterManagementAddonGVR).Update(context.TODO(), cma, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update ClusterManagementAddon: %w", err)
		}

		return nil
	})
}

func RemoveConfigFromPlacementInClusterManagementAddon(
	opt TestOptions,
	name string,
	placementName string,
	configGVR schema.GroupVersionResource,
	configName string,
	configNamespace string,
) error {
	clientDynamic := GetKubeClientDynamic(opt, true)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cma, err := clientDynamic.Resource(clusterManagementAddonGVR).Get(context.TODO(), name, metav1.GetOptions{})
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
			placement, ok := p.(map[string]interface{})
			if !ok {
				continue
			}
			if placement["name"] == placementName {
				configs, found, err := unstructured.NestedSlice(placement, "configs")
				if err != nil || !found {
					return fmt.Errorf("failed to get configs from placement %s or it's not found: %w", placementName, err)
				}

				newConfigs := []interface{}{}
				for _, config := range configs {
					configMap, ok := config.(map[string]interface{})
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

		_, err = clientDynamic.Resource(clusterManagementAddonGVR).Update(context.TODO(), cma, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update ClusterManagementAddon: %w", err)
		}

		return nil
	})
}
