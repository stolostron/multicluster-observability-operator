// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package mcoa

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	prometheusv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/observabilityendpoint"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlbuilder "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// MCOAAgentReconciler reconciles the MCOA components on the managed cluster.
type MCOAAgentReconciler struct {
	client.Client
	Log                           logr.Logger
	Scheme                        *runtime.Scheme
	Recorder                      events.EventRecorder
	Namespace                     string
	ClusterID                     string
	ClusterName                   string
	HubAlertmanagerURL            string
	CASecret                      string
	CertSecret                    string
	AccessorSecret                string
	EnablePlatformAlertForwarding bool
	EnableUWLAlertForwarding      bool
	OLMAvailable                  bool
}

// NewMCOAAgentReconciler creates a new MCOAAgentReconciler.
func NewMCOAAgentReconciler(
	client client.Client,
	log logr.Logger,
	scheme *runtime.Scheme,
	recorder events.EventRecorder,
	namespace string,
	clusterID string,
	clusterName string,
	hubAlertmanagerURL string,
	caSecret string,
	certSecret string,
	accessorSecret string,
	enablePlatformAlertForwarding bool,
	enableUWLAlertForwarding bool,
	olmAvailable bool,
) *MCOAAgentReconciler {
	return &MCOAAgentReconciler{
		Client:                        client,
		Log:                           log,
		Scheme:                        scheme,
		Recorder:                      recorder,
		Namespace:                     namespace,
		ClusterID:                     clusterID,
		ClusterName:                   clusterName,
		HubAlertmanagerURL:            hubAlertmanagerURL,
		CASecret:                      caSecret,
		CertSecret:                    certSecret,
		AccessorSecret:                accessorSecret,
		EnablePlatformAlertForwarding: enablePlatformAlertForwarding,
		EnableUWLAlertForwarding:      enableUWLAlertForwarding,
		OLMAvailable:                  olmAvailable,
	}
}

func (r *MCOAAgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log.V(1).Info("Reconciling MCOA Agent", "name", req.Name, "namespace", req.Namespace)

	cooStatusErr := r.WriteCOOStatus(ctx)
	if cooStatusErr != nil {
		r.Log.Error(cooStatusErr, "failed to update COO status ClusterClaims")
	}

	switch {
	case req.Name == operatorconfig.OCPClusterMonitoringConfigMapName && req.Namespace == operatorconfig.OCPClusterMonitoringNamespace:
		if err := r.ReconcileCMOPlatformConfig(ctx); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to reconcile CMO config: %w", err)
		}
	case req.Name == operatorconfig.OCPUserWorkloadMonitoringConfigMap && req.Namespace == operatorconfig.OCPUserWorkloadMonitoringNamespace:
		if err := r.ReconcileCMOUWLConfig(ctx); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to reconcile UWL config: %w", err)
		}

	case isManagedCRDName(req.Name) && req.Namespace == "":
		r.Log.Info("OBO CRD event, re-applying all managed CRDs", "crd", req.Name)
		if err := DeployCRDs(ctx, r.Client); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to restore OBO CRDs after event on %s: %w", req.Name, err)
		}

	default:
		r.Log.V(1).Info("Ignoring event for unmanaged resource", "name", req.Name, "namespace", req.Namespace)
	}

	if cooStatusErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update COO status ClusterClaims: %w", cooStatusErr)
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MCOAAgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	b := ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}, ctrlbuilder.WithPredicates(observabilityendpoint.ConfigMapDataChangedPredicate("", ""))).
		WatchesMetadata(
			&apiextensionsv1.CustomResourceDefinition{},
			&handler.EnqueueRequestForObject{},
			ctrlbuilder.WithPredicates(predicate.Funcs{
				DeleteFunc: func(e event.DeleteEvent) bool {
					if e.Object == nil {
						return false
					}
					return isManagedCRDName(e.Object.GetName())
				},
				CreateFunc:  func(_ event.CreateEvent) bool { return false },
				UpdateFunc:  func(_ event.UpdateEvent) bool { return false },
				GenericFunc: func(_ event.GenericEvent) bool { return false },
			}),
		).
		Watches(
			&prometheusv1alpha1.ScrapeConfig{},
			handler.EnqueueRequestsFromMapFunc(r.mapComponentLabelToRequests(
				platformMetricsCollectorRawComponent,
				userWorkloadMetricsCollectorRawComponent,
			)),
		).
		Watches(
			&prometheusv1alpha1.PrometheusAgent{},
			handler.EnqueueRequestsFromMapFunc(r.mapComponentLabelToRequests(
				platformMetricsCollectorComponent,
				userWorkloadMetricsCollectorComponent,
			)),
		)

	if r.OLMAvailable {
		r.Log.Info("OLM detected, watching Subscriptions for COO status")
		b = b.Watches(
			subscriptionUnstructured(),
			handler.EnqueueRequestsFromMapFunc(func(_ context.Context, _ client.Object) []reconcile.Request {
				return []reconcile.Request{{NamespacedName: client.ObjectKey{
					Name:      CooInstalledClaimName,
					Namespace: "",
				}}}
			}),
			ctrlbuilder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					return isCOOSubscriptionObject(e.Object)
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return isCOOSubscriptionObject(e.Object)
				},
				UpdateFunc:  func(_ event.UpdateEvent) bool { return false },
				GenericFunc: func(_ event.GenericEvent) bool { return false },
			}),
		)
	} else {
		r.Log.Info("OLM not detected, skipping Subscription watch for COO status")
	}

	return b.Complete(r)
}

func (r *MCOAAgentReconciler) mapComponentLabelToRequests(
	platformComp, uwlComp string,
) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		if obj.GetNamespace() != r.Namespace {
			return nil
		}
		labels := obj.GetLabels()
		if labels == nil {
			return nil
		}
		comp := labels[labelKeyComponent]
		switch comp {
		case platformComp:
			return []reconcile.Request{
				{
					NamespacedName: client.ObjectKey{
						Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
						Namespace: operatorconfig.OCPClusterMonitoringNamespace,
					},
				},
			}
		case uwlComp:
			return []reconcile.Request{
				{
					NamespacedName: client.ObjectKey{
						Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
						Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
					},
				},
			}
		}
		return nil
	}
}

func subscriptionUnstructured() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "operators.coreos.com",
		Version: "v1alpha1",
		Kind:    "Subscription",
	})
	return obj
}

func isCOOSubscriptionObject(obj client.Object) bool {
	if obj == nil {
		return false
	}
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return obj.GetName() == cooSubscriptionName
	}
	specName, _, _ := unstructured.NestedString(u.Object, "spec", "name")
	return specName == cooSubscriptionName
}
