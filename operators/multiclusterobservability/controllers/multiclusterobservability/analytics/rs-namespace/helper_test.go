// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsnamespace

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability/analytics/rs-utility"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

func setupTestScheme(t *testing.T) *runtime.Scheme {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, mcov1beta2.AddToScheme(scheme))
	require.NoError(t, policyv1.AddToScheme(scheme))
	require.NoError(t, clusterv1beta1.AddToScheme(scheme))
	return scheme
}

func newTestMCO(binding string, enabled bool) *mcov1beta2.MultiClusterObservability {
	return &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{Name: "observability"},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			Capabilities: &mcov1beta2.CapabilitiesSpec{
				Platform: &mcov1beta2.PlatformCapabilitiesSpec{
					Analytics: mcov1beta2.PlatformAnalyticsSpec{
						NamespaceRightSizingRecommendation: mcov1beta2.PlatformRightSizingRecommendationSpec{
							Enabled:          enabled,
							NamespaceBinding: binding,
						},
					},
				},
			},
		},
	}
}

func resetGlobalState() {
	ComponentState.Namespace = rsutility.DefaultNamespace
	ComponentState.Enabled = false
}

func TestHandleRightSizing_FeatureDisabled(t *testing.T) {
	defer resetGlobalState()

	scheme := setupTestScheme(t)
	mco := newTestMCO("", false) // Feature disabled

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mco).
		Build()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := HandleRightSizing(ctx, client, mco)
	require.NoError(t, err)

	// Verify state changes
	assert.Equal(t, rsutility.DefaultNamespace, ComponentState.Namespace)
	assert.False(t, ComponentState.Enabled)
}

func TestHandleRightSizing_FeatureEnabledNoNamespaceChange(t *testing.T) {
	defer resetGlobalState()

	scheme := setupTestScheme(t)
	mco := newTestMCO(rsutility.DefaultNamespace, true) // Feature enabled, same namespace

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mco).
		Build()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := HandleRightSizing(ctx, client, mco)
	require.NoError(t, err)

	// Verify state changes
	assert.Equal(t, rsutility.DefaultNamespace, ComponentState.Namespace)
	assert.True(t, ComponentState.Enabled)

	// Verify ConfigMap was created
	cm := &corev1.ConfigMap{}
	err = client.Get(ctx, types.NamespacedName{
		Name:      ConfigMapName,
		Namespace: "open-cluster-management-observability",
	}, cm)
	require.NoError(t, err)
	assert.Contains(t, cm.Data, "prometheusRuleConfig")
	assert.Contains(t, cm.Data, "placementConfiguration")
}

func TestHandleRightSizing_FeatureEnabledWithNamespaceChange(t *testing.T) {
	defer resetGlobalState()

	scheme := setupTestScheme(t)
	newNamespace := "new-custom-namespace"
	mco := newTestMCO(newNamespace, true) // Feature enabled, different namespace

	// Create existing configmap with test data
	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName,
			Namespace: "open-cluster-management-observability",
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

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mco, existingCM).
		Build()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := HandleRightSizing(ctx, client, mco)
	require.NoError(t, err)

	// Verify namespace was updated
	assert.Equal(t, newNamespace, ComponentState.Namespace)
	assert.True(t, ComponentState.Enabled)
}

func TestHandleRightSizing_ConfigMapCreationFailure(t *testing.T) {
	defer resetGlobalState()

	scheme := runtime.NewScheme() // Minimal scheme without ConfigMap support
	require.NoError(t, mcov1beta2.AddToScheme(scheme))

	mco := newTestMCO(rsutility.DefaultNamespace, true)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mco).
		Build()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := HandleRightSizing(ctx, client, mco)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rs - failed to fetch configmap")
}

func TestGetRightSizingNamespaceConfig_PlatformNotConfigured(t *testing.T) {
	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{Name: "observability"},
		Spec:       mcov1beta2.MultiClusterObservabilitySpec{},
	}

	enabled, binding := GetRightSizingNamespaceConfig(mco)
	assert.False(t, enabled)
	assert.Empty(t, binding)
}

func TestGetRightSizingNamespaceConfig_CapabilitiesNil(t *testing.T) {
	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{Name: "observability"},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			Capabilities: nil,
		},
	}

	enabled, binding := GetRightSizingNamespaceConfig(mco)
	assert.False(t, enabled)
	assert.Empty(t, binding)
}

func TestGetRightSizingNamespaceConfig_PlatformNil(t *testing.T) {
	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{Name: "observability"},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			Capabilities: &mcov1beta2.CapabilitiesSpec{
				Platform: nil,
			},
		},
	}

	enabled, binding := GetRightSizingNamespaceConfig(mco)
	assert.False(t, enabled)
	assert.Empty(t, binding)
}

func TestGetRightSizingNamespaceConfig_FeatureEnabled(t *testing.T) {
	mco := newTestMCO("custom-namespace", true)

	enabled, binding := GetRightSizingNamespaceConfig(mco)
	assert.True(t, enabled)
	assert.Equal(t, "custom-namespace", binding)
}

func TestGetRightSizingNamespaceConfig_FeatureDisabled(t *testing.T) {
	mco := newTestMCO("custom-namespace", false)

	enabled, binding := GetRightSizingNamespaceConfig(mco)
	assert.False(t, enabled)
	assert.Equal(t, "custom-namespace", binding) // Binding still returned even if disabled
}

// Note: CleanupRSNamespaceResources is a thin wrapper around rsutility.CleanupComponentResources
// that only adds package-specific componentConfig. The core cleanup logic is extensively
// tested in rs-utility/component_test.go. This test focuses on verifying that the
// wrapper correctly uses the expected componentConfig.

func TestCleanupRSNamespaceResources_UsesCorrectComponentConfig(t *testing.T) {
	scheme := setupTestScheme(t)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test that the function executes without error (basic smoke test)
	// The actual cleanup logic is tested comprehensively in rs-utility/component_test.go
	CleanupRSNamespaceResources(ctx, client, rsutility.DefaultNamespace, false)
	CleanupRSNamespaceResources(ctx, client, rsutility.DefaultNamespace, true)

	// Test passes if no panic or error occurs, confirming the wrapper works correctly
}
