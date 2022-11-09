// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ManagedClusterLabelAllowListConfigMapName = "observability-managed-cluster-label-allowlist"
	ManagedClusterLabelAllowListConfigMapKey  = "managed_cluster.yaml"
	ManagedClusterLabelMetricName             = "managed_cluster_labels"

	RBACProxyLabelMetricName = "acm_label_names"
)

var (
	ManagedLabelList ManagedClusterLabelList
)

// GetManagedClusterLabelAllowListConfigMapKey return the key name for the managedcluster labels
func GetManagedClusterLabelAllowListConfigMapKey() string {
	return ManagedClusterLabelAllowListConfigMapKey
}

// GetManagedClusterLabelConfigMapName return the name for the managedcluster labels configmap
func GetManagedClusterLabelAllowListConfigMapName() string {
	return ManagedClusterLabelAllowListConfigMapName
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

// CreateManagedClusterLabelAllowListCM creates a managedcluster label allowlist configmap object
func CreateManagedClusterLabelAllowListCM() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: GetManagedClusterLabelAllowListConfigMapName(),
		},
		Data: map[string]string{
			GetManagedClusterLabelAllowListConfigMapKey(): `labels: []

blacklist_labels:
- clusterID
- cluster.open-cluster-management.io/clusterset
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
- name
`}}
}
