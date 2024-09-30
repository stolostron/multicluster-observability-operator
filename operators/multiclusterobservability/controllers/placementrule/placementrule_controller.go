// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	operatorv1 "github.com/openshift/api/operator/v1"
	"golang.org/x/exp/slices"
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

	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	mcov1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	commonutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
)

const (
	ownerLabelKey                                  = "owner"
	ownerLabelValue                                = "multicluster-observability-operator"
	managedClusterObsCertName                      = "observability-managed-cluster-certs"
	nonOCP                                         = "N/A"
	disableAddonAutomaticInstallationAnnotationKey = "addon.open-cluster-management.io/disable-automatic-installation"
)

var (
	log                           = logf.Log.WithName("controller_placementrule")
	isCRoleCreated                = false
	clusterAddon                  = &addonv1alpha1.ClusterManagementAddOn{}
	defaultAddonDeploymentConfig  = &addonv1alpha1.AddOnDeploymentConfig{}
	isplacementControllerRunnning = false
	managedClusterList            = sync.Map{}
	managedClusterListMutex       = &sync.RWMutex{}
	installMetricsWithoutAddon    = false
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
			managedClusterList.Delete("local-cluster")
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

	// ACM 8509: Special case for hub/local cluster metrics collection
	// We want to ensure that the local-cluster is always in the managedClusterList
	// In the case when hubSelfManagement is enabled, we will delete it from the list and modify the object
	// to cater to the use case of deploying in open-cluster-management-observability namespace
	managedClusterList.Delete("local-cluster")
	if _, ok := managedClusterList.Load("local-cluster"); !ok {
		obj := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "local-cluster",
				Namespace: config.GetDefaultNamespace(),
				Labels: map[string]string{
					"openshiftVersion": "mimical",
				},
			},
		}
		installMetricsWithoutAddon = true
		updateManagedClusterList(obj)
	}

	if !deleteAll && !mco.Spec.ObservabilityAddonSpec.EnableMetrics {
		reqLogger.Info("EnableMetrics is set to false. Delete Observability addons")
		deleteAll = true
	}

	// check if the MCH CRD exists
	mchCrdExists := r.CRDMap[config.MCHCrdName]
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
			// if the servser certificate for managedcluster is not ready, then
			// requeue the request after 10s to avoid useless reconcile loop.
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

	if !deleteAll && installMetricsWithoutAddon {
		obsAddonList.Items = append(obsAddonList.Items, mcov1beta1.ObservabilityAddon{
			ObjectMeta: metav1.ObjectMeta{
				Name:      obsAddonName,
				Namespace: config.GetDefaultNamespace(),
				Labels: map[string]string{
					ownerLabelKey: ownerLabelValue,
				},
			},
		})
		err = deleteObsAddon(r.Client, localClusterName)
		if err != nil {
			log.Error(err, "Failed to delete observabilityaddon")
			return ctrl.Result{}, err
		}
	}
	if operatorconfig.IsMCOTerminating {
		delete(managedClusterList, "local-cluster")
	}

	if !deleteAll {
		if err := createAllRelatedRes(
			r.Client,
			req,
			mco,
			obsAddonList,
			r.CRDMap,
		); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		if err := deleteAllObsAddons(r.Client, obsAddonList); err != nil {
			return ctrl.Result{}, err
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
			// ACM 8509: Special case for hub metrics collector
			// In the upgrade case we want to clean up the obs add on and manifest work that was created
			// for local-cluster before the upgrade that is why we check for the local-cluster namespace
			reqLogger.Info("To delete invalid manifestwork", "name", work.Name, "namespace", work.Namespace)
			err = deleteManifestWork(r.Client, work.Name, work.Namespace)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		if !slices.Contains(latestClusters, work.Namespace) {
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
		if !slices.Contains(latestClusters, mcaddon.Namespace) && mcaddon.Namespace != config.GetDefaultNamespace() {
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
		// delete managedclusteraddon for local-cluster
		err = deleteManagedClusterRes(r.Client, localClusterName)
		if err != nil {
			return ctrl.Result{}, err
		}

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
	request ctrl.Request,
	mco *mcov1beta2.MultiClusterObservability,
	obsAddonList *mcov1beta1.ObservabilityAddonList,
	CRDMap map[string]bool,
) error {
	var err error
	// create the clusterrole if not there
	if !isCRoleCreated {
		err = createClusterRole(c)
		if err != nil {
			return err
		}
		err = createResourceRole(c)
		if err != nil {
			return err
		}
		isCRoleCreated = true
	}

	// Get or create ClusterManagementAddon
	clusterAddon, err = util.CreateClusterManagementAddon(c)
	if err != nil {
		return err
	}

	// Always start this loop with an empty addon deployment config.
	// This simplifies the logic for the cases where:
	// - There is nothing in `Spec.SupportedConfigs`.
	// - There's something in `Spec.SupportedConfigs`, but none of them are for
	//   the group and resource that we care about.
	// - There is something in `Spec.SupportedConfigs`, the group and resource are correct,
	//   but the default config is not present in the manifest or it is not found
	//   (i.e. was deleted or there's a typo).
	defaultAddonDeploymentConfig = &addonv1alpha1.AddOnDeploymentConfig{}
	for _, config := range clusterAddon.Spec.SupportedConfigs {
		if config.ConfigGroupResource.Group == util.AddonGroup &&
			config.ConfigGroupResource.Resource == util.AddonDeploymentConfigResource {
			if config.DefaultConfig != nil {
				addonConfig := &addonv1alpha1.AddOnDeploymentConfig{}
				err = c.Get(context.TODO(),
					types.NamespacedName{
						Name:      config.DefaultConfig.Name,
						Namespace: config.DefaultConfig.Namespace,
					},
					addonConfig,
				)
				if err != nil {
					return err
				}
				log.Info("There is default AddonDeploymentConfig for current addon")
				defaultAddonDeploymentConfig = addonConfig
				break
			}
		}
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
		return err
	}

	// regenerate the hubinfo secret if empty
	if hubInfoSecret == nil {
		var err error
		if hubInfoSecret, err = generateHubInfoSecret(c, config.GetDefaultNamespace(), spokeNameSpace, CRDMap[config.IngressControllerCRD]); err != nil {
			return err
		}
	}

	failedCreateManagedClusterRes := false
	managedClusterListMutex.RLock()
	managedClusterList.Range(func(key, value interface{}) bool {
		managedCluster := key.(string)
		openshiftVersion := value.(string)
		currentClusters = commonutil.Remove(currentClusters, managedCluster)
		if isReconcileRequired(request, managedCluster) {
			log.Info(
				"Monitoring operator should be installed in cluster",
				"cluster_name",
				managedCluster,
				"request.name",
				request.Name,
				"request.namespace",
				request.Namespace,
				"openshiftVersion",
				openshiftVersion,
			)
			if openshiftVersion == "3" {
				err = createManagedClusterRes(c, mco,
					managedCluster, managedCluster,
					works, ocp311metricsAllowlistConfigMap, crdv1beta1Work, endpointMetricsOperatorDeploy, hubInfoSecret, false)
			} else if openshiftVersion == nonOCP {
				err = createManagedClusterRes(c, mco,
					managedCluster, managedCluster,
					works, metricsAllowlistConfigMap, crdv1Work, endpointMetricsOperatorDeploy, hubInfoSecret, true)
			} else if openshiftVersion == "mimical" {
				installProm := false
				if mco.Annotations["test-env"] == "kind-test" {
					installProm = true
				}
				// Create copy of hub-info-secret for local-cluster since hubInfo is global variable
				hubInfoSecretCopy := hubInfoSecret.DeepCopy()
				err = createManagedClusterRes(c, mco,
					managedCluster, config.GetDefaultNamespace(),
					works, metricsAllowlistConfigMap, crdv1Work, endpointMetricsOperatorDeploy, hubInfoSecretCopy, installProm)
			} else {
				err = createManagedClusterRes(c, mco,
					managedCluster, managedCluster,
					works, metricsAllowlistConfigMap, crdv1Work, endpointMetricsOperatorDeploy, hubInfoSecret, false)
			}
			if err != nil {
				failedCreateManagedClusterRes = true
				log.Error(err, "Failed to create managedcluster resources", "namespace", managedCluster)
			}
			if request.Namespace == managedCluster {
				return false
			}
		}
		return true
	})

	// Look through the obsAddonList items and find clusters
	// which are no longer to be managed and therefore needs deletion
	clustersToCleanup := []string{}
	for _, ep := range obsAddonList.Items {
		if _, ok := managedClusterList.Load(ep.Namespace); !ok {
			clustersToCleanup = append(clustersToCleanup, ep.Namespace)
		}
	}
	managedClusterListMutex.RUnlock()

	failedDeleteOba := false
	for _, cluster := range currentClusters {
		if cluster != config.GetDefaultNamespace() {
			err = deleteObsAddon(c, cluster)
			if err != nil {
				failedDeleteOba = true
				log.Error(err, "Failed to delete observabilityaddon", "namespace", cluster)
			}
		}
	}

	if failedCreateManagedClusterRes || failedDeleteOba {
		return errors.New("failed to create managedcluster resources or failed to delete observabilityaddon, skip and reconcile later")
	}

	return nil
}

func deleteAllObsAddons(
	client client.Client,
	obsAddonList *mcov1beta1.ObservabilityAddonList,
) error {
	for _, ep := range obsAddonList.Items {
		err := deleteObsAddon(client, ep.Namespace)
		if err != nil {
			log.Error(err, "Failed to delete observabilityaddon", "namespace", ep.Namespace)
			return err
		}
	}
	return nil
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
	// delete ClusterManagementAddon
	err = util.DeleteClusterManagementAddon(c)
	if err != nil {
		return err
	}
	return nil
}

func createManagedClusterRes(
	c client.Client,
	mco *mcov1beta2.MultiClusterObservability,
	name string,
	namespace string,
	works []workv1.Manifest,
	allowlist *corev1.ConfigMap,
	crdWork *workv1.Manifest,
	dep *appsv1.Deployment,
	hubInfo *corev1.Secret,
	installProm bool,
) error {
	err := createObsAddon(c, namespace)
	if err != nil {
		log.Error(err, "Failed to create observabilityaddon")
		return err
	}

	err = createRolebindings(c, namespace, name)
	if err != nil {
		return err
	}

	addon, err := util.CreateManagedClusterAddonCR(c, namespace, ownerLabelKey, ownerLabelValue)
	if err != nil {
		log.Error(err, "Failed to create ManagedClusterAddon")
		return err
	}
	addonConfig := &addonv1alpha1.AddOnDeploymentConfig{}
	isCustomConfig := false
	for _, config := range addon.Spec.Configs {
		if config.ConfigGroupResource.Group == util.AddonGroup &&
			config.ConfigGroupResource.Resource == util.AddonDeploymentConfigResource {
			err = c.Get(context.TODO(),
				types.NamespacedName{
					Name:      config.ConfigReferent.Name,
					Namespace: config.ConfigReferent.Namespace,
				},
				addonConfig,
			)
			if err != nil {
				return err
			}
			isCustomConfig = true
			log.Info("There is AddonDeploymentConfig for current addon", "namespace", namespace)
			break
		}
	}
	if !isCustomConfig {
		addonConfig = defaultAddonDeploymentConfig
	}

	if err = createManifestWorks(c, namespace, name, mco, works, allowlist, crdWork, dep, hubInfo, addonConfig, installProm); err != nil {
		log.Error(err, "Failed to create manifestwork")
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

// areManagedClusterLabelsReady check if labels automatically set in the managed cluster
// are ready to be accessed. These labels are: "vendor" and "openshiftVersion".
// Labels are considered not ready when:
//
// - The "vendor" label isn't found.
// - The "vendor" label has value "auto-detect".
// - The "vendor" has value "openshift" but there is no "openshiftVersion".
func areManagedClusterLabelsReady(obj client.Object) bool {
	labels := obj.GetLabels()
	vendor, foundVendor := labels["vendor"]

	if !foundVendor || vendor == "" || vendor == "auto-detect" {
		log.Info("ManagedCluster labels are not ready", "cluster", obj.GetName())
		return false
	}

	if vendor == "OpenShift" {
		_, foundOpenshiftVersion := labels["openshiftVersion"]
		if !foundOpenshiftVersion {
			log.Info("ManagedCluster labels are not ready", "cluster", obj.GetName())
			return false
		}
	}

	return true
}

func updateManagedClusterList(obj client.Object) {
	//ACM 8509: Special case for local-cluster, we deploy endpoint and metrics collector in the hub
	//whether hubSelfManagement is enabled or not
	managedClusterListMutex.Lock()
	defer managedClusterListMutex.Unlock()
	if version, ok := obj.GetLabels()["openshiftVersion"]; ok {
		managedClusterList.Store(obj.GetName(), version)
	} else {
		managedClusterList.Store(obj.GetName(), nonOCP)
	}
}

// Do not reconcile objects if this instance of mch has the
// `disableAddonAutomaticInstallationAnnotationKey` annotation
func isAutomaticAddonInstallationDisabled(obj client.Object) bool {
	annotations := obj.GetAnnotations()
	if val, ok := annotations[disableAddonAutomaticInstallationAnnotationKey]; ok && strings.EqualFold(val, "true") {
		log.Info("Cluster has disable addon automatic installation annotation. Skip addon deploy")
		return true
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
// TODO refactor (if possible) to match format of observabilityaddon_controller.go
func (r *PlacementRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	c := mgr.GetClient()
	ingressCtlCrdExists := r.CRDMap[config.IngressControllerCRD]

	clusterPred := getClusterPreds()

	// Watch changes for AddonDeploymentConfig
	addOnDeploymentConfigPred := GetAddOnDeploymentConfigPredicates()

	// Watch changes to endpoint-operator deployment
	hubEndpointOperatorPred := getHubEndpointOperatorPredicates()

	obsAddonPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetName() == obsAddonName &&
				e.ObjectNew.GetLabels()[ownerLabelKey] == ownerLabelValue &&
				e.ObjectNew.GetNamespace() != localClusterName &&
				!reflect.DeepEqual(e.ObjectNew.(*mcov1beta1.ObservabilityAddon).Status.Conditions,
					e.ObjectOld.(*mcov1beta1.ObservabilityAddon).Status.Conditions) {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetName() == obsAddonName &&
				e.Object.GetLabels()[ownerLabelKey] == ownerLabelValue &&
				e.Object.GetNamespace() != localClusterName {
				log.Info(
					"DeleteFunc",
					"obsAddonNamespace",
					e.Object.GetNamespace(),
					"obsAddonName",
					e.Object.GetName(),
				)
				if err := removePostponeDeleteAnnotationForManifestwork(c, e.Object.GetNamespace()); err != nil {
					log.Error(err, "postpone delete annotation for manifestwork could not be removed")
					return false
				}
				return true
			}
			return false
		},
	}

	allowlistPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if (e.Object.GetName() == config.AllowlistCustomConfigMapName ||
				e.Object.GetName() == operatorconfig.AllowlistConfigMapName) &&
				e.Object.GetNamespace() == config.GetDefaultNamespace() {
				// generate the metrics allowlist configmap
				log.Info("generate metric allow list configmap for allowlist configmap CREATE")
				metricsAllowlistConfigMap, ocp311metricsAllowlistConfigMap, _ = generateMetricsListCM(c)
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if (e.ObjectNew.GetName() == config.AllowlistCustomConfigMapName ||
				e.ObjectNew.GetName() == operatorconfig.AllowlistConfigMapName) &&
				e.ObjectNew.GetNamespace() == config.GetDefaultNamespace() &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				// regenerate the metrics allowlist configmap
				log.Info("generate metric allow list configmap for allowlist configmap UPDATE")
				metricsAllowlistConfigMap, ocp311metricsAllowlistConfigMap, _ = generateMetricsListCM(c)
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if (e.Object.GetName() == config.AllowlistCustomConfigMapName ||
				e.Object.GetName() == operatorconfig.AllowlistConfigMapName) &&
				e.Object.GetNamespace() == config.GetDefaultNamespace() {
				// regenerate the metrics allowlist configmap
				log.Info("generate metric allow list configmap for allowlist configmap UPDATE")
				metricsAllowlistConfigMap, ocp311metricsAllowlistConfigMap, _ = generateMetricsListCM(c)
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
			if e.Object.GetName() == config.ServerCACerts &&
				e.Object.GetNamespace() == config.GetDefaultNamespace() {
				return true
			}
			return false
		},
	}

	ingressControllerPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetName() == config.OpenshiftIngressOperatorCRName &&
				e.Object.GetNamespace() == config.OpenshiftIngressOperatorNamespace {
				// generate the hubInfo secret
				hubInfoSecret, _ = generateHubInfoSecret(
					c,
					config.GetDefaultNamespace(),
					spokeNameSpace,
					ingressCtlCrdExists,
				)
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetName() == config.OpenshiftIngressOperatorCRName &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() &&
				e.ObjectNew.GetNamespace() == config.OpenshiftIngressOperatorNamespace {
				// regenerate the hubInfo secret
				hubInfoSecret, _ = generateHubInfoSecret(
					c,
					config.GetDefaultNamespace(),
					spokeNameSpace,
					ingressCtlCrdExists,
				)
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetName() == config.OpenshiftIngressOperatorCRName &&
				e.Object.GetNamespace() == config.OpenshiftIngressOperatorNamespace {
				// regenerate the hubInfo secret
				hubInfoSecret, _ = generateHubInfoSecret(
					c,
					config.GetDefaultNamespace(),
					spokeNameSpace,
					ingressCtlCrdExists,
				)
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
				hubInfoSecret, _ = generateHubInfoSecret(
					c,
					config.GetDefaultNamespace(),
					spokeNameSpace,
					ingressCtlCrdExists,
				)
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
				hubInfoSecret, _ = generateHubInfoSecret(
					c,
					config.GetDefaultNamespace(),
					spokeNameSpace,
					ingressCtlCrdExists,
				)
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetNamespace() == config.GetDefaultNamespace() &&
				(e.Object.GetName() == config.AlertmanagerRouteBYOCAName ||
					e.Object.GetName() == config.AlertmanagerRouteBYOCERTName) {
				// regenerate the hubInfo secret
				hubInfoSecret, _ = generateHubInfoSecret(
					c,
					config.GetDefaultNamespace(),
					spokeNameSpace,
					ingressCtlCrdExists,
				)
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
				hubInfoSecret, _ = generateHubInfoSecret(
					c,
					config.GetDefaultNamespace(),
					spokeNameSpace,
					ingressCtlCrdExists,
				)
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
				hubInfoSecret, _ = generateHubInfoSecret(
					c,
					config.GetDefaultNamespace(),
					spokeNameSpace,
					ingressCtlCrdExists,
				)
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
				if err := wait.Poll(2*time.Second, 10*time.Second, func() (bool, error) {
					var err error
					log.Info("generate amAccessorTokenSecret for alertmanager access serviceaccount CREATE")
					if amAccessorTokenSecret, err = generateAmAccessorTokenSecret(c); err == nil {
						return true, nil
					}
					return false, err
				}); err != nil {
					log.Error(err, "error polling in createfunc")
					return false
				}
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
		}), builder.WithPredicates(getMCOPred(c, ingressCtlCrdExists))).

		// secondary watch for custom allowlist configmap
		Watches(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(allowlistPred)).

		// secondary watch for certificate secrets
		Watches(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(certSecretPred)).

		// secondary watch for alertmanager accessor serviceaccount
		Watches(&source.Kind{Type: &corev1.ServiceAccount{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(amAccessorSAPred))

	// watch for AddOnDeploymentConfig
	addOnDeploymentConfigGroupKind := schema.GroupKind{Group: addonv1alpha1.GroupVersion.Group, Kind: "AddOnDeploymentConfig"}
	if _, err := r.RESTMapper.RESTMapping(addOnDeploymentConfigGroupKind, addonv1alpha1.GroupVersion.Version); err == nil {
		ctrBuilder = ctrBuilder.Watches(
			&source.Kind{Type: &addonv1alpha1.AddOnDeploymentConfig{}},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				return []reconcile.Request{
					{NamespacedName: types.NamespacedName{
						Name: config.AddonDeploymentConfigUpdateName,
					}},
				}
			}),
			builder.WithPredicates(addOnDeploymentConfigPred),
		)
	}
	manifestWorkGroupKind := schema.GroupKind{Group: workv1.GroupVersion.Group, Kind: "ManifestWork"}
	if _, err := r.RESTMapper.RESTMapping(manifestWorkGroupKind, workv1.GroupVersion.Version); err == nil {
		workPred := getManifestworkPred()
		// secondary watch for manifestwork
		ctrBuilder = ctrBuilder.Watches(
			&source.Kind{Type: &workv1.ManifestWork{}},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(workPred),
		)
	}

	clusterMgmtGroupKind := schema.GroupKind{Group: addonv1alpha1.GroupVersion.Group, Kind: "ClusterManagementAddOn"}
	if _, err := r.RESTMapper.RESTMapping(clusterMgmtGroupKind, addonv1alpha1.GroupVersion.Version); err == nil {
		clusterMgmtPred := getClusterMgmtAddonPredFunc()

		// secondary watch for clustermanagementaddon
		ctrBuilder = ctrBuilder.Watches(
			&source.Kind{Type: &addonv1alpha1.ClusterManagementAddOn{}},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				return []reconcile.Request{
					{NamespacedName: types.NamespacedName{
						Name: config.ClusterManagementAddOnUpdateName,
					}},
				}
			}),
			builder.WithPredicates(clusterMgmtPred),
		)
	}

	mgClusterGroupKind := schema.GroupKind{Group: addonv1alpha1.GroupVersion.Group, Kind: "ManagedClusterAddOn"}
	if _, err := r.RESTMapper.RESTMapping(mgClusterGroupKind, addonv1alpha1.GroupVersion.Version); err == nil {
		mgClusterGroupKindPred := getMgClusterAddonPredFunc()

		// secondary watch for managedclusteraddon
		ctrBuilder = ctrBuilder.Watches(
			&source.Kind{Type: &addonv1alpha1.ManagedClusterAddOn{}},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(mgClusterGroupKindPred),
		)
	}

	mchGroupKind := schema.GroupKind{Group: mchv1.GroupVersion.Group, Kind: "MultiClusterHub"}
	if _, err := r.RESTMapper.RESTMapping(mchGroupKind, mchv1.GroupVersion.Version); err == nil {
		mchPred := getMchPred(c)

		if ingressCtlCrdExists {
			// secondary watch for default ingresscontroller
			ctrBuilder = ctrBuilder.Watches(&source.Kind{Type: &operatorv1.IngressController{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(ingressControllerPred)).

				// secondary watch for alertmanager route byo cert secrets
				Watches(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(amRouterCertSecretPred)).

				// secondary watch for openshift route ca secret
				Watches(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(routeCASecretPred))
		}

		mchCrdExists := r.CRDMap[config.MCHCrdName]
		if mchCrdExists {
			// secondary watch for MCH
			ctrBuilder = ctrBuilder.Watches(
				&source.Kind{Type: &mchv1.MultiClusterHub{}},
				handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
					return []reconcile.Request{
						{NamespacedName: types.NamespacedName{
							Name:      config.MCHUpdatedRequestName,
							Namespace: obj.GetNamespace(),
						}},
					}
				}),
				builder.WithPredicates(mchPred),
			)
		}
	}

	// ACM 8509: Special case for hub/local cluster metrics collection
	// secondary watch for hub endpoint operator deployment

	ctrBuilder = ctrBuilder.Watches(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(hubEndpointOperatorPred)).
		Watches(
			&source.Kind{Type: &corev1.Secret{}},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(operatorconfig.HubInfoSecretName, config.GetDefaultNamespace(), false, false, true)),
		).
		Watches(
			&source.Kind{Type: &corev1.Secret{}},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(operatorconfig.HubMetricsCollectorMtlsCert, config.GetDefaultNamespace(), false, false, true)),
		).
		Watches(
			&source.Kind{Type: &corev1.Secret{}},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(managedClusterObsCertName, config.GetDefaultNamespace(), false, false, true)),
		).
		Watches(
			&source.Kind{Type: &corev1.ConfigMap{}},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(operatorconfig.ImageConfigMap, config.GetDefaultNamespace(), false, false, true)),
		).
		Watches(
			&source.Kind{Type: &appsv1.StatefulSet{}},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(operatorconfig.PrometheusUserWorkload, config.HubUwlMetricsCollectorNs, true, false, true)),
		).
		Watches(
			&source.Kind{Type: &corev1.Secret{}},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(config.AlertmanagerAccessorSecretName, config.GetDefaultNamespace(), false, false, true)),
		).
		Watches(
			&source.Kind{Type: &corev1.ServiceAccount{}},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(config.HubEndpointSaName, config.GetDefaultNamespace(), false, false, true)),
		)
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

// enter the loop for the following reconcile requests:
// 1. MCO CR change(request name is "mco-updated-request")
// 2. MCH resource change(request name is "mch-updated-request"), to handle image replacement in upgrade case.
// 3. ClusterManagementAddon change(request name is "clustermgmtaddon-updated-request")
// 4. configmap/secret... resource change from observability namespace
// 5. managedcluster change(request namespace is emprt string and request name is managedcluster name)
// 6. manifestwork/observabilityaddon/managedclusteraddon/rolebinding... change from managedcluster namespace
func isReconcileRequired(request ctrl.Request, managedCluster string) bool {
	if request.Name == config.MCOUpdatedRequestName ||
		request.Name == config.MCHUpdatedRequestName ||
		request.Name == config.ClusterManagementAddOnUpdateName ||
		request.Name == config.AddonDeploymentConfigUpdateName {
		return true
	}
	if request.Namespace == config.GetDefaultNamespace() ||
		(request.Namespace == "" && request.Name == managedCluster) ||
		request.Namespace == managedCluster {
		return true
	}
	return false
}
