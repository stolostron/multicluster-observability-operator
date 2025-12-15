// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	rightsizingctrl "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing"
	mcoctrl "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	corev1 "k8s.io/api/core/v1"
)

var log = logf.Log.WithName("controller_rightsizing")

var mcoGVK = schema.GroupVersionKind{
	Group:   "observability.open-cluster-management.io",
	Version: "v1beta2",
	Kind:    "MultiClusterObservability",
}

// AnalyticsReconciler reconciles a MultiClusterObservability object
type AnalyticsReconciler struct {
	Client client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=observability.open-cluster-management.io,resources=multiclusterobservabilities,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=observability.open-cluster-management.io,resources=multiclusterobservabilities/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=observability.open-cluster-management.io,resources=multiclusterobservabilities/finalizers,verbs=update

func (r *AnalyticsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	reqLogger.Info("Reconciling RightSizing")

	// Fetch the MultiClusterObservability instance
	mcoList := &mcov1beta2.MultiClusterObservabilityList{}
	err := r.Client.List(ctx, mcoList)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list MultiClusterObservability custom resources: %w", err)
	}
	if len(mcoList.Items) == 0 {
		reqLogger.Info("no MultiClusterObservability CR exists, nothing to do")
		return ctrl.Result{}, nil
	}

	instance := mcoList.Items[0].DeepCopy()

	// Do not reconcile objects if this instance of mch is labeled "paused"
	if config.IsPaused(instance.GetAnnotations()) {
		reqLogger.Info("MCO reconciliation is paused. Nothing more to do.")
		return ctrl.Result{}, nil
	}

	// Ensure defaults are set/persisted for analytics right-sizing
	instance, err = r.ensureRightSizingDefaults(ctx, instance, reqLogger)
	if err != nil {
		return ctrl.Result{}, err
	}

	// create rightsizing component
	err = rightsizingctrl.CreateRightSizingComponent(ctx, r.Client, instance)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create rightsizing component: %w", err)
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
			// Build a minimal patch that only contains the analytics fields we want to set
			patchData := map[string]interface{}{
				"spec": map[string]interface{}{
					"capabilities": map[string]interface{}{
						"platform": map[string]interface{}{
							"analytics": map[string]interface{}{},
						},
					},
				},
			}

			analytics := patchData["spec"].(map[string]interface{})["capabilities"].(map[string]interface{})["platform"].(map[string]interface{})["analytics"].(map[string]interface{})

			// Only set if not already present
			if !nsFound {
				analytics["namespaceRightSizingRecommendation"] = map[string]interface{}{
					"enabled": true,
				}
			} else {
				// Preserve existing value in patch
				analytics["namespaceRightSizingRecommendation"] = map[string]interface{}{
					"enabled": nsEnabled,
				}
			}

			if !virtFound {
				analytics["virtualizationRightSizingRecommendation"] = map[string]interface{}{
					"enabled": true,
				}
			} else {
				// Preserve existing value in patch
				analytics["virtualizationRightSizingRecommendation"] = map[string]interface{}{
					"enabled": virtEnabled,
				}
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
			if err := r.Client.Get(ctx, key, refreshed); err == nil {
				instance = refreshed.DeepCopy()
			}
		}
	}
	return instance, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AnalyticsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	c := mgr.GetClient()
	ctx := context.Background()

	mcoPred := mcoctrl.GetMCOPredicateFunc()
	cmNamespaceRSPred := rightsizingctrl.GetNamespaceRSConfigMapPredicateFunc(ctx, c)
	cmVirtualizationRSPred := rightsizingctrl.GetVirtualizationRSConfigMapPredicateFunc(ctx, c)

	return ctrl.NewControllerManagedBy(mgr).
		Named("rightsizing").
		For(&mcov1beta2.MultiClusterObservability{}, builder.WithPredicates(mcoPred)).
		Watches(&corev1.ConfigMap{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(cmNamespaceRSPred)).
		Watches(&corev1.ConfigMap{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(cmVirtualizationRSPred)).
		Complete(r)
}
