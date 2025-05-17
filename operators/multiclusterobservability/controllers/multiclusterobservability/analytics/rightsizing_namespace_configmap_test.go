// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics_test

import (
	"context"
	"testing"

	analyticsctrl "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability/analytics"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

const (
	rsConfigMapName = "rs-namespace-config"
	mockedNamespace = "open-cluster-management-observability" // Same namespace your real code uses
)

func TestEnsureRSNamespaceConfigMapExists_CreatesIfNotExists(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = policyv1.AddToScheme(scheme)
	_ = clusterv1beta1.AddToScheme(scheme)

	// Fake client initialized with the right scheme
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Call the real function
	err := analyticsctrl.EnsureRSNamespaceConfigMapExists(context.TODO(), client)
	assert.NoError(t, err)

	// Manually query in the correct namespace (hardcoded because config.GetDefaultNamespace() is not mockable)
	cm := &corev1.ConfigMap{}
	err = client.Get(context.TODO(), types.NamespacedName{
		Name:      rsConfigMapName,
		Namespace: mockedNamespace,
	}, cm)
	assert.NoError(t, err)

	// Check if Data is initialized
	if cm.Data == nil {
		t.Fatalf("Expected ConfigMap data to be initialized but got nil")
	}

	assert.Contains(t, cm.Data, "prometheusRuleConfig")
	assert.Contains(t, cm.Data, "placementConfiguration")
}

func TestGetRightSizingConfigData_Success(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = policyv1.AddToScheme(scheme)
	_ = clusterv1beta1.AddToScheme(scheme)

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Get default config data from the real code
	data := analyticsctrl.GetDefaultRSNamespaceConfig()

	// Create configmap manually in mocked namespace
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rsConfigMapName,
			Namespace: mockedNamespace,
		},
		Data: data,
	}

	// Save to fake client
	_ = client.Create(context.TODO(), cm)

	// Fetch again
	fetchedCM := &corev1.ConfigMap{}
	err := client.Get(context.TODO(), types.NamespacedName{
		Name:      rsConfigMapName,
		Namespace: mockedNamespace,
	}, fetchedCM)
	assert.NoError(t, err)

	configData, err := analyticsctrl.GetRightSizingConfigData(fetchedCM)
	assert.NoError(t, err)

	assert.NotEmpty(t, configData.PrometheusRuleConfig.NamespaceFilterCriteria.ExclusionCriteria)
	assert.NotNil(t, configData.PlacementConfiguration)
}

func TestGetRightSizingConfigData_InvalidYAML(t *testing.T) {
	// Simulate invalid YAML
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rsConfigMapName,
			Namespace: mockedNamespace,
		},
		Data: map[string]string{
			"prometheusRuleConfig":   ":invalid-yaml", // Invalid YAML
			"placementConfiguration": ":also-bad",     // Invalid YAML
		},
	}

	_, err := analyticsctrl.GetRightSizingConfigData(cm)
	assert.Error(t, err)
}
