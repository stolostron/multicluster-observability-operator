// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

import (
	"testing"

	"gopkg.in/yaml.v2"
)

func TestGetManagedClusterLabelAllowListConfigMapKey(t *testing.T) {
	cmKey := GetManagedClusterLabelAllowListConfigMapKey()

	if cmKey != ManagedClusterLabelAllowListConfigMapKey {
		t.Errorf("managedcluster configmap key (%v) is not the expected (%v)", cmKey, ManagedClusterLabelAllowListConfigMapKey)
	}
}

func TestGetManagedClusterLabelAllowListConfigMapName(t *testing.T) {
	cmName := GetManagedClusterLabelAllowListConfigMapName()

	if cmName != ManagedClusterLabelAllowListConfigMapName {
		t.Errorf("managedcluster configmap name (%v) is not the expected (%v)", cmName, ManagedClusterLabelAllowListConfigMapName)
	}
}

func TestGetManagedClusterLabelMetricName(t *testing.T) {
	metricName := GetManagedClusterLabelMetricName()

	if metricName != ManagedClusterLabelMetricName {
		t.Errorf("managedcluster label metric name (%v) is not the expected (%v)", metricName, ManagedClusterLabelMetricName)
	}
}

func TestGetManagedClusterLabelList(t *testing.T) {
	managedLabelList := GetManagedClusterLabelList()
	cm := CreateManagedClusterLabelAllowListCM()

	err := yaml.Unmarshal([]byte(cm.Data[GetManagedClusterLabelAllowListConfigMapKey()]), managedLabelList)
	if err != nil {
		t.Errorf("failed to unmarshal configmap: %s data to the managedLabelList", GetManagedClusterLabelAllowListConfigMapName())
	}
}

func TestGetRBACProxyLabelMetricName(t *testing.T) {
	metricName := GetRBACProxyLabelMetricName()

	if metricName != RBACProxyLabelMetricName {
		t.Errorf("managedcluster configmap key (%v) is not the expected (%v)", metricName, RBACProxyLabelMetricName)
	}
}
