// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

import (
	"context"
	"testing"

	"gopkg.in/yaml.v2"

	corev1 "k8s.io/api/core/v1"
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

func TestGetManagedClusterLabelAllowListConfigmap(t *testing.T) {
	testCase := struct {
		name      string
		namespace string
		expected  error
	}{"should get managedclsuter label allowlist configmap", "ns1", nil}

	client := fake.NewSimpleClientset().CoreV1()
	_, err := client.ConfigMaps(testCase.namespace).Create(
		context.TODO(),
		CreateManagedClusterLabelAllowListCM(),
		metav1.CreateOptions{},
	)
	if err != nil {
		t.Errorf("failed to create configmap: %v", err)
	}

	_, err = GetManagedClusterLabelAllowListConfigmap(client, testCase.namespace)
	if err != nil {
		t.Errorf("case (%v) output: (%v) is not the expected: (%v)", testCase.name, err, testCase.expected)
	}

	testCase.name = "should not get managedcluster label allowlist configmap"
	testCase.namespace = "ns2"

	_, err = GetManagedClusterLabelAllowListConfigmap(client, "ns2")
	if err == nil {
		t.Errorf("case (%v) output: (%v) is not the expected: (%v)", testCase.name, nil, err)
	}
}

func TestModifyManagedClusterLabelAllowListConfigMapData(t *testing.T) {
	testCase := struct {
		name      string
		configmap *corev1.ConfigMap
		expected  error
	}{
		"should modify managedcluster label allowlist data",
		CreateManagedClusterLabelAllowListCM(),
		nil,
	}

	clusterLabels := map[string]string{"environment": "dev", "department": "finance"}
	err := ModifyManagedClusterLabelAllowListConfigMapData(testCase.configmap, clusterLabels)
	if err != nil {
		t.Errorf("case: (%v) output: (%v) is not the expected (%v)", testCase.name, err, testCase.expected)
	}

	testCase.configmap.Data[GetManagedClusterLabelAllowListConfigMapKey()] += `
labels:
- app
	- source
`

	err = ModifyManagedClusterLabelAllowListConfigMapData(testCase.configmap, clusterLabels)
	if err == nil {
		t.Errorf("case: (%v) output: (%v) is not the expected (%v)", testCase.name, nil, err)
	}
}

func TestUpdateManagedClusterLabelAllowListConfigMap(t *testing.T) {
	testCase := struct {
		name      string
		namespace string
		expected  error
	}{"should update the managedcluster label allowlist data", "ns1", nil}
	cm := CreateManagedClusterLabelAllowListCM()

	client := fake.NewSimpleClientset().CoreV1()
	_, err := client.ConfigMaps("ns1").Create(context.TODO(), cm, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("failed to create configmap: %v", err)
	}

	err = UpdateManagedClusterLabelAllowListConfigMap(client, testCase.namespace, cm)
	if err != nil {
		t.Errorf("case: (%v) output: (%v) is not the expected (%v)", testCase.name, err, testCase.expected)
	}

	testCase.name = "should not update managedcluster label allowlist configmap"
	testCase.namespace = "ns2"

	err = UpdateManagedClusterLabelAllowListConfigMap(client, testCase.namespace, cm)
	if err == nil {
		t.Errorf("case: (%v) output: (%v) is not the expected (%v)", testCase.name, nil, err)
	}
}
