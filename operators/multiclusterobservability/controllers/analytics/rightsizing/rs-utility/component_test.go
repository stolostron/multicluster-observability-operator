// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsutility

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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

func setupComponentTestScheme(t *testing.T) *runtime.Scheme {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, mcov1beta2.AddToScheme(scheme))
	require.NoError(t, policyv1.AddToScheme(scheme))
	require.NoError(t, clusterv1beta1.AddToScheme(scheme))
	return scheme
}

func newTestMCOForComponent(componentType ComponentType, binding string, enabled bool) *mcov1beta2.MultiClusterObservability {
	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{Name: "observability"},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			Capabilities: &mcov1beta2.CapabilitiesSpec{
				Platform: &mcov1beta2.PlatformCapabilitiesSpec{
					Analytics: mcov1beta2.PlatformAnalyticsSpec{},
				},
			},
		},
	}

	switch componentType {
	case ComponentTypeNamespace:
		mco.Spec.Capabilities.Platform.Analytics.NamespaceRightSizingRecommendation = mcov1beta2.PlatformRightSizingRecommendationSpec{
			Enabled:          enabled,
			NamespaceBinding: binding,
		}
	case ComponentTypeVirtualization:
		mco.Spec.Capabilities.Platform.Analytics.VirtualizationRightSizingRecommendation = mcov1beta2.PlatformRightSizingRecommendationSpec{
			Enabled:          enabled,
			NamespaceBinding: binding,
		}
	}

	return mco
}

func mockApplyChangesFunc(ctx context.Context, c client.Client, configData RSNamespaceConfigMapData) error {
	return nil
}

func mockGetDefaultConfigFunc() map[string]string {
	return map[string]string{
		"prometheusRuleConfig":   "test-rule-config",
		"placementConfiguration": "test-placement-config",
	}
}

func TestGetComponentConfig_Namespace(t *testing.T) {
	mco := newTestMCOForComponent(ComponentTypeNamespace, "custom-namespace", true)

	enabled, binding, err := GetComponentConfig(mco, ComponentTypeNamespace)
	require.NoError(t, err)
	assert.True(t, enabled)
	assert.Equal(t, "custom-namespace", binding)
}

func TestGetComponentConfig_Virtualization(t *testing.T) {
	mco := newTestMCOForComponent(ComponentTypeVirtualization, "virt-namespace", false)

	enabled, binding, err := GetComponentConfig(mco, ComponentTypeVirtualization)
	require.NoError(t, err)
	assert.False(t, enabled)
	assert.Equal(t, "virt-namespace", binding)
}

func TestGetComponentConfig_PlatformNotConfigured(t *testing.T) {
	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{Name: "observability"},
		Spec:       mcov1beta2.MultiClusterObservabilitySpec{},
	}

	enabled, binding, err := GetComponentConfig(mco, ComponentTypeNamespace)
	require.NoError(t, err)
	assert.False(t, enabled)
	assert.Empty(t, binding)
}

func TestGetComponentConfig_UnknownType(t *testing.T) {
	mco := newTestMCOForComponent(ComponentTypeNamespace, "test", true)

	_, _, err := GetComponentConfig(mco, ComponentType("unknown"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown component type")
}

func TestHandleComponentRightSizing_FeatureDisabled(t *testing.T) {
	scheme := setupComponentTestScheme(t)
	mco := newTestMCOForComponent(ComponentTypeNamespace, "", false)

	componentConfig := ComponentConfig{
		ComponentType:            ComponentTypeNamespace,
		ConfigMapName:            "test-config",
		PlacementName:            "test-placement",
		PlacementBindingName:     "test-binding",
		PrometheusRulePolicyName: "test-policy",
		DefaultNamespace:         "default-ns",
		GetDefaultConfigFunc:     mockGetDefaultConfigFunc,
		ApplyChangesFunc:         mockApplyChangesFunc,
	}

	state := &ComponentState{
		Namespace: "old-namespace",
		Enabled:   true,
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mco).
		Build()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := HandleComponentRightSizing(ctx, client, mco, componentConfig, state)
	require.NoError(t, err)

	// Verify state changes
	assert.Equal(t, "default-ns", state.Namespace)
	assert.False(t, state.Enabled)
}

func TestHandleComponentRightSizing_FeatureEnabled(t *testing.T) {
	scheme := setupComponentTestScheme(t)
	mco := newTestMCOForComponent(ComponentTypeNamespace, "custom-ns", true)

	componentConfig := ComponentConfig{
		ComponentType:            ComponentTypeNamespace,
		ConfigMapName:            "test-config",
		PlacementName:            "test-placement",
		PlacementBindingName:     "test-binding",
		PrometheusRulePolicyName: "test-policy",
		DefaultNamespace:         "default-ns",
		GetDefaultConfigFunc:     mockGetDefaultConfigFunc,
		ApplyChangesFunc:         mockApplyChangesFunc,
	}

	state := &ComponentState{
		Namespace: "default-ns",
		Enabled:   false,
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mco).
		Build()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := HandleComponentRightSizing(ctx, client, mco, componentConfig, state)
	require.NoError(t, err)

	// Verify state changes
	assert.Equal(t, "custom-ns", state.Namespace)
	assert.True(t, state.Enabled)

	// Verify ConfigMap was created
	cm := &corev1.ConfigMap{}
	err = client.Get(ctx, types.NamespacedName{
		Name:      "test-config",
		Namespace: "open-cluster-management-observability",
	}, cm)
	require.NoError(t, err)
	assert.Contains(t, cm.Data, "prometheusRuleConfig")
}

func TestCleanupComponentResources_WithConfigMap(t *testing.T) {
	scheme := setupComponentTestScheme(t)

	componentConfig := ComponentConfig{
		ComponentType:            ComponentTypeNamespace,
		ConfigMapName:            "test-config",
		PlacementName:            "test-placement",
		PlacementBindingName:     "test-binding",
		PrometheusRulePolicyName: "test-policy",
		DefaultNamespace:         "test-ns",
	}

	// Create resources to be cleaned up
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-config",
			Namespace: "open-cluster-management-observability",
		},
	}

	placement := &clusterv1beta1.Placement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-placement",
			Namespace: "test-ns",
		},
	}

	placementBinding := &policyv1.PlacementBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-binding",
			Namespace: "test-ns",
		},
	}

	policy := &policyv1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "test-ns",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(configMap, placement, placementBinding, policy).
		Build()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test cleanup with bindingUpdated=false (should delete all resources including configmap)
	CleanupComponentResources(ctx, client, componentConfig, "test-ns", false)

	// Verify all resources were deleted
	err := client.Get(ctx, types.NamespacedName{Name: "test-config", Namespace: "open-cluster-management-observability"}, &corev1.ConfigMap{})
	assert.True(t, errors.IsNotFound(err))

	err = client.Get(ctx, types.NamespacedName{Name: "test-placement", Namespace: "test-ns"}, &clusterv1beta1.Placement{})
	assert.True(t, errors.IsNotFound(err))

	err = client.Get(ctx, types.NamespacedName{Name: "test-binding", Namespace: "test-ns"}, &policyv1.PlacementBinding{})
	assert.True(t, errors.IsNotFound(err))

	err = client.Get(ctx, types.NamespacedName{Name: "test-policy", Namespace: "test-ns"}, &policyv1.Policy{})
	assert.True(t, errors.IsNotFound(err))
}

func TestCleanupComponentResources_WithoutConfigMap(t *testing.T) {
	scheme := setupComponentTestScheme(t)

	componentConfig := ComponentConfig{
		ComponentType:            ComponentTypeVirtualization,
		ConfigMapName:            "test-config",
		PlacementName:            "test-placement",
		PlacementBindingName:     "test-binding",
		PrometheusRulePolicyName: "test-policy",
		DefaultNamespace:         "test-ns",
	}

	// Create resources to be cleaned up
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-config",
			Namespace: "open-cluster-management-observability",
		},
	}

	placement := &clusterv1beta1.Placement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-placement",
			Namespace: "test-ns",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(configMap, placement).
		Build()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test cleanup with bindingUpdated=true (should not delete configmap)
	CleanupComponentResources(ctx, client, componentConfig, "test-ns", true)

	// Verify configmap was not deleted
	err := client.Get(ctx, types.NamespacedName{Name: "test-config", Namespace: "open-cluster-management-observability"}, &corev1.ConfigMap{})
	assert.NoError(t, err) // ConfigMap should still exist

	// Verify other resources were deleted
	err = client.Get(ctx, types.NamespacedName{Name: "test-placement", Namespace: "test-ns"}, &clusterv1beta1.Placement{})
	assert.True(t, errors.IsNotFound(err))
}
