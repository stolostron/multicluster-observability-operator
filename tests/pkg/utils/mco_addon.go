// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"fmt"

	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
)

func CheckManagedClusterAddonStatus(opt TestOptions, name string) {
	gomega.Eventually(func() error {
		klog.V(1).Infof("Checking ManagedClusterAddon %s status", name)
		managedClusters, err := GetAvailableManagedClusters(opt)
		if err != nil {
			klog.Errorf("Error getting available managed clusters: %v", err)
			return err
		}

		for _, cluster := range managedClusters {
			klog.V(1).Infof("Checking ManagedClusterAddon %s on cluster %s", name, cluster.Name)
			addon, err := GetManagedClusterAddon(opt, name, cluster.Name)
			if err != nil {
				klog.Errorf("Error getting ManagedClusterAddon %s/%s: %v", cluster.Name, name, err)
				return err
			}

			if !meta.IsStatusConditionTrue(addon.Status.Conditions, "Available") {
				err := fmt.Errorf("ManagedClusterAddon %s on cluster %s is not available. Conditions: %v", name, cluster.Name, addon.Status.Conditions)
				klog.V(1).Infof("%v", err)
				return err
			}
			klog.V(1).Infof("ManagedClusterAddon %s on cluster %s is available", name, cluster.Name)
		}
		return nil
	}, 300, 5).Should(gomega.Not(gomega.HaveOccurred()))
}

// GetAvailableManagedClustersAsClusters returns a list of available managed clusters.
// The hub cluster is not included in the list.
func GetAvailableManagedClustersAsClusters(opt TestOptions) ([]Cluster, error) {
	availableManagedClusters, err := GetAvailableManagedClusters(opt)
	if err != nil {
		return nil, err
	}

	var clusterList []Cluster
	for _, managedCluster := range availableManagedClusters {
		if IsHubCluster(managedCluster) {
			continue
		}

		var cluster Cluster
		for _, c := range opt.ManagedClusters {
			if c.Name == managedCluster.Name {
				cluster = c
				break
			}
		}
		if cluster.Name != "" {
			clusterList = append(clusterList, cluster)
		}
	}
	return clusterList, nil
}

func GetManagedClusterAddon(opt TestOptions, name, namespace string) (*addonapiv1alpha1.ManagedClusterAddOn, error) {
	clientDynamic := GetKubeClientDynamic(opt, true)
	obj, err := clientDynamic.Resource(NewMCOManagedClusterAddonsGVR()).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ManagedClusterAddon %s/%s: %w", namespace, name, err)
	}

	mca := &addonapiv1alpha1.ManagedClusterAddOn{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, mca); err != nil {
		return nil, fmt.Errorf("failed to convert Unstructured to ManagedClusterAddon: %w", err)
	}

	return mca, nil
}
