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
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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

	r := &AnalyticsReconciler{Client: c}
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

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mco).
		Build()

	r := &AnalyticsReconciler{Client: c}
	_, err := r.Reconcile(context.TODO(), ctrl.Request{})
	require.NoError(t, err)
}

func TestAnalyticsReconciler_FeatureDisabled(t *testing.T) {
	scheme := setupTestScheme(t)

	mco := newTestMCO("", false, false)

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mco).
		Build()

	r := &AnalyticsReconciler{Client: c}
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

	r := &AnalyticsReconciler{Client: c}
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

	r := &AnalyticsReconciler{Client: c}
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

	// Pre-create ADC with "enabled" state (simulates steady-state before MCO deletion).
	// Phase 1 must flip these to "disabled" so MCOA stops deploying PrometheusRules.
	adc := &addonv1beta1.AddOnDeploymentConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.MultiClusterObservabilityAddon,
			Namespace: config.GetDefaultNamespace(),
		},
		Spec: addonv1beta1.AddOnDeploymentConfigSpec{
			CustomizedVariables: []addonv1beta1.CustomizedVariable{
				{Name: util.ADCKeyRightSizingDelegated, Value: "true"},
				{Name: util.ADCKeyPlatformNamespaceRightSizing, Value: "enabled"},
				{Name: util.ADCKeyPlatformVirtualizationRightSizing, Value: "enabled"},
			},
		},
	}

	// Pre-create labeled RS resources so Phase 2 cleanup is verified to actually delete them.
	rsLabels := map[string]string{"observability.open-cluster-management.io/managed-by": "analytics-rightsizing"}
	rsPlacement := &clusterv1beta1.Placement{
		ObjectMeta: metav1.ObjectMeta{Name: "rs-placement", Labels: rsLabels},
	}
	rsCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rs-namespace-config",
			Namespace: config.GetDefaultNamespace(),
			Labels:    rsLabels,
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mco, adc, rsPlacement, rsCM).
		Build()

	// Delete the object to set DeletionTimestamp (finalizers prevent actual removal)
	err := c.Delete(context.TODO(), mco)
	require.NoError(t, err)

	r := &AnalyticsReconciler{Client: c}

	// Phase 1: sync disabled state to ADC and start stabilization window
	result, err := r.Reconcile(context.TODO(), ctrl.Request{})
	require.NoError(t, err)
	require.Equal(t, analyticsStabilizationWindow, result.RequeueAfter)
	require.False(t, r.cleanupAt.IsZero(), "cleanupAt must be set after Phase 1")

	// Verify ADC was flipped to disabled after Phase 1
	updatedADC := &addonv1beta1.AddOnDeploymentConfig{}
	require.NoError(t, c.Get(context.TODO(), types.NamespacedName{
		Name:      config.MultiClusterObservabilityAddon,
		Namespace: config.GetDefaultNamespace(),
	}, updatedADC))
	adcVars := make(map[string]string)
	for _, cv := range updatedADC.Spec.CustomizedVariables {
		adcVars[cv.Name] = cv.Value
	}
	require.Equal(t, "false", adcVars[util.ADCKeyRightSizingDelegated])
	require.Equal(t, "disabled", adcVars[util.ADCKeyPlatformNamespaceRightSizing])
	require.Equal(t, "disabled", adcVars[util.ADCKeyPlatformVirtualizationRightSizing])

	// Simulate stabilization window elapsed
	r.cleanupAt = time.Now().Add(-(analyticsStabilizationWindow + time.Second))

	// Phase 2: re-sync disabled, cleanup resources, remove finalizer
	_, err = r.Reconcile(context.TODO(), ctrl.Request{})
	require.NoError(t, err)

	// Verify analytics finalizer was removed but MCO's finalizer remains
	updated := &mcov1beta2.MultiClusterObservability{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: "observability"}, updated)
	require.NoError(t, err)
	require.NotContains(t, updated.GetFinalizers(), analyticsFinalizer)
	require.Contains(t, updated.GetFinalizers(), "observability.open-cluster-management.io/res-cleanup")

	// Verify labeled RS resources were cleaned up by Phase 2
	err = c.Get(context.TODO(), types.NamespacedName{Name: "rs-placement"}, &clusterv1beta1.Placement{})
	require.True(t, apierrors.IsNotFound(err), "RS Placement should have been deleted in Phase 2 cleanup")
	err = c.Get(context.TODO(), types.NamespacedName{Name: "rs-namespace-config", Namespace: config.GetDefaultNamespace()}, &corev1.ConfigMap{})
	require.True(t, apierrors.IsNotFound(err), "RS ConfigMap should have been deleted in full cleanup (not migration)")
}

// TestAnalyticsReconciler_DeletionBlocksMidWindow verifies that reconciles arriving
// before the stabilization window elapses are blocked (returning a non-zero RequeueAfter)
// and do not remove the analytics finalizer prematurely.
func TestAnalyticsReconciler_DeletionBlocksMidWindow(t *testing.T) {
	scheme := setupTestScheme(t)

	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "observability",
			Finalizers: []string{"observability.open-cluster-management.io/res-cleanup", analyticsFinalizer},
		},
	}
	adc := &addonv1beta1.AddOnDeploymentConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.MultiClusterObservabilityAddon,
			Namespace: config.GetDefaultNamespace(),
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mco, adc).Build()
	require.NoError(t, c.Delete(context.TODO(), mco))

	r := &AnalyticsReconciler{Client: c}

	// Phase 1: starts stabilization window
	result1, err := r.Reconcile(context.TODO(), ctrl.Request{})
	require.NoError(t, err)
	require.Equal(t, analyticsStabilizationWindow, result1.RequeueAfter)
	require.False(t, r.cleanupAt.IsZero())

	// Simulate a mid-window early reconcile: set cleanupAt to 1ms ago so elapsed=1ms,
	// well within the window. 9.999s of buffer means the test can't flap.
	r.cleanupAt = time.Now().Add(-time.Millisecond)
	result2, err := r.Reconcile(context.TODO(), ctrl.Request{})
	require.NoError(t, err)
	require.Greater(t, result2.RequeueAfter, time.Duration(0), "mid-window reconcile must requeue")
	require.Less(t, result2.RequeueAfter, analyticsStabilizationWindow, "requeue must be less than full window")

	// Analytics finalizer must still be present — Phase 2 must not have run.
	live := &mcov1beta2.MultiClusterObservability{}
	require.NoError(t, c.Get(context.TODO(), types.NamespacedName{Name: "observability"}, live))
	require.Contains(t, live.GetFinalizers(), analyticsFinalizer, "finalizer must not be removed mid-window")
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

	r := &AnalyticsReconciler{Client: c}
	_, err = r.Reconcile(context.TODO(), ctrl.Request{})
	require.NoError(t, err)

	// Verify the other finalizer is still present (we didn't touch it)
	updated := &mcov1beta2.MultiClusterObservability{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: "observability"}, updated)
	require.NoError(t, err)
	require.Contains(t, updated.GetFinalizers(), "other-finalizer")
}

func TestReconcile_MigrationSetsFlag(t *testing.T) {
	scheme := setupTestScheme(t)
	mco := newTestMCOWithBothRS(true, true)
	mco.Finalizers = []string{analyticsFinalizer}

	adc := &addonv1beta1.AddOnDeploymentConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.MultiClusterObservabilityAddon,
			Namespace: config.GetDefaultNamespace(),
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mco, adc).Build()
	r := &AnalyticsReconciler{Client: c}

	require.False(t, r.migrationDone)
	_, err := r.Reconcile(context.TODO(), ctrl.Request{})
	require.NoError(t, err)
	require.True(t, r.migrationDone, "migrationDone should be true after first successful reconcile")

	// Verify ADC was synced with delegation enabled
	updatedADC := &addonv1beta1.AddOnDeploymentConfig{}
	require.NoError(t, c.Get(context.TODO(), types.NamespacedName{
		Name:      config.MultiClusterObservabilityAddon,
		Namespace: config.GetDefaultNamespace(),
	}, updatedADC))
	adcVars := make(map[string]string)
	for _, cv := range updatedADC.Spec.CustomizedVariables {
		adcVars[cv.Name] = cv.Value
	}
	require.Equal(t, "true", adcVars[util.ADCKeyRightSizingDelegated])
	require.Equal(t, "enabled", adcVars[util.ADCKeyPlatformNamespaceRightSizing])
	require.Equal(t, "enabled", adcVars[util.ADCKeyPlatformVirtualizationRightSizing])
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
	r := &AnalyticsReconciler{Client: c}

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
	r := &AnalyticsReconciler{Client: c}

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
	r := &AnalyticsReconciler{Client: c}

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
	r := &AnalyticsReconciler{Client: c}

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

func TestSyncRightSizingStateToADC_DeletionForcesDisabled(t *testing.T) {
	scheme := setupTestScheme(t)
	// MCO CR with both RS features enabled — deletion path must override to "disabled".
	mco := newTestMCOWithBothRS(true, true)

	// Pre-create ADC with "enabled" values simulating an active MCOA deployment.
	adc := &addonv1beta1.AddOnDeploymentConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.MultiClusterObservabilityAddon,
			Namespace: "open-cluster-management-observability",
		},
		Spec: addonv1beta1.AddOnDeploymentConfigSpec{
			CustomizedVariables: []addonv1beta1.CustomizedVariable{
				{Name: util.ADCKeyRightSizingDelegated, Value: "true"},
				{Name: util.ADCKeyPlatformNamespaceRightSizing, Value: "enabled"},
				{Name: util.ADCKeyPlatformVirtualizationRightSizing, Value: "enabled"},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mco, adc).Build()
	r := &AnalyticsReconciler{Client: c}

	// Deletion path: delegatingToMCOA=false must force all RS vars to disabled.
	err := r.syncRightSizingStateToADC(context.TODO(), mco, false, log)
	require.NoError(t, err)

	updated := &addonv1beta1.AddOnDeploymentConfig{}
	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      config.MultiClusterObservabilityAddon,
		Namespace: "open-cluster-management-observability",
	}, updated)
	require.NoError(t, err)

	for _, cv := range updated.Spec.CustomizedVariables {
		switch cv.Name {
		case util.ADCKeyRightSizingDelegated:
			require.Equal(t, "false", cv.Value, "deletion must set delegated=false")
		case util.ADCKeyPlatformNamespaceRightSizing:
			require.Equal(t, "disabled", cv.Value, "deletion must set namespace RS to disabled")
		case util.ADCKeyPlatformVirtualizationRightSizing:
			require.Equal(t, "disabled", cv.Value, "deletion must set virt RS to disabled")
		}
	}
}

func TestReconcile_MigrationRunsOnce(t *testing.T) {
	scheme := setupTestScheme(t)
	mco := newTestMCO("open-cluster-management-global-set", true, false)

	// Pre-create labeled legacy Policy and PlacementBinding to verify they are actually deleted.
	rsLabels := map[string]string{"observability.open-cluster-management.io/managed-by": "analytics-rightsizing"}
	legacyPolicy := &policyv1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rs-prom-rules-policy",
			Namespace: "open-cluster-management-global-set",
			Labels:    rsLabels,
		},
	}
	legacyPB := &policyv1.PlacementBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rs-policyset-binding",
			Namespace: "open-cluster-management-global-set",
			Labels:    rsLabels,
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mco, legacyPolicy, legacyPB).Build()
	r := &AnalyticsReconciler{Client: c}

	// First reconcile: migrationDone starts false, should run cleanup and transition to true.
	require.False(t, r.migrationDone)
	_, err := r.Reconcile(context.TODO(), ctrl.Request{})
	require.NoError(t, err)
	require.True(t, r.migrationDone, "migrationDone should be true after first reconcile")

	// Verify legacy Policy was deleted.
	deletedPolicy := &policyv1.Policy{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: "rs-prom-rules-policy", Namespace: "open-cluster-management-global-set"}, deletedPolicy)
	require.True(t, apierrors.IsNotFound(err), "legacy Policy should have been deleted by migration")

	// Verify legacy PlacementBinding was deleted.
	deletedPB := &policyv1.PlacementBinding{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: "rs-policyset-binding", Namespace: "open-cluster-management-global-set"}, deletedPB)
	require.True(t, apierrors.IsNotFound(err), "legacy PlacementBinding should have been deleted by migration")

	// Second reconcile: migrationDone already true, gate is bypassed (no-op).
	_, err = r.Reconcile(context.TODO(), ctrl.Request{})
	require.NoError(t, err)
	require.True(t, r.migrationDone, "migrationDone should remain true on subsequent reconciles")
}
