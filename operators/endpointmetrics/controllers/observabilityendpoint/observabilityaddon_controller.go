// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package observabilityendpoint

import (
	"context"
	"fmt"
	"os"

	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
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

	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/collector"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/hypershift"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/openshift"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/rendering"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/util"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	oav1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/deploying"
	rendererutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/status"
)

var (
	log       = ctrl.Log.WithName("controllers").WithName("ObservabilityAddon")
	globalRes = []*unstructured.Unstructured{}
)

const (
	obAddonName                     = "observability-addon"
	obsAddonFinalizer               = "observability.open-cluster-management.io/addon-cleanup"
	openShiftClusterMonitoringlabel = "openshift.io/cluster-monitoring"
	mtlsCertName                    = "observability-controller-open-cluster-management.io-observability-signer-client-cert"
	mtlsCaName                      = "observability-managed-cluster-certs"
	metricsCollectorName            = "metrics-collector-deployment"
	uwlMetricsCollectorName         = "uwl-metrics-collector-deployment"
	uwlNamespace                    = "openshift-user-workload-monitoring"
)
const (
	promSvcName   = operatorconfig.OCPClusterMonitoringPrometheusService
	promNamespace = operatorconfig.OCPClusterMonitoringNamespace
)

// ObservabilityAddonReconciler reconciles a ObservabilityAddon object.
type ObservabilityAddonReconciler struct {
	Client                client.Client
	Scheme                *runtime.Scheme
	HubClient             *util.ReloadableHubClient
	IsHubMetricsCollector bool
	Namespace             string
	HubNamespace          string
	ServiceAccountName    string
	InstallPrometheus     bool
}

// +kubebuilder:rbac:groups=observability.open-cluster-management.io.open-cluster-management.io,resources=observabilityaddons,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=observability.open-cluster-management.io.open-cluster-management.io,resources=observabilityaddons/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=observability.open-cluster-management.io.open-cluster-management.io,resources=observabilityaddons/finalizers,verbs=update

// Reconcile reads that state of the cluster for a ObservabilityAddon object and makes changes based on the state read
// and what is in the ObservabilityAddon.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ObservabilityAddonReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.Info("Reconciling", "Request", req.String())

	isHypershift := true
	if os.Getenv("UNIT_TEST") != "true" {
		var err error
		isHypershift, err = hypershift.IsHypershiftCluster()
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to check if the cluster is hypershift: %w", err)
		}
	}

	hubObsAddon := &oav1beta1.ObservabilityAddon{}
	obsAddon := &oav1beta1.ObservabilityAddon{}
	deleteFlag := false

	// ACM 8509: Special case for hub/local cluster metrics collection
	// We do not have an ObservabilityAddon instance in the local cluster so skipping the below block
	if !r.IsHubMetricsCollector {
		if err := r.ensureOpenShiftMonitoringLabelAndRole(ctx); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to ensure OpenShift monitoring label and role: %w", err)
		}

		// Fetch the ObservabilityAddon instance in hub cluster
		fetchAddon := func() error {
			return r.HubClient.Get(ctx, types.NamespacedName{Name: obAddonName, Namespace: r.HubNamespace}, hubObsAddon)
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
		err := r.Client.Get(ctx, types.NamespacedName{Name: obAddonName, Namespace: r.Namespace}, obsAddon)
		if err != nil {
			if !errors.IsNotFound(err) {
				return ctrl.Result{}, fmt.Errorf("failed to get observabilityaddon: %w", err)
			}
			obsAddon = nil
		}

		if obsAddon == nil {
			deleteFlag = true
		}
		// Init finalizers
		deleted, err := r.initFinalization(ctx, deleteFlag, hubObsAddon, isHypershift)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to init finalization: %w", err)
		}
		if deleted || deleteFlag {
			return ctrl.Result{}, nil
		}
	}

	// retrieve the hubInfo
	hubSecret := &corev1.Secret{}
	err := r.Client.Get(
		ctx,
		types.NamespacedName{Name: operatorconfig.HubInfoSecretName, Namespace: r.Namespace},
		hubSecret,
	)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get hub info secret %s/%s: %w", r.Namespace, operatorconfig.HubInfoSecretName, err)
	}
	hubInfo := &operatorconfig.HubInfo{}
	err = yaml.Unmarshal(hubSecret.Data[operatorconfig.HubInfoSecretKey], &hubInfo)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to unmarshal hub info: %w", err)
	}
	hubInfo.ClusterName = string(hubSecret.Data[operatorconfig.ClusterNameKey])

	clusterType := operatorconfig.DefaultClusterType
	clusterID := ""

	// read the image configmap
	imagesCM := &corev1.ConfigMap{}
	err = r.Client.Get(ctx, types.NamespacedName{
		Name:      operatorconfig.ImageConfigMap,
		Namespace: r.Namespace,
	}, imagesCM)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get images configmap: %w", err)
	}
	rendering.Images = imagesCM.Data

	if r.IsHubMetricsCollector && isHypershift {
		updatedHCs, err := hypershift.ReconcileHostedClustersServiceMonitors(ctx, r.Client)
		if err != nil {
			log.Error(err, "Failed to create ServiceMonitors for hypershift")
		} else {
			log.Info("Reconciled hypershift service monitors", "updatedHCs", updatedHCs)
		}
	}

	if !r.InstallPrometheus {
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
				if !r.IsHubMetricsCollector {
					statusReporter := status.NewStatus(r.Client, obsAddon.Name, obsAddon.Namespace, log)
					if wasReported, err := statusReporter.UpdateComponentCondition(ctx, status.MetricsCollector, status.NotSupported, "Prometheus service not found"); err != nil {
						log.Error(err, "Failed to report status")
					} else if wasReported {
						log.Info("Status updated", "component", status.MetricsCollector, "reason", status.NotSupported)
					}
				}

				return ctrl.Result{}, nil
			}
			return ctrl.Result{}, fmt.Errorf("failed to check prometheus resource: %w", err)
		}

		clusterID, err = openshift.GetClusterID(ctx, r.Client)
		if err != nil {
			if meta.IsNoMatchError(err) {
				// ClusterVersion kind does not exist in OCP 3.x
				log.Info("ClusterVersion kind does not exist, treat spoke as OCP 3.x", "error", err)
			} else if errors.IsNotFound(err) {
				// If no ClusterVersion found, treat it as OCP 3.x (should not happen)
				log.Info("Cluster id not found, treat spoke as OCP 3.x", "error", err)
			} else {
				return ctrl.Result{}, fmt.Errorf("failed to get cluster id: %w", err)
			}

			// OCP 3.11 has no cluster id, set it as empty string
			clusterID = ""
			// to differentiate ocp 3.x
			clusterType = operatorconfig.OcpThreeClusterType
		}

		if isSNO, err := openshift.IsSNO(ctx, r.Client); err != nil {
			log.Error(err, "Failed to check if the cluster is SNO")
		} else if isSNO {
			clusterType = operatorconfig.SnoClusterType
		}

		err = openshift.CreateMonitoringClusterRoleBinding(ctx, log, r.Client, r.Namespace, r.ServiceAccountName)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create monitoring cluster role binding: %w", err)
		}
		err = openshift.CreateCAConfigmap(ctx, r.Client, r.Namespace)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create CA configmap: %w", err)
		}
	} else {
		// Render the prometheus templates
		renderer := rendererutil.NewRenderer()
		toDeploy, err := rendering.Render(ctx, renderer, r.Client, hubInfo, r.Namespace)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to render prometheus templates: %w", err)
		}

		deployer := deploying.NewDeployer(r.Client)

		// Ordering resources to ensure they are applied in the correct order
		slices.SortFunc(toDeploy, func(a, b *unstructured.Unstructured) int {
			return resourcePriority(a) - resourcePriority(b)
		})

		for _, res := range toDeploy {
			if res.GetNamespace() != r.Namespace {
				globalRes = append(globalRes, res)
			}

			if !r.IsHubMetricsCollector {
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

			if err := deployer.Deploy(ctx, res); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to deploy %s %s/%s: %w", res.GetKind(), r.Namespace, res.GetName(), err)
			}
		}
	}

	// create or update the cluster-monitoring-config configmap and relevant resources
	if err := createOrUpdateClusterMonitoringConfig(ctx, hubInfo, clusterID, r.Client, r.InstallPrometheus, r.Namespace); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create or update cluster monitoring config: %w", err)
	}

	if r.IsHubMetricsCollector {
		mcoList := &oav1beta2.MultiClusterObservabilityList{}
		err := r.HubClient.List(ctx, mcoList, client.InNamespace(corev1.NamespaceAll))
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get multiclusterobservability: %w", err)
		}
		if len(mcoList.Items) != 1 {
			log.Error(nil, fmt.Sprintf("Expected 1 multiclusterobservability, found %d", len(mcoList.Items)))
			return ctrl.Result{}, nil
		}
		obsAddon.Spec = *mcoList.Items[0].Spec.ObservabilityAddonSpec
	}

	metricsCollector := collector.MetricsCollector{
		Client: r.Client,
		ClusterInfo: collector.ClusterInfo{
			ClusterID:             clusterID,
			ClusterType:           clusterType,
			InstallPrometheus:     r.InstallPrometheus,
			IsHubMetricsCollector: r.IsHubMetricsCollector,
		},
		HubInfo:            hubInfo,
		Log:                log.WithName("metrics-collector"),
		Namespace:          r.Namespace,
		ObsAddon:           obsAddon,
		ServiceAccountName: r.ServiceAccountName,
	}

	if err := metricsCollector.Update(ctx, req); err != nil {
		wrappedErr := fmt.Errorf("failed to update metrics collector: %w", err)
		if errors.IsConflict(err) || util.IsTransientClientErr(err) {
			log.Info("Retrying due to conflict or transient client error")
			return ctrl.Result{Requeue: true}, wrappedErr
		}
		return ctrl.Result{}, wrappedErr
	}

	return ctrl.Result{}, nil
}

func (r *ObservabilityAddonReconciler) initFinalization(
	ctx context.Context, delete bool, hubObsAddon *oav1beta1.ObservabilityAddon,
	isHypershift bool,
) (bool, error) {
	if delete && slices.Contains(hubObsAddon.GetFinalizers(), obsAddonFinalizer) {
		log.Info("To clean observability components/configurations in the cluster")

		metricsCollector := collector.MetricsCollector{
			Client:    r.Client,
			Log:       log.WithName("metrics-collector"),
			Namespace: r.Namespace,
		}
		if err := metricsCollector.Delete(ctx); err != nil {
			return false, fmt.Errorf("failed to delete metrics collector: %w", err)
		}

		// revert the change to cluster monitoring stack
		err := RevertClusterMonitoringConfig(ctx, r.Client)
		if err != nil {
			return false, err
		}
		if isHypershift {
			err = hypershift.DeleteServiceMonitors(ctx, r.Client)
			if err != nil {
				log.Error(err, "Failed to delete ServiceMonitors for hypershift")
				return false, err
			}
		}
		// Should we return bool from the delete functions for crb and cm? What
		// is it used for? Should we use the bool before removing finalizer?
		// SHould we return true if metricscollector is not found as that means
		// metrics collector is not present? Moved this part up as we need to clean
		// up cm and crb before we remove the finalizer - is that the right way to do it?
		if !r.InstallPrometheus {
			err = openshift.DeleteMonitoringClusterRoleBinding(ctx, r.Client)
			if err != nil {
				log.Error(err, "Failed to delete monitoring cluster role binding")
				return false, err
			}
			log.Info("clusterrolebinding deleted")
			err = openshift.DeleteCAConfigmap(ctx, r.Client, r.Namespace)
			if err != nil {
				log.Error(err, "Failed to delete CA configmap")
				return false, err
			}
			log.Info("configmap deleted")
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
	resNS := r.Namespace

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
				r.Namespace, openShiftClusterMonitoringlabel))
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
	ctrlBuilder := ctrl.NewControllerManagedBy(mgr).For(
		&oav1beta1.ObservabilityAddon{},
		builder.WithPredicates(getPred(obAddonName, r.Namespace, true, true, true)),
	)

	if r.IsHubMetricsCollector {
		ctrlBuilder = ctrlBuilder.Watches(
			&oav1beta2.MultiClusterObservability{},
			&handler.EnqueueRequestForObject{},
		)
	}

	return ctrlBuilder.
		Watches(
			&corev1.Secret{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(operatorconfig.HubInfoSecretName, r.Namespace, true, true, false)),
		).
		Watches(
			&corev1.Secret{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(mtlsCertName, r.Namespace, true, true, false)),
		).
		Watches(
			&corev1.Secret{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(mtlsCaName, r.Namespace, true, true, false)),
		).
		Watches(
			&corev1.Secret{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(hubAmAccessorSecretName, r.Namespace, true, true, false)),
		).
		Watches(
			&corev1.ConfigMap{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(operatorconfig.AllowlistConfigMapName, r.Namespace, true, true, false)),
		).
		Watches(
			&corev1.ConfigMap{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(operatorconfig.AllowlistCustomConfigMapName, "", true, true, true)),
		).
		Watches(
			&corev1.ConfigMap{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(openshift.CaConfigmapName, r.Namespace, false, true, true)),
		).
		Watches(
			&corev1.ConfigMap{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(operatorconfig.ImageConfigMap, r.Namespace, true, true, false)),
		).
		Watches(
			&corev1.ConfigMap{},
			enqueueForAPIServerAuth(r.Namespace),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&corev1.ConfigMap{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(configMapDataChangedPredicate(clusterMonitoringConfigName, promNamespace)),
		).
		Watches(
			&appsv1.Deployment{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(metricsCollectorName, r.Namespace, true, true, true)),
		).
		Watches(
			&appsv1.Deployment{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(uwlMetricsCollectorName, r.Namespace, true, true, true)),
		).
		Watches(
			&appsv1.StatefulSet{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(operatorconfig.PrometheusUserWorkload, uwlNamespace, true, false, true)),
		).
		Watches(
			&rbacv1.ClusterRoleBinding{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(getPred(openshift.ClusterRoleBindingName, "", false, true, true)),
		).
		Complete(r)
}

// Watch the kube-system extension-apiserver-authentication ConfigMap for changes
func enqueueForAPIServerAuth(namespace string) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(
		func(ctx context.Context, a client.Object) []reconcile.Request {
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
		})
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

// resourcePriority returns the priority of the resource.
// This is used to order the resources to be created in the correct order.
func resourcePriority(resource *unstructured.Unstructured) int {
	switch resource.GetKind() {
	case "Role", "ClusterRole":
		return 1
	case "RoleBinding", "ClusterRoleBinding":
		return 2
	case "CustomResourceDefinition":
		return 3
	default:
		return 4
	}
}
