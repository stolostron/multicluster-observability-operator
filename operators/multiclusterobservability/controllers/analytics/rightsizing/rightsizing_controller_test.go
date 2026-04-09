// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rightsizing

import (
	"context"
	"testing"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	rsnamespace "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing/rs-namespace"
	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing/rs-utility"
	rsvirtualization "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing/rs-virtualization"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

func TestCreateRightSizingComponent_FeatureEnabled(t *testing.T) {
	scheme := setupTestScheme(t)

	mco := newTestMCO("custom-ns", true)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rsnamespace.ConfigMapName,
			Namespace: rsutility.DefaultNamespace,
		},
		Data: map[string]string{
			"config.yaml": `
				prometheusRuleConfig:
				namespaceFilterCriteria:
					inclusionCriteria: ["ns1"]
					exclusionCriteria: []
				labelFilterCriteria: []
				recommendationPercentage: 110
				placementConfiguration:
				predicates: []
				`,
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mco, configMap).
		Build()

	err := CreateRightSizingComponent(context.TODO(), client, mco)
	require.NoError(t, err)
}

func TestCleanupRightSizingResources_DefaultNamespace(t *testing.T) {
	scheme := setupTestScheme(t)

	ns := rsutility.DefaultNamespace
	mco := newTestMCO("", true) // No custom binding, uses default namespace

	// Pre-create rightsizing resources that should be cleaned up
	existingResources := []runtime.Object{
		&policyv1.Policy{
			ObjectMeta: metav1.ObjectMeta{Name: rsnamespace.PrometheusRulePolicyName, Namespace: ns},
		},
		&clusterv1beta1.Placement{
			ObjectMeta: metav1.ObjectMeta{Name: rsnamespace.PlacementName, Namespace: ns},
		},
		&policyv1.PlacementBinding{
			ObjectMeta: metav1.ObjectMeta{Name: rsnamespace.PlacementBindingName, Namespace: ns},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: rsnamespace.ConfigMapName, Namespace: config.GetDefaultNamespace()},
		},
		&policyv1.Policy{
			ObjectMeta: metav1.ObjectMeta{Name: rsvirtualization.PrometheusRulePolicyName, Namespace: ns},
		},
		&clusterv1beta1.Placement{
			ObjectMeta: metav1.ObjectMeta{Name: rsvirtualization.PlacementName, Namespace: ns},
		},
		&policyv1.PlacementBinding{
			ObjectMeta: metav1.ObjectMeta{Name: rsvirtualization.PlacementBindingName, Namespace: ns},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: rsvirtualization.ConfigMapName, Namespace: config.GetDefaultNamespace()},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(existingResources...).
		Build()

	// Ensure resources exist before cleanup
	policy := &policyv1.Policy{}
	require.NoError(t, client.Get(context.TODO(), keyFor(rsnamespace.PrometheusRulePolicyName, ns), policy))

	// Run cleanup
	err := CleanupRightSizingResources(context.TODO(), client, mco)
	require.NoError(t, err)

	// Verify all namespace RS resources are deleted (NotFound, not just any error)
	require.True(t, apierrors.IsNotFound(client.Get(context.TODO(), keyFor(rsnamespace.PrometheusRulePolicyName, ns), &policyv1.Policy{})))
	require.True(t, apierrors.IsNotFound(client.Get(context.TODO(), keyFor(rsnamespace.PlacementName, ns), &clusterv1beta1.Placement{})))
	require.True(t, apierrors.IsNotFound(client.Get(context.TODO(), keyFor(rsnamespace.PlacementBindingName, ns), &policyv1.PlacementBinding{})))
	require.True(t, apierrors.IsNotFound(client.Get(context.TODO(), keyFor(rsnamespace.ConfigMapName, config.GetDefaultNamespace()), &corev1.ConfigMap{})))

	// Verify all virtualization RS resources are deleted (NotFound, not just any error)
	require.True(t, apierrors.IsNotFound(client.Get(context.TODO(), keyFor(rsvirtualization.PrometheusRulePolicyName, ns), &policyv1.Policy{})))
	require.True(t, apierrors.IsNotFound(client.Get(context.TODO(), keyFor(rsvirtualization.PlacementName, ns), &clusterv1beta1.Placement{})))
	require.True(t, apierrors.IsNotFound(client.Get(context.TODO(), keyFor(rsvirtualization.PlacementBindingName, ns), &policyv1.PlacementBinding{})))
	require.True(t, apierrors.IsNotFound(client.Get(context.TODO(), keyFor(rsvirtualization.ConfigMapName, config.GetDefaultNamespace()), &corev1.ConfigMap{})))
}

func TestCleanupRightSizingResources_CustomNamespace(t *testing.T) {
	scheme := setupTestScheme(t)

	customNS := "custom-ns"
	mco := newTestMCO(customNS, true) // Custom binding

	// Resources are in the custom namespace (as they would be when created with custom binding)
	existingResources := []runtime.Object{
		&policyv1.Policy{
			ObjectMeta: metav1.ObjectMeta{Name: rsnamespace.PrometheusRulePolicyName, Namespace: customNS},
		},
		&clusterv1beta1.Placement{
			ObjectMeta: metav1.ObjectMeta{Name: rsnamespace.PlacementName, Namespace: customNS},
		},
		&policyv1.PlacementBinding{
			ObjectMeta: metav1.ObjectMeta{Name: rsnamespace.PlacementBindingName, Namespace: customNS},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: rsnamespace.ConfigMapName, Namespace: config.GetDefaultNamespace()},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(existingResources...).
		Build()

	// Ensure resources exist before cleanup
	require.NoError(t, client.Get(context.TODO(), keyFor(rsnamespace.PrometheusRulePolicyName, customNS), &policyv1.Policy{}))

	// Run cleanup — should use namespace from MCO spec, not ComponentState
	err := CleanupRightSizingResources(context.TODO(), client, mco)
	require.NoError(t, err)

	// Verify resources in custom namespace are deleted (NotFound, not just any error)
	require.True(t, apierrors.IsNotFound(client.Get(context.TODO(), keyFor(rsnamespace.PrometheusRulePolicyName, customNS), &policyv1.Policy{})))
	require.True(t, apierrors.IsNotFound(client.Get(context.TODO(), keyFor(rsnamespace.PlacementName, customNS), &clusterv1beta1.Placement{})))
	require.True(t, apierrors.IsNotFound(client.Get(context.TODO(), keyFor(rsnamespace.PlacementBindingName, customNS), &policyv1.PlacementBinding{})))
}

func keyFor(name, namespace string) client.ObjectKey {
	return client.ObjectKey{Name: name, Namespace: namespace}
}

func TestCreateRightSizingComponent_FeatureDisabled(t *testing.T) {
	scheme := setupTestScheme(t)

	mco := newTestMCO("", false)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mco).
		Build()

	err := CreateRightSizingComponent(context.TODO(), client, mco)
	require.NoError(t, err)
}
