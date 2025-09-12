// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsutility

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const testNamespace = "open-cluster-management-observability"
const testConfigMapName = "test-rs-config"

func TestEnsureRSConfigMapExists_CreatesIfNotExists(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	getDefaultData := func() map[string]string {
		return map[string]string{
			"test": "data",
		}
	}

	err := EnsureRSConfigMapExists(context.TODO(), client, testConfigMapName, getDefaultData)
	require.NoError(t, err)

	// Verify ConfigMap was created
	fetchedCM := &corev1.ConfigMap{}
	err = client.Get(context.TODO(), types.NamespacedName{
		Name:      testConfigMapName,
		Namespace: testNamespace,
	}, fetchedCM)

	require.NoError(t, err)
	assert.Equal(t, testConfigMapName, fetchedCM.Name)
	assert.Equal(t, testNamespace, fetchedCM.Namespace)
	assert.Equal(t, "data", fetchedCM.Data["test"])
}

func TestEnsureRSConfigMapExists_SkipsIfExists(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testConfigMapName,
			Namespace: testNamespace,
		},
		Data: map[string]string{
			"existing": "data",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existingCM).
		Build()

	getDefaultData := func() map[string]string {
		return map[string]string{
			"test": "data",
		}
	}

	err := EnsureRSConfigMapExists(context.TODO(), client, testConfigMapName, getDefaultData)
	require.NoError(t, err)

	// Verify existing data is preserved
	fetchedCM := &corev1.ConfigMap{}
	err = client.Get(context.TODO(), types.NamespacedName{
		Name:      testConfigMapName,
		Namespace: testNamespace,
	}, fetchedCM)

	require.NoError(t, err)
	assert.Equal(t, "data", fetchedCM.Data["existing"])
	// Should not have new data since it already existed
	assert.NotContains(t, fetchedCM.Data, "test")
}

func TestGetRSConfigData_Success(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testConfigMapName,
			Namespace: testNamespace,
		},
		Data: map[string]string{
			"prometheusRuleConfig": `
namespaceFilterCriteria:
  exclusionCriteria:
    - "openshift.*"
recommendationPercentage: 110
`,
			"placementConfiguration": `
spec:
  predicates: []
`,
		},
	}

	configData, err := GetRSConfigData(cm)
	require.NoError(t, err)
	assert.NotEmpty(t, configData.PrometheusRuleConfig.NamespaceFilterCriteria.ExclusionCriteria)
	assert.Equal(t, 110, configData.PrometheusRuleConfig.RecommendationPercentage)
}

func TestGetRSConfigData_InvalidYAML(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testConfigMapName,
			Namespace: testNamespace,
		},
		Data: map[string]string{
			"prometheusRuleConfig": "invalid: yaml: content:",
		},
	}

	_, err := GetRSConfigData(cm)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal prometheusRuleConfig")
}
