// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsvirtualization

import (
	"context"
	"testing"

	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability/analytics/rs-utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const testNamespace = "open-cluster-management-observability"

func TestEnsureRSVirtualizationConfigMapExists_CreatesIfNotExists(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	err := EnsureRSVirtualizationConfigMapExists(context.TODO(), client)
	require.NoError(t, err)

	// Verify ConfigMap was created
	fetchedCM := &corev1.ConfigMap{}
	err = client.Get(context.TODO(), types.NamespacedName{
		Name:      ConfigMapName,
		Namespace: testNamespace,
	}, fetchedCM)

	require.NoError(t, err)
	assert.Equal(t, ConfigMapName, fetchedCM.Name)
	assert.Equal(t, testNamespace, fetchedCM.Namespace)
	assert.Contains(t, fetchedCM.Data, "prometheusRuleConfig")
	assert.Contains(t, fetchedCM.Data, "placementConfiguration")
}

func TestEnsureRSVirtualizationConfigMapExists_SkipsIfExists(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName,
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

	err := EnsureRSVirtualizationConfigMapExists(context.TODO(), client)
	require.NoError(t, err)

	// Verify existing data is preserved
	fetchedCM := &corev1.ConfigMap{}
	err = client.Get(context.TODO(), types.NamespacedName{
		Name:      ConfigMapName,
		Namespace: testNamespace,
	}, fetchedCM)

	require.NoError(t, err)
	assert.Equal(t, "data", fetchedCM.Data["existing"])
}

func TestGetDefaultRSVirtualizationConfig(t *testing.T) {
	config := GetDefaultRSVirtualizationConfig()

	assert.NotNil(t, config)
	assert.Contains(t, config, "prometheusRuleConfig")
	assert.Contains(t, config, "placementConfiguration")
	assert.NotEmpty(t, config["prometheusRuleConfig"])
	assert.NotEmpty(t, config["placementConfiguration"])
}

func TestGetRightSizingVirtualizationConfigData_Success(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName,
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

	configData, err := GetRightSizingVirtualizationConfigData(cm)
	require.NoError(t, err)
	assert.NotEmpty(t, configData.PrometheusRuleConfig.NamespaceFilterCriteria.ExclusionCriteria)
	assert.Equal(t, 110, configData.PrometheusRuleConfig.RecommendationPercentage)
}

func TestGetRightSizingVirtualizationConfigData_InvalidYAML(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName,
			Namespace: testNamespace,
		},
		Data: map[string]string{
			"prometheusRuleConfig": "invalid: yaml: content:",
		},
	}

	_, err := GetRightSizingVirtualizationConfigData(cm)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal prometheusRuleConfig")
}

func TestDefaultRecommendationPercentage(t *testing.T) {
	// Test that the default recommendation percentage from rs-utility is used
	config := rsutility.GetDefaultRSPrometheusRuleConfig()
	assert.Equal(t, rsutility.DefaultRecommendationPercentage, config.RecommendationPercentage)
	assert.Equal(t, 110, config.RecommendationPercentage)
}
