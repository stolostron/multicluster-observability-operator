// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

import (
	mcoConfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	managedClusterLabelConfigMapKey  = "managed_cluster.yaml"
	managedClusterLabelConfigMapName = "observability-managed-cluster-label-names"
)

var (
	labelList ClusterLabelList
)

// GetManagedClusterLabelConfigMapKey returns the key for the cluster label
func GetManagedClusterLabelConfigMapKey() string {
	return managedClusterLabelConfigMapKey
}

// GetManagedClusterLabelConfigMapName returns the name of the for the cluster label configmap
func GetManagedClusterLabelConfigMapName() string {
	return managedClusterLabelConfigMapName
}

func CreateClusterLabelConfigmap() *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetManagedClusterLabelConfigMapName(),
			Namespace: mcoConfig.GetDefaultNamespace(),
		},
		Data: map[string]string{
			GetManagedClusterLabelConfigMapKey(): `labels:
		- cloud
		- vendor`,
		},
	}

	return cm
}

func GetClusterLabelList() *ClusterLabelList {
	return &labelList
}
