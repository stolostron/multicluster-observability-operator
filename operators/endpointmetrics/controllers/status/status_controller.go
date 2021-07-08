// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project.
package status

import (
	"context"
	"os"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	oav1beta1 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
)

var (
	log = ctrl.Log.WithName("controllers").WithName("Status")
)

const (
	obAddonName = "observability-addon"
)

var (
	namespace    = os.Getenv("WATCH_NAMESPACE")
	hubNamespace = os.Getenv("HUB_NAMESPACE")
)

// StatusReconciler reconciles status object
type StatusReconciler struct {
	Client    client.Client
	Scheme    *runtime.Scheme
	HubClient client.Client
}

// Reconcile reads that state of the cluster for a ObservabilityAddon object and makes changes based on the state read
// and what is in the ObservabilityAddon.Status
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *StatusReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	log.Info("Reconciling")

	// Fetch the ObservabilityAddon instance in hub cluster
	hubObsAddon := &oav1beta1.ObservabilityAddon{}
	err := r.HubClient.Get(ctx, types.NamespacedName{Name: obAddonName, Namespace: hubNamespace}, hubObsAddon)
	if err != nil {
		log.Error(err, "Failed to get observabilityaddon in hub cluster", "namespace", hubNamespace)
		return ctrl.Result{}, err
	}

	// Fetch the ObservabilityAddon instance in local cluster
	obsAddon := &oav1beta1.ObservabilityAddon{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: obAddonName, Namespace: namespace}, obsAddon)
	if err != nil {
		log.Error(err, "Failed to get observabilityaddon", "namespace", namespace)
		return ctrl.Result{}, err
	}

	hubObsAddon.Status = obsAddon.Status

	err = r.HubClient.Status().Update(ctx, hubObsAddon)
	if err != nil {
		log.Error(err, "Failed to update status for observabilityaddon in hub cluster", "namespace", hubNamespace)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *StatusReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if os.Getenv("NAMESPACE") != "" {
		namespace = os.Getenv("NAMESPACE")
	}

	pred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetNamespace() == namespace &&
				!reflect.DeepEqual(e.ObjectNew.(*oav1beta1.ObservabilityAddon).Status,
					e.ObjectOld.(*oav1beta1.ObservabilityAddon).Status) {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&oav1beta1.ObservabilityAddon{}, builder.WithPredicates(pred)).
		Complete(r)
}
