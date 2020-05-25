// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"context"

	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appsv1 "github.com/open-cluster-management/multicloud-operators-placementrule/pkg/apis/apps/v1"
)

var log = logf.Log.WithName("controller_placementrule")
var watchNamespace, _ = k8sutil.GetWatchNamespace()

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new PlacementRule Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcilePlacementRule{
		client:    mgr.GetClient(),
		apiReader: mgr.GetAPIReader(),
		scheme:    mgr.GetScheme(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("placementrule-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	pred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if strings.TrimSpace(e.Meta.GetLabels()["agent"]) == "monitoring" && e.Meta.GetNamespace() == watchNamespace {
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if strings.TrimSpace(e.Meta.GetLabels()["agent"]) == "monitoring" && e.Meta.GetNamespace() == watchNamespace {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
			//return !e.DeleteStateUnknown
		},
	}

	// Watch for changes to primary resource PlacementRule
	err = c.Watch(&source.Kind{Type: &appsv1.PlacementRule{}}, &handler.EnqueueRequestForObject{}, pred)
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcilePlacementRule implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcilePlacementRule{}

// ReconcilePlacementRule reconciles a PlacementRule object
type ReconcilePlacementRule struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client    client.Client
	apiReader client.Reader
	scheme    *runtime.Scheme
}

// Reconcile reads that state of the cluster for a PlacementRule object and makes changes based on the state read
// and what is in the PlacementRule.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcilePlacementRule) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling PlacementRule")

	// Fetch the PlacementRule instance
	instance := &appsv1.PlacementRule{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	for _, decision := range instance.Status.Decisions {
		log.Info("Monitoring operator should be installed in cluster", "cluster_name", decision.ClusterName)
		err = createEndpointConfigCR(r.client, instance.Namespace, decision.ClusterNamespace, decision.ClusterName)
		if err != nil {
			reqLogger.Error(err, "Failed to create endpointmetrics")
			continue
		}
		err = createManifestWork(r.client, decision.ClusterNamespace)
		if err != nil {
			reqLogger.Error(err, "Failed to create manifestwork")
		}
	}

	return reconcile.Result{}, err
}
