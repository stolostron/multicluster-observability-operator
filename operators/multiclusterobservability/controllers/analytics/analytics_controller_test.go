// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics

import (
	"context"
	"testing"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	rsnamespace "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing/rs-namespace"
	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing/rs-utility"
	rsvirtualization "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing/rs-virtualization"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func setupTestScheme(t *testing.T) *runtime.Scheme {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, mcov1beta2.AddToScheme(scheme))
	require.NoError(t, clusterv1beta1.AddToScheme(scheme))
	require.NoError(t, policyv1.AddToScheme(scheme))
	return scheme
}

func newTestMCO(binding string, enabled bool, paused bool) *mcov1beta2.MultiClusterObservability {
	mco := &mcov1beta2.MultiClusterObservability{
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
	if paused {
		if mco.Annotations == nil {
			mco.Annotations = map[string]string{}
		}
		mco.Annotations["mco-pause"] = "true"
	}
	return mco
}

func TestEnsureRightSizingDefaultsAddsMissingFlags(t *testing.T) {
	scheme := setupTestScheme(t)
	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{Name: "observability"},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mco).
		Build()

	r := &AnalyticsReconciler{Client: c, Scheme: scheme}
	updated, err := r.ensureRightSizingDefaults(context.TODO(), mco.DeepCopy(), log)
	require.NoError(t, err)
	require.NotNil(t, updated.Spec.Capabilities)
	require.NotNil(t, updated.Spec.Capabilities.Platform)

	analytics := updated.Spec.Capabilities.Platform.Analytics
	require.True(t, analytics.NamespaceRightSizingRecommendation.Enabled)
	require.True(t, analytics.VirtualizationRightSizingRecommendation.Enabled)

	persisted := &mcov1beta2.MultiClusterObservability{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: mco.GetName()}, persisted)
	require.NoError(t, err)
	require.True(t, persisted.Spec.Capabilities.Platform.Analytics.NamespaceRightSizingRecommendation.Enabled)
	require.True(t, persisted.Spec.Capabilities.Platform.Analytics.VirtualizationRightSizingRecommendation.Enabled)
}

func TestAnalyticsReconciler_FeatureEnabled(t *testing.T) {
	scheme := setupTestScheme(t)

	mco := newTestMCO("custom-ns", true, false)

	// minimal required configmaps for RS paths used by analytics controller
	namespaceRSConfigMap := &corev1.ConfigMap{
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
	virtualizationRSConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rsvirtualization.ConfigMapName,
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

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mco, namespaceRSConfigMap, virtualizationRSConfigMap).
		Build()

	r := &AnalyticsReconciler{Client: c, Scheme: scheme}
	_, err := r.Reconcile(context.TODO(), ctrl.Request{})
	require.NoError(t, err)
}

func TestAnalyticsReconciler_FeatureDisabled(t *testing.T) {
	scheme := setupTestScheme(t)

	mco := newTestMCO("", false, false)

	// Provide RS configmaps so reconcile doesn't fail even if defaulting enables flags.
	namespaceRSConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rsnamespace.ConfigMapName,
			Namespace: rsutility.DefaultNamespace,
		},
		Data: map[string]string{"config.yaml": "test: true"},
	}
	virtualizationRSConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rsvirtualization.ConfigMapName,
			Namespace: rsutility.DefaultNamespace,
		},
		Data: map[string]string{"config.yaml": "test: true"},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mco, namespaceRSConfigMap, virtualizationRSConfigMap).
		Build()

	r := &AnalyticsReconciler{Client: c, Scheme: scheme}
	_, err := r.Reconcile(context.TODO(), ctrl.Request{})
	require.NoError(t, err)
}

func TestAnalyticsReconciler_PausedAnnotation(t *testing.T) {
	scheme := setupTestScheme(t)

	mco := newTestMCO("", true, true)

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mco).
		Build()

	r := &AnalyticsReconciler{Client: c, Scheme: scheme}
	_, err := r.Reconcile(context.TODO(), ctrl.Request{})
	require.NoError(t, err)
}
