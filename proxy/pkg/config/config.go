// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

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
