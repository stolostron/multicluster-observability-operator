// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package multiclusterobservability

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-logr/logr"
	ocpClientSet "github.com/openshift/client-go/config/clientset/versioned"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storv1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	mcov1beta1 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta1"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/deploying"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/rendering"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/util"
	observatoriumv1alpha1 "github.com/open-cluster-management/observatorium-operator/api/v1alpha1"
)

const (
	certFinalizer = "observability.open-cluster-management.io/cert-cleanup"
)

var (
	log                  = logf.Log.WithName("controller_multiclustermonitoring")
	enableHubRemoteWrite = os.Getenv("ENABLE_HUB_REMOTEWRITE")
)

// MultiClusterObservabilityReconciler reconciles a MultiClusterObservability object
type MultiClusterObservabilityReconciler struct {
	Client    client.Client
	Log       logr.Logger
	Scheme    *runtime.Scheme
	OcpClient ocpClientSet.Interface
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
	instance := &mcov1beta1.MultiClusterObservability{}
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

	//instance.Namespace = config.GetDefaultNamespace()
	instance.Spec.StorageConfig.StatefulSetStorageClass = storageClassSelected
	//Render the templates with a specified CR
	renderer := rendering.NewRenderer(instance)
	toDeploy, err := renderer.Render(r.Client)
	if err != nil {
		reqLogger.Error(err, "Failed to render multiClusterMonitoring templates")
		return ctrl.Result{}, err
	}
	deployer := deploying.NewDeployer(r.Client)
	//Deploy the resources
	for _, res := range toDeploy {
		if res.GetNamespace() == config.GetDefaultNamespace() {
			if err := controllerutil.SetControllerReference(instance, res, r.Scheme); err != nil {
				reqLogger.Error(err, "Failed to set controller reference")
			}
		}
		if err := deployer.Deploy(res); err != nil {
			reqLogger.Error(err, fmt.Sprintf("Failed to deploy %s %s/%s",
				res.GetKind(), config.GetDefaultNamespace(), res.GetName()))
			return ctrl.Result{}, err
		}
	}

	// expose observatorium api gateway
	result, err := GenerateAPIGatewayRoute(r.Client, r.Scheme, instance)
	if result != nil {
		return *result, err
	}

	// create the certificates
	err = createObservabilityCertificate(r.Client, r.Scheme, instance)
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

	enableManagedCluster, found := os.LookupEnv("ENABLE_MANAGED_CLUSTER")
	if !found || enableManagedCluster != "false" {
		// create the placementrule
		err = createPlacementRule(r.Client, r.Scheme, instance)
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
	mco *mcov1beta1.MultiClusterObservability) (*ctrl.Result, error) {
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
			found := &mcov1beta1.MultiClusterObservability{}
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
	mco *mcov1beta1.MultiClusterObservability) (bool, error) {
	if mco.GetDeletionTimestamp() != nil && util.Contains(mco.GetFinalizers(), certFinalizer) {
		log.Info("To delete issuer/certificate across namespaces")
		err := cleanIssuerCert(r.Client)
		if err != nil {
			return false, err
		}
		mco.SetFinalizers(util.Remove(mco.GetFinalizers(), certFinalizer))
		err = r.Client.Update(context.TODO(), mco)
		if err != nil {
			log.Error(err, "Failed to remove finalizer from mco resource")
			return false, err
		}
		log.Info("Finalizer removed from mco resource")
		return true, nil
	}
	if !util.Contains(mco.GetFinalizers(), certFinalizer) {
		mco.SetFinalizers(append(mco.GetFinalizers(), certFinalizer))
		err := r.Client.Update(context.TODO(), mco)
		if err != nil {
			log.Error(err, "Failed to add finalizer to mco resource")
			return false, err
		}
		log.Info("Finalizer added to mco resource")
	}
	return false, nil
}

func getStorageClass(mco *mcov1beta1.MultiClusterObservability, cl client.Client) (string, error) {
	storageClassSelected := mco.Spec.StorageConfig.StatefulSetStorageClass
	// for the test, the reader is just nil
	storageClassList := &storv1.StorageClassList{}
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
	// Create a new controller
	c, err := controller.New("multiclustermonitoring-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	pred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			//set request name to be used in placementrule controller
			config.SetMonitoringCRName(e.Object.GetName())
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return !e.DeleteStateUnknown
		},
	}
	// Watch for changes to primary resource MultiClusterObservability
	err = c.Watch(&source.Kind{Type: &mcov1beta1.MultiClusterObservability{}}, &handler.EnqueueRequestForObject{}, pred)
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Deployment and requeue the owner MultiClusterObservability
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &mcov1beta1.MultiClusterObservability{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource statefulSet and requeue the owner MultiClusterObservability
	err = c.Watch(&source.Kind{Type: &appsv1.StatefulSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &mcov1beta1.MultiClusterObservability{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource ConfigMap and requeue the owner MultiClusterObservability
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &mcov1beta1.MultiClusterObservability{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Secret and requeue the owner MultiClusterObservability
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &mcov1beta1.MultiClusterObservability{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Service and requeue the owner MultiClusterObservability
	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &mcov1beta1.MultiClusterObservability{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary Observatorium CR and requeue the owner MultiClusterObservability
	err = c.Watch(&source.Kind{Type: &observatoriumv1alpha1.Observatorium{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &mcov1beta1.MultiClusterObservability{},
	})
	if err != nil {
		return err
	}

	pred = predicate.Funcs{
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
	// Watch the configmap
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForObject{}, pred)
	if err != nil {
		return err
	}

	pred = predicate.Funcs{
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
	// Watch the Secret
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForObject{}, pred)
	if err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&mcov1beta1.MultiClusterObservability{}).
		Complete(r)
}
