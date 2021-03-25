// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"context"
	"errors"

	"github.com/go-logr/logr"
	certv1alpha1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha1"
	ocinfrav1 "github.com/openshift/api/config/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	addonv1alpha1 "github.com/open-cluster-management/api/addon/v1alpha1"
	workv1 "github.com/open-cluster-management/api/work/v1"
	placementv1 "github.com/open-cluster-management/multicloud-operators-placementrule/pkg/apis/apps/v1"
	mcov1beta1 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta1"
	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta2"
	mcoctrl "github.com/open-cluster-management/multicluster-observability-operator/controllers/multiclusterobservability"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/util"
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

// PlacementRuleReconciler reconciles a PlacementRule object
type PlacementRuleReconciler struct {
	Client     client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
	APIReader  client.Reader
	RESTMapper meta.RESTMapper
}

// +kubebuilder:rbac:groups=observability.open-cluster-management.io,resources=placementrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=observability.open-cluster-management.io,resources=placementrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=observability.open-cluster-management.io,resources=placementrules/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Modify the Reconcile function to compare the state specified by
// the PlacementRule object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.0/pkg/reconcile
func (r *PlacementRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	reqLogger.Info("Reconciling PlacementRule")

	if config.GetMonitoringCRName() == "" {
		reqLogger.Info("multicluster observability resource is not available")
		return ctrl.Result{}, nil
	}

	deleteAll := false
	// Fetch the MultiClusterObservability instance
	mco := &mcov1beta2.MultiClusterObservability{}
	err := r.Client.Get(context.TODO(),
		types.NamespacedName{
			Name: config.GetMonitoringCRName(),
		}, mco)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			deleteAll = true
		} else {
			// Error reading the object - requeue the request.
			return ctrl.Result{}, err
		}
	}
	placement := &placementv1.PlacementRule{}
	if !deleteAll {
		// Fetch the PlacementRule instance
		err = r.Client.Get(context.TODO(), req.NamespacedName, placement)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				deleteAll = true
			} else {
				// Error reading the object - requeue the request.
				return ctrl.Result{}, err
			}
		}
	}

	// Do not reconcile objects if this instance of mch is labeled "paused"
	if config.IsPaused(mco.GetAnnotations()) {
		reqLogger.Info("MCO reconciliation is paused. Nothing more to do.")
		return ctrl.Result{}, nil
	}

	//read image manifest configmap to be used to replace the image for each component.
	if _, err = config.ReadImageManifestConfigMap(r.Client); err != nil {
		return ctrl.Result{}, err
	}

	opts := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{ownerLabelKey: ownerLabelValue}),
	}
	obsAddonList := &mcov1beta1.ObservabilityAddonList{}
	err = r.Client.List(context.TODO(), obsAddonList, opts)
	if err != nil {
		reqLogger.Error(err, "Failed to list observabilityaddon resource")
		return ctrl.Result{}, err
	}

	if !deleteAll {
		res, err := createAllRelatedRes(r.Client, r.RESTMapper, req, mco, placement, obsAddonList)
		if err != nil {
			return res, err
		}
	} else {
		res, err := deleteAllObsAddons(r.Client, obsAddonList)
		if err != nil {
			return res, err
		}
	}

	obsAddonList = &mcov1beta1.ObservabilityAddonList{}
	err = r.Client.List(context.TODO(), obsAddonList, opts)
	if err != nil {
		reqLogger.Error(err, "Failed to list observabilityaddon resource")
		return ctrl.Result{}, err
	}
	workList := &workv1.ManifestWorkList{}
	err = r.Client.List(context.TODO(), workList, opts)
	if err != nil {
		reqLogger.Error(err, "Failed to list manifestwork resource")
		return ctrl.Result{}, err
	}
	latestClusters := []string{}
	staleAddons := []string{}
	for _, addon := range obsAddonList.Items {
		latestClusters = append(latestClusters, addon.Namespace)
		staleAddons = append(staleAddons, addon.Namespace)
	}
	for _, work := range workList.Items {
		if work.Name != work.Namespace+operatorWorkNameSuffix &&
			work.Name != work.Namespace+resWorkNameSuffix {
			reqLogger.Info("To delete invalid manifestwork", "name", work.Name, "namespace", work.Namespace)
			err = deleteManifestWork(r.Client, work.Name, work.Namespace)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		if !util.Contains(latestClusters, work.Namespace) {
			reqLogger.Info("To delete manifestwork", "namespace", work.Namespace)
			err = deleteManagedClusterRes(r.Client, work.Namespace)
			if err != nil {
				return ctrl.Result{}, err
			}
		} else {
			staleAddons = util.Remove(staleAddons, work.Namespace)
		}
	}

	// delete stale addons if manifestwork does not exist
	for _, addon := range staleAddons {
		err = deleteStaleObsAddon(r.Client, addon)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	err = updateAddonStatus(r.Client, *obsAddonList)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.Client.List(context.TODO(), workList, opts)
	if err != nil {
		reqLogger.Error(err, "Failed to list manifestwork resource")
		return ctrl.Result{}, err
	}
	if len(workList.Items) == 0 && deleteAll {
		err = deleteGlobalResource(r.Client)
	}

	return ctrl.Result{}, err
}

func createAllRelatedRes(
	client client.Client,
	restMapper meta.RESTMapper,
	request ctrl.Request,
	mco *mcov1beta2.MultiClusterObservability,
	placement *placementv1.PlacementRule,
	obsAddonList *mcov1beta1.ObservabilityAddonList) (ctrl.Result, error) {

	// create the clusterrole if not there
	if !isCRoleCreated {
		err := createClusterRole(client)
		if err != nil {
			return ctrl.Result{}, err
		}
		err = createResourceRole(client)
		if err != nil {
			return ctrl.Result{}, err
		}
		isCRoleCreated = true
	}
	//Check if ClusterManagementAddon is created or create it
	if !isClusterManagementAddonCreated {
		err := util.CreateClusterManagementAddon(client)
		if err != nil {
			return ctrl.Result{}, err
		}
		isClusterManagementAddonCreated = true
	}

	imagePullSecret := &corev1.Secret{}
	err := client.Get(context.TODO(),
		types.NamespacedName{
			Name:      mco.Spec.ImagePullSecret,
			Namespace: request.Namespace,
		}, imagePullSecret)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			imagePullSecret = nil
		} else {
			// Error reading the object - requeue the request.
			return ctrl.Result{}, err
		}
	}
	mco.Namespace = watchNamespace

	currentClusters := []string{}
	for _, ep := range obsAddonList.Items {
		currentClusters = append(currentClusters, ep.Namespace)
	}

	failedCreateManagedClusterRes := false
	for _, decision := range placement.Status.Decisions {
		log.Info("Monitoring operator should be installed in cluster", "cluster_name", decision.ClusterName)
		currentClusters = util.Remove(currentClusters, decision.ClusterNamespace)
		err = createManagedClusterRes(client, restMapper, mco, imagePullSecret,
			decision.ClusterName, decision.ClusterNamespace)
		if err != nil {
			failedCreateManagedClusterRes = true
			log.Error(err, "Failed to create managedcluster resources", "namespace", decision.ClusterNamespace)
		}
	}

	failedDeleteOba := false
	for _, cluster := range currentClusters {
		log.Info("To delete observabilityAddon", "namespace", cluster)
		err = deleteObsAddon(client, cluster)
		if err != nil {
			failedDeleteOba = true
			log.Error(err, "Failed to delete observabilityaddon", "namespace", cluster)
		}
	}

	if failedCreateManagedClusterRes || failedDeleteOba {
		return ctrl.Result{}, errors.New("Failed to create managedcluster resources or" +
			" failed to delete observabilityaddon, skip and reconcile later")
	}

	return ctrl.Result{}, nil
}

func deleteAllObsAddons(
	client client.Client,
	obsAddonList *mcov1beta1.ObservabilityAddonList) (ctrl.Result, error) {
	for _, ep := range obsAddonList.Items {
		err := deleteObsAddon(client, ep.Namespace)
		if err != nil {
			log.Error(err, "Failed to delete observabilityaddon", "namespace", ep.Namespace)
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func deleteGlobalResource(c client.Client) error {
	err := deleteClusterRole(c)
	if err != nil {
		return err
	}
	err = deleteResourceRole(c)
	if err != nil {
		return err
	}
	isCRoleCreated = false
	//delete ClusterManagementAddon
	err = util.DeleteClusterManagementAddon(c)
	if err != nil {
		return err
	}
	isClusterManagementAddonCreated = false
	return nil
}

func createManagedClusterRes(client client.Client, restMapper meta.RESTMapper,
	mco *mcov1beta2.MultiClusterObservability, imagePullSecret *corev1.Secret,
	name string, namespace string) error {
	org := mcoctrl.GetManagedClusterOrg()
	spec := mcoctrl.CreateCertificateSpec(certsName, true,
		mcoctrl.GetClientCAIssuer(), false,
		"mc-"+name, []string{org}, []string{})
	err := mcoctrl.CreateCertificate(client, nil, nil,
		certificateName, namespace, spec)
	if err != nil {
		return err
	}

	err = createObsAddon(client, namespace)
	if err != nil {
		log.Error(err, "Failed to create observabilityaddon")
		return err
	}

	err = util.CreateManagedClusterAddonCR(client, namespace)
	if err != nil {
		log.Error(err, "Failed to create ManagedClusterAddon")
		return err
	}

	err = createManifestWorks(client, restMapper, namespace, name, mco, imagePullSecret)
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
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}

	certificate := &certv1alpha1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certificateName,
			Namespace: namespace,
		},
	}
	err = client.Delete(context.TODO(), certificate)
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}

	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      leaseName,
			Namespace: namespace,
		},
	}
	err = client.Delete(context.TODO(), lease)
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}

	err = deleteManifestWorks(client, namespace)
	if err != nil {
		log.Error(err, "Failed to delete manifestwork")
		return err
	}
	return nil
}

func watchObservabilityaddon(c controller.Controller, mapFn handler.MapFunc) error {
	epPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetName() == obsAddonName &&
				e.ObjectNew.GetLabels()[ownerLabelKey] == ownerLabelValue {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetName() == obsAddonName &&
				e.Object.GetLabels()[ownerLabelKey] == ownerLabelValue {
				return true
			}
			return false
		},
	}

	err := c.Watch(&source.Kind{Type: &mcov1beta1.ObservabilityAddon{}},
		handler.EnqueueRequestsFromMapFunc(mapFn),
		epPred)
	if err != nil {
		return err
	}
	return nil
}

func watchManifestwork(c controller.Controller, mapFn handler.MapFunc) error {
	workPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetLabels()[ownerLabelKey] == ownerLabelValue &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetLabels()[ownerLabelKey] == ownerLabelValue {
				return true
			}
			return false
		},
	}

	err := c.Watch(&source.Kind{Type: &workv1.ManifestWork{}},
		handler.EnqueueRequestsFromMapFunc(mapFn),
		workPred)
	if err != nil {
		return err
	}
	return nil
}

func watchAllowlistCM(c controller.Controller, mapFn handler.MapFunc) error {
	customAllowlistPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetName() == config.AllowlistCustomConfigMapName &&
				e.Object.GetNamespace() == config.GetDefaultNamespace() {
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetName() == config.AllowlistCustomConfigMapName &&
				e.ObjectNew.GetNamespace() == config.GetDefaultNamespace() &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetName() == config.AllowlistCustomConfigMapName &&
				e.Object.GetNamespace() == config.GetDefaultNamespace() {
				return true
			}
			return false
		},
	}

	err := c.Watch(&source.Kind{Type: &corev1.ConfigMap{}},
		handler.EnqueueRequestsFromMapFunc(mapFn),
		customAllowlistPred)
	if err != nil {
		return err
	}
	return nil
}

func watchMCO(c controller.Controller, mapFn handler.MapFunc) error {
	mcoPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return true
		},
	}

	err := c.Watch(&source.Kind{Type: &mcov1beta2.MultiClusterObservability{}},
		handler.EnqueueRequestsFromMapFunc(mapFn),
		mcoPred)
	if err != nil {
		return err
	}
	return nil
}

func watchCertficate(c controller.Controller, mapFn handler.MapFunc) error {
	customAllowlistPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetName() == certsName ||
				e.Object.GetName() == config.ServerCerts &&
					e.Object.GetNamespace() == config.GetDefaultNamespace() {
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if (e.ObjectNew.GetName() == certsName ||
				e.ObjectNew.GetName() == config.ServerCerts &&
					e.ObjectNew.GetNamespace() == config.GetDefaultNamespace()) &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}

	err := c.Watch(&source.Kind{Type: &corev1.Secret{}},
		handler.EnqueueRequestsFromMapFunc(mapFn),
		customAllowlistPred)
	if err != nil {
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PlacementRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Create a new controller
	c, err := controller.New("placementrule-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	name := config.GetPlacementRuleName()

	pred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetName() == name && e.Object.GetNamespace() == watchNamespace {
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetName() == name &&
				e.ObjectNew.GetNamespace() == watchNamespace &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetName() == name && e.Object.GetNamespace() == watchNamespace {
				return e.DeleteStateUnknown
			}
			return false
		},
	}

	// Watch for changes to primary resource PlacementRule
	err = c.Watch(&source.Kind{Type: &placementv1.PlacementRule{}}, &handler.EnqueueRequestForObject{}, pred)
	if err != nil {
		return err
	}

	mapFn := handler.MapFunc(
		func(a client.Object) []ctrl.Request {
			return []ctrl.Request{
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
	_, err = r.RESTMapper.RESTMapping(gk, workv1.GroupVersion.Version)
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

	// secondary watch for custom allowlist configmap
	err = watchAllowlistCM(c, mapFn)
	if err != nil {
		return err
	}

	// secondary watch for certificate secrets
	err = watchCertficate(c, mapFn)
	if err != nil {
		return err
	}

	// watch APIServer for kubeconfig
	gk = schema.GroupKind{Group: ocinfrav1.GroupVersion.Group, Kind: "APIServer"}
	_, err = r.RESTMapper.RESTMapping(gk, ocinfrav1.GroupVersion.Version)
	if err == nil {
		pred = predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return false
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				if e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
					return true
				}
				return false
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return false
			},
		}
		err = c.Watch(&source.Kind{Type: &ocinfrav1.APIServer{}}, &handler.EnqueueRequestForObject{}, pred)
		if err != nil {
			return err
		}
	}

	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		For(&placementv1.PlacementRule{}).
		Complete(r)
}
