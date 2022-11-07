// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

import (
	"testing"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateManagedClusterLabelAllowListCM() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetManagedClusterLabelAllowListConfigMapName(),
			Namespace: "open-cluster-management-observability",
		},
		Data: map[string]string{
			GetManagedClusterLabelAllowListConfigMapKey(): `labels:
- cloud
- vendor

blacklist_label:
- clusterID
- name
- local-cluster
`}}
}

func TestGetManagedClusterLabelAllowListConfigMapKey(t *testing.T) {
	cmKey := GetManagedClusterLabelAllowListConfigMapKey()

	if cmKey != ManagedClusterLabelAllowListConfigMapKey {
		t.Errorf("ManagedCluster ConfigMap key (%v) is not the expected (%v)", cmKey, ManagedClusterLabelAllowListConfigMapKey)
	}
}

func TestGetManagedClusterLabelAllowListConfigMapName(t *testing.T) {
	cmName := GetManagedClusterLabelAllowListConfigMapName()

	if cmName != ManagedClusterLabelAllowListConfigMapName {
		t.Errorf("ManagedCluster ConfigMap name (%v) is not the expected (%v)", cmName, ManagedClusterLabelAllowListConfigMapName)
	}
}

func TestGetManagedClusterLabelMetricName(t *testing.T) {
	metricName := GetManagedClusterLabelMetricName()

	if metricName != ManagedClusterLabelMetricName {
		t.Errorf("ManagedCluster label metric name (%v) is not the expected (%v)", metricName, ManagedClusterLabelMetricName)
	}
}

func TestGetManagedClusterLabelList(t *testing.T) {
	managedLabelList := GetManagedClusterLabelList()
	cm := CreateManagedClusterLabelAllowListCM()

	err := yaml.Unmarshal([]byte(cm.Data[GetManagedClusterLabelAllowListConfigMapKey()]), managedLabelList)
	if err != nil {
		t.Errorf("Failed to unmarshal configmap: %s data to the managedLabelList", GetManagedClusterLabelAllowListConfigMapName())
	}
}

func TestGetRBACProxyLabelMetricName(t *testing.T) {
	metricName := GetRBACProxyLabelMetricName()

	if metricName != RBACProxyLabelMetricName {
		t.Errorf("ManagedCluster Config Map key (%v) is not the expected (%v)", metricName, RBACProxyLabelMetricName)
	}
}
