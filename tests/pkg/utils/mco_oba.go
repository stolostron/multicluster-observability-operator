// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

const (
	ManagedClusterAddOnDisabledMessage = "enableMetrics is set to False"
	OBMAddonEnabledMessage             = "Cluster metrics sent successfully"
	ManagedClusterAddOnEnabledMessage  = "observability-controller add-on is available"
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

	obaStatus := fmt.Sprint(oba.Object["status"])
	if strings.Contains(obaStatus, status) {
		return nil
	} else {
		// print logs
		clientKube := getKubeClient(opt, false)
		podList, err := GetAllOBAPods(opt)
		if err != nil {
			klog.Errorf("Failed to get all OBA pods: %v", err)
			return err
		}
		LogPodsDebugInfo(clientKube, podList, true)
		return fmt.Errorf("observability-addon is not ready for managed cluster %q with status %q: %v", namespace, obaStatus, oba.Object)
	}
}

func CheckOBADeleted(opt TestOptions, namespace string) error {
	dynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	_, err := dynClient.Resource(NewMCOAddonGVR()).Namespace(namespace).Get(context.TODO(), "observability-addon", metav1.GetOptions{})
	if err == nil || !errors.IsNotFound(err) {
		return fmt.Errorf("observability-addon is not properly deleted for managed cluster %s", namespace)
	}
	return nil
}

func CheckManagedClusterAddonsStatus(opt TestOptions, namespace, status string) error {
	dynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	mca, err := dynClient.Resource(NewMCOManagedClusterAddonsGVR()).
		Namespace(namespace).
		Get(context.TODO(), "observability-controller", metav1.GetOptions{})
	if err != nil {
		return err
	}
	if mca.Object["status"] != nil && strings.Contains(fmt.Sprint(mca.Object["status"]), status) {
		return nil
	} else {
		return fmt.Errorf("observability-controller is disabled for managed cluster %s", namespace)
	}
}

func CheckAllOBAsEnabled(opt TestOptions) error {
	clusters, err := ListManagedClusters(opt)
	if err != nil {
		return err
	}
	klog.V(1).Infof("Check OBA status for managedclusters: %v", clusters)

	for _, cluster := range clusters {
		err = CheckOBAStatus(opt, cluster, OBMAddonEnabledMessage)
		if err != nil {
			klog.V(1).Infof("Error checking OBA status for cluster %q: %v", cluster, err)
			return err
		}
	}
	return nil
}

func CheckAllOBADisabled(opt TestOptions) error {
	clusters, err := ListManagedClusters(opt)
	if err != nil {
		return err
	}
	for _, cluster := range clusters {
		err = CheckOBAStatus(opt, cluster, ManagedClusterAddOnDisabledMessage)
		if err != nil {
			return err
		}
		err = CheckManagedClusterAddonsStatus(opt, cluster, ManagedClusterAddOnDisabledMessage)
		if err != nil {
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
		err = CheckOBADeleted(opt, cluster)
		if err != nil {
			return err
		}
	}
	return nil
}
