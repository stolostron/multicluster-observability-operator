// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsvirtualization

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
						VirtualizationRightSizingRecommendation: mcov1beta2.PlatformRightSizingRecommendationSpec{
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
	Namespace = rsutility.DefaultNamespace
	Enabled = false
}

func TestHandleRightSizing_FeatureDisabled(t *testing.T) {
	defer resetGlobalState()

	scheme := setupTestScheme(t)
	mco := newTestMCO("", false) // Feature disabled

	// Set initial state
	Namespace = "custom-namespace"
	Enabled = true

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mco).
		Build()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := HandleRightSizing(ctx, client, mco)
	require.NoError(t, err)

	// Verify state changes
	assert.Equal(t, rsutility.DefaultNamespace, Namespace)
	assert.False(t, Enabled)
}

func TestHandleRightSizing_FeatureEnabledNoNamespaceChange(t *testing.T) {
	defer resetGlobalState()

	scheme := setupTestScheme(t)
	mco := newTestMCO(rsutility.DefaultNamespace, true) // Feature enabled, same namespace

	// Set initial state
	Namespace = rsutility.DefaultNamespace
	Enabled = false

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mco).
		Build()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := HandleRightSizing(ctx, client, mco)
	require.NoError(t, err)

	// Verify state changes
	assert.Equal(t, rsutility.DefaultNamespace, Namespace)
	assert.True(t, Enabled)

	// Verify ConfigMap was created
	cm := &corev1.ConfigMap{}
	err = client.Get(ctx, types.NamespacedName{
		Name:      ConfigMapName,
		Namespace: "open-cluster-management-observability", // config.GetDefaultNamespace()
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

	// Set initial state to simulate existing deployment
	Namespace = rsutility.DefaultNamespace
	Enabled = true

	// Create existing configmap with test data
	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName,
			Namespace: "open-cluster-management-observability", // config.GetDefaultNamespace()
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
	assert.Equal(t, newNamespace, Namespace)
	assert.True(t, Enabled)
}

func TestGetRightSizingVirtualizationConfig_PlatformNotConfigured(t *testing.T) {
	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{Name: "observability"},
		Spec:       mcov1beta2.MultiClusterObservabilitySpec{},
	}

	enabled, binding := GetRightSizingVirtualizationConfig(mco)
	assert.False(t, enabled)
	assert.Empty(t, binding)
}

func TestGetRightSizingVirtualizationConfig_CapabilitiesNil(t *testing.T) {
	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{Name: "observability"},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			Capabilities: nil,
		},
	}

	enabled, binding := GetRightSizingVirtualizationConfig(mco)
	assert.False(t, enabled)
	assert.Empty(t, binding)
}

func TestGetRightSizingVirtualizationConfig_PlatformNil(t *testing.T) {
	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{Name: "observability"},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			Capabilities: &mcov1beta2.CapabilitiesSpec{
				Platform: nil,
			},
		},
	}

	enabled, binding := GetRightSizingVirtualizationConfig(mco)
	assert.False(t, enabled)
	assert.Empty(t, binding)
}

func TestGetRightSizingVirtualizationConfig_FeatureEnabled(t *testing.T) {
	mco := newTestMCO("custom-namespace", true)

	enabled, binding := GetRightSizingVirtualizationConfig(mco)
	assert.True(t, enabled)
	assert.Equal(t, "custom-namespace", binding)
}

func TestGetRightSizingVirtualizationConfig_FeatureDisabled(t *testing.T) {
	mco := newTestMCO("custom-namespace", false)

	enabled, binding := GetRightSizingVirtualizationConfig(mco)
	assert.False(t, enabled)
	assert.Equal(t, "custom-namespace", binding) // Binding still returned even if disabled
}

func TestCleanupRSVirtualizationResources_WithoutConfigMap(t *testing.T) {
	scheme := setupTestScheme(t)

	// Create existing resources that should be cleaned up
	placement := &clusterv1beta1.Placement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PlacementName,
			Namespace: rsutility.DefaultNamespace,
		},
	}

	placementBinding := &policyv1.PlacementBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PlacementBindingName,
			Namespace: rsutility.DefaultNamespace,
		},
	}

	policy := &policyv1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PrometheusRulePolicyName,
			Namespace: rsutility.DefaultNamespace,
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(placement, placementBinding, policy).
		Build()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test cleanup with bindingUpdated=true (should not delete configmap)
	CleanupRSVirtualizationResources(ctx, client, rsutility.DefaultNamespace, true)

	// Verify resources were deleted
	err := client.Get(ctx, types.NamespacedName{Name: PlacementName, Namespace: rsutility.DefaultNamespace}, &clusterv1beta1.Placement{})
	assert.True(t, errors.IsNotFound(err))

	err = client.Get(ctx, types.NamespacedName{Name: PlacementBindingName, Namespace: rsutility.DefaultNamespace}, &policyv1.PlacementBinding{})
	assert.True(t, errors.IsNotFound(err))

	err = client.Get(ctx, types.NamespacedName{Name: PrometheusRulePolicyName, Namespace: rsutility.DefaultNamespace}, &policyv1.Policy{})
	assert.True(t, errors.IsNotFound(err))
}

func TestCleanupRSVirtualizationResources_WithConfigMap(t *testing.T) {
	scheme := setupTestScheme(t)

	// Create existing resources including configmap
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName,
			Namespace: "open-cluster-management-observability", // config.GetDefaultNamespace()
		},
	}

	placement := &clusterv1beta1.Placement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PlacementName,
			Namespace: rsutility.DefaultNamespace,
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(configMap, placement).
		Build()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test cleanup with bindingUpdated=false (should delete configmap too)
	CleanupRSVirtualizationResources(ctx, client, rsutility.DefaultNamespace, false)

	// Verify configmap was deleted
	err := client.Get(ctx, types.NamespacedName{Name: ConfigMapName, Namespace: "open-cluster-management-observability"}, &corev1.ConfigMap{})
	assert.True(t, errors.IsNotFound(err))

	// Verify other resources were deleted
	err = client.Get(ctx, types.NamespacedName{Name: PlacementName, Namespace: rsutility.DefaultNamespace}, &clusterv1beta1.Placement{})
	assert.True(t, errors.IsNotFound(err))
}

func TestCleanupRSVirtualizationResources_ResourceNotFound(t *testing.T) {
	scheme := setupTestScheme(t)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Should not error when resources don't exist
	CleanupRSVirtualizationResources(ctx, client, rsutility.DefaultNamespace, false)
	// Test passes if no panic or error occurs
}
