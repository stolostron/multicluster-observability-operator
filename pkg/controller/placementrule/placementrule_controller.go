// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	workv1 "github.com/open-cluster-management/api/work/v1"
	appsv1 "github.com/open-cluster-management/multicloud-operators-placementrule/pkg/apis/apps/v1"
	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/util"
)

const (
	placementRuleName = "open-cluster-management-observability"
	ownerLabelKey     = "owner"
	ownerLabelValue   = "multicluster-operator"
)

var log = logf.Log.WithName("controller_placementrule")
var watchNamespace = config.GetDefaultNamespace()

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
			if e.Meta.GetName() == placementRuleName && e.Meta.GetNamespace() == watchNamespace {
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.MetaNew.GetName() == placementRuleName && e.MetaNew.GetNamespace() == watchNamespace {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Meta.GetName() == placementRuleName && e.Meta.GetNamespace() == watchNamespace {
				return e.DeleteStateUnknown
			}
			return false
		},
	}

	// Watch for changes to primary resource PlacementRule
	err = c.Watch(&source.Kind{Type: &appsv1.PlacementRule{}}, &handler.EnqueueRequestForObject{}, pred)
	if err != nil {
		return err
	}

	mapFn := handler.ToRequestsFunc(
		func(a handler.MapObject) []reconcile.Request {
			return []reconcile.Request{
				{NamespacedName: types.NamespacedName{
					Name:      placementRuleName,
					Namespace: watchNamespace,
				}},
			}
		})

	// Only handle delete event for observabilityaddon
	epPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Meta.GetName() == epConfigName && e.Meta.GetAnnotations()[ownerLabelKey] == ownerLabelValue {
				return true
			}
			return false
		},
	}

	// secondary watch for observabilityaddon
	err = c.Watch(&source.Kind{Type: &mcov1beta1.ObservabilityAddon{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: mapFn,
		},
		epPred)
	if err != nil {
		return err
	}

	workPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.MetaNew.GetName() == workName && e.MetaNew.GetAnnotations()[ownerLabelKey] == ownerLabelValue {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Meta.GetName() == workName && e.Meta.GetAnnotations()[ownerLabelKey] == ownerLabelValue {
				return true
			}
			return false
		},
	}

	// secondary watch for manifestwork
	err = c.Watch(&source.Kind{Type: &workv1.ManifestWork{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: mapFn,
		},
		workPred)
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

	if config.GetMonitoringCRName() == "" {
		return reconcile.Result{}, nil
	}

	// Fetch the MultiClusterObservability instance
	mco := &mcov1beta1.MultiClusterObservability{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Name: config.GetMonitoringCRName(),
		}, mco)
	if err != nil {
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	imagePullSecret := &corev1.Secret{}
	err = r.client.Get(context.TODO(),
		types.NamespacedName{
			Name:      mco.Spec.ImagePullSecret,
			Namespace: request.Namespace,
		}, imagePullSecret)
	if err != nil {
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	mco.Namespace = watchNamespace
	// Fetch the PlacementRule instance
	instance := &appsv1.PlacementRule{}
	err = r.client.Get(context.TODO(), request.NamespacedName, instance)
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

	epList := &mcov1beta1.ObservabilityAddonList{}
	err = r.client.List(context.TODO(), epList)
	if err != nil {
		reqLogger.Error(err, "Failed to list observabilityaddon resource")
		return reconcile.Result{}, err
	}
	currentClusters := []string{}
	for _, ep := range epList.Items {
		if ep.Name == epConfigName && ep.Annotations[ownerLabelKey] == ownerLabelValue {
			currentClusters = append(currentClusters, ep.Namespace)
		}
	}

	for _, decision := range instance.Status.Decisions {
		reqLogger.Info("Monitoring operator should be installed in cluster", "cluster_name", decision.ClusterName)
		currentClusters = util.Remove(currentClusters, decision.ClusterNamespace)
		err = createEndpointConfigCR(r.client, mco.Namespace, decision.ClusterNamespace, decision.ClusterName)
		if err != nil {
			reqLogger.Error(err, "Failed to create observabilityaddon")
			return reconcile.Result{}, err
		}
		err = createManifestWork(r.client, decision.ClusterNamespace, mco, imagePullSecret)
		if err != nil {
			reqLogger.Error(err, "Failed to create manifestwork")
			return reconcile.Result{}, err
		}
	}

	for _, cluster := range currentClusters {
		reqLogger.Info("Monitoring opearator will be uninstalled", "namespace", cluster)
		err = deleteEndpointConfigCR(r.client, cluster)
		if err != nil {
			reqLogger.Error(err, "Failed to delete observabilityaddon", "namespace", cluster)
			return reconcile.Result{}, err
		}
		err = deleteManifestWork(r.client, cluster)
		if err != nil {
			reqLogger.Error(err, "Failed to create manifestwork")
			return reconcile.Result{}, err
		}

	}

	return reconcile.Result{}, nil
}
