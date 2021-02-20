/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-logr/logr"
	ocpClientSet "github.com/openshift/client-go/config/clientset/versioned"
	storv1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/api/v1beta1"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/deploying"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/rendering"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/util"
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
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MultiClusterObservability object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.0/pkg/reconcile
func (r *MultiClusterObservabilityReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("multiclusterobservability", req.NamespacedName)
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
	mco *mcov1beta1.MultiClusterObservability) (*reconcile.Result, error) {
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
				return &reconcile.Result{}, err
			}
			mco.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
			err = r.Client.Status().Update(context.TODO(), mco)
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

	return ctrl.NewControllerManagedBy(mgr).
		For(&mcov1beta1.MultiClusterObservability{}).
		Complete(r)
}
