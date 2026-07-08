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
	mcoctrl "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	commonutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	addonv1beta1 "open-cluster-management.io/api/addon/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("controller_rightsizing")

var mcoGVK = mcov1beta2.GroupVersion.WithKind("MultiClusterObservability")

const analyticsFinalizer = "observability.open-cluster-management.io/analytics-cleanup"

// analyticsStabilizationWindow is the minimum time between Phase 1 (sync ADC disabled)
// and Phase 2 (cleanup + remove finalizer). It gives MCOA time to see "disabled" and
// tear down its own resources before MCO sweeps up remaining RS objects.
const analyticsStabilizationWindow = 10 * time.Second

// AnalyticsReconciler reconciles a MultiClusterObservability object.
// Must run under leader election — migrationDone and cleanupAt are in-memory state
// that would not be consistent across replicas.
type AnalyticsReconciler struct {
	Client        client.Client
	migrationDone bool
	cleanupAt     time.Time
}

// +kubebuilder:rbac:groups=observability.open-cluster-management.io,resources=multiclusterobservabilities,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=observability.open-cluster-management.io,resources=multiclusterobservabilities/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=observability.open-cluster-management.io,resources=multiclusterobservabilities/finalizers,verbs=update

// Reconcile handles reconciliation of right-sizing analytics resources based on the MCO lifecycle.
func (r *AnalyticsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// TODO: Future enhancement - Add status subresource to track right-sizing state
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

		// Phase 1: Sync ADC to "disabled" and start the stabilization window.
		// This gives MCOA time to see "disabled" and clean up its own resources
		// (Placements, ConfigMaps) before we do our cleanup in Phase 2.
		if r.cleanupAt.IsZero() {
			reqLogger.Info("rs - MCO terminating, syncing disabled state to ADC before cleanup")
			if err := r.syncRightSizingStateToADC(ctx, instance, false, reqLogger); err != nil {
				reqLogger.Error(err, "rs - failed to sync disabled state to ADC, starting stabilization window")
			}
			r.cleanupAt = time.Now()
			return ctrl.Result{RequeueAfter: analyticsStabilizationWindow}, nil
		}

		// Block any reconcile that arrives before the stabilization window elapses.
		// ConfigMap watches and other events can trigger early reconciles.
		if elapsed := time.Since(r.cleanupAt); elapsed < analyticsStabilizationWindow {
			remaining := analyticsStabilizationWindow - elapsed
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

	// In ACM 5.0 GA, MCOA (ManifestWork-based) is the only right-sizing deployment model.
	// Run one-time migration on first reconcile to remove any legacy Policy resources
	// left from a pre-GA (Policy-based) installation. ConfigMaps are preserved because
	// MCOA reuses them for per-cluster configuration.
	reqLogger.Info("rs - right-sizing handled by MCOA (ManifestWork-based)")
	if !r.migrationDone {
		reqLogger.Info("rs - running one-time legacy Policy resource cleanup for upgrade migration")
		if err := rightsizingctrl.CleanupLegacyPolicyResources(ctx, r.Client, instance); err != nil {
			return ctrl.Result{}, fmt.Errorf("rs - failed to cleanup legacy Policy resources: %w", err)
		}
		r.migrationDone = true
	}

	// Sync MCO CR's right-sizing state to ADC so MCOA knows what to deploy.
	if err := r.syncRightSizingStateToADC(ctx, instance, true, reqLogger); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to sync RS state to ADC for MCOA: %w", err)
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

	if err := r.Client.Get(ctx, key, u); err != nil {
		if !apierrors.IsNotFound(err) {
			return instance, fmt.Errorf("ensureRightSizingDefaults: failed to get MCO CR: %w", err)
		}
		return instance, nil
	}
	// Check if the fields already exist
	nsEnabled, nsFound, _ := unstructured.NestedBool(u.Object,
		"spec", "capabilities", "platform", "analytics", "namespaceRightSizingRecommendation", "enabled")
	virtEnabled, virtFound, _ := unstructured.NestedBool(u.Object,
		"spec", "capabilities", "platform", "analytics", "virtualizationRightSizingRecommendation", "enabled")

	// Only patch if at least one field is missing
	if !nsFound || !virtFound {
		// Set true if not present, else preserve existing value.
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

		if err := r.Client.Patch(ctx, u, client.RawPatch(types.MergePatchType, patchBytes)); err != nil {
			return instance, fmt.Errorf("failed to persist default analytics right-sizing flags: %w", err)
		}
		reqLogger.Info("Defaulted analytics right-sizing flags to true (fresh install)")

		// Refresh typed instance so downstream logic sees updated flags.
		refreshed := &mcov1beta2.MultiClusterObservability{}
		if err := r.Client.Get(ctx, key, refreshed); err != nil {
			reqLogger.Error(err, "Failed to refresh MCO after patching defaults, using stale instance")
		} else {
			instance = refreshed.DeepCopy()
		}
	}
	return instance, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AnalyticsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	mcoPred := mcoctrl.GetMCOPredicateFunc()
	return ctrl.NewControllerManagedBy(mgr).
		Named("rightsizing").
		For(&mcov1beta2.MultiClusterObservability{}, builder.WithPredicates(mcoPred)).
		Complete(r)
}

// syncRightSizingStateToADC syncs right-sizing state to AddOnDeploymentConfig.
// When delegatingToMCOA=true: syncs the MCO CR's actual enabled/disabled state so MCOA knows what to deploy.
// When delegatingToMCOA=false: forces "disabled" to prevent MCOA from deploying PrometheusRules during MCO deletion.
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
			reqLogger.V(1).Info("rs - syncing disabled state to ADC before MCO cleanup",
				"delegated", delegatedValue, "namespace", nsValue, "virtualization", virtValue)
		}
		if err := r.Client.Update(ctx, adc); err != nil {
			return fmt.Errorf("failed to update AddOnDeploymentConfig: %w", err)
		}
	}

	return nil
}


