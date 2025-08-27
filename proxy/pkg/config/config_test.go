// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package config

import (
	"context"
	"testing"

	"gopkg.in/yaml.v2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
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

func TestGetManagedClusterLabelList(t *testing.T) {
	managedLabelList := GetManagedClusterLabelList()
	cm := CreateManagedClusterLabelAllowListCM("ns1")

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

func TestGetManagedClusterLabelAllowListConfigmap(t *testing.T) {
	testCase := struct {
		name      string
		namespace string
		expected  error
	}{"should get managedclsuter label allowlist configmap", "ns1", nil}

	client := fake.NewSimpleClientset()
	_, err := client.CoreV1().ConfigMaps(testCase.namespace).Create(
		context.Background(),
		CreateManagedClusterLabelAllowListCM(testCase.namespace),
		metav1.CreateOptions{},
	)
	if err != nil {
		t.Errorf("failed to create configmap: %v", err)
	}

	_, err = GetManagedClusterLabelAllowListConfigmap(context.Background(), client, testCase.namespace)
	if err != nil {
		t.Errorf("case (%v) output: (%v) is not the expected: (%v)", testCase.name, err, testCase.expected)
	}

	testCase.name = "should not get managedcluster label allowlist configmap"
	testCase.namespace = "ns2"

	_, err = GetManagedClusterLabelAllowListConfigmap(context.Background(), client, "ns2")
	if err == nil {
		t.Errorf("case (%v) output: (%v) is not the expected: (%v)", testCase.name, nil, err)
	}
}

func TestGetSyncLabelList(t *testing.T) {
	syncLabelList := GetSyncLabelList()
	cm := CreateManagedClusterLabelAllowListCM("ns1")

	err := yaml.Unmarshal([]byte(cm.Data[GetManagedClusterLabelAllowListConfigMapKey()]), syncLabelList)
	if err != nil {
		t.Errorf("failed to unmarshal configmap: %s data to the syncLabelList",
			GetManagedClusterLabelAllowListConfigMapName())
	}
}
