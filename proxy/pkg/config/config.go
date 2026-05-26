// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package config

import (
	"context"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"k8s.io/klog"
)

const (
	ManagedClusterLabelAllowListConfigMapName = "observability-managed-cluster-label-allowlist"
	ManagedClusterLabelAllowListConfigMapKey  = "managed_cluster.yaml"
	ManagedClusterLabelAllowListNamespace     = "open-cluster-management-observability"

	RBACProxyLabelMetricName              = "acm_label_names"
	ACMManagedClusterLabelNamesMetricName = "acm_managed_cluster_labels"
)

var (
	ManagedLabelList ManagedClusterLabelList
	SyncLabelList    ManagedClusterLabelList
)

var (
	requiredLabelList = []string{"name", "cluster.open-cluster-management.io/clusterset"}
)

// GetManagedClusterLabelAllowListConfigMapKey return the key name for the managedcluster labels.
func GetManagedClusterLabelAllowListConfigMapKey() string {
	return ManagedClusterLabelAllowListConfigMapKey
}

// GetManagedClusterLabelConfigMapName return the name for the managedcluster labels configmap.
func GetManagedClusterLabelAllowListConfigMapName() string {
	return ManagedClusterLabelAllowListConfigMapName
}

// GetManagedClusterLabelList will return the current cluster label list.
func GetManagedClusterLabelList() *ManagedClusterLabelList {
	return &ManagedLabelList
}

// GetSyncLabelList will return the synced label list.
func GetRequiredLabelList() []string {
	return requiredLabelList
}

// GetSyncLabelList will return the synced label list.
func GetSyncLabelList() *ManagedClusterLabelList {
	return &SyncLabelList
}

// GetRBACProxyLabelMetricName returns the name of the rbac query proxy label metric.
func GetRBACProxyLabelMetricName() string {
	return RBACProxyLabelMetricName
}

func GetACMManagedClusterLabelNamesMetricName() string {
	return ACMManagedClusterLabelNamesMetricName
}

// CreateManagedClusterLabelAllowListCM creates a managedcluster label allowlist configmap object.
func CreateManagedClusterLabelAllowListCM(namespace string) *v1.ConfigMap {
	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetManagedClusterLabelAllowListConfigMapName(),
			Namespace: namespace,
		},
		Data: map[string]string{
			GetManagedClusterLabelAllowListConfigMapKey(): `labels:
- cloud
- vendor

ignore_labels:
- clusterID
- feature.open-cluster-management.io/addon-application-manager
- feature.open-cluster-management.io/addon-cert-policy-controller
- feature.open-cluster-management.io/addon-cluster-proxy
- feature.open-cluster-management.io/addon-config-policy-controller
- feature.open-cluster-management.io/addon-governance-policy-framework
- feature.open-cluster-management.io/addon-iam-policy-controller
- feature.open-cluster-management.io/addon-observability-controller
- feature.open-cluster-management.io/addon-search-collector
- feature.open-cluster-management.io/addon-work-manager
- installer.name
- installer.namespace
- local-cluster
`}}
}

// GetManagedClusterLabelAllowListConfigmap returns the managedcluster label allowlist configmap.
func GetManagedClusterLabelAllowListConfigmap(kubeClient kubernetes.Interface, namespace string) (*v1.ConfigMap,
	error) {
	configmap, err := kubeClient.CoreV1().ConfigMaps(namespace).Get(
		context.TODO(),
		GetManagedClusterLabelAllowListConfigMapName(),
		metav1.GetOptions{},
	)
	if err != nil {
		klog.Errorf("failed to get managedcluster label allowlist configmap: %v", err)
		return nil, err
	}
	return configmap, nil
}
