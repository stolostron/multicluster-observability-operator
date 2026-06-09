// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package mcoa

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/observabilityendpoint"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlbuilder "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MCOAAgentReconciler reconciles the MCOA components on the managed cluster.
type MCOAAgentReconciler struct {
	client.Client
	Log            logr.Logger
	Scheme         *runtime.Scheme
	Recorder       record.EventRecorder
	Namespace      string
	ClusterID      string
	HubInfo        *operatorconfig.HubInfo
	CASecret       string
	CertSecret     string
	AccessorSecret string

	cmoReconciler *cmoConfigReconciler
}

// NewMCOAAgentReconciler creates a new MCOAAgentReconciler and initializes its sub-reconcilers.
func NewMCOAAgentReconciler(
	client client.Client,
	log logr.Logger,
	scheme *runtime.Scheme,
	recorder record.EventRecorder,
	namespace string,
	clusterID string,
	hubInfo *operatorconfig.HubInfo,
	caSecret string,
	certSecret string,
	accessorSecret string,
) *MCOAAgentReconciler {
	registerMetrics()
	return &MCOAAgentReconciler{
		Client:         client,
		Log:            log,
		Scheme:         scheme,
		Recorder:       recorder,
		Namespace:      namespace,
		ClusterID:      clusterID,
		HubInfo:        hubInfo,
		CASecret:       caSecret,
		CertSecret:     certSecret,
		AccessorSecret: accessorSecret,
		cmoReconciler: &cmoConfigReconciler{
			Client:         client,
			Log:            log.WithName("CMO"),
			Recorder:       recorder,
			Namespace:      namespace,
			ClusterID:      clusterID,
			HubInfo:        hubInfo,
			CASecret:       caSecret,
			CertSecret:     certSecret,
			AccessorSecret: accessorSecret,
		},
	}
}

func (r *MCOAAgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log.Info("Reconciling MCOA Agent", "name", req.Name, "namespace", req.Namespace)

	switch {
	case req.Name == operatorconfig.OCPClusterMonitoringConfigMapName && req.Namespace == operatorconfig.OCPClusterMonitoringNamespace:
		if err := r.cmoReconciler.reconcile(ctx, req.NamespacedName); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to reconcile CMO config: %w", err)
		}
	case req.Name == operatorconfig.OCPUserWorkloadMonitoringConfigMap && req.Namespace == operatorconfig.OCPUserWorkloadMonitoringNamespace:
		if err := r.cmoReconciler.reconcileUWLConfig(ctx); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to reconcile UWL config: %w", err)
		}
	default:
		r.Log.V(1).Info("Ignoring event for unmanaged resource", "name", req.Name, "namespace", req.Namespace)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MCOAAgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Watch ConfigMaps we manage across multiple namespaces.
		// The cache FieldSelectors already limit this to exactly the CMs we need.
		For(&corev1.ConfigMap{}, ctrlbuilder.WithPredicates(observabilityendpoint.ConfigMapDataChangedPredicate("", ""))).
		Complete(r)
}
