// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
)

func CheckOBAStatus(opt TestOptions, namespace string) error {
	dynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	// Check ManagedClusterAddOn status first
	mcaObj, err := dynClient.Resource(NewMCOManagedClusterAddonsGVR()).
		Namespace(namespace).
		Get(context.TODO(), "observability-controller", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get managedclusteraddon observability-controller: %w", err)
	}

	mca := &addonv1alpha1.ManagedClusterAddOn{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(mcaObj.Object, mca)
	if err != nil {
		return fmt.Errorf("failed to convert unstructured to managedclusteraddon: %w", err)
	}

	if !meta.IsStatusConditionTrue(mca.Status.Conditions, "Available") {
		return fmt.Errorf("managedclusteraddon observability-controller is not available in %s, conditions: %+v", namespace, mca.Status.Conditions)
	}

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

	if meta.IsStatusConditionTrue(addon.Status.Conditions, "MetricsCollector") {
		return nil
	}

	return fmt.Errorf("observability-addon is not ready for managed cluster %q, conditions: %+v", namespace, addon.Status.Conditions)
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
		if cluster.isLocalCluster {
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

// This will check that the ObservabilityAddon is removed from a specific managed cluster
func CheckOBADeletedOnHub(opt TestOptions) error {
	clusterName := GetManagedClusterName(opt)
	if clusterName == "" {
		return fmt.Errorf("managed cluster name is empty")
	}
	klog.V(1).Infof("Checking observability-addon deleted for managed cluster %s", clusterName)
	dynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	_, err := dynClient.Resource(NewMCOAddonGVR()).Namespace(clusterName).Get(context.TODO(), "observability-addon", metav1.GetOptions{})
	if err == nil {
		klog.Errorf("observability-addon on hub cluster still exists for managed cluster %s", clusterName)
		return fmt.Errorf("observability-addon on hub cluster still exists for managed cluster %s", clusterName)
	}
	if !errors.IsNotFound(err) {
		klog.Errorf("failed to get observability-addon on hub cluster for managed cluster %s: %v", clusterName, err)
		return fmt.Errorf("failed to get observability-addon on hub cluster for managed cluster %s: %w", clusterName, err)
	}
	return nil
}

func CheckOBADeletedOnManagedCluster(opt TestOptions) error {
	if len(opt.ManagedClusters) == 0 {
		return fmt.Errorf("no managed clusters found")
	}
	clusterName := GetManagedClusterName(opt)
	if clusterName == "" {
		return fmt.Errorf("managed cluster name is empty")
	}
	dynClient := NewKubeClientDynamic(
		opt.ManagedClusters[0].ClusterServerURL,
		opt.ManagedClusters[0].KubeConfig,
		opt.ManagedClusters[0].KubeContext)

	_, err := dynClient.Resource(NewMCOAddonGVR()).Namespace(MCO_ADDON_NAMESPACE).Get(context.TODO(), "observability-addon", metav1.GetOptions{})

	if err == nil {
		klog.Errorf("observability-addon still exists on managed cluster %s", clusterName)
		return fmt.Errorf("observability-addon still exists on managed cluster %s", clusterName)
	}
	if !errors.IsNotFound(err) {
		klog.Errorf("failed to get observability-addon on managed cluster %s: %v", clusterName, err)
		return fmt.Errorf("failed to get observability-addon on managed cluster %s: %w", clusterName, err)
	}
	return nil
}

// Check for the removal of the manifestwork from the hub cluster
func CheckObsAddonManifestWorkDeleted(opt TestOptions) error {
	clientDynamic := GetKubeClientDynamic(opt, true)
	clusterName := GetManagedClusterName(opt)
	if clusterName == "" {
		return fmt.Errorf("Cannot check for managed manifestwork on the hub cluster, managed cluster name is empty")
	}
	manifestWorkName := clusterName + "-observability"

	_, err := clientDynamic.Resource(NewOCMManifestworksGVR()).Namespace(clusterName).Get(context.TODO(), manifestWorkName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil
	}

	return fmt.Errorf("ManifestWork %s/%s still exists", clusterName, manifestWorkName)
}
