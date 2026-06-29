// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/go-logr/logr"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	rightsizingctrl "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing"
	rsnamespace "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing/rs-namespace"
	rsvirtualization "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing/rs-virtualization"
	mcoctrl "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	commonutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	addonv1beta1 "open-cluster-management.io/api/addon/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("controller_rightsizing")

var mcoGVK = mcov1beta2.GroupVersion.WithKind("MultiClusterObservability")

const analyticsFinalizer = "observability.open-cluster-management.io/analytics-cleanup"

// AnalyticsReconciler reconciles a MultiClusterObservability object
type AnalyticsReconciler struct {
	Client       client.Client
	Log          logr.Logger
	Scheme       *runtime.Scheme
	wasDelegated bool
	cleanupAt    time.Time
}

// +kubebuilder:rbac:groups=observability.open-cluster-management.io,resources=multiclusterobservabilities,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=observability.open-cluster-management.io,resources=multiclusterobservabilities/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=observability.open-cluster-management.io,resources=multiclusterobservabilities/finalizers,verbs=update

// Reconcile handles reconciliation of right-sizing analytics resources based on the MCO lifecycle.
func (r *AnalyticsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// TODO: Future enhancement - Add status subresource to track right-sizing state
	// This would allow users to see current mode (MCO Policy vs MCOA ManifestWork)
	// and configuration details via: kubectl get mco -o jsonpath='{.status.rightSizing}'

	reqLogger := log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	reqLogger.Info("Reconciling RightSizing")

	// Fetch the MultiClusterObservability instance
	mcoList := &mcov1beta2.MultiClusterObservabilityList{}
	if err := r.Client.List(ctx, mcoList); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list MultiClusterObservability custom resources: %w", err)
	}
	if len(mcoList.Items) == 0 {
		reqLogger.Info("no MultiClusterObservability CR exists, nothing to do")
		return ctrl.Result{}, nil
	}

	instance := mcoList.Items[0].DeepCopy()

	// Handle deletion: clean up RS resources and remove our finalizer.
	// Uses a stabilization window to let MCOA process the "disabled" ADC state
	// before MCO sweeps up remaining resources.
	if instance.GetDeletionTimestamp() != nil {
		if !slices.Contains(instance.GetFinalizers(), analyticsFinalizer) {
			return ctrl.Result{}, nil // not our responsibility (e.g., upgrade from older version)
		}

		const stabilizationWindow = 10 * time.Second

		// Phase 1: Sync ADC to "disabled" and start the stabilization window.
		// This gives MCOA time to see "disabled" and clean up its own resources
		// (Placements, ConfigMaps) before we do our cleanup in Phase 2.
		if r.cleanupAt.IsZero() {
			reqLogger.Info("rs - MCO terminating, syncing disabled state to ADC before cleanup")
			if err := r.syncRightSizingStateToADC(ctx, instance, false, reqLogger); err != nil {
				reqLogger.Error(err, "rs - failed to sync disabled state to ADC, continuing with cleanup")
			}
			r.cleanupAt = time.Now()
			return ctrl.Result{RequeueAfter: stabilizationWindow}, nil
		}

		// Block any reconcile that arrives before the stabilization window elapses.
		// ConfigMap watches and other events can trigger early reconciles.
		if elapsed := time.Since(r.cleanupAt); elapsed < stabilizationWindow {
			remaining := stabilizationWindow - elapsed
			reqLogger.Info("rs - waiting for stabilization window", "remaining", remaining.Round(time.Second))
			return ctrl.Result{RequeueAfter: remaining}, nil
		}

		// Phase 2: Stabilization window elapsed. Clean up and remove finalizer.
		reqLogger.Info("rs - stabilization complete, cleaning up right-sizing resources")

		// Re-sync "disabled" to ADC before cleanup. Phase 1's sync may have been
		// overwritten by the main controller's deployer, which renders ADC with the
		// CR's actual RS state before it sees the deletion timestamp. By Phase 2,
		// the main controller has processed the deletion and stopped deploying.
		if err := r.syncRightSizingStateToADC(ctx, instance, false, reqLogger); err != nil {
			reqLogger.Error(err, "rs - failed to re-sync disabled state before cleanup, ADC may already be deleted")
		}

		if err := rightsizingctrl.CleanupRightSizingResources(ctx, r.Client, instance); err != nil {
			return ctrl.Result{}, fmt.Errorf("rs - failed to cleanup right-sizing resources: %w", err)
		}
		instanceCopy := instance.DeepCopy()
		instance.SetFinalizers(commonutil.Remove(instance.GetFinalizers(), analyticsFinalizer))
		if err := r.Client.Patch(ctx, instance, client.MergeFrom(instanceCopy)); err != nil {
			return ctrl.Result{}, fmt.Errorf("rs - failed to remove analytics finalizer: %w", err)
		}
		reqLogger.Info("rs - Analytics finalizer removed after RS cleanup")
		return ctrl.Result{}, nil
	}

	// Reset stabilization window from a previous deletion cycle (e.g., MCO reinstalled
	// without restarting the operator pod). Without this, a stale cleanupAt from a
	// prior delete would skip Phase 1 on the next delete.
	r.cleanupAt = time.Time{}

	// Normal path: ensure our finalizer is present
	if !slices.Contains(instance.GetFinalizers(), analyticsFinalizer) {
		instanceCopy := instance.DeepCopy()
		instance.SetFinalizers(append(instance.GetFinalizers(), analyticsFinalizer))
		if err := r.Client.Patch(ctx, instance, client.MergeFrom(instanceCopy)); err != nil {
			return ctrl.Result{}, fmt.Errorf("rs - failed to add analytics finalizer: %w", err)
		}
		reqLogger.Info("rs - Analytics finalizer added to MCO CR")
		return ctrl.Result{Requeue: true}, nil
	}

	// Do not reconcile objects if this instance of mch is labeled "paused"
	if config.IsPaused(instance.GetAnnotations()) {
		reqLogger.Info("MCO reconciliation is paused. Nothing more to do.")
		return ctrl.Result{}, nil
	}

	// Ensure defaults are set/persisted for analytics right-sizing
	instance, err := r.ensureRightSizingDefaults(ctx, instance, reqLogger)
	if err != nil {
		return ctrl.Result{}, err
	}

	// ═══════════════════════════════════════════════════════════════════
	// MIGRATION GATE: Check if MCOA should handle right-sizing
	// ═══════════════════════════════════════════════════════════════════

	// Check if right-sizing is delegated to MCOA via MCO CR annotation.
	// The annotation is the authoritative signal — if present, MCOA manages right-sizing
	// via ManifestWork instead of MCO's Policy-based approach.
	mcoaCapable := util.IsRightSizingDelegated(instance)

	if mcoaCapable {
		reqLogger.Info("rs - right-sizing delegated to MCOA via MCO CR annotation")

		// On transition to MCOA mode: cleanup Policy resources (if any exist).
		// The existence check avoids unnecessary API calls on controller restarts
		// when cleanup already happened in a previous lifecycle.
		if !r.wasDelegated {
			hasResources, err := r.hasPolicyResourcesToCleanup(ctx)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to check for Policy resources to cleanup: %w", err)
			}
			if hasResources {
				if err := rightsizingctrl.CleanupPolicyResourcesForDelegation(ctx, r.Client, instance); err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to cleanup Policy resources for MCOA delegation: %w", err)
				}
			}
			r.wasDelegated = true
		}

		// Sync MCO CR's right-sizing state to ADC so MCOA knows what to deploy.
		// This is critical when switching from MCO mode (which sets "disabled" in ADC)
		// to MCOA mode — without this, MCOA would see stale "disabled" values.
		if err := r.syncRightSizingStateToADC(ctx, instance, true, reqLogger); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to sync RS state to ADC for MCOA delegation: %w", err)
		}

		return ctrl.Result{}, nil
	}

	// Reset delegation tracking when in MCO mode
	r.wasDelegated = false

	// ═══════════════════════════════════════════════════════════════════
	// MCO Mode: Create Policy resources (current GA behavior)
	// ═══════════════════════════════════════════════════════════════════
	reqLogger.V(1).Info("rs - MCO managing right-sizing via Policy")
	if err := rightsizingctrl.CreateRightSizingComponent(ctx, r.Client, instance); err != nil {
		return ctrl.Result{}, fmt.Errorf("rs - failed to create rightsizing component: %w", err)
	}

	// When MCO manages right-sizing, sync "disabled" to AddOnDeploymentConfig.
	// This tells MCOA to NOT deploy PrometheusRules via ManifestWork.
	// Return error to requeue — if this fails, MCOA keeps stale "enabled" values
	// and both MCO (Policy) and MCOA (ManifestWork) would deploy RS simultaneously.
	if err := r.syncRightSizingStateToADC(ctx, instance, false, reqLogger); err != nil {
		return ctrl.Result{}, fmt.Errorf("rs - failed to sync disabled state to ADC: %w", err)
	}

	return ctrl.Result{}, nil
}

// ensureRightSizingDefaults persists default right-sizing flags when absent and returns the (possibly updated) instance.
func (r *AnalyticsReconciler) ensureRightSizingDefaults(ctx context.Context, instance *mcov1beta2.MultiClusterObservability, reqLogger logr.Logger) (*mcov1beta2.MultiClusterObservability, error) {
	// Default-enable analytics right-sizing flags ONLY when absent on fresh installs.
	// Persist defaults back to the MCO spec so users can later override to true/false explicitly.
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(mcoGVK)
	key := types.NamespacedName{Name: instance.GetName()}

	if err := r.Client.Get(ctx, key, u); err == nil {
		// Check if the fields already exist
		nsEnabled, nsFound, _ := unstructured.NestedBool(u.Object,
			"spec", "capabilities", "platform", "analytics", "namespaceRightSizingRecommendation", "enabled")
		virtEnabled, virtFound, _ := unstructured.NestedBool(u.Object,
			"spec", "capabilities", "platform", "analytics", "virtualizationRightSizingRecommendation", "enabled")

		// Only patch if at least one field is missing
		if !nsFound || !virtFound {
			// Build a minimal patch that only contains the analytics fields we want to set.
			// Use typed locals to avoid chained type assertions (which can panic if the shape changes).
			// Set true if not present else preserve existing value
			analytics := map[string]any{
				"namespaceRightSizingRecommendation":      map[string]any{"enabled": !nsFound || nsEnabled},
				"virtualizationRightSizingRecommendation": map[string]any{"enabled": !virtFound || virtEnabled},
			}
			patchData := map[string]any{
				"spec": map[string]any{
					"capabilities": map[string]any{
						"platform": map[string]any{
							"analytics": analytics,
						},
					},
				},
			}

			patchBytes, err := json.Marshal(patchData)
			if err != nil {
				return instance, fmt.Errorf("failed to marshal patch data: %w", err)
			}

			// Use MergePatch to only update the specific fields without affecting others
			if err := r.Client.Patch(ctx, u, client.RawPatch(types.MergePatchType, patchBytes)); err != nil {
				return instance, fmt.Errorf("failed to persist default analytics right-sizing flags: %w", err)
			}
			reqLogger.Info("Defaulted analytics right-sizing flags to true (fresh install)")

			// refresh typed instance so downstream logic sees updated flags
			refreshed := &mcov1beta2.MultiClusterObservability{}
			if err := r.Client.Get(ctx, key, refreshed); err != nil {
				reqLogger.Error(err, "Failed to refresh MCO after patching defaults, using stale instance")
			} else {
				instance = refreshed.DeepCopy()
			}
		}
	}
	return instance, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AnalyticsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	mcoPred := mcoctrl.GetMCOPredicateFunc()
	cmNamespaceRSPred := rightsizingctrl.GetNamespaceRSConfigMapPredicateFunc()
	cmVirtualizationRSPred := rightsizingctrl.GetVirtualizationRSConfigMapPredicateFunc()
	return ctrl.NewControllerManagedBy(mgr).
		Named("rightsizing").
		For(&mcov1beta2.MultiClusterObservability{}, builder.WithPredicates(mcoPred)).
		Watches(&corev1.ConfigMap{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(cmNamespaceRSPred)).
		Watches(&corev1.ConfigMap{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(cmVirtualizationRSPred)).
		Complete(r)
}

// syncRightSizingStateToADC syncs right-sizing state to AddOnDeploymentConfig.
// When delegatingToMCOA=true: syncs the MCO CR's actual enabled/disabled state so MCOA knows what to deploy.
// When delegatingToMCOA=false: forces "disabled" so MCOA does NOT deploy PrometheusRules (MCO manages via Policy).
func (r *AnalyticsReconciler) syncRightSizingStateToADC(ctx context.Context, instance *mcov1beta2.MultiClusterObservability, delegatingToMCOA bool, reqLogger logr.Logger) error {
	const (
		valueEnabled  = "enabled"
		valueDisabled = "disabled"
	)

	// Determine target values based on mode
	delegatedValue := "false"
	nsValue := valueDisabled
	virtValue := valueDisabled
	if delegatingToMCOA {
		delegatedValue = "true"
		// When delegating to MCOA, sync the MCO CR's actual right-sizing state
		if instance.Spec.Capabilities != nil && instance.Spec.Capabilities.Platform != nil {
			if instance.Spec.Capabilities.Platform.Analytics.NamespaceRightSizingRecommendation.Enabled {
				nsValue = valueEnabled
			}
			if instance.Spec.Capabilities.Platform.Analytics.VirtualizationRightSizingRecommendation.Enabled {
				virtValue = valueEnabled
			}
		}
	}

	adc := &addonv1beta1.AddOnDeploymentConfig{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      config.MultiClusterObservabilityAddon,
		Namespace: config.GetDefaultNamespace(),
	}, adc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// ADC doesn't exist yet - nothing to sync
			return nil
		}
		return fmt.Errorf("failed to get AddOnDeploymentConfig: %w", err)
	}

	// Single-pass: find indices and track if update needed
	delegatedIdx, nsIdx, virtIdx := -1, -1, -1
	needsUpdate := false

	for i, cv := range adc.Spec.CustomizedVariables {
		switch cv.Name {
		case util.ADCKeyRightSizingDelegated:
			delegatedIdx = i
			if cv.Value != delegatedValue {
				adc.Spec.CustomizedVariables[i].Value = delegatedValue
				needsUpdate = true
			}
		case util.ADCKeyPlatformNamespaceRightSizing:
			nsIdx = i
			if cv.Value != nsValue {
				adc.Spec.CustomizedVariables[i].Value = nsValue
				needsUpdate = true
			}
		case util.ADCKeyPlatformVirtualizationRightSizing:
			virtIdx = i
			if cv.Value != virtValue {
				adc.Spec.CustomizedVariables[i].Value = virtValue
				needsUpdate = true
			}
		}
	}

	// Append if not found
	if delegatedIdx == -1 {
		adc.Spec.CustomizedVariables = append(adc.Spec.CustomizedVariables,
			addonv1beta1.CustomizedVariable{Name: util.ADCKeyRightSizingDelegated, Value: delegatedValue})
		needsUpdate = true
	}
	if nsIdx == -1 {
		adc.Spec.CustomizedVariables = append(adc.Spec.CustomizedVariables,
			addonv1beta1.CustomizedVariable{Name: util.ADCKeyPlatformNamespaceRightSizing, Value: nsValue})
		needsUpdate = true
	}
	if virtIdx == -1 {
		adc.Spec.CustomizedVariables = append(adc.Spec.CustomizedVariables,
			addonv1beta1.CustomizedVariable{Name: util.ADCKeyPlatformVirtualizationRightSizing, Value: virtValue})
		needsUpdate = true
	}

	if needsUpdate {
		if delegatingToMCOA {
			reqLogger.Info("rs - syncing right-sizing state to ADC for MCOA delegation",
				"delegated", delegatedValue, "namespace", nsValue, "virtualization", virtValue)
		} else {
			reqLogger.V(1).Info("rs - syncing disabled state to ADC (MCO takes over)",
				"delegated", delegatedValue, "namespace", nsValue, "virtualization", virtValue)
		}
		if err := r.Client.Update(ctx, adc); err != nil {
			return fmt.Errorf("failed to update AddOnDeploymentConfig: %w", err)
		}
	}

	return nil
}

// hasPolicyResourcesToCleanup checks whether any MCO-managed Policy resources
// exist for right-sizing (namespace or virtualization). Only apierrors.IsNotFound
// is treated as "not present"; any other GET error is returned so the caller
// can requeue instead of permanently skipping cleanup.
func (r *AnalyticsReconciler) hasPolicyResourcesToCleanup(ctx context.Context) (bool, error) {
	checks := []struct {
		name      string
		namespace string
	}{
		{rsnamespace.PrometheusRulePolicyName, rsnamespace.ComponentState.Namespace},
		{rsvirtualization.PrometheusRulePolicyName, rsvirtualization.ComponentState.Namespace},
	}

	for _, check := range checks {
		ns := check.namespace
		if ns == "" {
			ns = config.GetDefaultNamespace()
		}
		key := types.NamespacedName{Name: check.name, Namespace: ns}
		if err := r.Client.Get(ctx, key, &policyv1.Policy{}); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return false, fmt.Errorf("checking policy %s/%s: %w", ns, check.name, err)
		}
		return true, nil
	}

	return false, nil
}
