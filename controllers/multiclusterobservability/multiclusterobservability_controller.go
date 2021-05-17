// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package multiclusterobservability

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	ocpClientSet "github.com/openshift/client-go/config/clientset/versioned"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storev1 "k8s.io/api/storage/v1"
	crdClientSet "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta2"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/certificates"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/deploying"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/rendering"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/util"
	observatoriumv1alpha1 "github.com/open-cluster-management/observatorium-operator/api/v1alpha1"
)

const (
	resFinalizer = "observability.open-cluster-management.io/res-cleanup"
	// deprecated one
	certFinalizer = "observability.open-cluster-management.io/cert-cleanup"
)

var (
	log                              = logf.Log.WithName("controller_multiclustermonitoring")
	enableHubRemoteWrite             = os.Getenv("ENABLE_HUB_REMOTEWRITE")
	isAlertmanagerStorageSizeChanged = false
	isCompactStorageSizeChanged      = false
	isRuleStorageSizeChanged         = false
	isReceiveStorageSizeChanged      = false
	isStoreStorageSizeChanged        = false
)

// MultiClusterObservabilityReconciler reconciles a MultiClusterObservability object
type MultiClusterObservabilityReconciler struct {
	Client    client.Client
	Log       logr.Logger
	Scheme    *runtime.Scheme
	OcpClient ocpClientSet.Interface
	CrdClient crdClientSet.Interface
	APIReader client.Reader
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
func (r *MultiClusterObservabilityReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	reqLogger.Info("Reconciling MultiClusterObservability")

	// Fetch the MultiClusterObservability instance
	instance := &mcov1beta2.MultiClusterObservability{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{
		Name: config.GetMonitoringCRName(),
	}, instance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// Init finalizers
	isTerminating, err := r.initFinalization(instance)
	if err != nil {
		return ctrl.Result{}, err
	} else if isTerminating {
		reqLogger.Info("MCO instance is in Terminating status, skip the reconcile")
		return ctrl.Result{}, err
	}

	//read image manifest configmap to be used to replace the image for each component.
	if _, err = config.ReadImageManifestConfigMap(r.Client); err != nil {
		return ctrl.Result{}, err
	}

	// Do not reconcile objects if this instance of mch is labeled "paused"
	if config.IsPaused(instance.GetAnnotations()) {
		reqLogger.Info("MCO reconciliation is paused. Nothing more to do.")
		return ctrl.Result{}, nil
	}

	storageClassSelected, err := getStorageClass(instance, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	// handle storagesize changes
	result, err := r.HandleStorageSizeChange(instance)
	if result != nil {
		return *result, err
	}

	//instance.Namespace = config.GetDefaultNamespace()
	instance.Spec.StorageConfig.StorageClass = storageClassSelected
	//Render the templates with a specified CR
	renderer := rendering.NewRenderer(instance)
	toDeploy, err := renderer.Render(r.Client)
	if err != nil {
		reqLogger.Error(err, "Failed to render multiClusterMonitoring templates")
		return ctrl.Result{}, err
	}
	deployer := deploying.NewDeployer(r.Client)
	//Deploy the resources
	ns := &corev1.Namespace{}
	for _, res := range toDeploy {
		resNS := res.GetNamespace()
		if resNS == config.GetDefaultNamespace() {
			if err := controllerutil.SetControllerReference(instance, res, r.Scheme); err != nil {
				reqLogger.Error(err, "Failed to set controller reference")
			}
		}
		if resNS == "" {
			resNS = config.GetDefaultNamespace()
		}
		if err := r.Client.Get(context.TODO(), types.NamespacedName{Name: resNS}, ns); err != nil && apierrors.IsNotFound(err) {
			ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name: resNS,
			}}
			if err := r.Client.Create(context.TODO(), ns); err != nil {
				reqLogger.Error(err, fmt.Sprintf("Failed to create namespace %s", resNS))
				return ctrl.Result{}, err
			}
		}
		if err := deployer.Deploy(res); err != nil {
			reqLogger.Error(err, fmt.Sprintf("Failed to deploy %s %s/%s",
				res.GetKind(), config.GetDefaultNamespace(), res.GetName()))
			return ctrl.Result{}, err
		}
	}

	// expose alertmanager through route
	result, err = GenerateAlertmanagerRoute(r.Client, r.Scheme, instance)
	if result != nil {
		return *result, err
	}

	// expose observatorium api gateway
	result, err = GenerateAPIGatewayRoute(r.Client, r.Scheme, instance)
	if result != nil {
		return *result, err
	}

	// create the certificates
	err = certificates.CreateObservabilityCerts(r.Client, r.Scheme, instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	// create an Observatorium CR
	result, err = GenerateObservatoriumCR(r.Client, r.Scheme, instance)
	if result != nil {
		return *result, err
	}

	// generate grafana datasource to point to observatorium api gateway
	result, err = GenerateGrafanaDataSource(r.Client, r.Scheme, instance)
	if result != nil {
		return *result, err
	}

	pmCrdExists, err := util.CheckCRDExist(r.CrdClient, config.PlacementRuleCrdName)
	if err != nil {
		return ctrl.Result{}, err
	}

	if pmCrdExists {
		// create the placementrule
		err = createPlacementRule(r.Client, r.Scheme, instance)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	svmCrdExists, err := util.CheckCRDExist(r.CrdClient, config.StorageVersionMigrationCrdName)
	if err != nil {
		return ctrl.Result{}, err
	}

	if svmCrdExists {
		// create or update the storage version migration resource
		err = createOrUpdateObservabilityStorageVersionMigrationResource(r.Client, r.Scheme, instance)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	result, err = r.UpdateStatus(instance)
	if result != nil {
		return *result, err
	}

	return ctrl.Result{}, nil
}

// UpdateStatus override UpdateStatus interface
func (r *MultiClusterObservabilityReconciler) UpdateStatus(
	mco *mcov1beta2.MultiClusterObservability) (*ctrl.Result, error) {
	log.Info("Update MCO status")
	oldStatus := &mco.Status
	newStatus := oldStatus.DeepCopy()
	updateInstallStatus(&newStatus.Conditions)
	updateReadyStatus(&newStatus.Conditions, r.Client, mco)
	updateAddonSpecStatus(&newStatus.Conditions, mco)
	fillupStatus(&newStatus.Conditions)
	mco.Status.Conditions = newStatus.Conditions
	err := r.Client.Status().Update(context.TODO(), mco)
	if err != nil {
		if apierrors.IsConflict(err) {
			// Error from object being modified is normal behavior and should not be treated like an error
			log.Info("Failed to update status", "Reason", "Object has been modified")
			found := &mcov1beta2.MultiClusterObservability{}
			err = r.Client.Get(context.TODO(), types.NamespacedName{
				Name: config.GetMonitoringCRName(),
			}, found)
			if err != nil {
				log.Error(err, fmt.Sprintf("Failed to get existing mco %s", mco.Name))
				return &ctrl.Result{}, err
			}
			mco.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
			err = r.Client.Status().Update(context.TODO(), mco)
			if err != nil {
				log.Error(err, fmt.Sprintf("Failed to update %s status ", mco.Name))
				return &ctrl.Result{}, err
			}
			return &ctrl.Result{Requeue: true, RequeueAfter: time.Second}, nil
		}

		log.Error(err, fmt.Sprintf("Failed to update %s status ", mco.Name))
		return &ctrl.Result{}, err
	}

	if findStatusCondition(newStatus.Conditions, "Ready") == nil {
		return &ctrl.Result{Requeue: true, RequeueAfter: time.Second * 2}, nil
	}

	return nil, nil
}

// labelsForMultiClusterMonitoring returns the labels for selecting the resources
// belonging to the given MultiClusterObservability CR name.
func labelsForMultiClusterMonitoring(name string) map[string]string {
	return map[string]string{"observability.open-cluster-management.io/name": name}
}

func (r *MultiClusterObservabilityReconciler) initFinalization(
	mco *mcov1beta2.MultiClusterObservability) (bool, error) {
	if mco.GetDeletionTimestamp() != nil && util.Contains(mco.GetFinalizers(), resFinalizer) {
		log.Info("To delete resources across namespaces")
		svmCrdExists, err := util.CheckCRDExist(r.CrdClient, config.StorageVersionMigrationCrdName)
		if err != nil {
			return false, err
		}
		if svmCrdExists {
			// remove the StorageVersionMigration resource and ignore error
			cleanObservabilityStorageVersionMigrationResource(r.Client, mco)
		}
		// clean up the cluster resources, eg. clusterrole, clusterrolebinding, etc
		if err = cleanUpClusterScopedResources(r.Client, mco); err != nil {
			log.Error(err, "Failed to remove cluster scoped resources")
			return false, err
		}

		mco.SetFinalizers(util.Remove(mco.GetFinalizers(), resFinalizer))
		err = r.Client.Update(context.TODO(), mco)
		if err != nil {
			log.Error(err, "Failed to remove finalizer from mco resource")
			return false, err
		}
		log.Info("Finalizer removed from mco resource")
		return true, nil
	}
	if !util.Contains(mco.GetFinalizers(), resFinalizer) {
		mco.SetFinalizers(util.Remove(mco.GetFinalizers(), certFinalizer))
		mco.SetFinalizers(append(mco.GetFinalizers(), resFinalizer))
		err := r.Client.Update(context.TODO(), mco)
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
	mcoPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			//set request name to be used in placementrule controller
			config.SetMonitoringCRName(e.Object.GetName())
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			checkStorageChanged(e.ObjectOld.(*mcov1beta2.MultiClusterObservability).Spec.StorageConfig,
				e.ObjectNew.(*mcov1beta2.MultiClusterObservability).Spec.StorageConfig)
			return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return !e.DeleteStateUnknown
		},
	}

	deployPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetNamespace() == config.GetDefaultNamespace() {
				return updateObservatoriumReplicas(e.Object, nil, "Deployment")
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetNamespace() == config.GetDefaultNamespace() {
				return updateObservatoriumReplicas(e.ObjectNew, e.ObjectOld, "Deployment")
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetNamespace() == config.GetDefaultNamespace() {
				return !e.DeleteStateUnknown
			}
			return false
		},
	}

	statefulsetPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetNamespace() == config.GetDefaultNamespace() {
				return updateObservatoriumReplicas(e.Object, nil, "StatefulSet")
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetNamespace() == config.GetDefaultNamespace() {
				return updateObservatoriumReplicas(e.ObjectNew, e.ObjectOld, "StatefulSet")
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetNamespace() == config.GetDefaultNamespace() {
				return !e.DeleteStateUnknown
			}
			return false
		},
	}

	cmPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetName() == config.AlertRuleCustomConfigMapName &&
				e.Object.GetNamespace() == config.GetDefaultNamespace() {
				config.SetCustomRuleConfigMap(true)
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Find a way to restart the alertmanager to take the update
			// if e.ObjectNew.GetName() == config.AlertRuleCustomConfigMapName &&
			// 	e.ObjectNew.GetNamespace() == config.GetDefaultNamespace() {
			// 	config.SetCustomRuleConfigMap(true)
			// 	return e.ObjectOld.GetResourceVersion() != e.ObjectNew.GetResourceVersion()
			// }
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetName() == config.AlertRuleCustomConfigMapName &&
				e.Object.GetNamespace() == config.GetDefaultNamespace() {
				config.SetCustomRuleConfigMap(false)
				return true
			}
			return false
		},
	}

	secretPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetName() == config.AlertmanagerConfigName &&
				e.Object.GetNamespace() == config.GetDefaultNamespace() {
				return true
			}
			return false
		},
	}

	// create a new controller and start watch for relevant resources
	return ctrl.NewControllerManagedBy(mgr).
		// Watch for changes to primary resource MultiClusterObservability with predicate
		For(&mcov1beta2.MultiClusterObservability{}, builder.WithPredicates(mcoPred)).
		// Watch for changes to secondary resource Deployment and requeue the owner Observatorium
		Watches(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(deployPred)).
		// Watch for changes to secondary resource statefulSet and requeue the owner Observatorium
		Watches(&source.Kind{Type: &appsv1.StatefulSet{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(statefulsetPred)).
		// Watch for changes to secondary resource ConfigMap and requeue the owner MultiClusterObservability
		Owns(&corev1.ConfigMap{}).
		// Watch for changes to secondary resource Secret and requeue the owner MultiClusterObservability
		Owns(&corev1.Secret{}).
		// Watch for changes to secondary resource Service and requeue the owner MultiClusterObservability
		Owns(&corev1.Service{}).
		// Watch for changes to secondary Observatorium CR and requeue the owner MultiClusterObservability
		Owns(&observatoriumv1alpha1.Observatorium{}).
		// Watch the configmap for thanos-ruler-custom-rules update
		Watches(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(cmPred)).
		// Watch the secret for deleting event of alertmanager-config
		Watches(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(secretPred)).
		// actually create the controller with the reconciler
		Complete(r)
}

func updateObservatoriumReplicas(objectNew, objectOld client.Object, watchedType string) bool {
	if objectNew.GetNamespace() != config.GetDefaultNamespace() {
		return false
	}
	switch watchedType {
	case "Deployment":
		newReplicas := objectNew.(*appsv1.Deployment).Spec.Replicas
		deployName := objectNew.GetName()
		if objectOld != nil {
			oldReplicas := objectOld.(*appsv1.Deployment).Spec.Replicas
			if newReplicas != oldReplicas {
				if *newReplicas != 0 {
					config.SetObservabilityComponentReplicas(deployName, newReplicas)
				}
			}
			return objectNew.GetResourceVersion() != objectOld.GetResourceVersion()
		} else {
			config.SetObservabilityComponentReplicas(deployName, newReplicas)
			return true
		}
	case "StatefulSet":
		newReplicas := objectNew.(*appsv1.StatefulSet).Spec.Replicas
		stsName := objectNew.GetName()
		if objectOld != nil {
			oldReplicas := objectOld.(*appsv1.StatefulSet).Spec.Replicas
			if newReplicas != oldReplicas {
				if *newReplicas != 0 {
					config.SetObservabilityComponentReplicas(stsName, newReplicas)
				}
			}
			return objectNew.GetResourceVersion() != objectOld.GetResourceVersion()
		} else {
			config.SetObservabilityComponentReplicas(stsName, newReplicas)
			return true
		}
	}
	return false
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
	mco *mcov1beta2.MultiClusterObservability) (*reconcile.Result, error) {

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

	pvcList := []corev1.PersistentVolumeClaim{}
	stsList := []appsv1.StatefulSet{}

	pvcList, err := util.GetPVCList(c, matchLabels)
	if err != nil {
		return err
	}

	stsList, err = util.GetStatefulSetList(c, matchLabels)
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
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
		log.Info("Successfully delete sts due to storage size changed", "sts", sts.Name)
	}
	return nil
}

// GenerateAlertmanagerRoute create route for Alertmanager endpoint
func GenerateAlertmanagerRoute(
	runclient client.Client, scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability) (*ctrl.Result, error) {
	amGateway := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.AlertmanagerRouteName,
			Namespace: config.GetDefaultNamespace(),
		},
		Spec: routev1.RouteSpec{
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

	// Set MultiClusterObservability instance as the owner and controller
	if err := controllerutil.SetControllerReference(mco, amGateway, scheme); err != nil {
		return &ctrl.Result{}, err
	}

	err := runclient.Get(context.TODO(), types.NamespacedName{Name: amGateway.Name, Namespace: amGateway.Namespace}, &routev1.Route{})
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new route to expose alertmanager", "amGateway.Namespace", amGateway.Namespace, "amGateway.Name", amGateway.Name)
		err = runclient.Create(context.TODO(), amGateway)
		if err != nil {
			return &ctrl.Result{}, err
		}
	}
	return nil, nil
}

// cleanUpClusterScopedResources delete the cluster scoped resources created by the MCO operator
// The cluster scoped resources need to be deleted manually because they don't have ownerrefenence set as the MCO CR
func cleanUpClusterScopedResources(cl client.Client, mco *mcov1beta2.MultiClusterObservability) error {
	matchLabels := map[string]string{config.GetCrLabelKey(): mco.Name}
	listOpts := []client.ListOption{
		client.MatchingLabels(matchLabels),
	}

	clusterRoleList := &rbacv1.ClusterRoleList{}
	err := cl.List(context.TODO(), clusterRoleList, listOpts...)
	if err != nil {
		return err
	}
	for _, clusterRoleRes := range clusterRoleList.Items {
		err := cl.Delete(context.TODO(), &clusterRoleRes, &client.DeleteOptions{})
		if err != nil {
			return err
		}
	}

	clusterRoleBindingList := &rbacv1.ClusterRoleBindingList{}
	err = cl.List(context.TODO(), clusterRoleBindingList, listOpts...)
	if err != nil {
		return err
	}
	for _, clusterRoleBindingRes := range clusterRoleBindingList.Items {
		err := cl.Delete(context.TODO(), &clusterRoleBindingRes, &client.DeleteOptions{})
		if err != nil {
			return err
		}
	}

	return nil
}
