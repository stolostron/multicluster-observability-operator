// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package multiclusterobservability

import (
	"context"
	cerr "errors"
	"fmt"
	"os"
	"reflect"
	slices0 "slices"
	"strings"
	"time"

	imagev1 "github.com/openshift/api/image/v1"
	imagev1client "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"

	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"

	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringv1aplha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	observatoriumv1alpha1 "github.com/stolostron/observatorium-operator/api/v1alpha1"
	"golang.org/x/exp/slices"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	analyticsctrl "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability/analytics"
	placementctrl "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/placementrule"
	certctrl "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/certificates"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/rendering"
	smctrl "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/servicemonitor"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/deploying"
	commonutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
	operatorsutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
)

const (
	resFinalizer = "observability.open-cluster-management.io/res-cleanup"
	// deprecated one.
	certFinalizer = "observability.open-cluster-management.io/cert-cleanup"
)

const (
	infoAddingBackupLabel  = "adding backup label"
	errorAddingBackupLabel = "failed to add backup label"
)

var (
	log                              = logf.Log.WithName("controller_multiclustermonitoring")
	isAlertmanagerStorageSizeChanged = false
	isCompactStorageSizeChanged      = false
	isRuleStorageSizeChanged         = false
	isReceiveStorageSizeChanged      = false
	isStoreStorageSizeChanged        = false
	isLegacyResourceRemoved          = false
	lastLogTime                      = time.Now()
)

// MultiClusterObservabilityReconciler reconciles a MultiClusterObservability object
type MultiClusterObservabilityReconciler struct {
	Manager     manager.Manager
	Client      client.Client
	Log         logr.Logger
	Scheme      *runtime.Scheme
	CRDMap      map[string]bool
	APIReader   client.Reader
	RESTMapper  meta.RESTMapper
	ImageClient imagev1client.ImageV1Interface
}

// +kubebuilder:rbac:groups=observability.open-cluster-management.io,resources=multiclusterobservabilities,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=observability.open-cluster-management.io,resources=multiclusterobservabilities/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=observability.open-cluster-management.io,resources=multiclusterobservabilities/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Modify the Reconcile function to compare the state specified by
// the MultiClusterObservability object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.0/pkg/reconcile
// In ACM 2.9, we need to ensure that the openshift.io/cluster-monitoring is added to the same namespace as the
// Multi-cluster Observability Operator to avoid conflicts with the openshift-* namespace when deploying
// PrometheusRules and ServiceMonitors in ACM.
func (r *MultiClusterObservabilityReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	reqLogger.Info("Reconciling MultiClusterObservability")

	if res, ok := config.BackupResourceMap[req.Name]; ok {
		reqLogger.Info(infoAddingBackupLabel)
		var err error = nil
		resourceTypeStr := ""
		switch res {
		case config.ResourceTypeConfigMap:
			resourceTypeStr = "ConfigMap"
			err = util.AddBackupLabelToConfigMap(r.Client, req.Name, config.GetDefaultNamespace())
		case config.ResourceTypeSecret:
			resourceTypeStr = "Secret"
			err = util.AddBackupLabelToSecret(r.Client, req.Name, config.GetDefaultNamespace())
		default:
			// we should never be here
			log.Info("unknown type " + res)
		}

		if err != nil {
			reqLogger.Error(err, errorAddingBackupLabel)
			return ctrl.Result{}, fmt.Errorf("failed to add backup label to %s %s in namespace %s: %w", resourceTypeStr, req.Name, config.GetDefaultNamespace(), err)
		}
	}

	// Fetch the MultiClusterObservability instance
	mcoList := &mcov1beta2.MultiClusterObservabilityList{}
	err := r.Client.List(context.TODO(), mcoList)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list MultiClusterObservability custom resources: %w", err)
	}
	if len(mcoList.Items) > 1 {
		reqLogger.Info("more than one MultiClusterObservability CR exists, only one should exist")
		return ctrl.Result{}, nil
	}
	if len(mcoList.Items) == 0 {
		reqLogger.Info("no MultiClusterObservability CR exists, nothing to do")
		return ctrl.Result{}, nil
	}

	instance := mcoList.Items[0].DeepCopy()
	if config.GetMonitoringCRName() != instance.GetName() {
		config.SetMonitoringCRName(instance.GetName())
	}

	// start to update mco status
	StartStatusUpdate(r.Client, instance)

	if _, ok := os.LookupEnv("UNIT_TEST"); !ok {
		crdClient, err := operatorsutil.GetOrCreateCRDClient()
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get or create CRD client: %w", err)
		}
		mcghCrdExists, err := operatorsutil.CheckCRDExist(crdClient, config.MCGHCrdName)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to check for CRD %s: %w", config.MCGHCrdName, err)
		}
		if mcghCrdExists {
			// Do not start the MCO if the MCGH CRD exists
			reqLogger.Info("MCGH CRD exists, Observability is not supported")
			return ctrl.Result{}, nil
		}
	}

	ingressCtlCrdExists := r.CRDMap[config.IngressControllerCRD]
	if _, ok := os.LookupEnv("UNIT_TEST"); !ok {
		// start placement controller
		err := placementctrl.StartPlacementController(r.Manager, r.CRDMap)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to start placement controller: %w", err)
		}
		// setup ocm addon manager
		certctrl.Start(r.Client, ingressCtlCrdExists)

		// start servicemonitor controller
		smctrl.Start()
	}

	// Init finalizers
	operatorconfig.IsMCOTerminating, err = r.initFinalization(ctx, instance)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to initialize finalization: %w", err)
	} else if operatorconfig.IsMCOTerminating {
		reqLogger.Info("MCO instance is in Terminating status, skip the reconcile")
		return ctrl.Result{}, nil
	}

	// check if the MCH CRD exists
	mchCrdExists := r.CRDMap[config.MCHCrdName]
	// requeue after 10 seconds if the mch crd exists and image image manifests map is empty
	if mchCrdExists && len(config.GetImageManifests()) == 0 {
		currentTime := time.Now()

		// Log the message if it has been longer than 5 minutes since the last log
		if currentTime.Sub(lastLogTime) > 5*time.Minute {
			reqLogger.Info("Waiting for the mch CR to be ready", "mchCrdExists", mchCrdExists, "imageManifests", len(config.GetImageManifests()))
			lastLogTime = currentTime
		}

		// if the mch CR is not ready, then requeue the request after 10s
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Do not reconcile objects if this instance of mch is labeled "paused"
	if config.IsPaused(instance.GetAnnotations()) {
		reqLogger.Info("MCO reconciliation is paused. Nothing more to do.")
		return ctrl.Result{}, nil
	}

	if _, ok := config.BackupResourceMap[instance.Spec.StorageConfig.MetricObjectStorage.Name]; !ok {
		log.Info(infoAddingBackupLabel, "Secret", instance.Spec.StorageConfig.MetricObjectStorage.Name)
		config.BackupResourceMap[instance.Spec.StorageConfig.MetricObjectStorage.Name] = config.ResourceTypeSecret
		err = util.AddBackupLabelToSecret(r.Client, instance.Spec.StorageConfig.MetricObjectStorage.Name, config.GetDefaultNamespace())
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add backup label to metric object storage secret %s: %w", instance.Spec.StorageConfig.MetricObjectStorage.Name, err)
		}
	}

	imagePullSecret := config.GetImagePullSecret(instance.Spec)
	if _, ok := config.BackupResourceMap[imagePullSecret]; !ok {
		log.Info(infoAddingBackupLabel, "Secret", imagePullSecret)
		config.BackupResourceMap[imagePullSecret] = config.ResourceTypeSecret
		err = util.AddBackupLabelToSecret(r.Client, imagePullSecret, config.GetDefaultNamespace())
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add backup label to image pull secret %s: %w", imagePullSecret, err)
		}
	}

	storageClassSelected, err := getStorageClass(instance, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get storage class: %w", err)
	}

	// handle storagesize changes
	result, err := r.HandleStorageSizeChange(instance)
	if result != nil {
		// If err is non-nil, wrap it. fmt.Errorf with %w handles nil err gracefully (returns nil).
		return *result, fmt.Errorf("error during storage size change handling: %w", err)
	}

	// set operand names to cover the upgrade case since we have name changed in new release
	err = config.SetOperandNames(r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set operand names: %w", err)
	}
	instance.Spec.StorageConfig.StorageClass = storageClassSelected

	// Disable rendering the MCOA ClusterManagementAddOn resource if already exists
	mcoaCMAO := &addonv1alpha1.ClusterManagementAddOn{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: config.MultiClusterObservabilityAddon}, mcoaCMAO)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("failed to get ClusterManagementAddOn %s: %w", config.MultiClusterObservabilityAddon, err)
		}
	}
	disableMCOACMAORender := !apierrors.IsNotFound(err)

	obsAPIURL, err := mcoconfig.GetObsAPIExternalURL(ctx, r.Client, mcoconfig.GetDefaultNamespace())
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get the Observatorium API URL: %w", err) // Already wrapped
	}

	// Build render options
	rendererOptions := &rendering.RendererOptions{
		MCOAOptions: rendering.MCOARendererOptions{
			DisableCMAORender:  disableMCOACMAORender,
			MetricsHubHostname: obsAPIURL.Host,
		},
	}

	// Render the templates with a specified CR
	renderer := rendering.NewMCORenderer(instance, r.Client, r.ImageClient).WithRendererOptions(rendererOptions)
	toDeploy, err := renderer.Render()
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to render MCO templates for %s/%s: %w", instance.GetNamespace(), instance.GetName(), err)
	}
	deployer := deploying.NewDeployer(r.Client)
	// Deploy the resources
	ns := &corev1.Namespace{}
	for _, res := range toDeploy {
		resNS := res.GetNamespace()
		if err := controllerutil.SetControllerReference(instance, res, r.Scheme); err != nil {
			reqLogger.Error(err, "Failed to set controller reference", "kind", res.GetKind(), "name", res.GetName())
		}
		if resNS == "" {
			resNS = config.GetDefaultNamespace()
		}
		if err := r.Client.Get(context.TODO(), types.NamespacedName{Name: resNS}, ns); err != nil &&
			apierrors.IsNotFound(err) {
			ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name: resNS,
			}}
			if err := r.Client.Create(context.TODO(), ns); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to create namespace %s during resource deployment: %w", resNS, err)
			}
		}
		if err := deployer.Deploy(ctx, res); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to deploy %s %s/%s: %w", res.GetKind(), resNS, res.GetName(), err)
		}
	}

	if !rendering.MCOAEnabled(instance) {
		namespace, labels := renderer.NamespaceAndLabels()
		toDelete, err := renderer.MCOAResources(namespace, labels)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to list MCOA resources for deletion in namespace %s: %w", namespace, err)
		}
		for _, res := range toDelete {
			resNS := res.GetNamespace()
			if err := deployer.Undeploy(ctx, res); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to undeploy %s %s/%s: %w", res.GetKind(), resNS, res.GetName(), err)
			}
		}
	}

	_, err = r.ensureOpenShiftNamespaceLabel(ctx, instance)
	if err != nil {
		r.Log.Error(err, "Failed to add to %s label to namespace: %s", config.OpenShiftClusterMonitoringlabel,
			instance.GetNamespace())
		return ctrl.Result{}, fmt.Errorf("failed to ensure %q label on namespace %s: %w", config.OpenShiftClusterMonitoringlabel, instance.GetNamespace(), err)
	}

	// the route resource won't be created in testing env, for instance, KinD
	// in the testing env, the service can be accessed via service name, we assume that
	// in testing env, the local-cluster is the only allowed managedcluster
	if ingressCtlCrdExists {
		// expose alertmanager through route
		result, err = GenerateAlertmanagerRoute(r.Client, r.Scheme, instance)
		if result != nil {
			return *result, fmt.Errorf("failed to generate Alertmanager route: %w", err)
		}

		// expose observatorium api gateway
		result, err = GenerateAPIGatewayRoute(ctx, r.Client, r.Scheme, instance)
		if result != nil {
			return *result, fmt.Errorf("failed to generate API Gateway route: %w", err)
		}

		// expose rbac proxy through route
		result, err = GenerateProxyRoute(r.Client, r.Scheme, instance)
		if result != nil {
			return *result, fmt.Errorf("failed to generate proxy route: %w", err)
		}

		// expose grafana through route
		result, err = GenerateGrafanaRoute(r.Client, r.Scheme, instance)
		if result != nil {
			return *result, fmt.Errorf("failed to generate Grafana route: %w", err)
		}
		result, err = GenerateGrafanaOauthClient(r.Client, r.Scheme, instance)
		if result != nil {
			return *result, fmt.Errorf("failed to generate Grafana OAuth client: %w", err)
		}
	}

	// create the certificates
	err = certctrl.CreateObservabilityCerts(r.Client, r.Scheme, instance, ingressCtlCrdExists)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create observability certs: %w", err)
	}

	// create an Observatorium CR
	result, err = GenerateObservatoriumCR(r.Client, r.Scheme, instance)
	if result != nil {
		return *result, fmt.Errorf("failed to generate the observatorium CR: %w", err)
	}

	// generate grafana datasource to point to observatorium api gateway
	result, err = GenerateGrafanaDataSource(r.Client, r.Scheme, instance)
	if result != nil {
		return *result, fmt.Errorf("failed to generate Grafana data source: %w", err)
	}

	svmCrdExists := r.CRDMap[config.StorageVersionMigrationCrdName]
	if svmCrdExists {
		// create or update the storage version migration resource
		err = createOrUpdateObservabilityStorageVersionMigrationResource(r.Client, r.Scheme, instance)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create or update ObservabilityStorageVersionMigration resource: %w", err)
		}
	}

	// create rightsizing component
	err = analyticsctrl.CreateRightSizingComponent(ctx, r.Client, instance)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create rightsizing component: %w", err)
	}

	if _, ok := os.LookupEnv("UNIT_TEST"); !ok && !isLegacyResourceRemoved {
		// Delete PrometheusRule from openshift-monitoring namespace
		if err := r.deleteSpecificPrometheusRule(ctx); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete specific PrometheusRule in openshift-monitoring namespace: %w", err)
		}
		// Delete ServiceMonitor from openshft-monitoring namespace
		if err := r.deleteServiceMonitorInOpenshiftMonitoringNamespace(ctx); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete ServiceMonitor in openshift-monitoring namespace: %w", err)
		}
		isLegacyResourceRemoved = true
	}

	// update status
	requeueStatusUpdate <- struct{}{}

	return ctrl.Result{}, nil
}

func (r *MultiClusterObservabilityReconciler) initFinalization(ctx context.Context, mco *mcov1beta2.MultiClusterObservability) (bool, error) {
	if mco.GetDeletionTimestamp() != nil && slices.Contains(mco.GetFinalizers(), resFinalizer) {
		log.Info("To delete resources across namespaces")
		// clean up the cluster resources, eg. clusterrole, clusterrolebinding, etc
		operatorconfig.IsMCOTerminating = true
		if err := cleanUpClusterScopedResources(r, mco); err != nil {
			log.Error(err, "Failed to remove cluster scoped resources")
			return false, err
		}
		if err := placementctrl.DeleteHubMetricsCollectionDeployments(ctx, r.Client); err != nil {
			log.Error(err, "Failed to delete hub metrics collection deployments and resources")
			return false, err
		}
		// clean up operand names
		config.CleanUpOperandNames()

		mco.SetFinalizers(commonutil.Remove(mco.GetFinalizers(), resFinalizer))
		err := r.Client.Update(ctx, mco)
		if err != nil {
			log.Error(err, "Failed to remove finalizer from mco resource")
			return false, err
		}
		log.Info("Finalizer removed from mco resource")

		// stop update status routine
		stopStatusUpdate <- struct{}{}

		return true, nil
	}
	if !slices.Contains(mco.GetFinalizers(), resFinalizer) {
		mco.SetFinalizers(commonutil.Remove(mco.GetFinalizers(), certFinalizer))
		mco.SetFinalizers(append(mco.GetFinalizers(), resFinalizer))
		err := r.Client.Update(ctx, mco)
		if err != nil {
			log.Error(err, "Failed to add finalizer to mco resource")
			return false, err
		}
		log.Info("Finalizer added to mco resource")
	}
	return false, nil
}

func getStorageClass(mco *mcov1beta2.MultiClusterObservability, cl client.Client) (string, error) {
	storageClassSelected := mco.Spec.StorageConfig.StorageClass
	// for the test, the reader is just nil
	storageClassList := &storev1.StorageClassList{}
	err := cl.List(context.TODO(), storageClassList, &client.ListOptions{})
	if err != nil {
		return "", err
	}
	configuredWithValidSC := false
	storageClassDefault := ""
	for _, storageClass := range storageClassList.Items {
		if storageClass.ObjectMeta.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			storageClassDefault = storageClass.ObjectMeta.Name
		}
		if storageClass.ObjectMeta.Name == storageClassSelected {
			configuredWithValidSC = true
		}
	}
	if !configuredWithValidSC {
		storageClassSelected = storageClassDefault
	}
	return storageClassSelected, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MultiClusterObservabilityReconciler) SetupWithManager(mgr ctrl.Manager) error {
	c := mgr.GetClient()
	ctx := context.Background()

	mcoPred := GetMCOPredicateFunc()
	cmPred := GetConfigMapPredicateFunc()
	cmNamespaceRSPred := analyticsctrl.GetNamespaceRSConfigMapPredicateFunc(ctx, c)
	cmVirtualizationRSPred := analyticsctrl.GetVirtualizationRSConfigMapPredicateFunc(ctx, c)
	secretPred := GetAlertManagerSecretPredicateFunc()
	namespacePred := GetNamespacePredicateFunc()
	mcoaCRDPred := GetMCOACRDPredicateFunc()

	ctrBuilder := ctrl.NewControllerManagedBy(mgr).
		// Watch for changes to primary resource MultiClusterObservability with predicate
		For(&mcov1beta2.MultiClusterObservability{}, builder.WithPredicates(mcoPred)).
		// Watch for changes to secondary resource Deployment and requeue the owner MultiClusterObservability
		Owns(&appsv1.Deployment{}).
		// Watch for changes to secondary resource statefulSet and requeue the owner MultiClusterObservability
		Owns(&appsv1.StatefulSet{}).
		// Watch for changes to secondary resource ConfigMap and requeue the owner c
		Owns(&corev1.ConfigMap{}).
		// Watch for changes to secondary resource Secret and requeue the owner MultiClusterObservability
		Owns(&corev1.Secret{}).
		// Watch for changes to secondary resource Service and requeue the owner MultiClusterObservability
		Owns(&corev1.Service{}).
		// Watch for changes to secondary Observatorium CR and requeue the owner MultiClusterObservability
		Owns(&observatoriumv1alpha1.Observatorium{}).
		// Watch for changes to secondary AddOnDeploymentConfig CR and requeue the owner MultiClusterObservability
		Owns(&addonv1alpha1.AddOnDeploymentConfig{}).
		// Watch for changes to secondary ClusterManagementAddOn CR and requeue the owner MultiClusterObservability
		Owns(&addonv1alpha1.ClusterManagementAddOn{}).
		// Watch for changes to secondary PrometheusRule CR and requeue the owner MultiClusterObservability
		Owns(&monitoringv1.PrometheusRule{}).

		// Watch the configmap for rightsizing recommendation update (keep in its own watcher as it applies some processing)
		Watches(&corev1.ConfigMap{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(cmNamespaceRSPred)).
		// Watch the configmap for virtualization rightsizing recommendation update
		Watches(&corev1.ConfigMap{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(cmVirtualizationRSPred)).
		// Watch the configmap for thanos-ruler-custom-rules update
		Watches(&corev1.ConfigMap{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(cmPred)).
		// Watch the secret for deleting event of alertmanager-config
		Watches(&corev1.Secret{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(secretPred)).
		// Watch the namespace for changes
		Watches(&corev1.Namespace{}, &handler.EnqueueRequestForObject{},
			builder.WithPredicates(namespacePred)).
		// Watch the kube-system extension-apiserver-authentication ConfigMap for changes
		Watches(&corev1.ConfigMap{}, handler.EnqueueRequestsFromMapFunc(
			func(ctx context.Context, a client.Object) []reconcile.Request {
				if a.GetName() == "extension-apiserver-authentication" && a.GetNamespace() == "kube-system" {
					return []reconcile.Request{
						{NamespacedName: types.NamespacedName{
							Name:      "alertmanager-clientca-metric",
							Namespace: config.GetMCONamespace(),
						}},
					}
				}
				return nil
			}), builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})).
		Watches(&apiextensionsv1.CustomResourceDefinition{}, newMCOACRDEventHandler(c), builder.WithPredicates(mcoaCRDPred))

	if _, err := mgr.GetRESTMapper().KindFor(schema.GroupVersionResource{
		Group:    "image.openshift.io",
		Version:  "v1",
		Resource: "imagestreams",
	}); err != nil {
		if meta.IsNoMatchError(err) {
			log.Info("image.openshift.io/v1/imagestreams is not available")
		} else {
			log.Error(err, "failed to get kind for image.openshift.io/v1/imagestreams")
			os.Exit(1)
		}
	} else {
		// Images stream is only available in OpenShift
		imageStreamPred := GetImageStreamPredicateFunc()
		ctrBuilder = ctrBuilder.Watches(&imagev1.ImageStream{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(imageStreamPred))
	}

	mchGroupKind := schema.GroupKind{Group: mchv1.GroupVersion.Group, Kind: "MultiClusterHub"}
	if _, err := r.RESTMapper.RESTMapping(mchGroupKind, mchv1.GroupVersion.Version); err == nil {
		mchPred := GetMCHPredicateFunc(c)
		mchCrdExists := r.CRDMap[config.MCHCrdName]
		if mchCrdExists {
			// secondary watch for MCH
			ctrBuilder = ctrBuilder.Watches(
				&mchv1.MultiClusterHub{},
				handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, a client.Object) []reconcile.Request {
					return []reconcile.Request{
						{NamespacedName: types.NamespacedName{
							Name:      config.MCHUpdatedRequestName,
							Namespace: a.GetNamespace(),
						}},
					}
				}),
				builder.WithPredicates(mchPred),
			)
		}
	}

	// Only watch owned ScrapeConfigs CR when the CRD exists (used by MCOA)
	if exists, ok := r.CRDMap[config.PrometheusScrapeConfigsCrdName]; ok && exists {
		ctrBuilder = ctrBuilder.Owns(&monitoringv1aplha1.ScrapeConfig{})
	} else {
		log.Info("ScrapeConfig CRD will not be watched", "exists", exists, "ok", ok)
	}

	// create and return a new controller
	return ctrBuilder.Complete(r)
}

func checkStorageChanged(mcoOldConfig, mcoNewConfig *mcov1beta2.StorageConfig) {
	if mcoOldConfig.AlertmanagerStorageSize != mcoNewConfig.AlertmanagerStorageSize {
		isAlertmanagerStorageSizeChanged = true
	}
	if mcoOldConfig.CompactStorageSize != mcoNewConfig.CompactStorageSize {
		isCompactStorageSizeChanged = true
	}
	if mcoOldConfig.RuleStorageSize != mcoNewConfig.RuleStorageSize {
		isRuleStorageSizeChanged = true
	}
	if mcoOldConfig.ReceiveStorageSize != mcoNewConfig.ReceiveStorageSize {
		isReceiveStorageSizeChanged = true
	}
	if mcoOldConfig.StoreStorageSize != mcoNewConfig.StoreStorageSize {
		isStoreStorageSizeChanged = true
	}
}

// HandleStorageSizeChange is used to deal with the storagesize change in CR
// 1. Directly changed the StatefulSet pvc's size on the pvc itself for
// 2. Removed StatefulSet and
// wait for operator to re-create the StatefulSet with the correct size on the claim
func (r *MultiClusterObservabilityReconciler) HandleStorageSizeChange(
	mco *mcov1beta2.MultiClusterObservability,
) (*reconcile.Result, error) {
	if isAlertmanagerStorageSizeChanged {
		isAlertmanagerStorageSizeChanged = false
		err := updateStorageSizeChange(r.Client,
			map[string]string{
				"observability.open-cluster-management.io/name": mco.GetName(),
				"alertmanager": "observability",
			}, mco.Spec.StorageConfig.AlertmanagerStorageSize)
		if err != nil {
			return &reconcile.Result{}, err
		}
	}

	if isReceiveStorageSizeChanged {
		isReceiveStorageSizeChanged = false
		err := updateStorageSizeChange(r.Client,
			map[string]string{
				"app.kubernetes.io/instance": mco.GetName(),
				"app.kubernetes.io/name":     "thanos-receive",
			}, mco.Spec.StorageConfig.ReceiveStorageSize)
		if err != nil {
			return &reconcile.Result{}, err
		}
	}

	if isCompactStorageSizeChanged {
		isCompactStorageSizeChanged = false
		err := updateStorageSizeChange(r.Client,
			map[string]string{
				"app.kubernetes.io/instance": mco.GetName(),
				"app.kubernetes.io/name":     "thanos-compact",
			}, mco.Spec.StorageConfig.CompactStorageSize)
		if err != nil {
			return &reconcile.Result{}, err
		}
	}

	if isRuleStorageSizeChanged {
		isRuleStorageSizeChanged = false
		err := updateStorageSizeChange(r.Client,
			map[string]string{
				"app.kubernetes.io/instance": mco.GetName(),
				"app.kubernetes.io/name":     "thanos-rule",
			}, mco.Spec.StorageConfig.RuleStorageSize)
		if err != nil {
			return &reconcile.Result{}, err
		}
	}

	if isStoreStorageSizeChanged {
		isStoreStorageSizeChanged = false
		err := updateStorageSizeChange(r.Client,
			map[string]string{
				"app.kubernetes.io/instance": mco.GetName(),
				"app.kubernetes.io/name":     "thanos-store",
			}, mco.Spec.StorageConfig.StoreStorageSize)
		if err != nil {
			return &reconcile.Result{}, err
		}
	}
	return nil, nil
}

func updateStorageSizeChange(c client.Client, matchLabels map[string]string, storageSize string) error {
	pvcList, err := commonutil.GetPVCList(c, config.GetDefaultNamespace(), matchLabels)
	if err != nil {
		return err
	}

	stsList, err := commonutil.GetStatefulSetList(c, config.GetDefaultNamespace(), matchLabels)
	if err != nil {
		return err
	}

	// update pvc directly
	for index, pvc := range pvcList {
		if !pvc.Spec.Resources.Requests.Storage().Equal(resource.MustParse(storageSize)) {
			pvcList[index].Spec.Resources.Requests = corev1.ResourceList{
				corev1.ResourceName(corev1.ResourceStorage): resource.MustParse(storageSize),
			}
			err := c.Update(context.TODO(), &pvcList[index])
			if err != nil {
				return err
			}
			log.Info("Update storage size for PVC", "pvc", pvc.Name)
		}
	}
	// update sts
	for index, sts := range stsList {
		err := c.Delete(context.TODO(), &stsList[index], &client.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		log.Info("Successfully delete sts due to storage size changed", "sts", sts.Name)
	}
	return nil
}

// GenerateAlertmanagerRoute create route for Alertmanager endpoint
func GenerateAlertmanagerRoute(
	runclient client.Client, scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability,
) (*ctrl.Result, error) {
	amGateway := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.AlertmanagerRouteName,
			Namespace: config.GetDefaultNamespace(),
		},
		Spec: routev1.RouteSpec{
			Path: "/api/v2",
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromString("oauth-proxy"),
			},
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: config.AlertmanagerServiceName,
			},
			TLS: &routev1.TLSConfig{
				Termination:                   routev1.TLSTerminationReencrypt,
				InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
			},
		},
	}

	amRouteBYOCaSrt := &corev1.Secret{}
	amRouteBYOCertSrt := &corev1.Secret{}
	err1 := runclient.Get(
		context.TODO(),
		types.NamespacedName{Name: config.AlertmanagerRouteBYOCAName, Namespace: config.GetDefaultNamespace()},
		amRouteBYOCaSrt,
	)
	err2 := runclient.Get(
		context.TODO(),
		types.NamespacedName{Name: config.AlertmanagerRouteBYOCERTName, Namespace: config.GetDefaultNamespace()},
		amRouteBYOCertSrt,
	)

	if err1 == nil && err2 == nil {
		log.Info("BYO CA/Certificate found for the Route of Alertmanager, will using BYO CA/certificate for the Route of Alertmanager")
		amRouteCA, ok := amRouteBYOCaSrt.Data["tls.crt"]
		if !ok {
			return &ctrl.Result{}, cerr.New("invalid BYO CA for the Route of Alertmanager")
		}
		amGateway.Spec.TLS.CACertificate = string(amRouteCA)

		amRouteCert, ok := amRouteBYOCertSrt.Data["tls.crt"]
		if !ok {
			return &ctrl.Result{}, cerr.New("invalid BYO Certificate for the Route of Alertmanager")
		}
		amGateway.Spec.TLS.Certificate = string(amRouteCert)

		amRouteCertKey, ok := amRouteBYOCertSrt.Data["tls.key"]
		if !ok {
			return &ctrl.Result{}, cerr.New("invalid BYO Certificate Key for the Route of Alertmanager")
		}
		amGateway.Spec.TLS.Key = string(amRouteCertKey)
	}

	// Set MultiClusterObservability instance as the owner and controller
	if err := controllerutil.SetControllerReference(mco, amGateway, scheme); err != nil {
		return &ctrl.Result{}, err
	}

	found := &routev1.Route{}
	err := runclient.Get(
		context.TODO(),
		types.NamespacedName{Name: amGateway.Name, Namespace: amGateway.Namespace},
		found,
	)
	if err != nil && apierrors.IsNotFound(err) {
		log.Info(
			"Creating a new route to expose alertmanager",
			"amGateway.Namespace",
			amGateway.Namespace,
			"amGateway.Name",
			amGateway.Name,
		)
		err = runclient.Create(context.TODO(), amGateway)
		if err != nil {
			return &ctrl.Result{}, err
		}
		return nil, nil
	}
	if !reflect.DeepEqual(found.Spec.TLS, amGateway.Spec.TLS) {
		log.Info("Found update for the TLS configuration of the Alertmanager Route, try to update the Route")
		amGateway.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
		err = runclient.Update(context.TODO(), amGateway)
		if err != nil {
			return &ctrl.Result{}, err
		}
	}
	return nil, nil
}

// GenerateProxyRoute create route for Proxy endpoint
func GenerateProxyRoute(
	runclient client.Client, scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability,
) (*ctrl.Result, error) {
	proxyGateway := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.ProxyRouteName,
			Namespace: config.GetDefaultNamespace(),
		},
		Spec: routev1.RouteSpec{
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromString("https"),
			},
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: config.ProxyServiceName,
			},
			TLS: &routev1.TLSConfig{
				Termination:                   routev1.TLSTerminationReencrypt,
				InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
			},
		},
	}

	proxyRouteBYOCaSrt := &corev1.Secret{}
	proxyRouteBYOCertSrt := &corev1.Secret{}
	err1 := runclient.Get(
		context.TODO(),
		types.NamespacedName{Name: config.ProxyRouteBYOCAName, Namespace: config.GetDefaultNamespace()},
		proxyRouteBYOCaSrt,
	)
	err2 := runclient.Get(
		context.TODO(),
		types.NamespacedName{Name: config.ProxyRouteBYOCERTName, Namespace: config.GetDefaultNamespace()},
		proxyRouteBYOCertSrt,
	)

	if err1 == nil && err2 == nil {
		log.Info("BYO CA/Certificate found for the Route of Proxy, will using BYO CA/certificate for the Route of Proxy")
		proxyRouteCA, ok := proxyRouteBYOCaSrt.Data["tls.crt"]
		if !ok {
			return &ctrl.Result{}, cerr.New("invalid BYO CA for the Route of Proxy")
		}
		proxyGateway.Spec.TLS.CACertificate = string(proxyRouteCA)

		proxyRouteCert, ok := proxyRouteBYOCertSrt.Data["tls.crt"]
		if !ok {
			return &ctrl.Result{}, cerr.New("invalid BYO Certificate for the Route of Proxy")
		}
		proxyGateway.Spec.TLS.Certificate = string(proxyRouteCert)

		proxyRouteCertKey, ok := proxyRouteBYOCertSrt.Data["tls.key"]
		if !ok {
			return &ctrl.Result{}, cerr.New("invalid BYO Certificate Key for the Route of Proxy")
		}
		proxyGateway.Spec.TLS.Key = string(proxyRouteCertKey)
	}

	// Set MultiClusterObservability instance as the owner and controller
	if err := controllerutil.SetControllerReference(mco, proxyGateway, scheme); err != nil {
		return &ctrl.Result{}, err
	}

	found := &routev1.Route{}
	err := runclient.Get(
		context.TODO(),
		types.NamespacedName{Name: proxyGateway.Name, Namespace: proxyGateway.Namespace},
		found,
	)
	if err != nil && apierrors.IsNotFound(err) {
		log.Info(
			"Creating a new route to expose rbac proxy",
			"proxyGateway.Namespace",
			proxyGateway.Namespace,
			"proxyGateway.Name",
			proxyGateway.Name,
		)
		err = runclient.Create(context.TODO(), proxyGateway)
		if err != nil {
			return &ctrl.Result{}, err
		}
		return nil, nil
	}
	if !reflect.DeepEqual(found.Spec.TLS, proxyGateway.Spec.TLS) {
		log.Info("Found update for the TLS configuration of the Proxy Route, try to update the Route")
		proxyGateway.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
		err = runclient.Update(context.TODO(), proxyGateway)
		if err != nil {
			return &ctrl.Result{}, err
		}
	}
	return nil, nil
}

// cleanUpClusterScopedResources delete the cluster scoped resources created by the MCO operator
// The cluster scoped resources need to be deleted manually because they don't have ownerrefenence set as the MCO CR
func cleanUpClusterScopedResources(
	r *MultiClusterObservabilityReconciler,
	mco *mcov1beta2.MultiClusterObservability,
) error {
	matchLabels := map[string]string{config.GetCrLabelKey(): mco.Name}
	listOpts := []client.ListOption{
		client.MatchingLabels(matchLabels),
	}

	clusterRoleList := &rbacv1.ClusterRoleList{}
	err := r.Client.List(context.TODO(), clusterRoleList, listOpts...)
	if err != nil {
		return err
	}
	for idx := range clusterRoleList.Items {
		err := r.Client.Delete(context.TODO(), &clusterRoleList.Items[idx], &client.DeleteOptions{})
		if err != nil {
			return err
		}
	}

	clusterRoleBindingList := &rbacv1.ClusterRoleBindingList{}
	err = r.Client.List(context.TODO(), clusterRoleBindingList, listOpts...)
	if err != nil {
		return err
	}
	for idx := range clusterRoleBindingList.Items {
		err := r.Client.Delete(context.TODO(), &clusterRoleBindingList.Items[idx], &client.DeleteOptions{})
		if err != nil {
			return err
		}
	}

	ingressCtlCrdExists := r.CRDMap[config.IngressControllerCRD]
	if ingressCtlCrdExists {
		return DeleteGrafanaOauthClient(r.Client)
	}

	return nil
}

func (r *MultiClusterObservabilityReconciler) ensureOpenShiftNamespaceLabel(ctx context.Context,
	m *mcov1beta2.MultiClusterObservability,
) (reconcile.Result, error) {
	log := logf.FromContext(ctx)
	existingNs := &corev1.Namespace{}
	resNS := m.GetNamespace()
	if resNS == "" {
		resNS = config.GetDefaultNamespace()
	}

	err := r.Client.Get(ctx, types.NamespacedName{Name: resNS}, existingNs)
	if err != nil || apierrors.IsNotFound(err) {
		log.Error(err, fmt.Sprintf("Failed to find namespace for Multicluster Operator: %s", resNS))
		return reconcile.Result{Requeue: true}, err
	}

	if len(existingNs.ObjectMeta.Labels) == 0 {
		existingNs.ObjectMeta.Labels = make(map[string]string)
	}

	if _, ok := existingNs.ObjectMeta.Labels[config.OpenShiftClusterMonitoringlabel]; !ok {
		log.Info(fmt.Sprintf("Adding label: %s to namespace: %s", config.OpenShiftClusterMonitoringlabel, resNS))
		existingNs.ObjectMeta.Labels[config.OpenShiftClusterMonitoringlabel] = "true"

		err = r.Client.Update(ctx, existingNs)
		if err != nil {
			log.Error(err, fmt.Sprintf("Failed to update namespace for MultiClusterHub: %s with the label: %s",
				m.GetNamespace(), config.OpenShiftClusterMonitoringlabel))
			return reconcile.Result{Requeue: true}, err
		}
	}

	return reconcile.Result{}, nil
}

func (r *MultiClusterObservabilityReconciler) deleteSpecificPrometheusRule(ctx context.Context) error {
	promRule := &monitoringv1.PrometheusRule{}
	err := r.Client.Get(ctx, client.ObjectKey{
		Name:      "acm-observability-alert-rules",
		Namespace: "openshift-monitoring",
	}, promRule)
	if err == nil {
		err = r.Client.Delete(ctx, promRule)
		if err != nil {
			log.Error(err, "Failed to delete PrometheusRule in openshift-monitoring namespace")
			return err
		}
		log.Info("Deleted PrometheusRule from openshift-monitoring namespace")
	} else if !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to fetch PrometheusRule")
		return err
	}

	return nil
}

func (r *MultiClusterObservabilityReconciler) deleteServiceMonitorInOpenshiftMonitoringNamespace(ctx context.Context) error {
	serviceMonitorList := &monitoringv1.ServiceMonitorList{}
	err := r.Client.List(ctx, serviceMonitorList, client.InNamespace("openshift-monitoring"))
	if !apierrors.IsNotFound(err) && err != nil {
		log.Error(err, "Failed to fetch ServiceMonitors")
		return err
	}

	for _, sm := range serviceMonitorList.Items {
		if strings.HasPrefix(sm.Name, "observability-") {
			err = r.Client.Delete(ctx, sm)
			if err != nil {
				log.Error(err, "Failed to delete ServiceMonitor", "ServiceMonitorName", sm.Name)
				return err
			}
			log.Info("Deleted ServiceMonitor", "ServiceMonitorName", sm.Name)
		}
	}
	return nil
}

func newMCOACRDEventHandler(c client.Client) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(
		func(ctx context.Context, obj client.Object) []reconcile.Request {
			var reqs []reconcile.Request

			var isDependency bool
			if slices0.Contains(config.GetMCOASupportedCRDNames(), obj.GetName()) {
				isDependency = true
			}

			if !isDependency {
				return reqs
			}

			mcos := &mcov1beta2.MultiClusterObservabilityList{}
			err := c.List(ctx, mcos, &client.ListOptions{})
			if err != nil {
				return nil
			}

			for _, mco := range mcos.Items {
				reqs = append(reqs, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      mco.GetName(),
						Namespace: mco.GetNamespace(),
					},
				})
			}

			return reqs
		},
	)
}
