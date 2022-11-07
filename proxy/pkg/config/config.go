// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ManagedClusterLabelConfigMapName         = "observability-managed-cluster-label-allowlist"
	ManagedClusterLabelConfigMapLabelListKey = "managed_cluster.yaml"
	ManagedClusterLabelMetricName            = "managed_cluster_labels"

	RBACProxyLabelMetricName = "acm_label_names"
)

var (
	ManagedLabelList ManagedClusterLabelList
)

// GetManagedClusterLabelConfigMapLabelListKey return the key name for the managedcluster labels
func GetManagedClusterLabelConfigMapLabelListKey() string {
	return ManagedClusterLabelConfigMapLabelListKey
}

// GetManagedClusterLabelConfigMapName return the name for the managedcluster labels configmap
func GetManagedClusterLabelConfigMapName() string {
	return ManagedClusterLabelConfigMapName
}

// GetManagedClusterLabelMetricName return the name of the managedcluster label metric
func GetManagedClusterLabelMetricName() string {
	return ManagedClusterLabelMetricName
}

// GetManagedClusterLabelList will return the current cluster label list
func GetManagedClusterLabelList() *ManagedClusterLabelList {
	return &ManagedLabelList
}

// GetRBACProxyLabelMetricName returns the name of the rbac query proxy label metric
func GetRBACProxyLabelMetricName() string {
	return RBACProxyLabelMetricName
}

// CreateManagedClusterLabelNamesCM create a managed cluster label names configmap
func CreateManagedClusterLabelNamesCM(c client.Client) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetManagedClusterLabelConfigMapName(),
			Namespace: "open-cluster-management-observability",
		},
		Data: map[string]string{
			GetManagedClusterLabelConfigMapLabelListKey(): `labels:
- cloud
- vendor

blacklist_label:
- clusterID
- cluster.open-cluster-management.io/clusterset
- installer.name
- installer.namespace
- name
- local-cluster
- feature.open-cluster-management.io/addon-application-manager
- feature.open-cluster-management.io/addon-cert-policy-controller
- feature.open-cluster-management.io/addon-cluster-proxy
- feature.open-cluster-management.io/addon-config-policy-controller
- feature.open-cluster-management.io/addon-governance-policy-framework
- feature.open-cluster-management.io/addon-iam-policy-controller
- feature.open-cluster-management.io/addon-observability-controller
- feature.open-cluster-management.io/addon-work-manager
`,
		},
	}

	err := c.Create(context.TODO(), cm)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			return nil
		}

		klog.Errorf("failed to create configmap: %v", GetManagedClusterLabelConfigMapName())
		return err
	}

	klog.Infof("created configmap: %v", GetManagedClusterLabelConfigMapName())
	return nil
}
