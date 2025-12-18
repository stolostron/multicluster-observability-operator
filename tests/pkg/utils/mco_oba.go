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
