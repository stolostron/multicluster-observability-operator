// Copyright (c) 2020 Red Hat, Inc.

package multiclusterobservability

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	observatoriumv1alpha1 "github.com/observatorium/operator/api/v1alpha1"
	ocpClientSet "github.com/openshift/client-go/config/clientset/versioned"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/deploying"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/rendering"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/util"
)

const (
	certFinalizer   = "observability.open-cluster-management.io/cert-cleanup"
	Installing      = "Installing"
	Failed          = "Failed"
	Ready           = "Ready"
	MetricsDisabled = "Disabled"
)

var (
	log                             = logf.Log.WithName("controller_multiclustermonitoring")
	enableHubRemoteWrite            = os.Getenv("ENABLE_HUB_REMOTEWRITE")
	isClusterManagementAddonCreated = false
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

	// Watch for changes to primary resource MultiClusterObservability
	err = c.Watch(&source.Kind{Type: &mcov1beta1.MultiClusterObservability{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Deployment and requeue the owner MultiClusterObservability
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &mcov1beta1.MultiClusterObservability{},
	})

	// Watch for changes to secondary resource statefulSet and requeue the owner MultiClusterObservability
	err = c.Watch(&source.Kind{Type: &appsv1.StatefulSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &mcov1beta1.MultiClusterObservability{},
	})

	// Watch for changes to secondary resource ConfigMap and requeue the owner MultiClusterObservability
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &mcov1beta1.MultiClusterObservability{},
	})

	// Watch for changes to secondary resource Secret and requeue the owner MultiClusterObservability
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &mcov1beta1.MultiClusterObservability{},
	})

	// Watch for changes to secondary resource Service and requeue the owner MultiClusterObservability
	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &mcov1beta1.MultiClusterObservability{},
	})

	// Watch for changes to secondary Observatorium CR and requeue the owner MultiClusterObservability
	err = c.Watch(&source.Kind{Type: &observatoriumv1alpha1.Observatorium{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &mcov1beta1.MultiClusterObservability{},
	})

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

	//set request name to be used in placementrule controller
	config.SetMonitoringCRName(request.Name)

	//Check if ClusterManagementAddon is created or create it
	if !isClusterManagementAddonCreated {
		err := util.CreateClusterManagementAddon(r.client)
		if err != nil {
			return reconcile.Result{}, err
		}
		isClusterManagementAddonCreated = true
	}
	// Fetch the MultiClusterObservability instance
	instance := &mcov1beta1.MultiClusterObservability{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
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

	if result, err := config.GenerateMonitoringCR(r.client, instance); result != nil {
		return *result, err
	}

	//set configured image repo and image tag from annotations
	config.SetAnnotationImageInfo(instance.GetAnnotations())

	// Do not reconcile objects if this instance of mch is labeled "paused"
	if config.IsPaused(instance.GetAnnotations()) {
		reqLogger.Info("MCO reconciliation is paused. Nothing more to do.")
		return reconcile.Result{}, nil
	}

	instance.Namespace = config.GetDefaultNamespace()
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
		if res.GetNamespace() == instance.Namespace {
			if err := controllerutil.SetControllerReference(instance, res, r.scheme); err != nil {
				reqLogger.Error(err, "Failed to set controller reference")
			}
		}
		if err := deployer.Deploy(res); err != nil {
			reqLogger.Error(err, fmt.Sprintf("Failed to deploy %s %s/%s", res.GetKind(), instance.Namespace, res.GetName()))
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

	// create the placementrule
	err = createPlacementRule(r.client, r.scheme, instance)
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

	result, err = r.UpdateStatus(instance)
	if result != nil {
		return *result, err
	}

	return reconcile.Result{}, nil
}

// UpdateStatus override UpdateStatus interface
func (r *ReconcileMultiClusterObservability) UpdateStatus(
	mco *mcov1beta1.MultiClusterObservability) (*reconcile.Result, error) {

	reqLogger := log.WithValues("Request.Namespace", mco.Namespace, "Request.Name", mco.Name)
	requeue := false

	deployList := &appsv1.DeploymentList{}
	deploymentListOpts := []client.ListOption{
		client.InNamespace(mco.Namespace),
		client.MatchingLabels(labelsForMultiClusterMonitoring(mco.Name)),
	}

	err := r.client.List(context.TODO(), deployList, deploymentListOpts...)
	if err != nil {
		reqLogger.Error(err, "Failed to list deployments.",
			"MultiClusterObservability.Namespace", mco.Namespace,
			"MemcaMultiClusterMonitoringched.Name", mco.Name,
		)
		return &reconcile.Result{}, err
	}

	installingCondition := CheckInstallStatus(r.client, mco)
	conditions := []mcov1beta1.MCOCondition{}
	allDeploymentReady := true
	failedDeployment := ""
	for _, deployment := range deployList.Items {
		if deployment.Status.ReadyReplicas < 1 || deployment.Status.AvailableReplicas < 1 {
			allDeploymentReady = false
			failedDeployment = deployment.Name
			break
		}
	}

	// validate s3 conf
	s3condition := CheckS3Conf(r.client, mco)
	if s3condition != nil {
		conditions = append(conditions, *s3condition)
		requeue = true
	} else {
		if installingCondition.Type != "Ready" {
			conditions = append(conditions, installingCondition)
			requeue = true
		} else if len(deployList.Items) == 0 {
			conditions = append(conditions, mcov1beta1.MCOCondition{
				Type:    Failed,
				Reason:  Failed,
				Message: "No deployment found.",
			})
			requeue = true
		} else {
			if allDeploymentReady {
				ready := mcov1beta1.MCOCondition{}
				requeue = false
				if mco.Spec.ObservabilityAddonSpec != nil && mco.Spec.ObservabilityAddonSpec.EnableMetrics == false {
					ready = mcov1beta1.MCOCondition{
						Type:    Ready,
						Reason:  Ready,
						Message: "Observability components are deployed and running",
					}
					conditions = append(conditions, ready)
					enableMetrics := mcov1beta1.MCOCondition{
						Type:    MetricsDisabled,
						Reason:  MetricsDisabled,
						Message: "Collect metrics from the managed clusters is disabled",
					}
					conditions = append(conditions, enableMetrics)
				} else {
					ready = mcov1beta1.MCOCondition{
						Type:    Ready,
						Reason:  Ready,
						Message: "Observability components are deployed and running",
					}
					conditions = append(conditions, ready)
				}

			} else {
				failedMessage := fmt.Sprintf("Deployment is failed for %s", failedDeployment)
				failed := mcov1beta1.MCOCondition{
					Type:    Failed,
					Reason:  Failed,
					Message: failedMessage,
				}
				conditions = append(conditions, failed)
				requeue = true
			}
		}
	}

	mco.Status.Conditions = conditions
	err = r.client.Status().Update(context.TODO(), mco)
	if err != nil {
		if apierrors.IsConflict(err) {
			// Error from object being modified is normal behavior and should not be treated like an error
			log.Info("Failed to update status", "Reason", "Object has been modified")
			return &reconcile.Result{RequeueAfter: time.Second}, nil
		}

		log.Error(err, fmt.Sprintf("Failed to update %s/%s status ", mco.Namespace, mco.Name))
		return &reconcile.Result{}, err
	}
	if requeue {
		return &reconcile.Result{RequeueAfter: time.Second * 2}, nil
	}
	return nil, nil
}

// labelsForMultiClusterMonitoring returns the labels for selecting the resources
// belonging to the given MultiClusterObservability CR name.
func labelsForMultiClusterMonitoring(name string) map[string]string {
	return map[string]string{"observability.open-cluster-management.io/name": name}
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
			log.Error(err, "Failed to remove finalizer from mco resource", "namespace", mco.Namespace)
			return false, err
		}
		log.Info("Finalizer removed from mco resource")
		return true, nil
	}
	if !util.Contains(mco.GetFinalizers(), certFinalizer) {
		mco.SetFinalizers(append(mco.GetFinalizers(), certFinalizer))
		err := r.client.Update(context.TODO(), mco)
		if err != nil {
			log.Error(err, "Failed to add finalizer to mco resource", "namespace", mco.Namespace)
			return false, err
		}
		log.Info("Finalizer added to mco resource")
	}
	return false, nil
}

func getResourceConditions(podList *corev1.PodList, statefulSetList *appsv1.StatefulSetList,
	watchingPods []string, watchingStatefulSets []string,
	allPodsReady *bool, allstatefulSetReady *bool,
	podCounter *int, statefulSetCounter *int,
) {
	for _, pod := range podList.Items {
		for _, name := range watchingPods {
			if strings.HasPrefix(pod.Name, name) {
				*podCounter++
				singlePodReady := false
				for _, podCondition := range pod.Status.Conditions {
					if podCondition.Type == Ready {
						singlePodReady = true
					}
				}
				if singlePodReady == false {
					*allPodsReady = false
				}
			}
		}
	}
	for _, statefulSet := range statefulSetList.Items {
		for _, name := range watchingStatefulSets {
			if strings.HasPrefix(statefulSet.Name, name) {
				*statefulSetCounter++
				singleStatefulSetReady := false
				if statefulSet.Status.ReadyReplicas >= 1 {
					singleStatefulSetReady = true
				}
				if singleStatefulSetReady == false {
					*allstatefulSetReady = false
				}
			}
		}
	}
	return
}

func CheckInstallStatus(c client.Client,
	mco *mcov1beta1.MultiClusterObservability) mcov1beta1.MCOCondition {

	installingCondition := mcov1beta1.MCOCondition{}
	if len(mco.Status.Conditions) == 0 {
		installingCondition = mcov1beta1.MCOCondition{
			Type:    Installing,
			Reason:  Installing,
			Message: "Installation is in progress",
		}
	} else if mco.Status.Conditions[0].Type == Installing {
		installingCondition = mco.Status.Conditions[0]
		watchingPods := []string{
			strings.Join([]string{mco.ObjectMeta.Name, "observatorium-observatorium-api"}, "-"),
			strings.Join([]string{mco.ObjectMeta.Name, "observatorium-thanos-query"}, "-"),
			strings.Join([]string{mco.ObjectMeta.Name, "observatorium-thanos-receive-controller"}, "-"),
			"grafana",
		}
		podList := &corev1.PodList{}
		podListOpts := []client.ListOption{
			client.InNamespace(mco.Namespace),
		}
		err := c.List(context.TODO(), podList, podListOpts...)
		if err != nil {
			log.Error(err, "Failed to list pods.",
				"MultiClusterObservability.Namespace", mco.Namespace,
			)
			return installingCondition
		}
		podCounter := 0
		allPodsReady := true
		watchingStatefulSets := []string{
			strings.Join([]string{mco.ObjectMeta.Name, "observatorium-thanos-compact"}, "-"),
			strings.Join([]string{mco.ObjectMeta.Name, "observatorium-thanos-receive-default"}, "-"),
			strings.Join([]string{mco.ObjectMeta.Name, "observatorium-thanos-rule"}, "-"),
			strings.Join([]string{mco.ObjectMeta.Name, "observatorium-thanos-store-memcached"}, "-"),
			strings.Join([]string{mco.ObjectMeta.Name, "observatorium-thanos-store-shard-0"}, "-"),
		}
		statefulSetList := &appsv1.StatefulSetList{}
		statefulSetListOpts := []client.ListOption{
			client.InNamespace(mco.Namespace),
		}
		err = c.List(context.TODO(), statefulSetList, statefulSetListOpts...)
		if err != nil {
			log.Error(err, "Failed to list statefulSets.",
				"MultiClusterObservability.Namespace", mco.Namespace,
			)
			return installingCondition
		}
		statefulSetCounter := 0
		allstatefulSetReady := true
		getResourceConditions(podList, statefulSetList, watchingPods, watchingStatefulSets,
			&allPodsReady, &allstatefulSetReady, &podCounter, &statefulSetCounter)

		if podCounter == 0 || statefulSetCounter == 0 || allPodsReady != true ||
			podCounter < len(watchingPods) || allstatefulSetReady != true || statefulSetCounter < len(watchingStatefulSets) {
			installingCondition = mcov1beta1.MCOCondition{
				Type:    Installing,
				Reason:  Installing,
				Message: "Installation is still in process",
			}
		} else {
			installingCondition = mcov1beta1.MCOCondition{
				Type: Ready,
			}
		}
	} else {
		installingCondition = mcov1beta1.MCOCondition{
			Type: Ready,
		}
	}

	return installingCondition
}

func CheckS3Conf(c client.Client,
	mco *mcov1beta1.MultiClusterObservability) *mcov1beta1.MCOCondition {
	objStorageConf := mco.Spec.StorageConfig.MetricObjectStorage
	secret := &corev1.Secret{}
	namespacedName := types.NamespacedName{
		Name:      objStorageConf.Name,
		Namespace: mco.Namespace,
	}

	failed := mcov1beta1.MCOCondition{
		Type:   Failed,
		Reason: Failed,
	}

	err := c.Get(context.TODO(), namespacedName, secret)
	if err != nil {
		failed.Message = err.Error()
		return &failed
	}

	data, ok := secret.Data[objStorageConf.Key]
	if !ok {
		failed.Message = "Failed to found the object storage configuration"
		return &failed
	}

	ok, err = config.IsValidS3Conf(data)
	if !ok {
		failed.Message = err.Error()
		return &failed
	}

	return nil
}
