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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlbuilder "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// MCOAAgentReconciler reconciles the MCOA components on the managed cluster.
type MCOAAgentReconciler struct {
	client.Client
	Log       logr.Logger
	Scheme    *runtime.Scheme
	Recorder  record.EventRecorder
	Namespace string
	HubInfo   *operatorconfig.HubInfo

	cmoReconciler *cmoConfigReconciler
}

// NewMCOAAgentReconciler creates a new MCOAAgentReconciler and initializes its sub-reconcilers.
func NewMCOAAgentReconciler(client client.Client, log logr.Logger, scheme *runtime.Scheme, recorder record.EventRecorder, namespace string, hubInfo *operatorconfig.HubInfo) *MCOAAgentReconciler {
	registerMetrics()
	return &MCOAAgentReconciler{
		Client:    client,
		Log:       log,
		Scheme:    scheme,
		Recorder:  recorder,
		Namespace: namespace,
		HubInfo:   hubInfo,
		cmoReconciler: &cmoConfigReconciler{
			Client:    client,
			Log:       log.WithName("CMO"),
			Recorder:  recorder,
			Namespace: namespace,
			HubInfo:   hubInfo,
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
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []ctrl.Request {
				hubAmAccessorSecret := observabilityendpoint.AppendHubClusterID(observabilityendpoint.HubAmAccessorSecretName, r.HubInfo)
				hubAmRouterCASecret := observabilityendpoint.AppendHubClusterID(observabilityendpoint.HubAmRouterCASecretName, r.HubInfo)

				if obj.GetName() != hubAmAccessorSecret && obj.GetName() != hubAmRouterCASecret {
					return nil
				}

				var requests []ctrl.Request
				// Monitoring stacks in OpenShift are namespace-scoped for secrets.
				// We only trigger the ConfigMap in the same namespace as the secret.
				switch obj.GetNamespace() {
				case operatorconfig.OCPClusterMonitoringNamespace:
					requests = append(requests, ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
							Namespace: operatorconfig.OCPClusterMonitoringNamespace,
						},
					})
				case operatorconfig.OCPUserWorkloadMonitoringNamespace:
					requests = append(requests, ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
							Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
						},
					})
				}
				return requests
			}),
			// Only react to secrets in the namespaces we monitor
			ctrlbuilder.WithPredicates(observabilityendpoint.SecretDataChangedPredicate("", "")),
		).
		Complete(r)
}
