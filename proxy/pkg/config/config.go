// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
)

const (
	managedClusterLabelMetricName = "managed_cluster_labels"
	rbacProxyLabelMetricName      = "acm_label_names"
)

var (
	labelList ClusterLabelList
)

// GetClusterLabelList will return the current cluster label list
func GetClusterLabelList() *ClusterLabelList {
	return &labelList
}

// GetManagedClusterLabelMetricName returns the name of the manged cluster label metric name
func GetManagedClusterLabelMetricName() string {
	return managedClusterLabelMetricName
}

// GetRBACProxyLabelMetricName returns the name of the rbac query proxy label metric
func GetRBACProxyLabelMetricName() string {
	return rbacProxyLabelMetricName
}

func CreateClusterLabelConfigmap() *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcoconfig.GetManagedClusterLabelConfigMapName(),
			Namespace: mcoconfig.GetDefaultNamespace(),
		},
		Data: map[string]string{
			mcoconfig.GetManagedClusterLabelConfigMapKey(): `
labels:
  - cloud
  - vendor
`},
	}

	return cm
}
