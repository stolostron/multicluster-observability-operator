// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics

import (
	"context"
	"testing"
	"time"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	rsnamespace "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing/rs-namespace"
	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing/rs-utility"
	rsvirtualization "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing/rs-virtualization"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	addonv1beta1 "open-cluster-management.io/api/addon/v1beta1"
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
	require.NoError(t, addonv1alpha1.AddToScheme(scheme))
	require.NoError(t, addonv1beta1.AddToScheme(scheme))
	require.NoError(t, monitoringv1.AddToScheme(scheme))
	return scheme
}

func newTestMCOWithBothRS(nsEnabled, virtEnabled bool) *mcov1beta2.MultiClusterObservability {
	return &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{Name: "observability"},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			Capabilities: &mcov1beta2.CapabilitiesSpec{
				Platform: &mcov1beta2.PlatformCapabilitiesSpec{
					Analytics: mcov1beta2.PlatformAnalyticsSpec{
						NamespaceRightSizingRecommendation: mcov1beta2.PlatformRightSizingRecommendationSpec{
							Enabled: nsEnabled,
						},
						VirtualizationRightSizingRecommendation: mcov1beta2.PlatformRightSizingRecommendationSpec{
							Enabled: virtEnabled,
						},
					},
				},
			},
		},
	}
}

func newTestMCO(binding string, enabled bool, paused bool) *mcov1beta2.MultiClusterObservability {
	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "observability",
			Finalizers: []string{analyticsFinalizer},
		},
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

func TestAnalyticsReconciler_AddsFinalizer(t *testing.T) {
	scheme := setupTestScheme(t)

	// MCO without analytics finalizer
	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{Name: "observability"},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			Capabilities: &mcov1beta2.CapabilitiesSpec{
				Platform: &mcov1beta2.PlatformCapabilitiesSpec{
					Analytics: mcov1beta2.PlatformAnalyticsSpec{
						NamespaceRightSizingRecommendation: mcov1beta2.PlatformRightSizingRecommendationSpec{
							Enabled: true,
						},
					},
				},
			},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mco).
		Build()

	r := &AnalyticsReconciler{Client: c, Scheme: scheme}
	_, err := r.Reconcile(context.TODO(), ctrl.Request{})
	require.NoError(t, err)

	// Verify finalizer was added
	updated := &mcov1beta2.MultiClusterObservability{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: "observability"}, updated)
	require.NoError(t, err)
	require.Contains(t, updated.GetFinalizers(), analyticsFinalizer)
}

func TestAnalyticsReconciler_DeletionCleansUp(t *testing.T) {
	scheme := setupTestScheme(t)

	// Include a second finalizer (simulating MCO's resFinalizer) so the object
	// survives after removing only the analytics finalizer.
	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "observability",
			Finalizers: []string{"observability.open-cluster-management.io/res-cleanup", analyticsFinalizer},
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			Capabilities: &mcov1beta2.CapabilitiesSpec{
				Platform: &mcov1beta2.PlatformCapabilitiesSpec{
					Analytics: mcov1beta2.PlatformAnalyticsSpec{
						NamespaceRightSizingRecommendation: mcov1beta2.PlatformRightSizingRecommendationSpec{
							Enabled: true,
						},
					},
				},
			},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mco).
		Build()

	// Delete the object to set DeletionTimestamp (finalizers prevent actual removal)
	err := c.Delete(context.TODO(), mco)
	require.NoError(t, err)

	r := &AnalyticsReconciler{Client: c, Scheme: scheme}

	// Phase 1: sync disabled state to ADC and start stabilization window
	_, err = r.Reconcile(context.TODO(), ctrl.Request{})
	require.NoError(t, err)

	// Simulate stabilization window elapsed
	r.cleanupAt = time.Now().Add(-20 * time.Second)

	// Phase 2: re-sync disabled, cleanup resources, remove finalizer
	_, err = r.Reconcile(context.TODO(), ctrl.Request{})
	require.NoError(t, err)

	// Verify analytics finalizer was removed but MCO's finalizer remains
	updated := &mcov1beta2.MultiClusterObservability{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: "observability"}, updated)
	require.NoError(t, err)
	require.NotContains(t, updated.GetFinalizers(), analyticsFinalizer)
	require.Contains(t, updated.GetFinalizers(), "observability.open-cluster-management.io/res-cleanup")
}

func TestAnalyticsReconciler_DeletionSkipsWithoutFinalizer(t *testing.T) {
	scheme := setupTestScheme(t)

	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "observability",
			Finalizers: []string{"other-finalizer"}, // no analytics finalizer
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mco).
		Build()

	// Delete the object to set DeletionTimestamp (finalizer prevents actual removal)
	err := c.Delete(context.TODO(), mco)
	require.NoError(t, err)

	r := &AnalyticsReconciler{Client: c, Scheme: scheme}
	_, err = r.Reconcile(context.TODO(), ctrl.Request{})
	require.NoError(t, err)

	// Verify the other finalizer is still present (we didn't touch it)
	updated := &mcov1beta2.MultiClusterObservability{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: "observability"}, updated)
	require.NoError(t, err)
	require.Contains(t, updated.GetFinalizers(), "other-finalizer")
}

func TestSyncRightSizingStateToADC_DelegatingEnabled(t *testing.T) {
	scheme := setupTestScheme(t)
	mco := newTestMCO("", true, false)

	// Create ADC with stale "disabled" values (simulates MCO → MCOA transition)
	adc := &addonv1beta1.AddOnDeploymentConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.MultiClusterObservabilityAddon,
			Namespace: "open-cluster-management-observability",
		},
		Spec: addonv1beta1.AddOnDeploymentConfigSpec{
			CustomizedVariables: []addonv1beta1.CustomizedVariable{
				{Name: util.ADCKeyPlatformNamespaceRightSizing, Value: "disabled"},
				{Name: util.ADCKeyPlatformVirtualizationRightSizing, Value: "disabled"},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mco, adc).Build()
	r := &AnalyticsReconciler{Client: c, Scheme: scheme}

	err := r.syncRightSizingStateToADC(context.TODO(), mco, true, log)
	require.NoError(t, err)

	// Verify ADC was updated to "enabled"
	updated := &addonv1beta1.AddOnDeploymentConfig{}
	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      config.MultiClusterObservabilityAddon,
		Namespace: "open-cluster-management-observability",
	}, updated)
	require.NoError(t, err)

	foundDelegated := false
	for _, cv := range updated.Spec.CustomizedVariables {
		switch cv.Name {
		case util.ADCKeyRightSizingDelegated:
			foundDelegated = true
			require.Equal(t, "true", cv.Value)
		case util.ADCKeyPlatformNamespaceRightSizing:
			require.Equal(t, "enabled", cv.Value)
		case util.ADCKeyPlatformVirtualizationRightSizing:
			// virtualization was not set in newTestMCO, so it defaults to disabled
			require.Equal(t, "disabled", cv.Value)
		}
	}
	require.True(t, foundDelegated, "missing customized variable %q", util.ADCKeyRightSizingDelegated)
}

func TestSyncRightSizingStateToADC_MCOManaging(t *testing.T) {
	scheme := setupTestScheme(t)
	mco := newTestMCO("", true, false)

	// Create ADC without RS keys
	adc := &addonv1beta1.AddOnDeploymentConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.MultiClusterObservabilityAddon,
			Namespace: "open-cluster-management-observability",
		},
		Spec: addonv1beta1.AddOnDeploymentConfigSpec{},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mco, adc).Build()
	r := &AnalyticsReconciler{Client: c, Scheme: scheme}

	// MCO managing (not delegating) → should force "disabled"
	err := r.syncRightSizingStateToADC(context.TODO(), mco, false, log)
	require.NoError(t, err)

	updated := &addonv1beta1.AddOnDeploymentConfig{}
	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      config.MultiClusterObservabilityAddon,
		Namespace: "open-cluster-management-observability",
	}, updated)
	require.NoError(t, err)

	foundDelegated := false
	for _, cv := range updated.Spec.CustomizedVariables {
		switch cv.Name {
		case util.ADCKeyRightSizingDelegated:
			foundDelegated = true
			require.Equal(t, "false", cv.Value)
		case util.ADCKeyPlatformNamespaceRightSizing:
			require.Equal(t, "disabled", cv.Value)
		case util.ADCKeyPlatformVirtualizationRightSizing:
			require.Equal(t, "disabled", cv.Value)
		}
	}
	require.True(t, foundDelegated, "missing customized variable %q", util.ADCKeyRightSizingDelegated)
}

func TestSyncRightSizingStateToADC_BothEnabled(t *testing.T) {
	scheme := setupTestScheme(t)
	mco := newTestMCOWithBothRS(true, true)

	adc := &addonv1beta1.AddOnDeploymentConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.MultiClusterObservabilityAddon,
			Namespace: "open-cluster-management-observability",
		},
		Spec: addonv1beta1.AddOnDeploymentConfigSpec{
			CustomizedVariables: []addonv1beta1.CustomizedVariable{
				{Name: util.ADCKeyPlatformNamespaceRightSizing, Value: "disabled"},
				{Name: util.ADCKeyPlatformVirtualizationRightSizing, Value: "disabled"},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mco, adc).Build()
	r := &AnalyticsReconciler{Client: c, Scheme: scheme}

	err := r.syncRightSizingStateToADC(context.TODO(), mco, true, log)
	require.NoError(t, err)

	updated := &addonv1beta1.AddOnDeploymentConfig{}
	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      config.MultiClusterObservabilityAddon,
		Namespace: "open-cluster-management-observability",
	}, updated)
	require.NoError(t, err)

	foundDelegated := false
	for _, cv := range updated.Spec.CustomizedVariables {
		switch cv.Name {
		case util.ADCKeyRightSizingDelegated:
			foundDelegated = true
			require.Equal(t, "true", cv.Value)
		case util.ADCKeyPlatformNamespaceRightSizing:
			require.Equal(t, "enabled", cv.Value)
		case util.ADCKeyPlatformVirtualizationRightSizing:
			require.Equal(t, "enabled", cv.Value)
		}
	}
	require.True(t, foundDelegated, "missing customized variable %q", util.ADCKeyRightSizingDelegated)
}

func TestSyncRightSizingStateToADC_ADCNotFound(t *testing.T) {
	scheme := setupTestScheme(t)
	mco := newTestMCO("", true, false)

	// No ADC created — should return nil (not an error)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mco).Build()
	r := &AnalyticsReconciler{Client: c, Scheme: scheme}

	err := r.syncRightSizingStateToADC(context.TODO(), mco, true, log)
	require.NoError(t, err)
}

func TestSyncRightSizingStateToADC_NoUpdateWhenValuesMatch(t *testing.T) {
	scheme := setupTestScheme(t)
	mco := newTestMCOWithBothRS(true, false)

	// ADC already has correct values — should not trigger an update
	adc := &addonv1beta1.AddOnDeploymentConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.MultiClusterObservabilityAddon,
			Namespace: "open-cluster-management-observability",
		},
		Spec: addonv1beta1.AddOnDeploymentConfigSpec{
			CustomizedVariables: []addonv1beta1.CustomizedVariable{
				{Name: util.ADCKeyRightSizingDelegated, Value: "true"},
				{Name: util.ADCKeyPlatformNamespaceRightSizing, Value: "enabled"},
				{Name: util.ADCKeyPlatformVirtualizationRightSizing, Value: "disabled"},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mco, adc).Build()
	r := &AnalyticsReconciler{Client: c, Scheme: scheme}

	err := r.syncRightSizingStateToADC(context.TODO(), mco, true, log)
	require.NoError(t, err)

	// Verify values unchanged
	updated := &addonv1beta1.AddOnDeploymentConfig{}
	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      config.MultiClusterObservabilityAddon,
		Namespace: "open-cluster-management-observability",
	}, updated)
	require.NoError(t, err)

	foundDelegated := false
	for _, cv := range updated.Spec.CustomizedVariables {
		switch cv.Name {
		case util.ADCKeyRightSizingDelegated:
			foundDelegated = true
			require.Equal(t, "true", cv.Value)
		case util.ADCKeyPlatformNamespaceRightSizing:
			require.Equal(t, "enabled", cv.Value)
		case util.ADCKeyPlatformVirtualizationRightSizing:
			require.Equal(t, "disabled", cv.Value)
		}
	}
	require.True(t, foundDelegated, "missing customized variable %q", util.ADCKeyRightSizingDelegated)
}

func TestWasDelegated_EventOnlyOnTransition(t *testing.T) {
	scheme := setupTestScheme(t)
	mco := newTestMCO("", true, false)
	mco.Annotations = map[string]string{
		util.RightSizingCapableAnnotation: "v1",
	}

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

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(mco, namespaceRSConfigMap, virtualizationRSConfigMap).
		Build()

	r := &AnalyticsReconciler{Client: c, Scheme: scheme}

	// First reconcile: wasDelegated starts false, should transition to true
	require.False(t, r.wasDelegated)
	_, err := r.Reconcile(context.TODO(), ctrl.Request{})
	require.NoError(t, err)
	require.True(t, r.wasDelegated, "wasDelegated should be true after first delegated reconcile")

	// Second reconcile: wasDelegated already true, no transition
	_, err = r.Reconcile(context.TODO(), ctrl.Request{})
	require.NoError(t, err)
	require.True(t, r.wasDelegated, "wasDelegated should remain true on subsequent reconciles")

	// Remove delegation annotation → MCO mode
	// Re-fetch to get latest resourceVersion (ensureRightSizingDefaults patches MCO)
	freshMCO := &mcov1beta2.MultiClusterObservability{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: "observability"}, freshMCO)
	require.NoError(t, err)
	freshMCO.Annotations = nil
	err = c.Update(context.TODO(), freshMCO)
	require.NoError(t, err)

	_, err = r.Reconcile(context.TODO(), ctrl.Request{})
	require.NoError(t, err)
	require.False(t, r.wasDelegated, "wasDelegated should reset to false when delegation removed")
}
