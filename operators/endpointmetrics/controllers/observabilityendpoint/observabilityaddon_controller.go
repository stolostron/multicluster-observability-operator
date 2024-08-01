// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package observabilityendpoint

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/rendering"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/util"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/deploying"
	rendererutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering"
	operatorutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
)

var (
	log                  = ctrl.Log.WithName("controllers").WithName("ObservabilityAddon")
	installPrometheus, _ = strconv.ParseBool(os.Getenv(operatorconfig.InstallPrometheus))
	globalRes            = []*unstructured.Unstructured{}
)

const (
	obAddonName       = "observability-addon"
	mcoCRName         = "observability"
	ownerLabelKey     = "owner"
	ownerLabelValue   = "observabilityaddon"
	obsAddonFinalizer = "observability.open-cluster-management.io/addon-cleanup"
	promSvcName       = "prometheus-k8s"
	promNamespace     = "openshift-monitoring"
)

var (
	namespace    = os.Getenv("WATCH_NAMESPACE")
	hubNamespace = os.Getenv("HUB_NAMESPACE")
)

// ObservabilityAddonReconciler reconciles a ObservabilityAddon object.
type ObservabilityAddonReconciler struct {
	Client    client.Client
	Scheme    *runtime.Scheme
	HubClient client.Client
}

// +kubebuilder:rbac:groups=observability.open-cluster-management.io.open-cluster-management.io,resources=observabilityaddons,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=observability.open-cluster-management.io.open-cluster-management.io,resources=observabilityaddons/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=observability.open-cluster-management.io.open-cluster-management.io,resources=observabilityaddons/finalizers,verbs=update

// Reconcile reads that state of the cluster for a ObservabilityAddon object and makes changes based on the state read
// and what is in the ObservabilityAddon.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ObservabilityAddonReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	log.Info("Reconciling")

	// Fetch the ObservabilityAddon instance in hub cluster
	hubObsAddon := &oav1beta1.ObservabilityAddon{}
	err := r.HubClient.Get(ctx, types.NamespacedName{Name: obAddonName, Namespace: hubNamespace}, hubObsAddon)
	if err != nil {
		hubClient, obsAddon, err := util.RenewAndRetry(ctx)
		if err != nil {
			return ctrl.Result{}, err
		}
		r.HubClient = hubClient
		hubObsAddon = obsAddon
	}

	// Fetch the ObservabilityAddon instance in local cluster
	obsAddon := &oav1beta1.ObservabilityAddon{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: obAddonName, Namespace: namespace}, obsAddon)
	if err != nil {
		if errors.IsNotFound(err) {
			obsAddon = nil
		} else {
			log.Error(err, "Failed to get observabilityaddon", "namespace", namespace)
			return ctrl.Result{}, err
		}
	}

	// Init finalizers
	deleteFlag := false
	if obsAddon == nil {
		deleteFlag = true
	}
	isHypershift := true
	if os.Getenv("UNIT_TEST") != "true" {
		crdClient, err := operatorutil.GetOrCreateCRDClient()
		if err != nil {
			return ctrl.Result{}, err
		}
		isHypershift, err = operatorutil.CheckCRDExist(crdClient, "hostedclusters.hypershift.openshift.io")
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	deleted, err := r.initFinalization(ctx, deleteFlag, hubObsAddon, isHypershift)
	if err != nil {
		return ctrl.Result{}, err
	}
	if deleted || deleteFlag {
		return ctrl.Result{}, nil
	}

	// retrieve the hubInfo
	hubSecret := &corev1.Secret{}
	err = r.Client.Get(
		ctx,
		types.NamespacedName{Name: operatorconfig.HubInfoSecretName, Namespace: namespace},
		hubSecret,
	)
	if err != nil {
		return ctrl.Result{}, err
	}
	hubInfo := &operatorconfig.HubInfo{}
	err = yaml.Unmarshal(hubSecret.Data[operatorconfig.HubInfoSecretKey], &hubInfo)
	if err != nil {
		log.Error(err, "Failed to unmarshal hub info")
		return ctrl.Result{}, err
	}
	hubInfo.ClusterName = string(hubSecret.Data[operatorconfig.ClusterNameKey])

	clusterType := ""
	clusterID := ""

	//read the image configmap
	imagesCM := &corev1.ConfigMap{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: operatorconfig.ImageConfigMap,
		Namespace: namespace}, imagesCM)
	if err != nil {
		log.Error(err, "Failed to get images configmap")
		return ctrl.Result{}, err
	}
	rendering.Images = imagesCM.Data

	if isHypershift {
		err = createServiceMonitors(ctx, r.Client)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if !installPrometheus {
		// If no prometheus service found, set status as NotSupported
		promSvc := &corev1.Service{}
		err = r.Client.Get(ctx, types.NamespacedName{Name: promSvcName,
			Namespace: promNamespace}, promSvc)
		if err != nil {
			if errors.IsNotFound(err) {
				log.Error(err, "OCP prometheus service does not exist")
				util.ReportStatus(ctx, r.Client, obsAddon, "NotSupported")
				return ctrl.Result{}, nil
			}
			log.Error(err, "Failed to check prometheus resource")
			return ctrl.Result{}, err
		}

		clusterID, err = getClusterID(ctx, r.Client)
		if err != nil {
			// OCP 3.11 has no cluster id, set it as empty string
			clusterID = ""
			// to differentiate ocp 3.x
			clusterType = "ocp3"
		}
		isSNO, err := isSNO(ctx, r.Client)
		if err == nil && isSNO {
			clusterType = "SNO"
		}
		err = createMonitoringClusterRoleBinding(ctx, r.Client)
		if err != nil {
			return ctrl.Result{}, err
		}
		err = createCAConfigmap(ctx, r.Client)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		//Render the prometheus templates
		renderer := rendererutil.NewRenderer()
		toDeploy, err := rendering.Render(renderer, r.Client, hubInfo)
		if err != nil {
			log.Error(err, "Failed to render prometheus templates")
			return ctrl.Result{}, err
		}
		deployer := deploying.NewDeployer(r.Client)
		for _, res := range toDeploy {
			if err := controllerutil.SetControllerReference(obsAddon, res, r.Scheme); err != nil {
				log.Info("Failed to set controller reference", "resource", res.GetName())
				globalRes = append(globalRes, res)
			}
			if err := deployer.Deploy(res); err != nil {
				log.Error(err, fmt.Sprintf("Failed to deploy %s %s/%s",
					res.GetKind(), namespace, res.GetName()))
				return ctrl.Result{}, err
			}
		}
	}

	// create or update the cluster-monitoring-config configmap and relevant resources
	if err := createOrUpdateClusterMonitoringConfig(ctx, hubInfo, clusterID, r.Client, installPrometheus); err != nil {
		return ctrl.Result{}, err
	}

	if obsAddon.Spec.EnableMetrics {
		forceRestart := false
		if req.Name == mtlsCertName || req.Name == mtlsCaName || req.Name == caConfigmapName {
			forceRestart = true
		}
		created, err := updateMetricsCollectors(
			ctx,
			r.Client,
			obsAddon.Spec,
			*hubInfo, clusterID,
			clusterType,
			1,
			forceRestart)

		if err != nil {
			util.ReportStatus(ctx, r.Client, obsAddon, "Degraded")
			return ctrl.Result{}, err
		}
		if created {
			util.ReportStatus(ctx, r.Client, obsAddon, "Deployed")
		}
	} else {
		deleted, err := updateMetricsCollectors(ctx, r.Client, obsAddon.Spec, *hubInfo, clusterID, clusterType, 0, false)
		if err != nil {
			return ctrl.Result{}, err
		}
		if deleted {
			util.ReportStatus(ctx, r.Client, obsAddon, "Disabled")
		}
	}

	//TODO: UPDATE
	return ctrl.Result{}, nil
}

func (r *ObservabilityAddonReconciler) initFinalization(
	ctx context.Context, delete bool, hubObsAddon *oav1beta1.ObservabilityAddon,
	isHypershift bool) (bool, error) {
	if delete && slices.Contains(hubObsAddon.GetFinalizers(), obsAddonFinalizer) {
		log.Info("To clean observability components/configurations in the cluster")
		err := deleteMetricsCollector(ctx, r.Client, metricsCollectorName)
		if err != nil {
			return false, err
		}
		err = deleteMetricsCollector(ctx, r.Client, uwlMetricsCollectorName)
		if err != nil {
			return false, err
		}
		// revert the change to cluster monitoring stack
		err = revertClusterMonitoringConfig(ctx, r.Client)
		if err != nil {
			return false, err
		}
		if isHypershift {
			err = deleteServiceMonitors(ctx, r.Client)
			if err != nil {
				return false, err
			}
		}
		// Should we return bool from the delete functions for crb and cm? What
		// is it used for? Should we use the bool before removing finalizer?
		// SHould we return true if metricscollector is not found as that means
		// metrics collector is not present? Moved this part up as we need to clean
		// up cm and crb before we remove the finalizer - is that the right way to do it?
		if !installPrometheus {
			err = deleteMonitoringClusterRoleBinding(ctx, r.Client)
			if err != nil {
				return false, err
			}
			err = deleteCAConfigmap(ctx, r.Client)
			if err != nil {
				return false, err
			}
		} else {
			// delete resources which is not namespace scoped or located in other namespaces
			for _, res := range globalRes {
				err = r.Client.Delete(context.TODO(), res)
				if err != nil && !errors.IsNotFound(err) {
					return false, err
				}
			}
		}
		hubObsAddon.SetFinalizers(remove(hubObsAddon.GetFinalizers(), obsAddonFinalizer))
		err = r.HubClient.Update(ctx, hubObsAddon)
		if err != nil {
			log.Error(err, "Failed to remove finalizer to observabilityaddon", "namespace", hubObsAddon.Namespace)
			return false, err
		}
		log.Info("Finalizer removed from observabilityaddon resource")
		return true, nil
	}
	if !slices.Contains(hubObsAddon.GetFinalizers(), obsAddonFinalizer) {
		hubObsAddon.SetFinalizers(append(hubObsAddon.GetFinalizers(), obsAddonFinalizer))
		err := r.HubClient.Update(ctx, hubObsAddon)
		if err != nil {
			log.Error(err, "Failed to add finalizer to observabilityaddon", "namespace", hubObsAddon.Namespace)
			return false, err
		}
		log.Info("Finalizer added to observabilityaddon resource")
	}
	return false, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ObservabilityAddonReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if os.Getenv("NAMESPACE") != "" {
		namespace = os.Getenv("NAMESPACE")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(
			&oav1beta1.ObservabilityAddon{},
			builder.WithPredicates(getPred(obAddonName, namespace, true, true, true)),
		).
		Watches(
			&source.Kind{Type: &corev1.Secret{}},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(operatorconfig.HubInfoSecretName, namespace, true, true, false)),
		).
		Watches(
			&source.Kind{Type: &corev1.Secret{}},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(mtlsCertName, namespace, true, true, false)),
		).
		Watches(
			&source.Kind{Type: &corev1.Secret{}},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(mtlsCaName, namespace, true, true, false)),
		).
		Watches(
			&source.Kind{Type: &corev1.Secret{}},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(hubAmAccessorSecretName, namespace, true, true, false)),
		).
		Watches(
			&source.Kind{Type: &corev1.ConfigMap{}},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(operatorconfig.AllowlistConfigMapName, namespace, true, true, false)),
		).
		Watches(
			&source.Kind{Type: &corev1.ConfigMap{}},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(operatorconfig.AllowlistCustomConfigMapName, "", true, true, true)),
		).
		Watches(
			&source.Kind{Type: &corev1.ConfigMap{}},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(caConfigmapName, namespace, false, true, true)),
		).
		Watches(
			&source.Kind{Type: &corev1.ConfigMap{}},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(clusterMonitoringConfigName, promNamespace, false, true, true)),
		).
		Watches(
			&source.Kind{Type: &appsv1.Deployment{}},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(metricsCollectorName, namespace, true, true, true)),
		).
		Watches(
			&source.Kind{Type: &appsv1.Deployment{}},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(uwlMetricsCollectorName, namespace, true, true, true)),
		).
		Watches(
			&source.Kind{Type: &rbacv1.ClusterRoleBinding{}},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(clusterRoleBindingName, "", false, true, true)),
		).
		Watches(
			&source.Kind{Type: &corev1.ConfigMap{}},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(operatorconfig.ImageConfigMap, namespace, true, true, false)),
		).
		Watches(
			&source.Kind{Type: &appsv1.StatefulSet{}},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(operatorconfig.PrometheusUserWorkload, uwlNamespace, true, false, true)),
		).
		Complete(r)
}

func remove(list []string, s string) []string {
	result := []string{}
	for _, v := range list {
		if v != s {
			result = append(result, v)
		}
	}
	return result
}
