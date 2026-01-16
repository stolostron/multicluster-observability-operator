// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	operatorv1 "github.com/openshift/api/operator/v1"
	mcov1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	cert_controller "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/certificates"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/rendering/templates"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	templatesutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering/templates"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	ownerLabelKey                                  = "owner"
	ownerLabelValue                                = "multicluster-observability-operator"
	managedClusterObsCertName                      = "observability-managed-cluster-certs"
	nonOCP                                         = "N/A"
	disableAddonAutomaticInstallationAnnotationKey = "addon.open-cluster-management.io/disable-automatic-installation"
)

var (
	log                               = logf.Log.WithName("controller_placementrule")
	clusterAddon                      = &addonv1alpha1.ClusterManagementAddOn{}
	defaultAddonDeploymentConfig      = &addonv1alpha1.AddOnDeploymentConfig{}
	isplacementControllerRunnning     = false
	managedClustersHaveReconciledOnce bool // Ensures that all managedClusters are reconciled once on MCO reboot
)

// PlacementRuleReconciler reconciles a PlacementRule object
type PlacementRuleReconciler struct {
	Client     client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
	CRDMap     map[string]bool
	RESTMapper meta.RESTMapper
	KubeClient kubernetes.Interface

	statusIsInitialized bool
	statusMu            sync.Mutex
}

// +kubebuilder:rbac:groups=observability.open-cluster-management.io,resources=placementrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=observability.open-cluster-management.io,resources=placementrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=observability.open-cluster-management.io,resources=placementrules/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Modify the Reconcile function to compare the state specified by
// the MultiClusterObservability object against the actual cluster state, and then
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

	var mcoIsNotFound bool
	// Fetch the MultiClusterObservability instance
	mco := &mcov1beta2.MultiClusterObservability{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: config.GetMonitoringCRName()}, mco)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			mcoIsNotFound = true
		} else {
			return ctrl.Result{}, fmt.Errorf("failed to get MCO CR: %w", err)
		}
	}

	// Do not reconcile objects if this instance of mch is labeled "paused"
	if config.IsPaused(mco.GetAnnotations()) {
		reqLogger.Info("MCO reconciliation is paused. Nothing more to do.")
		return ctrl.Result{}, nil
	}

	if r.waitForImageList(reqLogger) {
		reqLogger.Info("Wait for image list, requeuing")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// only update managedclusteraddon status when obs addon's status updated
	// ensure the status is updated once in the reconcile loop when the controller starts
	if err := r.updateStatus(ctx, req); err != nil {
		reqLogger.Info("Failed to update status", "error", err.Error())
	}

	// When MCOA is enabled, additionnally clean the hub resources as they are deployed wihtout the addon resource,
	// and thus are not removed by the cleanResources function.
	if mcoaForMetricsIsEnabled(mco) {
		reqLogger.Info("Ensuring MCOA resources on the hub")
		if err := r.ensureMCOAResources(ctx, mco); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to ensure MCOA resources: %w", err)
		}
		// Force regeneration of the hubInfo secret to ensure MCOA settings are up to date
		if err := DeleteHubMetricsCollectorResourcesNotNeededForMCOA(ctx, r.Client); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete hub metrics collection resources: %w", err)
		}
	}

	// Clean spokes addon resources (except the hub collector) if metrics are disabled.
	metricsAreDisabled := mco.Spec.ObservabilityAddonSpec != nil && !mco.Spec.ObservabilityAddonSpec.EnableMetrics
	if mcoIsNotFound || metricsAreDisabled || mcoaForMetricsIsEnabled(mco) {
		reqLogger.Info("Cleaning all spokes resources", "mcoIsNotFound", mcoIsNotFound, "metricsAreDisabled",
			metricsAreDisabled, "mcoaIsEnabled", mcoaForMetricsIsEnabled(mco))
		if requeue, err := r.cleanSpokesAddonResources(ctx); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to clean all resources: %w", err)
		} else if requeue {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		if mcoIsNotFound || metricsAreDisabled {
			if err := DeleteHubMetricsCollectionDeployments(ctx, r.Client); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to delete hub metrics collection deployments and resources: %w", err)
			}
			// Also clear the amAccessorToken global var, to ensure it's not re-used on re-deploys
			amAccessorTokenSecret = nil
		}

		// Don't return right away from here because the above cleanup is not complete and it requires
		// call to cleanOrphanResources for manifest works.
	} else {
		reqLogger.Info("Creating all addon resources")
		opts := &client.ListOptions{
			LabelSelector: labels.SelectorFromSet(map[string]string{ownerLabelKey: ownerLabelValue}),
		}
		if req.Namespace != "" && req.Namespace != config.GetDefaultNamespace() {
			opts.Namespace = req.Namespace
		}
		obsAddonList := &mcov1beta1.ObservabilityAddonList{}
		if err := r.Client.List(ctx, obsAddonList, opts); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to list observabilityaddon resource: %w", err)
		}

		if err := createAllRelatedRes(
			ctx,
			r.Client,
			req,
			mco,
			r.CRDMap,
			r.KubeClient,
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create all related resources: %w", err)
		}
	}

	// This cleanup must be kept at the end of the reconcile as createAllRelatedRes can remove some observabilityAddon
	// resources that trigger then some cleanup here. Same for cleanResources that leaves resources behind.
	if requeue, err := r.cleanOrphanResources(ctx, req); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to clean orphaned resources: %w", err)
	} else if requeue {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// updateStatus ensures that the addonStatuses are updated at least once on the first reconcile
// to avoid stalled statuses, and then whenever the reconcile trigger is an observabilityAddon.
func (r *PlacementRuleReconciler) updateStatus(ctx context.Context, req ctrl.Request) error {
	r.statusMu.Lock()
	defer r.statusMu.Unlock()

	if req.Name != obsAddonName && r.statusIsInitialized {
		return nil
	}

	opts := &client.ListOptions{LabelSelector: labels.SelectorFromSet(map[string]string{ownerLabelKey: ownerLabelValue})}
	if r.statusIsInitialized && req.Namespace != "" {
		opts.Namespace = req.Namespace
	}
	obsAddonList := &mcov1beta1.ObservabilityAddonList{}
	if err := r.Client.List(ctx, obsAddonList, opts); err != nil {
		return fmt.Errorf("failed to list observabilityaddon resource: %w", err)
	}

	if err := updateAddonStatus(ctx, r.Client, *obsAddonList); err != nil {
		return err
	}

	r.statusIsInitialized = true
	return nil
}

func (r *PlacementRuleReconciler) cleanOrphanResources(ctx context.Context, req ctrl.Request) (bool, error) {
	opts := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{ownerLabelKey: ownerLabelValue}),
	}
	if req.Namespace != "" && req.Namespace != config.GetDefaultNamespace() {
		opts.Namespace = req.Namespace
	}

	obsAddonList := &mcov1beta1.ObservabilityAddonList{}
	if err := r.Client.List(ctx, obsAddonList, opts); err != nil {
		return false, fmt.Errorf("failed to list owned observabilityaddon resources: %w", err)
	}

	workList := &workv1.ManifestWorkList{}
	if err := r.Client.List(ctx, workList, opts); err != nil {
		return false, fmt.Errorf("failed to list owned manifestwork resources: %w", err)
	}

	managedclusteraddonList := &addonv1alpha1.ManagedClusterAddOnList{}
	if err := r.Client.List(ctx, managedclusteraddonList, opts); err != nil {
		return false, fmt.Errorf("failed to list owned managedclusteraddon resources: %w", err)
	}

	currentAddonNamespaces := map[string]mcov1beta1.ObservabilityAddon{}
	for _, addon := range obsAddonList.Items {
		currentAddonNamespaces[addon.GetNamespace()] = addon
	}

	managedClusterList, err := getManagedClustersList(ctx, r.Client)
	if err != nil {
		return false, fmt.Errorf("failed to get managed clusters list: %w", err)
	}
	managedClustersNamespaces := make(map[string]struct{}, len(managedClusterList))
	for _, mc := range managedClusterList {
		if mc.IsLocalCluster {
			// local cluster resources live in a different namespace than the ManagedClusterName
			managedClustersNamespaces[config.GetDefaultNamespace()] = struct{}{}
		} else {
			managedClustersNamespaces[mc.Name] = struct{}{}
		}
	}

	requeue := false
	// Detect and delete ObservabilityAddons for missing ManagedClusters.
	// When a managedCluster resource gets the label 'observability: disabled', it disappears from the cache
	// as defined in main.go (filtered cache). Then this resource becomes invisible from the controller's
	// client. This is why this cleaning using the managedCluster resource is necessary.
	for ns := range currentAddonNamespaces {
		if _, ok := managedClustersNamespaces[ns]; ok {
			continue
		}
		log.Info("Deleting orphaned ObservabilityAddon (ManagedCluster missing)", "namespace", ns)
		// deleteObsAddon triggers the cleanup on the spoke by removing the ObservabilityAddon from the ManifestWork.
		rq, err := deleteObsAddon(ctx, r.Client, ns)
		if err != nil {
			return false, fmt.Errorf("failed to delete orphaned observabilityaddon in namespace %q: %w", ns, err)
		}
		if rq {
			requeue = true
			// We skip the deletion of the ManagedClusterAddOn for now to allow the endpoint operator
			// (which is managed via the ManagedClusterAddOn) to clean up the resources in the managed cluster.
			continue
		}
		// Also ensure managed cluster resources are gone
		if err := deleteManifestWorks(ctx, r.Client, ns); err != nil {
			return false, fmt.Errorf("failed to delete manifestworks in namespace %q: %w", ns, err)
		}
		if err := deleteManagedClusterAddOn(ctx, r.Client, ns); err != nil {
			return false, fmt.Errorf("failed to delete orphaned managed cluster resources in namespace %q: %w", ns, err)
		}
		// Remove from map so we don't process it again in the next loop
		delete(currentAddonNamespaces, ns)
	}

	namespacesWithResources := map[string]struct{}{}
	for _, work := range workList.Items {
		if work.Name != work.Namespace+workNameSuffix {
			log.Info("Deleting ManifestWork with invalid name", "namespace", work.Namespace, "name", work.Name)
			if err := deleteManifestWork(ctx, r.Client, work.Name, work.Namespace); err != nil {
				return false, fmt.Errorf("failed to delete invalid ManifestWork: %w", err)
			}
		}
		namespacesWithResources[work.GetNamespace()] = struct{}{}
	}

	// after the managedcluster is detached, the manifestwork for observability will be delete be the cluster manager,
	// but the managedclusteraddon for observability will not deleted by the cluster manager, so check against the
	// managedclusteraddon list to remove the managedcluster resources after the managedcluster is detached.
	for _, mcaddon := range managedclusteraddonList.Items {
		namespacesWithResources[mcaddon.GetNamespace()] = struct{}{}
	}

	// Delete orphen resources in namespaces with no observability addon
	for ns := range namespacesWithResources {
		if _, ok := currentAddonNamespaces[ns]; ok {
			continue
		}

		if ns == config.GetDefaultNamespace() {
			// Local cluster has no ObservabilityAddon, skip if the namespace matches
			continue
		}

		log.Info("Deleting orphaned ManagedCluster resources", "namespace", ns)
		if err := deleteManifestWorks(ctx, r.Client, ns); err != nil {
			return false, fmt.Errorf("failed to delete manifestworks in namespace %q: %w", ns, err)
		}
		if err := deleteManagedClusterAddOn(ctx, r.Client, ns); err != nil {
			return false, fmt.Errorf("failed to delete managed cluster resources in namespace %q: %w", ns, err)
		}
	}

	// Delete observability addon in namespaces with no resources that may be stalled
	for ns := range currentAddonNamespaces {
		if _, ok := namespacesWithResources[ns]; ok {
			continue
		}

		rq, err := deleteObsAddonObject(ctx, r.Client, ns)
		if err != nil {
			return false, fmt.Errorf("failed to delete stalled observability addon in namespace %q: %w", ns, err)
		}
		if rq {
			requeue = true
		}
	}

	if requeue {
		log.Info("Some resources are still pending deletion, requeueing")
	}

	return requeue, nil
}

func (r *PlacementRuleReconciler) waitForImageList(reqLogger logr.Logger) bool {
	// check if the MCH CRD exists
	mchCrdExists := r.CRDMap[config.MCHCrdName]
	// requeue after 10 seconds if the mch crd exists and image image manifests map is empty
	if mchCrdExists && len(config.GetImageManifests()) == 0 {
		// if the mch CR is not ready, then requeue the request after 10s
		reqLogger.Info("Empty images manifest, requeuing")
		return true
	}

	return false
}

func (r *PlacementRuleReconciler) cleanSpokesAddonResources(ctx context.Context) (bool, error) {
	opts := &client.ListOptions{LabelSelector: labels.SelectorFromSet(map[string]string{ownerLabelKey: ownerLabelValue})}
	obsAddonList := &mcov1beta1.ObservabilityAddonList{}
	if err := r.Client.List(ctx, obsAddonList, opts); err != nil {
		return false, fmt.Errorf("failed to list observabilityaddon resource: %w", err)
	}

	requeue, err := deleteAllObsAddons(ctx, r.Client, obsAddonList)
	if err != nil {
		return false, fmt.Errorf("failed to delete all observability addons: %w", err)
	}
	if requeue {
		// We skip the deletion of the ManagedClusterAddOns for now to allow the endpoint operators
		// to clean up the resources in the managed clusters. We will proceed with the final
		// deletion once the ObservabilityAddon finalizers are removed.
		return true, nil
	}

	// Force deletion of ManagedCluster resources to ensure immediate cleanup
	// instead of waiting for cleanOrphanResources which might be delayed by finalizers.
	for _, addon := range obsAddonList.Items {
		if err := deleteManifestWorks(ctx, r.Client, addon.Namespace); err != nil {
			log.Error(err, "Failed to delete manifestworks", "namespace", addon.Namespace)
		}
		if err := deleteManagedClusterAddOn(ctx, r.Client, addon.Namespace); err != nil {
			log.Error(err, "Failed to delete managed cluster resources", "namespace", addon.Namespace)
		}
	}

	opts.Namespace = ""
	workList := &workv1.ManifestWorkList{}
	if err := r.Client.List(ctx, workList, opts); err != nil {
		return false, fmt.Errorf("failed to list manifestwork resource: %w", err)
	}

	if len(workList.Items) == 0 {
		if err := deleteGlobalResource(ctx, r.Client); err != nil {
			return false, fmt.Errorf("failed to delete global resources: %w", err)
		}
	}

	return false, nil
}

// ensureMCOAResources reconciliates resources needed for MCOA (both hub and spoke).
// This includes:
// - The hub server CA cert to trust when sending metrics
// - The Hub AlertManager Token to forward alerts from the spoke cluster's Prometheus
// - The image list configMap
// - the mTLS key and cert for sending metrics to the hub
func (r *PlacementRuleReconciler) ensureMCOAResources(ctx context.Context, mco *mcov1beta2.MultiClusterObservability) error {
	resourcesToCreate := []client.Object{}
	hubServerCaCertSecret, err := generateObservabilityServerCACerts(ctx, r.Client)
	if err != nil {
		return fmt.Errorf("failed to generate observability server ca certs: %w", err)
	}
	hubServerCaCertSecret.SetNamespace(config.GetDefaultNamespace())
	if err := controllerutil.SetControllerReference(mco, hubServerCaCertSecret, r.Client.Scheme()); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}
	resourcesToCreate = append(resourcesToCreate, hubServerCaCertSecret)

	amAccessorTokenSecret, err = generateAmAccessorTokenSecret(ctx, r.Client, r.KubeClient)
	if err != nil {
		return fmt.Errorf("failed to generate alertManager token secret: %w", err)
	}
	amAccessorTokenSecret.SetNamespace(config.GetDefaultNamespace())
	if err := controllerutil.SetControllerReference(mco, amAccessorTokenSecret, r.Client.Scheme()); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}
	resourcesToCreate = append(resourcesToCreate, amAccessorTokenSecret)

	imageListCm, err := generateImageListConfigMap(mco)
	if err != nil {
		return fmt.Errorf("failed to generate image list configmap: %w", err)
	}
	imageListCm.SetNamespace(config.GetDefaultNamespace())
	if err := controllerutil.SetControllerReference(mco, imageListCm, r.Client.Scheme()); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}
	resourcesToCreate = append(resourcesToCreate, imageListCm)

	for _, obj := range resourcesToCreate {
		res, err := ctrl.CreateOrUpdate(ctx, r.Client, obj, mutateHubResourceFn(obj.DeepCopyObject().(client.Object), obj))
		if err != nil {
			return fmt.Errorf("failed to create or update resource %s: %w", obj.GetName(), err)
		}
		if res != controllerutil.OperationResultNone {
			log.Info("resource created or updated", "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName(), "action", res)
		}
	}

	if err := cert_controller.CreateUpdateMtlsCertSecretForHubCollector(ctx, r.Client); err != nil {
		log.Error(err, "Failed to create client cert secret for hub metrics collection")
		return err
	}

	return nil
}

func generateImageListConfigMap(mco *mcov1beta2.MultiClusterObservability) (*corev1.ConfigMap, error) {
	endpointObsTemplates, err := templates.GetOrLoadEndpointObservabilityTemplates(templatesutil.GetTemplateRenderer())
	if err != nil {
		return nil, fmt.Errorf("failed to load observability templates: %w", err)
	}

	var imageListCm *corev1.ConfigMap
	for _, tplt := range endpointObsTemplates {
		if tplt.GetKind() != "ConfigMap" || tplt.GetName() != operatorconfig.ImageConfigMap {
			continue
		}

		obj, err := updateRes(tplt, mco)
		if err != nil {
			return nil, fmt.Errorf("failed to generate image list resource")
		}
		// TODO: Apply special image changes when using custom registry

		var ok bool
		imageListCm, ok = obj.(*corev1.ConfigMap)
		if !ok {
			return nil, fmt.Errorf("failed to type assert image list configmap")
		}

		break
	}

	if imageListCm == nil {
		return nil, errors.New("image list not found in templates")
	}

	return imageListCm, nil
}

func createAllRelatedRes(
	ctx context.Context,
	c client.Client,
	request ctrl.Request,
	mco *mcov1beta2.MultiClusterObservability,
	crdMap map[string]bool,
	kubeClient kubernetes.Interface,
) error {
	var err error
	// create the clusterrole if not there
	if err := createReadMCOClusterRole(ctx, c); err != nil {
		return fmt.Errorf("failed to ensure cluster rule: %w", err)
	}
	if err := createResourceRole(ctx, c); err != nil {
		return fmt.Errorf("failed to ensure resource role: %w", err)
	}

	// Get or create ClusterManagementAddon
	clusterAddon, err = util.CreateClusterManagementAddon(ctx, c)
	if err != nil {
		return fmt.Errorf("failed to ensure ClusterManagementAddon: %w", err)
	}

	if err := setDefaultDeploymentConfigVar(ctx, c); err != nil {
		return fmt.Errorf("failed to set default deployment config: %w", err)
	}

	// need to reload the template and update the the corresponding resources
	// the loadTemplates method is now lightweight operations as we have cache the templates in memory.
	rawExtensionList, obsAddonCRDv1, obsAddonCRDv1beta1,
		endpointMetricsOperatorDeploy, imageListConfigMap, err = loadTemplates(mco)
	if err != nil {
		return fmt.Errorf("failed to load templates: %w", err)
	}

	works, crdv1Work, err := generateGlobalManifestResources(ctx, c, mco, kubeClient)
	if err != nil {
		return err
	}

	// regenerate the hubinfo secret if empty
	if hubInfoSecret == nil {
		var err error
		if hubInfoSecret, err = generateHubInfoSecret(ctx, c, config.GetDefaultNamespace(), spokeNameSpace, crdMap, config.IsUWMAlertingDisabledInSpec(mco)); err != nil {
			return fmt.Errorf("failed to generate hub info secret: %w", err)
		}
	}

	managedClusterList, err := getManagedClustersList(ctx, c)
	if err != nil {
		return fmt.Errorf("failed to get managed clusters list: %w", err)
	}

	var allErrors []error
	for _, mci := range managedClusterList {
		managedCluster := mci.Name
		openshiftVersion := mci.OpenshiftVersion

		if managedClustersHaveReconciledOnce && !isReconcileRequired(request, managedCluster) {
			continue
		}

		log.Info("Reconciling managed cluster resources",
			"cluster", managedCluster,
			"openshiftVersion", openshiftVersion,
			"triggered_by", request.Name,
			"in_namespace", request.Namespace)
		var installProm bool
		namespace := managedCluster
		switch openshiftVersion {
		case nonOCP:
			installProm = true
		case "mimical":
			if mco.Annotations["test-env"] == "kind-test" {
				installProm = true
			}
			namespace = config.GetDefaultNamespace()
		}

		addonDeployCfg, err := createManagedClusterRes(ctx, c, mco, managedCluster, namespace)
		if err != nil {
			allErrors = append(allErrors, fmt.Errorf("failed to createManagedClusterRes: %w", err))
			log.Error(err, "Failed to create managedcluster resources", "namespace", managedCluster)
			continue
		}
		manifestWork, err := createManifestWorks(
			ctx,
			c,
			namespace,
			mci,
			mco,
			works,
			metricsAllowlistConfigMap,
			crdv1Work,
			endpointMetricsOperatorDeploy,
			hubInfoSecret.DeepCopy(),
			addonDeployCfg,
			installProm,
		)
		if err != nil {
			allErrors = append(allErrors, fmt.Errorf("failed to create manifestworks: %w", err))
			log.Error(err, "Failed to create manifestworks")
			continue
		}

		if managedCluster != namespace && os.Getenv("UNIT_TEST") != "true" {
			// ACM 8509: Special case for hub/local cluster metrics collection
			// install the endpoint operator into open-cluster-management-observability namespace for the hub cluster
			log.Info("Creating resource for hub metrics collection", "cluster", managedCluster)
			if err := ensureResourcesForHubMetricsCollection(ctx, c, mco, manifestWork.Spec.Workload.Manifests); err != nil {
				allErrors = append(allErrors, fmt.Errorf("failed to ensure resources for hub metrics collection: %w", err))
				log.Error(err, "Failed to ensure resources for hub metrics collection")
				continue
			}
		} else {
			retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				return createManifestwork(ctx, c, manifestWork)
			})
			if retryErr != nil {
				allErrors = append(allErrors, fmt.Errorf("failed to create manifestwork: %w", retryErr))
				log.Error(retryErr, "Failed to create manifestwork")
				continue
			}
		}
	}

	if len(allErrors) == 0 {
		managedClustersHaveReconciledOnce = true
	}

	if len(allErrors) > 0 {
		return errors.Join(allErrors...)
	}

	return nil
}

func setDefaultDeploymentConfigVar(ctx context.Context, c client.Client) error {
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
		if config.Group == util.AddonGroup &&
			config.Resource == util.AddonDeploymentConfigResource {
			if config.DefaultConfig != nil {
				addonConfig := &addonv1alpha1.AddOnDeploymentConfig{}
				err := c.Get(ctx,
					types.NamespacedName{
						Name:      config.DefaultConfig.Name,
						Namespace: config.DefaultConfig.Namespace,
					},
					addonConfig,
				)
				if err != nil {
					return fmt.Errorf("failed to get default config: %w", err)
				}
				log.Info("Setting the default AddonDeploymentConfig variable for current addon")
				defaultAddonDeploymentConfig = addonConfig
				break
			}
		}
	}

	return nil
}

func deleteAllObsAddons(
	ctx context.Context,
	client client.Client,
	obsAddonList *mcov1beta1.ObservabilityAddonList,
) (bool, error) {
	requeue := false
	for _, ep := range obsAddonList.Items {
		rq, err := deleteObsAddon(ctx, client, ep.Namespace)
		if err != nil {
			log.Error(err, "Failed to delete observabilityaddon", "namespace", ep.Namespace)
			return false, err
		}
		if rq {
			requeue = true
		}
	}
	if requeue {
		log.Info("At least one observabilityaddon is still pending deletion, requeueing")
	}
	return requeue, nil
}

func deleteGlobalResource(ctx context.Context, c client.Client) error {
	err := deleteClusterRole(ctx, c)
	if err != nil {
		return err
	}
	err = deleteResourceRole(ctx, c)
	if err != nil {
		return err
	}
	// delete ClusterManagementAddon
	err = util.DeleteClusterManagementAddon(ctx, c)
	if err != nil {
		return err
	}
	return nil
}

// createManagedClusterRes creates:
// - the observability addon in the namespace
// - the role bindings for system groups
// - the managedClusterAddon named "observability-controller"
func createManagedClusterRes(ctx context.Context, c client.Client, mco *mcov1beta2.MultiClusterObservability, name string, namespace string) (*addonv1alpha1.AddOnDeploymentConfig, error) {
	if err := createObsAddon(ctx, mco, c, namespace); err != nil {
		return nil, fmt.Errorf("failed to create observabilityaddon: %w", err)
	}

	if err := createRolebindings(ctx, c, namespace, name); err != nil {
		return nil, fmt.Errorf("failed to create role bindings: %w", err)
	}

	addon, err := util.CreateManagedClusterAddonCR(ctx, c, namespace, ownerLabelKey, ownerLabelValue)
	if err != nil {
		return nil, fmt.Errorf("failed to create ManagedClusterAddon: %w", err)
	}

	addonConfig := &addonv1alpha1.AddOnDeploymentConfig{}
	isCustomConfig := false
	for _, config := range addon.Spec.Configs {
		if config.Group == util.AddonGroup &&
			config.Resource == util.AddonDeploymentConfigResource {
			err = c.Get(ctx,
				types.NamespacedName{
					Name:      config.Name,
					Namespace: config.Namespace,
				},
				addonConfig,
			)
			if err != nil {
				return nil, err
			}
			isCustomConfig = true
			log.Info("There is AddonDeploymentConfig for current addon", "namespace", namespace)
			break
		}
	}
	if !isCustomConfig {
		addonConfig = defaultAddonDeploymentConfig
	}

	return addonConfig, nil
}

func deleteManagedClusterAddOn(ctx context.Context, c client.Client, namespace string) error {
	managedclusteraddon := &addonv1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.ManagedClusterAddonName,
			Namespace: namespace,
		},
	}
	err := c.Delete(ctx, managedclusteraddon)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Error(err, "Failed to delete managedclusteraddon")
			return err
		}
	} else {
		log.Info("Deleted managed cluster addon", "namespace", namespace, "name", managedclusteraddon.Name)
	}

	err = deleteRolebindings(ctx, c, namespace)
	if err != nil {
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

type managedClusterInfo struct {
	Name             string
	OpenshiftVersion string
	IsLocalCluster   bool
}

// getManagedClustersList returns the list of managed clusters info,
// including the local cluster with a specific OpenshiftVersion for trigerring specific processings
func getManagedClustersList(ctx context.Context, c client.Client) ([]managedClusterInfo, error) {
	managedClustersList := &clusterv1.ManagedClusterList{}
	if err := c.List(ctx, managedClustersList); err != nil {
		return nil, fmt.Errorf("failed to list managed clusters: %w", err)
	}

	ret := make([]managedClusterInfo, 0, len(managedClustersList.Items))
	appended := false

	for _, mc := range managedClustersList.Items {
		if mc.Labels["local-cluster"] == "true" && !appended {
			ret = append(ret, managedClusterInfo{
				Name:             mc.GetName(),
				OpenshiftVersion: "mimical",
				IsLocalCluster:   true,
			})
			appended = true
			continue
		}

		if mc.GetDeletionTimestamp() != nil {
			// ignore deleted clusters
			continue
		}

		// ACM-27834: Skip clusters whose labels aren't ready yet
		// This prevents treating any OCP cluster still provisioning or waiting for clusterverion to settle
		// as non-OCP before openshiftVersion label is set
		if !areManagedClusterLabelsReady(&mc) {
			log.Info("Skipping managed cluster - labels not ready",
				"cluster", mc.GetName(),
				"vendor", mc.GetLabels()["vendor"],
				"hasOpenshiftVersion", mc.GetLabels()["openshiftVersion"] != "")
			continue // Will be picked up when labels are updated (watch triggers reconcile)
		}

		// Labels are ready - safe to use openshiftVersion
		openshiftVersion := nonOCP
		if version, ok := mc.GetLabels()["openshiftVersion"]; ok {
			openshiftVersion = version
		}

		ret = append(ret, managedClusterInfo{
			Name:             mc.GetName(),
			OpenshiftVersion: openshiftVersion,
			IsLocalCluster:   false,
		})
	}

	// When hubSelfManagement is disabled, the local cluster is not registered as a managed cluster,
	// so we need to add it manually to the list of managed clusters.
	if !appended {
		ret = append(ret, managedClusterInfo{
			Name:             "local-cluster",
			OpenshiftVersion: "mimical",
			IsLocalCluster:   true,
		})
	}

	return ret, nil
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

func getObsAddonPred(c client.Client) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(_ event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			equalStatus := equality.Semantic.DeepEqual(e.ObjectNew.(*mcov1beta1.ObservabilityAddon).Status.Conditions,
				e.ObjectOld.(*mcov1beta1.ObservabilityAddon).Status.Conditions)
			equalSpec := equality.Semantic.DeepEqual(e.ObjectNew.(*mcov1beta1.ObservabilityAddon).Spec,
				e.ObjectOld.(*mcov1beta1.ObservabilityAddon).Spec)
			equalAnnotations := equality.Semantic.DeepEqual(e.ObjectNew.(*mcov1beta1.ObservabilityAddon).Annotations,
				e.ObjectOld.(*mcov1beta1.ObservabilityAddon).Annotations)
			equalFinalizers := equality.Semantic.DeepEqual(e.ObjectNew.(*mcov1beta1.ObservabilityAddon).Finalizers,
				e.ObjectOld.(*mcov1beta1.ObservabilityAddon).Finalizers)

			if e.ObjectNew.GetName() == obsAddonName &&
				e.ObjectNew.GetLabels()[ownerLabelKey] == ownerLabelValue &&
				(!equalStatus || !equalSpec || !equalAnnotations || !equalFinalizers) {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetName() == obsAddonName &&
				e.Object.GetLabels()[ownerLabelKey] == ownerLabelValue {
				log.Info(
					"DeleteFunc",
					"obsAddonNamespace",
					e.Object.GetNamespace(),
					"obsAddonName",
					e.Object.GetName(),
				)

				if err := removePostponeDeleteAnnotationForManifestwork(context.Background(), c, e.Object.GetNamespace()); err != nil {
					log.Error(err, "postpone delete annotation for manifestwork could not be removed")
					return false
				}
				return true
			}
			return false
		},
	}
}

func getAllowlistPred(c client.Client) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if (e.Object.GetName() == config.AllowlistCustomConfigMapName ||
				e.Object.GetName() == operatorconfig.AllowlistConfigMapName) &&
				e.Object.GetNamespace() == config.GetDefaultNamespace() {
				// generate the metrics allowlist configmap
				log.Info("generate metric allow list configmap for allowlist configmap CREATE")
				metricsAllowlistConfigMap, _ = generateMetricsListCM(context.Background(), c)
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
				metricsAllowlistConfigMap, _ = generateMetricsListCM(context.Background(), c)
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
				metricsAllowlistConfigMap, _ = generateMetricsListCM(context.Background(), c)
				return true
			}
			return false
		},
	}
}

var certSecretPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		if e.Object.GetName() == config.ServerCACerts &&
			e.Object.GetNamespace() == config.GetDefaultNamespace() {
			return true
		}
		return false
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		if (e.ObjectNew.GetName() == config.ServerCACerts &&
			e.ObjectNew.GetNamespace() == config.GetDefaultNamespace()) &&
			e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
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

func getIngressControllerPred(c client.Client, crdMap map[string]bool) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetName() == config.OpenshiftIngressOperatorCRName &&
				e.Object.GetNamespace() == config.OpenshiftIngressOperatorNamespace {
				return updateHubInfoSecret(c, crdMap)
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetName() == config.OpenshiftIngressOperatorCRName &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() &&
				e.ObjectNew.GetNamespace() == config.OpenshiftIngressOperatorNamespace {
				return updateHubInfoSecret(c, crdMap)
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetName() == config.OpenshiftIngressOperatorCRName &&
				e.Object.GetNamespace() == config.OpenshiftIngressOperatorNamespace {
				return updateHubInfoSecret(c, crdMap)
			}
			return false
		},
	}
}

func getAmRouterCertSecretPred(c client.Client, crdMap map[string]bool) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetNamespace() == config.GetDefaultNamespace() &&
				(e.Object.GetName() == config.AlertmanagerRouteBYOCAName ||
					e.Object.GetName() == config.AlertmanagerRouteBYOCERTName) {
				return updateHubInfoSecret(c, crdMap)
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetNamespace() == config.GetDefaultNamespace() &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() &&
				(e.ObjectNew.GetName() == config.AlertmanagerRouteBYOCAName ||
					e.ObjectNew.GetName() == config.AlertmanagerRouteBYOCERTName) {
				return updateHubInfoSecret(c, crdMap)
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetNamespace() == config.GetDefaultNamespace() &&
				(e.Object.GetName() == config.AlertmanagerRouteBYOCAName ||
					e.Object.GetName() == config.AlertmanagerRouteBYOCERTName) {
				return updateHubInfoSecret(c, crdMap)
			}
			return false
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *PlacementRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	c := mgr.GetClient()
	ingressCtlCrdExists := r.CRDMap[config.IngressControllerCRD]

	clusterPred := getClusterPreds()

	// Watch changes for AddonDeploymentConfig
	addOnDeploymentConfigPred := GetAddOnDeploymentConfigPredicates()

	// Watch changes to endpoint-operator deployment
	hubEndpointOperatorPred := getHubEndpointOperatorPredicates()

	obsAddonPred := getObsAddonPred(c)

	allowlistPred := getAllowlistPred(c)

	ingressControllerPred := getIngressControllerPred(c, r.CRDMap)

	amRouterCertSecretPred := getAmRouterCertSecretPred(c, r.CRDMap)

	routeCASecretPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if (e.Object.GetNamespace() == config.OpenshiftIngressOperatorNamespace &&
				e.Object.GetName() == config.OpenshiftIngressRouteCAName) ||
				(e.Object.GetNamespace() == config.OpenshiftIngressNamespace &&
					e.Object.GetName() == config.OpenshiftIngressDefaultCertName) {
				return updateHubInfoSecret(c, r.CRDMap)
			}
			// Check if this secret might be a custom ingress certificate
			if e.Object.GetNamespace() == config.OpenshiftIngressNamespace {
				if isCustomIngressCertificate(c, e.Object.GetName()) {
					return updateHubInfoSecret(c, r.CRDMap)
				}
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if ((e.ObjectNew.GetNamespace() == config.OpenshiftIngressOperatorNamespace &&
				e.ObjectNew.GetName() == config.OpenshiftIngressRouteCAName) ||
				(e.ObjectNew.GetNamespace() == config.OpenshiftIngressNamespace &&
					e.ObjectNew.GetName() == config.OpenshiftIngressDefaultCertName)) &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				return updateHubInfoSecret(c, r.CRDMap)
			}
			// Check if this secret might be a custom ingress certificate
			if e.ObjectNew.GetNamespace() == config.OpenshiftIngressNamespace &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				if isCustomIngressCertificate(c, e.ObjectNew.GetName()) {
					return updateHubInfoSecret(c, r.CRDMap)
				}
			}
			return false
		},
		DeleteFunc: func(_ event.DeleteEvent) bool {
			return false
		},
	}

	amAccessorSAPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetName() == config.AlertmanagerAccessorSAName &&
				e.Object.GetNamespace() == config.GetDefaultNamespace() {
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if (e.ObjectNew.GetName() == config.AlertmanagerAccessorSAName &&
				e.ObjectNew.GetNamespace() == config.GetDefaultNamespace()) &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				return true
			}
			return false
		},
		DeleteFunc: func(_ event.DeleteEvent) bool {
			return false
		},
	}

	ctrBuilder := ctrl.NewControllerManagedBy(mgr).
		// Watch for changes to primary resource ManagedCluster with predicate
		For(&clusterv1.ManagedCluster{}, builder.WithPredicates(clusterPred)).
		// secondary watch for observabilityaddon
		Watches(&mcov1beta1.ObservabilityAddon{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(obsAddonPred)).

		// secondary watch for MCO
		Watches(&mcov1beta2.MultiClusterObservability{}, handler.EnqueueRequestsFromMapFunc(func(_ context.Context, _ client.Object) []reconcile.Request {
			return []reconcile.Request{
				{NamespacedName: types.NamespacedName{
					Name: config.MCOUpdatedRequestName,
				}},
			}
		}), builder.WithPredicates(getMCOPred(c, r.CRDMap))).

		// secondary watch for custom allowlist configmap
		Watches(&corev1.ConfigMap{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(allowlistPred)).

		// secondary watch for certificate secrets
		Watches(&corev1.Secret{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(certSecretPred)).

		// secondary watch for alertmanager accessor serviceaccount
		Watches(&corev1.ServiceAccount{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(amAccessorSAPred))

	// watch for AddOnDeploymentConfig
	addOnDeploymentConfigGroupKind := schema.GroupKind{Group: addonv1alpha1.GroupVersion.Group, Kind: "AddOnDeploymentConfig"}
	if _, err := r.RESTMapper.RESTMapping(addOnDeploymentConfigGroupKind, addonv1alpha1.GroupVersion.Version); err == nil {
		ctrBuilder = ctrBuilder.Watches(
			&addonv1alpha1.AddOnDeploymentConfig{},
			handler.EnqueueRequestsFromMapFunc(func(_ context.Context, _ client.Object) []reconcile.Request {
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
			&workv1.ManifestWork{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(workPred),
		)
	}

	clusterMgmtGroupKind := schema.GroupKind{Group: addonv1alpha1.GroupVersion.Group, Kind: "ClusterManagementAddOn"}
	if _, err := r.RESTMapper.RESTMapping(clusterMgmtGroupKind, addonv1alpha1.GroupVersion.Version); err == nil {
		clusterMgmtPred := getClusterMgmtAddonPredFunc()

		// secondary watch for clustermanagementaddon
		ctrBuilder = ctrBuilder.Watches(
			&addonv1alpha1.ClusterManagementAddOn{},
			handler.EnqueueRequestsFromMapFunc(func(_ context.Context, _ client.Object) []reconcile.Request {
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
			&addonv1alpha1.ManagedClusterAddOn{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(mgClusterGroupKindPred),
		)
	}

	mchGroupKind := schema.GroupKind{Group: mchv1.GroupVersion.Group, Kind: "MultiClusterHub"}
	if _, err := r.RESTMapper.RESTMapping(mchGroupKind, mchv1.GroupVersion.Version); err == nil {
		mchPred := getMchPred(c)

		if ingressCtlCrdExists {
			// secondary watch for default ingresscontroller
			ctrBuilder = ctrBuilder.Watches(&operatorv1.IngressController{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(ingressControllerPred)).

				// secondary watch for alertmanager route byo cert secrets
				Watches(&corev1.Secret{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(amRouterCertSecretPred)).

				// secondary watch for openshift route ca secret
				Watches(&corev1.Secret{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(routeCASecretPred))
		}

		mchCrdExists := r.CRDMap[config.MCHCrdName]
		if mchCrdExists {
			// secondary watch for MCH
			ctrBuilder = ctrBuilder.Watches(
				&mchv1.MultiClusterHub{},
				handler.EnqueueRequestsFromMapFunc(func(_ context.Context, obj client.Object) []reconcile.Request {
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

	ctrBuilder = ctrBuilder.Watches(&appsv1.Deployment{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(hubEndpointOperatorPred)).
		Watches(
			&corev1.Secret{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(operatorconfig.HubInfoSecretName, config.GetDefaultNamespace(), false, false, true)),
		).
		Watches(
			&corev1.Secret{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(operatorconfig.HubMetricsCollectorMtlsCert, config.GetDefaultNamespace(), false, false, true)),
		).
		Watches(
			&corev1.Secret{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(managedClusterObsCertName, config.GetDefaultNamespace(), false, false, true)),
		).
		Watches(
			&corev1.ConfigMap{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(operatorconfig.ImageConfigMap, config.GetDefaultNamespace(), false, false, true)),
		).
		Watches(
			&appsv1.StatefulSet{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(operatorconfig.PrometheusUserWorkload, config.HubUwlMetricsCollectorNs, true, false, true)),
		).
		Watches(
			&corev1.Secret{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(config.AlertmanagerAccessorSecretName, config.GetDefaultNamespace(), false, false, true)),
		).
		Watches(
			&corev1.ServiceAccount{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(config.HubEndpointSaName, config.GetDefaultNamespace(), false, false, true)),
		)
	// create and return a new controller
	return ctrBuilder.Complete(r)
}

func StartPlacementController(_ context.Context, mgr manager.Manager, crdMap map[string]bool) error {
	if isplacementControllerRunnning {
		return nil
	}
	isplacementControllerRunnning = true

	//nolint:contextcheck // SetupWithManager creates a controller that runs in the background and manages its own context.
	if err := (&PlacementRuleReconciler{
		Client:     mgr.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName("PlacementRule"),
		Scheme:     mgr.GetScheme(),
		CRDMap:     crdMap,
		RESTMapper: mgr.GetRESTMapper(),
		KubeClient: kubernetes.NewForConfigOrDie(mgr.GetConfig()),
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

func mcoaForMetricsIsEnabled(mco *mcov1beta2.MultiClusterObservability) bool {
	if mco.Spec.Capabilities == nil {
		return false
	}

	if mco.Spec.Capabilities.Platform != nil && mco.Spec.Capabilities.Platform.Metrics.Default.Enabled {
		return true
	}

	if mco.Spec.Capabilities.UserWorkloads != nil && mco.Spec.Capabilities.UserWorkloads.Metrics.Default.Enabled {
		return true
	}

	return false
}

// isCustomIngressCertificate checks if the given secret name is referenced by the IngressController
// as a custom default certificate
func isCustomIngressCertificate(c client.Client, secretName string) bool {
	ingressOperator := &operatorv1.IngressController{}
	err := c.Get(context.Background(), types.NamespacedName{
		Name:      config.OpenshiftIngressOperatorCRName,
		Namespace: config.OpenshiftIngressOperatorNamespace,
	}, ingressOperator)
	if err != nil {
		log.Error(err, "Failed to get IngressController to check custom certificate")
		return false
	}

	// Check if this secret is referenced as the custom default certificate
	if ingressOperator.Spec.DefaultCertificate != nil &&
		ingressOperator.Spec.DefaultCertificate.Name == secretName {
		return true
	}

	return false
}

// updateHubInfoSecret gets the MCO instance and updates the hub info secret
func updateHubInfoSecret(c client.Client, crdMap map[string]bool) bool {
	// get the MCO instance
	mco := &mcov1beta2.MultiClusterObservability{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: config.GetMonitoringCRName()}, mco); err != nil {
		log.Error(err, "Failed to get MCO instance")
		return false
	}
	// generate the hubInfo secret
	var err error
	hubInfoSecret, err = generateHubInfoSecret(
		context.Background(),
		c,
		config.GetDefaultNamespace(),
		spokeNameSpace,
		crdMap,
		config.IsUWMAlertingDisabledInSpec(mco),
	)
	if err != nil {
		log.Error(err, "Failed to generate hub info secret")
		return false
	}
	return true
}
