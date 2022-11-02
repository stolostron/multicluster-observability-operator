// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

const (
	rbacProxyLabelMetricName = "acm_label_names"
)

var (
	labelList ClusterLabelList
)

// GetClusterLabelList will return the current cluster label list
func GetClusterLabelList() *ClusterLabelList {
	return &labelList
}

// GetRBACProxyLabelMetricName returns the name of the rbac query proxy label metric
func GetRBACProxyLabelMetricName() string {
	return rbacProxyLabelMetricName
}
