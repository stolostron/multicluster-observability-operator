// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	operatorv1 "github.com/openshift/api/operator/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	mcov1beta1 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	commonutil "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/util"
	mchv1 "github.com/open-cluster-management/multiclusterhub-operator/pkg/apis/operator/v1"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
)

const (
	ownerLabelKey             = "owner"
	ownerLabelValue           = "multicluster-observability-operator"
	managedClusterObsCertName = "observability-managed-cluster-certs"
	nonOCP                    = "N/A"
)

var (
	log                             = logf.Log.WithName("controller_placementrule")
	watchNamespace                  = config.GetDefaultNamespace()
	isCRoleCreated                  = false
	isClusterManagementAddonCreated = false
	isplacementControllerRunnning   = false
	managedClusterList              = map[string]string{}
)

// PlacementRuleReconciler reconciles a PlacementRule object
type PlacementRuleReconciler struct {
	Client     client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
	CRDMap     map[string]bool
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

	// Do not reconcile objects if this instance of mch is labeled "paused"
	if config.IsPaused(mco.GetAnnotations()) {
		reqLogger.Info("MCO reconciliation is paused. Nothing more to do.")
		return ctrl.Result{}, nil
	}

	// check if the MCH CRD exists
	mchCrdExists, _ := r.CRDMap[config.MCHCrdName]
	// requeue after 10 seconds if the mch crd exists and image image manifests map is empty
	if mchCrdExists && len(config.GetImageManifests()) == 0 {
		// if the mch CR is not ready, then requeue the request after 10s
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// check if the server certificate for managedcluster
	if managedClusterObsCert == nil {
		var err error
		managedClusterObsCert, err = generateObservabilityServerCACerts(r.Client)
		if err != nil && k8serrors.IsNotFound(err) {
			// if the servser certificate for managedcluster is not ready, then requeue the request after 10s to avoid useless reconcile loop.
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
	}

	opts := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{ownerLabelKey: ownerLabelValue}),
	}
	if req.Namespace != config.GetDefaultNamespace() &&
		req.Namespace != "" {
		opts.Namespace = req.Namespace
	}

	obsAddonList := &mcov1beta1.ObservabilityAddonList{}
	err = r.Client.List(context.TODO(), obsAddonList, opts)
	if err != nil {
		reqLogger.Error(err, "Failed to list observabilityaddon resource")
		return ctrl.Result{}, err
	}

	if !deleteAll {
		res, err := createAllRelatedRes(r.Client, r.RESTMapper, req, mco, obsAddonList, r.CRDMap[config.IngressControllerCRD])
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
	managedclusteraddonList := &addonv1alpha1.ManagedClusterAddOnList{}
	err = r.Client.List(context.TODO(), managedclusteraddonList, opts)
	if err != nil {
		reqLogger.Error(err, "Failed to list managedclusteraddon resource")
		return ctrl.Result{}, err
	}
	latestClusters := []string{}
	staleAddons := []string{}
	for _, addon := range obsAddonList.Items {
		latestClusters = append(latestClusters, addon.Namespace)
		staleAddons = append(staleAddons, addon.Namespace)
	}
	for _, work := range workList.Items {
		if work.Name != work.Namespace+workNameSuffix {
			reqLogger.Info("To delete invalid manifestwork", "name", work.Name, "namespace", work.Namespace)
			err = deleteManifestWork(r.Client, work.Name, work.Namespace)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		if !commonutil.Contains(latestClusters, work.Namespace) {
			reqLogger.Info("To delete manifestwork", "namespace", work.Namespace)
			err = deleteManagedClusterRes(r.Client, work.Namespace)
			if err != nil {
				return ctrl.Result{}, err
			}
		} else {
			staleAddons = commonutil.Remove(staleAddons, work.Namespace)
		}
	}

	// after the managedcluster is detached, the manifestwork for observability will be delete be the cluster manager,
	// but the managedclusteraddon for observability will not deleted by the cluster manager, so check against the
	// managedclusteraddon list to remove the managedcluster resources after the managedcluster is detached.
	for _, mcaddon := range managedclusteraddonList.Items {
		if !commonutil.Contains(latestClusters, mcaddon.Namespace) {
			reqLogger.Info("To delete managedcluster resources", "namespace", mcaddon.Namespace)
			err = deleteManagedClusterRes(r.Client, mcaddon.Namespace)
			if err != nil {
				return ctrl.Result{}, err
			}
		} else {
			staleAddons = commonutil.Remove(staleAddons, mcaddon.Namespace)
		}
	}

	// delete stale addons if manifestwork does not exist
	for _, addon := range staleAddons {
		err = deleteStaleObsAddon(r.Client, addon, true)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// only update managedclusteraddon status when obs addon's status updated
	if req.Name == obsAddonName {
		err = updateAddonStatus(r.Client, *obsAddonList)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if deleteAll {
		opts.Namespace = ""
		err = r.Client.List(context.TODO(), workList, opts)
		if err != nil {
			reqLogger.Error(err, "Failed to list manifestwork resource")
			return ctrl.Result{}, err
		}
		if len(workList.Items) == 0 {
			err = deleteGlobalResource(r.Client)
		}
	}

	return ctrl.Result{}, err
}

func createAllRelatedRes(
	c client.Client,
	restMapper meta.RESTMapper,
	request ctrl.Request,
	mco *mcov1beta2.MultiClusterObservability,
	obsAddonList *mcov1beta1.ObservabilityAddonList,
	ingressCtlCrdExists bool) (ctrl.Result, error) {

	// create the clusterrole if not there
	if !isCRoleCreated {
		err := createClusterRole(c)
		if err != nil {
			return ctrl.Result{}, err
		}
		err = createResourceRole(c)
		if err != nil {
			return ctrl.Result{}, err
		}
		isCRoleCreated = true
	}
	//Check if ClusterManagementAddon is created or create it
	if !isClusterManagementAddonCreated {
		err := util.CreateClusterManagementAddon(c)
		if err != nil {
			return ctrl.Result{}, err
		}
		isClusterManagementAddonCreated = true
	}

	currentClusters := []string{}
	for _, ep := range obsAddonList.Items {
		currentClusters = append(currentClusters, ep.Namespace)
	}

	// need to reload the template and update the the corresponding resources
	// the loadTemplates method is now lightweight operations as we have cache the templates in memory.
	log.Info("load and update templates for managedcluster resources")
	rawExtensionList, obsAddonCRDv1, obsAddonCRDv1beta1,
		endpointMetricsOperatorDeploy, imageListConfigMap, _ = loadTemplates(mco)

	works, crdv1Work, crdv1beta1Work, err := generateGlobalManifestResources(c, mco)
	if err != nil {
		return ctrl.Result{}, err
	}

	// regenerate the hubinfo secret if empty
	if hubInfoSecret == nil {
		var err error
		if hubInfoSecret, err = generateHubInfoSecret(c, config.GetDefaultNamespace(), spokeNameSpace, ingressCtlCrdExists); err != nil {
			return ctrl.Result{}, err
		}
	}

	failedCreateManagedClusterRes := false
	for managedCluster, openshiftVersion := range managedClusterList {
		currentClusters = commonutil.Remove(currentClusters, managedCluster)
		// enter the loop for the following reconcile requests:
		// 1. MCO CR change(request name is "mco-updated-request")
		// 2. MCH resource change(request name is "mch-updated-request"), to handle image replacement in upgrade case.
		// 3. configmap/secret... resource change from observability namespace
		// 4. managedcluster change(request namespace is emprt string and request name is managedcluster name)
		// 5. manifestwork/observabilityaddon/managedclusteraddon/rolebinding... change from managedcluster namespace
		if request.Name == config.MCOUpdatedRequestName ||
			request.Name == config.MCHUpdatedRequestName ||
			request.Namespace == config.GetDefaultNamespace() ||
			(request.Namespace == "" && request.Name == managedCluster) ||
			request.Namespace == managedCluster {
			log.Info("Monitoring operator should be installed in cluster", "cluster_name", managedCluster, "request.name", request.Name, "request.namespace", request.Namespace)
			if openshiftVersion == "3" {
				err = createManagedClusterRes(c, restMapper, mco,
					managedCluster, managedCluster,
					works, crdv1beta1Work, endpointMetricsOperatorDeploy, hubInfoSecret, false)
			} else if openshiftVersion == nonOCP {
				err = createManagedClusterRes(c, restMapper, mco,
					managedCluster, managedCluster,
					works, crdv1Work, endpointMetricsOperatorDeploy, hubInfoSecret, true)
			} else {
				err = createManagedClusterRes(c, restMapper, mco,
					managedCluster, managedCluster,
					works, crdv1Work, endpointMetricsOperatorDeploy, hubInfoSecret, false)
			}
			if err != nil {
				failedCreateManagedClusterRes = true
				log.Error(err, "Failed to create managedcluster resources", "namespace", managedCluster)
			}
			if request.Namespace == managedCluster {
				break
			}
		}
	}

	failedDeleteOba := false
	for _, cluster := range currentClusters {
		log.Info("To delete observabilityAddon", "namespace", cluster)
		err = deleteObsAddon(c, cluster)
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
	mco *mcov1beta2.MultiClusterObservability, name string, namespace string,
	works []workv1.Manifest, crdWork *workv1.Manifest, dep *appsv1.Deployment,
	hubInfo *corev1.Secret, installProm bool) error {
	err := createObsAddon(client, namespace)
	if err != nil {
		log.Error(err, "Failed to create observabilityaddon")
		return err
	}

	err = createRolebindings(client, namespace, name)
	if err != nil {
		return err
	}

	err = createManifestWorks(client, restMapper, namespace, name, mco, works, crdWork, dep, hubInfo, installProm)
	if err != nil {
		log.Error(err, "Failed to create manifestwork")
		return err
	}

	err = util.CreateManagedClusterAddonCR(client, namespace, ownerLabelKey, ownerLabelValue)
	if err != nil {
		log.Error(err, "Failed to create ManagedClusterAddon")
		return err
	}

	return nil
}

func deleteManagedClusterRes(c client.Client, namespace string) error {

	managedclusteraddon := &addonv1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.ManagedClusterAddonName,
			Namespace: namespace,
		},
	}
	err := c.Delete(context.TODO(), managedclusteraddon)
	if err != nil && !k8serrors.IsNotFound(err) {
		log.Error(err, "Failed to delete managedclusteraddon")
		return err
	}

	err = deleteRolebindings(c, namespace)
	if err != nil {
		return err
	}

	err = deleteManifestWorks(c, namespace)
	if err != nil {
		log.Error(err, "Failed to delete manifestwork")
		return err
	}
	return nil
}

func updateManagedClusterList(obj client.Object) {
	if version, ok := obj.GetLabels()["openshiftVersion"]; ok {
		managedClusterList[obj.GetName()] = version
	} else {
		managedClusterList[obj.GetName()] = nonOCP
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *PlacementRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	c := mgr.GetClient()
	ingressCtlCrdExists, _ := r.CRDMap[config.IngressControllerCRD]
	clusterPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			log.Info("CreateFunc", "managedCluster", e.Object.GetName())
			updateManagedClusterList(e.Object)
			updateManagedClusterImageRegistry(e.Object)
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			log.Info("UpdateFunc", "managedCluster", e.ObjectNew.GetName())
			if e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				if e.ObjectNew.GetDeletionTimestamp() != nil {
					log.Info("managedcluster is in terminating state", "managedCluster", e.ObjectNew.GetName())
					delete(managedClusterList, e.ObjectNew.GetName())
					delete(managedClusterImageRegistry, e.ObjectNew.GetName())
				} else {
					updateManagedClusterList(e.ObjectNew)
					updateManagedClusterImageRegistry(e.ObjectNew)
				}
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			log.Info("DeleteFunc", "managedCluster", e.Object.GetName())
			delete(managedClusterList, e.Object.GetName())
			delete(managedClusterImageRegistry, e.Object.GetName())
			return true
		},
	}

	obsAddonPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetName() == obsAddonName &&
				e.ObjectNew.GetLabels()[ownerLabelKey] == ownerLabelValue &&
				!reflect.DeepEqual(e.ObjectNew.(*mcov1beta1.ObservabilityAddon).Status.Conditions,
					e.ObjectOld.(*mcov1beta1.ObservabilityAddon).Status.Conditions) {
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

	mcoPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// generate the image pull secret
			pullSecret, _ = generatePullSecret(c, config.GetImagePullSecret(e.Object.(*mcov1beta2.MultiClusterObservability).Spec))
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			// only reconcile when ObservabilityAddonSpec updated
			if e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() &&
				!reflect.DeepEqual(e.ObjectNew.(*mcov1beta2.MultiClusterObservability).Spec.ObservabilityAddonSpec,
					e.ObjectOld.(*mcov1beta2.MultiClusterObservability).Spec.ObservabilityAddonSpec) {
				if e.ObjectNew.(*mcov1beta2.MultiClusterObservability).Spec.ImagePullSecret != e.ObjectOld.(*mcov1beta2.MultiClusterObservability).Spec.ImagePullSecret {
					// regenerate the image pull secret
					pullSecret, _ = generatePullSecret(c, config.GetImagePullSecret(e.ObjectNew.(*mcov1beta2.MultiClusterObservability).Spec))
				}
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return true
		},
	}

	customAllowlistPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetName() == config.AllowlistCustomConfigMapName &&
				e.Object.GetNamespace() == config.GetDefaultNamespace() {
				// generate the metrics allowlist configmap
				log.Info("generate metric allow list configmap for custom configmap CREATE")
				metricsAllowlistConfigMap, _ = generateMetricsListCM(c)
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetName() == config.AllowlistCustomConfigMapName &&
				e.ObjectNew.GetNamespace() == config.GetDefaultNamespace() &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				// regenerate the metrics allowlist configmap
				log.Info("generate metric allow list configmap for custom configmap UPDATE")
				metricsAllowlistConfigMap, _ = generateMetricsListCM(c)
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetName() == config.AllowlistCustomConfigMapName &&
				e.Object.GetNamespace() == config.GetDefaultNamespace() {
				// regenerate the metrics allowlist configmap
				log.Info("generate metric allow list configmap for custom configmap UPDATE")
				metricsAllowlistConfigMap, _ = generateMetricsListCM(c)
				return true
			}
			return false
		},
	}

	certSecretPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetName() == config.ServerCACerts &&
				e.Object.GetNamespace() == config.GetDefaultNamespace() {
				// generate the certificate for managed cluster
				log.Info("generate managedcluster observability certificate for server certificate CREATE")
				managedClusterObsCert, _ = generateObservabilityServerCACerts(c)
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if (e.ObjectNew.GetName() == config.ServerCACerts &&
				e.ObjectNew.GetNamespace() == config.GetDefaultNamespace()) &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				// regenerate the certificate for managed cluster
				log.Info("generate managedcluster observability certificate for server certificate UPDATE")
				managedClusterObsCert, _ = generateObservabilityServerCACerts(c)
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}

	ingressControllerPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetName() == config.OpenshiftIngressOperatorCRName &&
				e.Object.GetNamespace() == config.OpenshiftIngressOperatorNamespace {
				// generate the hubInfo secret
				hubInfoSecret, _ = generateHubInfoSecret(c, config.GetDefaultNamespace(), spokeNameSpace, ingressCtlCrdExists)
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetName() == config.OpenshiftIngressOperatorCRName &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() &&
				e.ObjectNew.GetNamespace() == config.OpenshiftIngressOperatorNamespace {
				// regenerate the hubInfo secret
				hubInfoSecret, _ = generateHubInfoSecret(c, config.GetDefaultNamespace(), spokeNameSpace, ingressCtlCrdExists)
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetName() == config.OpenshiftIngressOperatorCRName &&
				e.Object.GetNamespace() == config.OpenshiftIngressOperatorNamespace {
				// regenerate the hubInfo secret
				hubInfoSecret, _ = generateHubInfoSecret(c, config.GetDefaultNamespace(), spokeNameSpace, ingressCtlCrdExists)
				return true
			}
			return false
		},
	}

	amRouterCertSecretPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetNamespace() == config.GetDefaultNamespace() &&
				(e.Object.GetName() == config.AlertmanagerRouteBYOCAName ||
					e.Object.GetName() == config.AlertmanagerRouteBYOCERTName) {
				// generate the hubInfo secret
				hubInfoSecret, _ = generateHubInfoSecret(c, config.GetDefaultNamespace(), spokeNameSpace, ingressCtlCrdExists)
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetNamespace() == config.GetDefaultNamespace() &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() &&
				(e.ObjectNew.GetName() == config.AlertmanagerRouteBYOCAName ||
					e.ObjectNew.GetName() == config.AlertmanagerRouteBYOCERTName) {
				// regenerate the hubInfo secret
				hubInfoSecret, _ = generateHubInfoSecret(c, config.GetDefaultNamespace(), spokeNameSpace, ingressCtlCrdExists)
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetNamespace() == config.GetDefaultNamespace() &&
				(e.Object.GetName() == config.AlertmanagerRouteBYOCAName ||
					e.Object.GetName() == config.AlertmanagerRouteBYOCERTName) {
				// regenerate the hubInfo secret
				hubInfoSecret, _ = generateHubInfoSecret(c, config.GetDefaultNamespace(), spokeNameSpace, ingressCtlCrdExists)
				return true
			}
			return false
		},
	}

	routeCASecretPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if (e.Object.GetNamespace() == config.OpenshiftIngressOperatorNamespace &&
				e.Object.GetName() == config.OpenshiftIngressRouteCAName) ||
				(e.Object.GetNamespace() == config.OpenshiftIngressNamespace &&
					e.Object.GetName() == config.OpenshiftIngressDefaultCertName) {
				// generate the hubInfo secret
				hubInfoSecret, _ = generateHubInfoSecret(c, config.GetDefaultNamespace(), spokeNameSpace, ingressCtlCrdExists)
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if ((e.ObjectNew.GetNamespace() == config.OpenshiftIngressOperatorNamespace &&
				e.ObjectNew.GetName() == config.OpenshiftIngressRouteCAName) ||
				(e.ObjectNew.GetNamespace() == config.OpenshiftIngressNamespace &&
					e.ObjectNew.GetName() == config.OpenshiftIngressDefaultCertName)) &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				// regenerate the hubInfo secret
				hubInfoSecret, _ = generateHubInfoSecret(c, config.GetDefaultNamespace(), spokeNameSpace, ingressCtlCrdExists)
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}

	amAccessorSAPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetName() == config.AlertmanagerAccessorSAName &&
				e.Object.GetNamespace() == config.GetDefaultNamespace() {
				// wait 10s for access_token of alertmanager and generate the secret that contains the access_token
				/* #nosec */
				wait.Poll(2*time.Second, 10*time.Second, func() (bool, error) {
					var err error
					log.Info("generate amAccessorTokenSecret for alertmanager access serviceaccount CREATE")
					if amAccessorTokenSecret, err = generateAmAccessorTokenSecret(c); err == nil {
						return true, nil
					}
					return false, err
				})
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if (e.ObjectNew.GetName() == config.AlertmanagerAccessorSAName &&
				e.ObjectNew.GetNamespace() == config.GetDefaultNamespace()) &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				// regenerate the secret that contains the access_token for the Alertmanager in the Hub cluster
				amAccessorTokenSecret, _ = generateAmAccessorTokenSecret(c)
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}

	ctrBuilder := ctrl.NewControllerManagedBy(mgr).
		// Watch for changes to primary resource ManagedCluster with predicate
		For(&clusterv1.ManagedCluster{}, builder.WithPredicates(clusterPred)).
		// secondary watch for observabilityaddon
		Watches(&source.Kind{Type: &mcov1beta1.ObservabilityAddon{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(obsAddonPred)).
		// secondary watch for MCO
		Watches(&source.Kind{Type: &mcov1beta2.MultiClusterObservability{}}, handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
			return []reconcile.Request{
				{NamespacedName: types.NamespacedName{
					Name: config.MCOUpdatedRequestName,
				}},
			}
		}), builder.WithPredicates(mcoPred)).
		// secondary watch for custom allowlist configmap
		Watches(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(customAllowlistPred)).
		// secondary watch for certificate secrets
		Watches(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(certSecretPred)).
		// secondary watch for alertmanager accessor serviceaccount
		Watches(&source.Kind{Type: &corev1.ServiceAccount{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(amAccessorSAPred))

	manifestWorkGroupKind := schema.GroupKind{Group: workv1.GroupVersion.Group, Kind: "ManifestWork"}
	if _, err := r.RESTMapper.RESTMapping(manifestWorkGroupKind, workv1.GroupVersion.Version); err == nil {
		workPred := predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return false
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				if e.ObjectNew.GetLabels()[ownerLabelKey] == ownerLabelValue &&
					e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() &&
					!reflect.DeepEqual(e.ObjectNew.(*workv1.ManifestWork).Spec.Workload.Manifests,
						e.ObjectOld.(*workv1.ManifestWork).Spec.Workload.Manifests) {
					return true
				}
				return false
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return e.Object.GetLabels()[ownerLabelKey] == ownerLabelValue
			},
		}

		// secondary watch for manifestwork
		ctrBuilder = ctrBuilder.Watches(&source.Kind{Type: &workv1.ManifestWork{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(workPred))
	}

	mchGroupKind := schema.GroupKind{Group: mchv1.SchemeGroupVersion.Group, Kind: "MultiClusterHub"}
	if _, err := r.RESTMapper.RESTMapping(mchGroupKind, mchv1.SchemeGroupVersion.Version); err == nil {
		mchPred := predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				// this is for operator restart, the mch CREATE event will be caught and the mch should be ready
				if e.Object.GetNamespace() == config.GetMCONamespace() &&
					e.Object.(*mchv1.MultiClusterHub).Status.CurrentVersion != "" &&
					e.Object.(*mchv1.MultiClusterHub).Status.DesiredVersion == e.Object.(*mchv1.MultiClusterHub).Status.CurrentVersion {
					// only read the image manifests configmap and enqueue the request when the MCH is installed/upgraded successfully
					ok, err := config.ReadImageManifestConfigMap(c, e.Object.(*mchv1.MultiClusterHub).Status.CurrentVersion)
					if err != nil {
						return false
					}
					return ok
				}
				return false
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				if e.ObjectNew.GetNamespace() == config.GetMCONamespace() &&
					e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() &&
					e.ObjectNew.(*mchv1.MultiClusterHub).Status.CurrentVersion != "" &&
					e.ObjectNew.(*mchv1.MultiClusterHub).Status.DesiredVersion == e.ObjectNew.(*mchv1.MultiClusterHub).Status.CurrentVersion {
					/// only read the image manifests configmap and enqueue the request when the MCH is installed/upgraded successfully
					ok, err := config.ReadImageManifestConfigMap(c, e.ObjectNew.(*mchv1.MultiClusterHub).Status.CurrentVersion)
					if err != nil {
						return false
					}
					return ok
				}
				return false
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return false
			},
		}

		if ingressCtlCrdExists {
			// secondary watch for default ingresscontroller
			ctrBuilder = ctrBuilder.Watches(&source.Kind{Type: &operatorv1.IngressController{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(ingressControllerPred)).
				// secondary watch for alertmanager route byo cert secrets
				Watches(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(amRouterCertSecretPred)).
				// secondary watch for openshift route ca secret
				Watches(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(routeCASecretPred))
		}

		mchCrdExists, _ := r.CRDMap[config.MCHCrdName]
		if mchCrdExists {
			// secondary watch for MCH
			ctrBuilder = ctrBuilder.Watches(&source.Kind{Type: &mchv1.MultiClusterHub{}}, handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				return []reconcile.Request{
					{NamespacedName: types.NamespacedName{
						Name:      config.MCHUpdatedRequestName,
						Namespace: obj.GetNamespace(),
					}},
				}
			}), builder.WithPredicates(mchPred))
		}
	}

	// create and return a new controller
	return ctrBuilder.Complete(r)
}

func StartPlacementController(mgr manager.Manager, crdMap map[string]bool) error {
	if isplacementControllerRunnning {
		return nil
	}
	isplacementControllerRunnning = true

	if err := (&PlacementRuleReconciler{
		Client:     mgr.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName("PlacementRule"),
		Scheme:     mgr.GetScheme(),
		CRDMap:     crdMap,
		RESTMapper: mgr.GetRESTMapper(),
	}).SetupWithManager(mgr); err != nil {
		log.Error(err, "unable to create controller", "controller", "PlacementRule")
		return err
	}

	return nil
}
