// Copyright (c) 2021 Red Hat, Inc.

package multiclusterobservability

import (
	"context"
	"fmt"
	"os"
	"time"

	ocpClientSet "github.com/openshift/client-go/config/clientset/versioned"
	observatoriumv1alpha1 "github.com/stolostron/observatorium-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storv1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	mcov1beta1 "github.com/stolostron/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	"github.com/stolostron/multicluster-monitoring-operator/pkg/config"
	"github.com/stolostron/multicluster-monitoring-operator/pkg/deploying"
	"github.com/stolostron/multicluster-monitoring-operator/pkg/rendering"
	"github.com/stolostron/multicluster-monitoring-operator/pkg/util"
)

const (
	certFinalizer = "observability.open-cluster-management.io/cert-cleanup"
)

var (
	log                  = logf.Log.WithName("controller_multiclustermonitoring")
	enableHubRemoteWrite = os.Getenv("ENABLE_HUB_REMOTEWRITE")
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new MultiClusterObservability Controller and adds it to the Manager. The Manager will set fields on
// the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	// Create OCP client
	ocpClient, err := util.CreateOCPClient()
	if err != nil {
		log.Error(err, "Failed to create the OpenShift client")
		return nil
	}
	return &ReconcileMultiClusterObservability{
		client:    mgr.GetClient(),
		ocpClient: ocpClient,
		apiReader: mgr.GetAPIReader(),
		scheme:    mgr.GetScheme(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("multiclustermonitoring-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	pred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			//set request name to be used in placementrule controller
			config.SetMonitoringCRName(e.Meta.GetName())
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return e.MetaOld.GetGeneration() != e.MetaNew.GetGeneration()
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
			if e.Meta.GetName() == config.AlertRuleCustomConfigMapName &&
				e.Meta.GetNamespace() == config.GetDefaultNamespace() {
				config.SetCustomRuleConfigMap(true)
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Find a way to restart the alertmanager to take the update
			// if e.MetaNew.GetName() == config.AlertRuleCustomConfigMapName &&
			// 	e.MetaNew.GetNamespace() == config.GetDefaultNamespace() {
			// 	config.SetCustomRuleConfigMap(true)
			// 	return e.MetaOld.GetResourceVersion() != e.MetaNew.GetResourceVersion()
			// }
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Meta.GetName() == config.AlertRuleCustomConfigMapName &&
				e.Meta.GetNamespace() == config.GetDefaultNamespace() {
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
			if e.Meta.GetName() == config.AlertmanagerConfigName &&
				e.Meta.GetNamespace() == config.GetDefaultNamespace() {
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

	return nil
}

// blank assignment to verify that ReconcileMultiClusterObservability implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileMultiClusterObservability{}

// ReconcileMultiClusterObservability reconciles a MultiClusterObservability object
type ReconcileMultiClusterObservability struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client    client.Client
	ocpClient ocpClientSet.Interface
	apiReader client.Reader
	scheme    *runtime.Scheme
}

// Reconcile reads that state of the cluster for a MultiClusterObservability object and makes changes
// based on the state read and what is in the MultiClusterObservability.Spec
// Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileMultiClusterObservability) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling MultiClusterObservability")

	// Fetch the MultiClusterObservability instance
	instance := &mcov1beta1.MultiClusterObservability{}
	err := r.client.Get(context.TODO(), types.NamespacedName{
		Name: config.GetMonitoringCRName(),
	}, instance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Init finalizers
	isTerminating, err := r.initFinalization(instance)
	if err != nil {
		return reconcile.Result{}, err
	} else if isTerminating {
		reqLogger.Info("MCO instance is in Terminating status, skip the reconcile")
		return reconcile.Result{}, err
	}
	//read image manifest configmap to be used to replace the image for each component.
	if _, err = config.ReadImageManifestConfigMap(r.client); err != nil {
		return reconcile.Result{}, err
	}

	if result, err := config.GenerateMonitoringCR(r.client, instance); result != nil {
		return *result, err
	}

	// Do not reconcile objects if this instance of mch is labeled "paused"
	if config.IsPaused(instance.GetAnnotations()) {
		reqLogger.Info("MCO reconciliation is paused. Nothing more to do.")
		return reconcile.Result{}, nil
	}

	storageClassSelected, err := getStorageClass(instance, r.client)
	if err != nil {
		return reconcile.Result{}, err
	}

	//set operand names to cover the upgrade case since we have name changed in new release
	err = config.SetOperandNames(r.client)
	if err != nil {
		return reconcile.Result{}, err
	}

	//instance.Namespace = config.GetDefaultNamespace()
	instance.Spec.StorageConfig.StatefulSetStorageClass = storageClassSelected
	//Render the templates with a specified CR
	renderer := rendering.NewRenderer(instance)
	toDeploy, err := renderer.Render(r.client)
	if err != nil {
		reqLogger.Error(err, "Failed to render multiClusterMonitoring templates")
		return reconcile.Result{}, err
	}
	deployer := deploying.NewDeployer(r.client)
	//Deploy the resources
	for _, res := range toDeploy {
		if res.GetNamespace() == config.GetDefaultNamespace() {
			if err := controllerutil.SetControllerReference(instance, res, r.scheme); err != nil {
				reqLogger.Error(err, "Failed to set controller reference")
			}
		}
		if err := deployer.Deploy(res); err != nil {
			reqLogger.Error(err, fmt.Sprintf("Failed to deploy %s %s/%s",
				res.GetKind(), config.GetDefaultNamespace(), res.GetName()))
			return reconcile.Result{}, err
		}
	}

	// expose observatorium api gateway
	result, err := GenerateAPIGatewayRoute(r.client, r.scheme, instance)
	if result != nil {
		return *result, err
	}

	// create the certificates
	err = createObservabilityCertificate(r.client, r.scheme, instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	// create an Observatorium CR
	result, err = GenerateObservatoriumCR(r.client, r.scheme, instance)
	if result != nil {
		return *result, err
	}

	// generate grafana datasource to point to observatorium api gateway
	result, err = GenerateGrafanaDataSource(r.client, r.scheme, instance)
	if result != nil {
		return *result, err
	}

	enableManagedCluster, found := os.LookupEnv("ENABLE_MANAGED_CLUSTER")
	if !found || enableManagedCluster != "false" {
		// create the placementrule
		err = createPlacementRule(r.client, r.scheme, instance)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	result, err = r.UpdateStatus(instance)
	if result != nil {
		return *result, err
	}

	return reconcile.Result{}, nil
}

// UpdateStatus override UpdateStatus interface
func (r *ReconcileMultiClusterObservability) UpdateStatus(
	mco *mcov1beta1.MultiClusterObservability) (*reconcile.Result, error) {
	log.Info("Update MCO status")
	oldStatus := &mco.Status
	newStatus := oldStatus.DeepCopy()
	updateInstallStatus(&newStatus.Conditions)
	updateReadyStatus(&newStatus.Conditions, r.client, mco)
	updateAddonSpecStatus(&newStatus.Conditions, mco)
	fillupStatus(&newStatus.Conditions)
	mco.Status.Conditions = newStatus.Conditions
	err := r.client.Status().Update(context.TODO(), mco)
	if err != nil {
		if apierrors.IsConflict(err) {
			// Error from object being modified is normal behavior and should not be treated like an error
			log.Info("Failed to update status", "Reason", "Object has been modified")
			found := &mcov1beta1.MultiClusterObservability{}
			err = r.client.Get(context.TODO(), types.NamespacedName{
				Name: config.GetMonitoringCRName(),
			}, found)
			if err != nil {
				log.Error(err, fmt.Sprintf("Failed to get existing mco %s", mco.Name))
				return &reconcile.Result{}, err
			}
			mco.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
			err = r.client.Status().Update(context.TODO(), mco)
			if err != nil {
				log.Error(err, fmt.Sprintf("Failed to update %s status ", mco.Name))
				return &reconcile.Result{}, err
			}
			return &reconcile.Result{Requeue: true, RequeueAfter: time.Second}, nil
		}

		log.Error(err, fmt.Sprintf("Failed to update %s status ", mco.Name))
		return &reconcile.Result{}, err
	}

	if findStatusCondition(newStatus.Conditions, "Ready") == nil {
		return &reconcile.Result{Requeue: true, RequeueAfter: time.Second * 2}, nil
	}

	return nil, nil
}

func (r *ReconcileMultiClusterObservability) initFinalization(
	mco *mcov1beta1.MultiClusterObservability) (bool, error) {
	if mco.GetDeletionTimestamp() != nil && util.Contains(mco.GetFinalizers(), certFinalizer) {
		log.Info("To delete issuer/certificate across namespaces")
		err := cleanIssuerCert(r.client)
		if err != nil {
			return false, err
		}
		mco.SetFinalizers(util.Remove(mco.GetFinalizers(), certFinalizer))
		err = r.client.Update(context.TODO(), mco)
		if err != nil {
			log.Error(err, "Failed to remove finalizer from mco resource")
			return false, err
		}
		log.Info("Finalizer removed from mco resource")
		return true, nil
	}
	if !util.Contains(mco.GetFinalizers(), certFinalizer) {
		mco.SetFinalizers(append(mco.GetFinalizers(), certFinalizer))
		err := r.client.Update(context.TODO(), mco)
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
