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
	OBMAddonEnabledMessage = "Cluster metrics sent successfully"
)

func CheckOBAStatus(opt TestOptions, clusterInfo ClustersInfo, status string) error {
	dynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	oba, err := dynClient.Resource(NewMCOAddonGVR()).
		Namespace(clusterInfo.Name).
		Get(context.TODO(), "observability-addon", metav1.GetOptions{})
	if err != nil {
		return err
	}

	obaStatus := fmt.Sprint(oba.Object["status"])
	if strings.Contains(obaStatus, status) {
		return nil
	} else {
		return fmt.Errorf("observability-addon is not ready for managed cluster %q with status %q: %v", clusterInfo.Name, obaStatus, oba.Object)
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

func CheckAllOBAsEnabled(opt TestOptions) error {
	clusters, err := ListManagedClusters(opt)
	if err != nil {
		return err
	}
	klog.V(1).Infof("Check OBA status for managedclusters: %v", clusters)

	for _, cluster := range clusters {
		// skip the check for local-cluster
		if cluster.isLocalCluster {
			klog.V(1).Infof("Skip OBA status for managedcluster: %v", cluster)
			continue
		}
		err = CheckOBAStatus(opt, cluster, OBMAddonEnabledMessage)
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
		if cluster.isLocalCluster {
			klog.V(1).Infof("Skip OBA status for managedcluster: %v", cluster)
			continue
		}
		err = CheckOBADeleted(opt, cluster.Name)
		if err != nil {
			return err
		}
	}
	return nil
}
