// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"context"
	"fmt"
	"os"

	certv1alpha1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

	addonv1alpha1 "github.com/open-cluster-management/api/addon/v1alpha1"
	workv1 "github.com/open-cluster-management/api/work/v1"
	appsv1 "github.com/open-cluster-management/multicloud-operators-placementrule/pkg/apis/apps/v1"
	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/controller/multiclusterobservability"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/util"
)

const (
	ownerLabelKey   = "owner"
	ownerLabelValue = "multicluster-observability-operator"
	certificateName = "observability-managed-cluster-certificate"
	certsName       = "observability-managed-cluster-certs"
	leaseName       = "observability-lease"
)

var (
	log                             = logf.Log.WithName("controller_placementrule")
	watchNamespace                  = config.GetDefaultNamespace()
	isCRoleCreated                  = false
	isClusterManagementAddonCreated = false
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new PlacementRule Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	enableManagedCluster, found := os.LookupEnv("ENABLE_MANAGED_CLUSTER")
	if found && enableManagedCluster == "false" {
		return nil
	}
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcilePlacementRule{
		client:     mgr.GetClient(),
		apiReader:  mgr.GetAPIReader(),
		scheme:     mgr.GetScheme(),
		restMapper: mgr.GetRESTMapper(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {

	// Create a new controller
	c, err := controller.New("placementrule-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	name := config.GetPlacementRuleName()

	pred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Meta.GetName() == name && e.Meta.GetNamespace() == watchNamespace {
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.MetaNew.GetName() == name &&
				e.MetaNew.GetNamespace() == watchNamespace &&
				e.MetaNew.GetResourceVersion() != e.MetaOld.GetResourceVersion() {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Meta.GetName() == name && e.Meta.GetNamespace() == watchNamespace {
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
					Name:      name,
					Namespace: watchNamespace,
				}},
			}
		})

	// secondary watch for observabilityaddon
	err = watchObservabilityaddon(c, mapFn)
	if err != nil {
		return err
	}

	// secondary watch for manifestwork
	gk := schema.GroupKind{Group: workv1.GroupVersion.Group, Kind: "ManifestWork"}
	_, err = r.(*ReconcilePlacementRule).restMapper.RESTMapping(gk, workv1.GroupVersion.Version)
	if err == nil {
		err = watchManifestwork(c, mapFn)
		if err != nil {
			return err
		}
	}

	// secondary watch for mco
	err = watchMCO(c, mapFn)
	if err != nil {
		return err
	}

	// secondary watch for custom whitelist configmap
	err = watchWhitelistCM(c, mapFn)
	if err != nil {
		return err
	}

	// secondary watch for certificate secrets
	err = watchCertficate(c, mapFn)
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
	client     client.Client
	apiReader  client.Reader
	scheme     *runtime.Scheme
	restMapper meta.RESTMapper
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
		return reconcile.Result{}, fmt.Errorf("multicluster observability resource is not available")
	}

	// Fetch the MultiClusterObservability instance
	deleteAll := false
	mco := &mcov1beta1.MultiClusterObservability{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Name: config.GetMonitoringCRName(),
		}, mco)
	if err != nil {
		if errors.IsNotFound(err) {
			deleteAll = true
		} else {
			// Error reading the object - requeue the request.
			return reconcile.Result{}, err
		}
	}

	// Do not reconcile objects if this instance of mch is labeled "paused"
	if config.IsPaused(mco.GetAnnotations()) {
		reqLogger.Info("MCO reconciliation is paused. Nothing more to do.")
		return reconcile.Result{}, nil
	}

	//read image manifest configmap to be used to replace the image for each component.
	if _, err = config.ReadImageManifestConfigMap(r.client); err != nil {
		return reconcile.Result{}, err
	}

	opts := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{ownerLabelKey: ownerLabelValue}),
	}
	epList := &mcov1beta1.ObservabilityAddonList{}
	err = r.client.List(context.TODO(), epList, opts)
	if err != nil {
		reqLogger.Error(err, "Failed to list observabilityaddon resource")
		return reconcile.Result{}, err
	}
	if !deleteAll {
		// create the clusterrole if not there
		if !isCRoleCreated {
			err = createClusterRole(r.client)

			if err != nil {
				return reconcile.Result{}, err
			}
			isCRoleCreated = true
		}
		//Check if ClusterManagementAddon is created or create it
		if !isClusterManagementAddonCreated {
			err := util.CreateClusterManagementAddon(r.client)
			if err != nil {
				return reconcile.Result{}, err
			}
			isClusterManagementAddonCreated = true
		}

		imagePullSecret := &corev1.Secret{}
		err = r.client.Get(context.TODO(),
			types.NamespacedName{
				Name:      mco.Spec.ImagePullSecret,
				Namespace: request.Namespace,
			}, imagePullSecret)
		if err != nil {
			if errors.IsNotFound(err) {
				imagePullSecret = nil
			} else {
				// Error reading the object - requeue the request.
				return reconcile.Result{}, err
			}
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

		currentClusters := []string{}
		for _, ep := range epList.Items {
			currentClusters = append(currentClusters, ep.Namespace)
		}

		for _, decision := range instance.Status.Decisions {
			reqLogger.Info("Monitoring operator should be installed in cluster", "cluster_name", decision.ClusterName)
			currentClusters = util.Remove(currentClusters, decision.ClusterNamespace)
			err = createManagedClusterRes(r.client, mco, imagePullSecret,
				decision.ClusterName, decision.ClusterNamespace)
			if err != nil {
				return reconcile.Result{}, err
			}
		}

		for _, cluster := range currentClusters {
			reqLogger.Info("To delete observabilityAddon", "namespace", cluster)
			err = deleteEndpointConfigCR(r.client, cluster)
			if err != nil {
				reqLogger.Error(err, "Failed to delete observabilityaddon", "namespace", cluster)
				return reconcile.Result{}, err
			}
		}
	} else {
		for _, ep := range epList.Items {
			err = deleteEndpointConfigCR(r.client, ep.Namespace)
			if err != nil {
				reqLogger.Error(err, "Failed to delete observabilityaddon", "namespace", ep.Namespace)
				return reconcile.Result{}, err
			}
		}
		err = deleteClusterRole(r.client)
		if err != nil {
			return reconcile.Result{}, err
		}
		isCRoleCreated = false
		//delete ClusterManagementAddon
		err = util.DeleteClusterManagementAddon(r.client)
		if err != nil {
			return reconcile.Result{}, err
		}
		isClusterManagementAddonCreated = false
	}

	epList = &mcov1beta1.ObservabilityAddonList{}
	err = r.client.List(context.TODO(), epList, opts)
	if err != nil {
		reqLogger.Error(err, "Failed to list observabilityaddon resource")
		return reconcile.Result{}, err
	}
	workList := &workv1.ManifestWorkList{}
	err = r.client.List(context.TODO(), workList, opts)
	if err != nil {
		reqLogger.Error(err, "Failed to list manifestwork resource")
		return reconcile.Result{}, err
	}
	latestClusters := []string{}
	for _, ep := range epList.Items {
		latestClusters = append(latestClusters, ep.Namespace)
	}
	for _, work := range workList.Items {
		if !util.Contains(latestClusters, work.Namespace) {
			reqLogger.Info("To delete manifestwork", "namespace", work.Namespace)
			err = deleteManagedClusterRes(r.client, work.Namespace)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	return reconcile.Result{}, nil
}

func createManagedClusterRes(client client.Client,
	mco *mcov1beta1.MultiClusterObservability, imagePullSecret *corev1.Secret,
	name string, namespace string) error {
	org := multiclusterobservability.GetManagedClusterOrg()
	spec := multiclusterobservability.CreateCertificateSpec(certsName, true,
		multiclusterobservability.GetClientCAIssuer(), false,
		"mc-"+name, []string{org}, []string{})
	err := multiclusterobservability.CreateCertificate(client, nil, nil,
		certificateName, namespace, spec)
	if err != nil {
		return err
	}

	err = createEndpointConfigCR(client, namespace)
	if err != nil {
		log.Error(err, "Failed to create observabilityaddon")
		return err
	}

	err = util.CreateManagedClusterAddonCR(client, namespace)
	if err != nil {
		log.Error(err, "Failed to create ManagedClusterAddon")
		return err
	}

	err = createManifestWork(client, namespace, name, mco, imagePullSecret)
	if err != nil {
		log.Error(err, "Failed to create manifestwork")
		return err
	}
	return nil
}

func deleteManagedClusterRes(client client.Client, namespace string) error {

	managedclusteraddon := &addonv1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.ManagedClusterAddonName,
			Namespace: namespace,
		},
	}
	err := client.Delete(context.TODO(), managedclusteraddon)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	certificate := &certv1alpha1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certificateName,
			Namespace: namespace,
		},
	}
	err = client.Delete(context.TODO(), certificate)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      leaseName,
			Namespace: namespace,
		},
	}
	err = client.Delete(context.TODO(), lease)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	err = deleteManifestWork(client, namespace)
	if err != nil {
		log.Error(err, "Failed to delete manifestwork")
		return err
	}
	return nil
}

func watchObservabilityaddon(c controller.Controller, mapFn handler.ToRequestsFunc) error {
	// Only handle delete event for observabilityaddon
	epPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Meta.GetName() == epConfigName &&
				e.Meta.GetLabels()[ownerLabelKey] == ownerLabelValue {
				return true
			}
			return false
		},
	}

	err := c.Watch(&source.Kind{Type: &mcov1beta1.ObservabilityAddon{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: mapFn,
		},
		epPred)
	if err != nil {
		return err
	}
	return nil
}

func watchManifestwork(c controller.Controller, mapFn handler.ToRequestsFunc) error {
	workPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.MetaNew.GetName() == workName &&
				e.MetaNew.GetLabels()[ownerLabelKey] == ownerLabelValue &&
				e.MetaNew.GetResourceVersion() != e.MetaOld.GetResourceVersion() {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Meta.GetName() == workName && e.Meta.GetLabels()[ownerLabelKey] == ownerLabelValue {
				return true
			}
			return false
		},
	}

	err := c.Watch(&source.Kind{Type: &workv1.ManifestWork{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: mapFn,
		},
		workPred)
	if err != nil {
		return err
	}
	return nil
}

func watchWhitelistCM(c controller.Controller, mapFn handler.ToRequestsFunc) error {
	customWhitelistPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Meta.GetName() == config.WhitelistCustomConfigMapName &&
				e.Meta.GetNamespace() == config.GetDefaultNamespace() {
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.MetaNew.GetName() == config.WhitelistCustomConfigMapName &&
				e.MetaNew.GetNamespace() == config.GetDefaultNamespace() &&
				e.MetaNew.GetResourceVersion() != e.MetaOld.GetResourceVersion() {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Meta.GetName() == config.WhitelistCustomConfigMapName &&
				e.Meta.GetNamespace() == config.GetDefaultNamespace() {
				return true
			}
			return true
		},
	}

	err := c.Watch(&source.Kind{Type: &corev1.ConfigMap{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: mapFn,
		},
		customWhitelistPred)
	if err != nil {
		return err
	}
	return nil
}

func watchMCO(c controller.Controller, mapFn handler.ToRequestsFunc) error {
	mcoPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return true
		},
	}

	err := c.Watch(&source.Kind{Type: &mcov1beta1.MultiClusterObservability{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: mapFn,
		},
		mcoPred)
	if err != nil {
		return err
	}
	return nil
}

func watchCertficate(c controller.Controller, mapFn handler.ToRequestsFunc) error {
	customWhitelistPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Meta.GetName() == certsName ||
				e.Meta.GetName() == config.ServerCerts &&
					e.Meta.GetNamespace() == config.GetDefaultNamespace() {
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if (e.MetaNew.GetName() == certsName ||
				e.MetaNew.GetName() == config.ServerCerts &&
					e.MetaNew.GetNamespace() == config.GetDefaultNamespace()) &&
				e.MetaNew.GetResourceVersion() != e.MetaOld.GetResourceVersion() {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}

	err := c.Watch(&source.Kind{Type: &corev1.Secret{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: mapFn,
		},
		customWhitelistPred)
	if err != nil {
		return err
	}
	return nil
}
