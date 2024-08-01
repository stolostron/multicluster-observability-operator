// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package observabilityendpoint

import (
	"context"
	"fmt"
	"os"
	"strconv"

	operatorutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/util"

	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/rendering"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/util"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	oav1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/deploying"
	rendererutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering"
)

var (
	log                  = ctrl.Log.WithName("controllers").WithName("ObservabilityAddon")
	installPrometheus, _ = strconv.ParseBool(os.Getenv(operatorconfig.InstallPrometheus))
	globalRes            = []*unstructured.Unstructured{}
)

const (
	obAddonName                     = "observability-addon"
	ownerLabelKey                   = "owner"
	ownerLabelValue                 = "observabilityaddon"
	obsAddonFinalizer               = "observability.open-cluster-management.io/addon-cleanup"
	promSvcName                     = "prometheus-k8s"
	promNamespace                   = "openshift-monitoring"
	openShiftClusterMonitoringlabel = "openshift.io/cluster-monitoring"
)

const (
	defaultClusterType  = ""
	ocpThreeClusterType = "ocp3"
	snoClusterType      = "SNO"
)

var (
	namespace             = os.Getenv("WATCH_NAMESPACE")
	hubNamespace          = os.Getenv("HUB_NAMESPACE")
	isHubMetricsCollector = os.Getenv("HUB_ENDPOINT_OPERATOR") == "true"
)

// ObservabilityAddonReconciler reconciles a ObservabilityAddon object.
type ObservabilityAddonReconciler struct {
	Client    client.Client
	Scheme    *runtime.Scheme
	HubClient *util.ReloadableHubClient
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
	hubObsAddon := &oav1beta1.ObservabilityAddon{}
	obsAddon := &oav1beta1.ObservabilityAddon{}
	deleteFlag := false

	// ACM 8509: Special case for hub/local cluster metrics collection
	// We do not have an ObservabilityAddon instance in the local cluster so skipping the below block
	if !isHubMetricsCollector {
		if err := r.ensureOpenShiftMonitoringLabelAndRole(ctx); err != nil {
			return ctrl.Result{}, err
		}

		// Fetch the ObservabilityAddon instance in hub cluster
		fetchAddon := func() error {
			return r.HubClient.Get(ctx, types.NamespacedName{Name: obAddonName, Namespace: hubNamespace}, hubObsAddon)
		}
		if err := fetchAddon(); err != nil {
			if r.HubClient, err = r.HubClient.Reload(); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reload the hub client: %w", err)
			}

			// Retry the operation once with the reloaded client
			if err := fetchAddon(); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to get ObservabilityAddon in hub cluster: %w", err)
			}
		}

		// Fetch the ObservabilityAddon instance in local cluster
		err := r.Client.Get(ctx, types.NamespacedName{Name: obAddonName, Namespace: namespace}, obsAddon)
		if err != nil {
			if errors.IsNotFound(err) {
				obsAddon = nil
			} else {
				log.Error(err, "Failed to get observabilityaddon", "namespace", namespace)
				return ctrl.Result{}, err
			}
		}

		if obsAddon == nil {
			deleteFlag = true
		}
		// Init finalizers
		deleted, err := r.initFinalization(ctx, deleteFlag, hubObsAddon, isHypershift)
		if err != nil {
			return ctrl.Result{}, err
		}
		if deleted || deleteFlag {
			return ctrl.Result{}, nil
		}
	}

	// retrieve the hubInfo
	hubSecret := &corev1.Secret{}
	err := r.Client.Get(
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

	clusterType := defaultClusterType
	clusterID := ""

	// read the image configmap
	imagesCM := &corev1.ConfigMap{}
	err = r.Client.Get(ctx, types.NamespacedName{
		Name:      operatorconfig.ImageConfigMap,
		Namespace: namespace,
	}, imagesCM)
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
		err = r.Client.Get(ctx, types.NamespacedName{
			Name:      promSvcName,
			Namespace: promNamespace,
		}, promSvc)
		if err != nil {
			if errors.IsNotFound(err) {
				log.Error(err, "OCP prometheus service does not exist")
				// ACM 8509: Special case for hub/local cluster metrics collection
				// We do not report status for hub endpoint operator
				if !isHubMetricsCollector {
					if err := util.ReportStatus(ctx, r.Client, util.NotSupported, obsAddon.Name, obsAddon.Namespace); err != nil {
						log.Error(err, "Failed to report status")
					}
				}

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
			clusterType = ocpThreeClusterType
		}
		isSNO, err := isSNO(ctx, r.Client)
		if err == nil && isSNO {
			clusterType = snoClusterType
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
		// Render the prometheus templates
		renderer := rendererutil.NewRenderer()
		toDeploy, err := rendering.Render(renderer, r.Client, hubInfo)
		if err != nil {
			log.Error(err, "Failed to render prometheus templates")
			return ctrl.Result{}, err
		}
		deployer := deploying.NewDeployer(r.Client)
		for _, res := range toDeploy {
			if res.GetNamespace() != namespace {
				globalRes = append(globalRes, res)
			}

			if !isHubMetricsCollector {
				// For kind tests we need to deploy prometheus in hub but cannot set controller
				// reference as there is no observabilityaddon

				// skip setting controller reference for resources that don't need it
				// and for which we lack permission to set it
				skipResources := []string{"Role", "RoleBinding", "ClusterRole", "ClusterRoleBinding"}
				if !slices.Contains(skipResources, res.GetKind()) {
					if err := controllerutil.SetControllerReference(obsAddon, res, r.Scheme); err != nil {
						log.Info("Failed to set controller reference", "resource", res.GetName(), "kind", res.GetKind(), "error", err.Error())
					}
				}
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

	forceRestart := req.Name == mtlsCertName || req.Name == mtlsCaName || req.Name == caConfigmapName

	if obsAddon.Spec.EnableMetrics || isHubMetricsCollector {
		if isHubMetricsCollector {
			mcoList := &oav1beta2.MultiClusterObservabilityList{}
			err := r.HubClient.List(ctx, mcoList, client.InNamespace(corev1.NamespaceAll))
			if err != nil {
				log.Error(err, "Failed to get multiclusterobservability")
				return ctrl.Result{}, err
			}
			if len(mcoList.Items) != 1 {
				log.Error(nil, fmt.Sprintf("Expected 1 multiclusterobservability, found %d", len(mcoList.Items)))
				return ctrl.Result{}, nil
			}
			obsAddon.Spec = *mcoList.Items[0].Spec.ObservabilityAddonSpec
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
			if !isHubMetricsCollector {
				if err := util.ReportStatus(ctx, r.Client, util.Degraded, obsAddon.Name, obsAddon.Namespace); err != nil {
					log.Error(err, "Failed to report status")
				}
			}
			return ctrl.Result{}, fmt.Errorf("failed to update metrics collectors: %w", err)
		}
		if created && !isHubMetricsCollector {
			if err := util.ReportStatus(ctx, r.Client, util.Deployed, obsAddon.Name, obsAddon.Namespace); err != nil {
				log.Error(err, "Failed to report status")
			}
		}
	} else {
		deleted, err := updateMetricsCollectors(ctx, r.Client, obsAddon.Spec, *hubInfo, clusterID, clusterType, 0, false)
		if err != nil {
			return ctrl.Result{}, err
		}
		if deleted && !isHubMetricsCollector {
			if err := util.ReportStatus(ctx, r.Client, util.Disabled, obsAddon.Name, obsAddon.Namespace); err != nil {
				log.Error(err, "Failed to report status")
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *ObservabilityAddonReconciler) initFinalization(
	ctx context.Context, delete bool, hubObsAddon *oav1beta1.ObservabilityAddon,
	isHypershift bool,
) (bool, error) {
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

func (r *ObservabilityAddonReconciler) ensureOpenShiftMonitoringLabelAndRole(ctx context.Context) error {
	existingNs := &corev1.Namespace{}
	resNS := namespace

	role := rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prometheus-k8s-addon-obs",
			Namespace: resNS,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"services", "endpoints", "pods"},
				Verbs:     []string{"get", "list", "watch"},
			},
		},
	}

	roleBinding := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prometheus-k8s-addon-obs",
			Namespace: resNS,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "prometheus-k8s-addon-obs",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "prometheus-k8s",
				Namespace: "openshift-monitoring",
			},
		},
	}

	err := r.Client.Get(ctx, types.NamespacedName{Name: resNS}, existingNs)
	if err != nil || errors.IsNotFound(err) {
		log.Error(err, fmt.Sprintf("Failed to find namespace for Endpoint Operator: %s", resNS))
		return err
	}

	if existingNs.ObjectMeta.Labels == nil || len(existingNs.ObjectMeta.Labels) == 0 {
		existingNs.ObjectMeta.Labels = make(map[string]string)
	}

	if _, ok := existingNs.ObjectMeta.Labels[openShiftClusterMonitoringlabel]; !ok {
		log.Info(fmt.Sprintf("Adding label: %s to namespace: %s", openShiftClusterMonitoringlabel, resNS))
		existingNs.ObjectMeta.Labels[openShiftClusterMonitoringlabel] = "true"

		err = r.Client.Update(ctx, existingNs)
		if err != nil {
			log.Error(err, fmt.Sprintf("Failed to update namespace for Endpoint Operator: %s with the label: %s",
				namespace, openShiftClusterMonitoringlabel))
			return err
		}
	}

	foundRole := &rbacv1.Role{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: role.Name, Namespace: resNS}, foundRole)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info(fmt.Sprintf("Creating role: %s in namespace: %s", role.Name, resNS))
			err = r.Client.Create(ctx, &role)
			if err != nil {
				log.Error(err, fmt.Sprintf("Failed to create role: %s in namespace: %s", role.Name, resNS))
				return err
			}
		} else {
			log.Error(err, fmt.Sprintf("Failed to get role: %s in namespace: %s", role.Name, resNS))
			return err
		}
	}

	foundRoleBinding := &rbacv1.RoleBinding{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: roleBinding.Name, Namespace: resNS}, foundRoleBinding)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info(fmt.Sprintf("Creating role binding: %s in namespace: %s", roleBinding.Name, resNS))
			err = r.Client.Create(ctx, &roleBinding)
			if err != nil {
				log.Error(err, fmt.Sprintf("Failed to create role binding: %s in namespace: %s", roleBinding.Name, resNS))
				return err
			}
		} else {
			log.Error(err, fmt.Sprintf("Failed to get role binding: %s in namespace: %s", roleBinding.Name, resNS))
			return err
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ObservabilityAddonReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if os.Getenv("NAMESPACE") != "" {
		namespace = os.Getenv("NAMESPACE")
	}

	ctrlBuilder := ctrl.NewControllerManagedBy(mgr).For(
		&oav1beta1.ObservabilityAddon{},
		builder.WithPredicates(getPred(obAddonName, namespace, true, true, true)),
	)

	if isHubMetricsCollector {
		ctrlBuilder = ctrlBuilder.Watches(
			&source.Kind{Type: &oav1beta2.MultiClusterObservability{}},
			&handler.EnqueueRequestForObject{},
		)
	}

	return ctrlBuilder.
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
		// Watch the kube-system extension-apiserver-authentication ConfigMap for changes
		Watches(&source.Kind{Type: &corev1.ConfigMap{}}, handler.EnqueueRequestsFromMapFunc(
			func(a client.Object) []reconcile.Request {
				if a.GetName() == "extension-apiserver-authentication" && a.GetNamespace() == "kube-system" {
					return []reconcile.Request{
						{NamespacedName: types.NamespacedName{
							Name:      "metrics-collector-clientca-metric",
							Namespace: namespace,
						}},
						{NamespacedName: types.NamespacedName{
							Name:      "uwl-metrics-collector-clientca-metric",
							Namespace: namespace,
						}},
					}
				}
				return nil
			}), builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
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
