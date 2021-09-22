package utils

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ManagedClusterAddOnDisabledMessage = "enableMetrics is set to False"
	ManagedClusterAddOnEnabledMessage  = "Cluster metrics sent successfully"
)

func CheckOBAStatus(opt TestOptions, namespace, status string) error {
	dynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	oba, err := dynClient.Resource(NewMCOAddonGVR()).Namespace(namespace).Get(context.TODO(), "observability-addon", metav1.GetOptions{})
	if err != nil {
		return err
	}
	if oba.Object["status"] != nil && strings.Contains(fmt.Sprint(oba.Object["status"]), status) {
		return nil
	} else {
		return fmt.Errorf("observability-addon is not ready for managed cluster %s", namespace)
	}
}

func CheckManagedClusterAddonsStatus(opt TestOptions, namespace, status string) error {
	dynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	mca, err := dynClient.Resource(NewMCOManagedClusterAddonsGVR()).Namespace(namespace).Get(context.TODO(), "observability-controller", metav1.GetOptions{})
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
	for _, cluster := range clusters {
		err = CheckOBAStatus(opt, cluster, ManagedClusterAddOnEnabledMessage)
		if err != nil {
			return err
		}
		err = CheckManagedClusterAddonsStatus(opt, cluster, ManagedClusterAddOnEnabledMessage)
		if err != nil {
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
