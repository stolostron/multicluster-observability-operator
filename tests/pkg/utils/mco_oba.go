package utils

import (
	"context"
	"fmt"
	"strings"

	cerr "github.com/efficientgo/core/errors"
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
	if oba.Object["status"] != nil && strings.Contains(fmt.Sprint(oba.Object["status"]), status) {
		return nil
	} else {
		return cerr.Newf("observability-addon is not ready for managed cluster %s", namespace)
	}
}

func CheckOBADeleted(opt TestOptions, namespace string) error {
	dynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	_, err := dynClient.Resource(NewMCOAddonGVR()).Namespace(namespace).Get(context.TODO(), "observability-addon", metav1.GetOptions{})
	if err == nil || !errors.IsNotFound(err) {
		return cerr.Newf("observability-addon is not properly deleted for managed cluster %s", namespace)
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
		return cerr.Newf("observability-controller is disabled for managed cluster %s", namespace)
	}
}

func CheckAllOBAsEnabled(opt TestOptions) error {
	clusters, err := ListManagedClusters(opt)
	if err != nil {
		return err
	}
	klog.V(1).Infof("Have the following managedclusters: <%v>", clusters)

	for _, cluster := range clusters {
		klog.V(1).Infof("Check OBA status for cluster <%v>", cluster)
		err = CheckOBAStatus(opt, cluster, OBMAddonEnabledMessage)
		if err != nil {
			return err
		}

		// klog.V(1).Infof("Check managedcluster addon status for cluster <%v>", cluster)
		// // NOTE: Managed cluster add-on status gets set to "Cluster metrics sent successfully"
		// // for a very brief period of time, but it quickly gets overwritten with
		// // "observability-controller add-on is available" when managed cluster addon's lease gets updated.
		// // Updating the test case to only check for the later more persistent message.
		// err = CheckManagedClusterAddonsStatus(opt, cluster, ManagedClusterAddOnEnabledMessage)
		// if err != nil {
		// 	return err
		// }
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
