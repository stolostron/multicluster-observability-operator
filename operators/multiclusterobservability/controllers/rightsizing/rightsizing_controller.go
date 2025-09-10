// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rightsizing

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	mcoctrl "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability"
	analyticsctrl "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability/analytics"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	corev1 "k8s.io/api/core/v1"
)

var log = logf.Log.WithName("controller_rightsizing")

// RightSizingReconciler reconciles a MultiClusterObservability object
type RightSizingReconciler struct {
	Client client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=observability.open-cluster-management.io,resources=multiclusterobservabilities,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=observability.open-cluster-management.io,resources=multiclusterobservabilities/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=observability.open-cluster-management.io,resources=multiclusterobservabilities/finalizers,verbs=update

func (r *RightSizingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	reqLogger.Info("Reconciling RightSizing")

	// Fetch the MultiClusterObservability instance
	mcoList := &mcov1beta2.MultiClusterObservabilityList{}
	err := r.Client.List(context.TODO(), mcoList)
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

	// create rightsizing component
	err = analyticsctrl.CreateRightSizingComponent(ctx, r.Client, instance)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create rightsizing component: %w", err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RightSizingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	c := mgr.GetClient()
	ctx := context.Background()

	mcoPred := mcoctrl.GetMCOPredicateFunc()
	cmNamespaceRSPred := analyticsctrl.GetNamespaceRSConfigMapPredicateFunc(ctx, c)
	cmVirtualizationRSPred := analyticsctrl.GetVirtualizationRSConfigMapPredicateFunc(ctx, c)

	return ctrl.NewControllerManagedBy(mgr).
		Named("rightsizing").
		For(&mcov1beta2.MultiClusterObservability{}, builder.WithPredicates(mcoPred)).
		Watches(&corev1.ConfigMap{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(cmNamespaceRSPred)).
		Watches(&corev1.ConfigMap{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(cmVirtualizationRSPred)).
		Complete(r)
}
