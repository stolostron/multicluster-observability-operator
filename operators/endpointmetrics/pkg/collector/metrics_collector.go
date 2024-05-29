// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package collector

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/openshift"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/rendering"
	oashared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
)

const (
	metricsCollectorName    = "metrics-collector-deployment"
	uwlMetricsCollectorName = "uwl-metrics-collector-deployment"
	metricsCollector        = "metrics-collector"
	uwlMetricsCollector     = "uwl-metrics-collector"
	selectorKey             = "component"
	selectorValue           = metricsCollector
	caMounthPath            = "/etc/serving-certs-ca-bundle"
	caVolName               = "serving-certs-ca-bundle"
	mtlsCertName            = "observability-controller-open-cluster-management.io-observability-signer-client-cert"
	mtlsCaName              = "observability-managed-cluster-certs"
	mtlsServerCaName        = "observability-server-ca-certs"
	limitBytes              = 1073741824
	defaultInterval         = "30s"
	uwlNamespace            = "openshift-user-workload-monitoring"
	uwlSts                  = "prometheus-user-workload"
)

const (
	restartLabel    = "cert/time-restarted"
	ownerLabelKey   = "owner"
	ownerLabelValue = "observabilityaddon"
)

var (
	ocpPromURL  = "https://prometheus-k8s.openshift-monitoring.svc:9091"
	uwlPromURL  = "https://prometheus-user-workload.openshift-user-workload-monitoring.svc:9092"
	uwlQueryURL = "https://thanos-querier.openshift-monitoring.svc:9091"
	promURL     = "https://prometheus-k8s:9091"
)

type ClusterInfo struct {
	ClusterID             string
	ClusterType           string
	InstallPrometheus     bool
	IsHubMetricsCollector bool
}

type MetricsCollector struct {
	Client             client.Client
	ClusterInfo        ClusterInfo
	HubInfo            *operatorconfig.HubInfo
	Log                logr.Logger
	Namespace          string
	ObsAddonSpec       *oashared.ObservabilityAddonSpec
	ServiceAccountName string
}

type proxyConfig struct {
	caBundle   string
	httpProxy  string
	httpsProxy string
	noProxy    string
}

type DeploymentParams struct {
	allowlist    *operatorconfig.MetricsAllowlist
	forceRestart bool
	nodeSelector map[string]string
	proxyConfig  proxyConfig
	tolerations  []corev1.Toleration
	uwlList      *operatorconfig.MetricsAllowlist
}

func (m *MetricsCollector) Update(ctx context.Context, req ctrl.Request) error {
	deployParams, err := m.generateDeployParams(ctx, req)
	if err != nil {
		return err
	}

	if err := m.updateMetricsCollector(ctx, false, deployParams); err != nil {
		return err
	}

	isUwl, err := m.isUWLMonitoringEnabled(ctx)
	if err != nil {
		return err
	}

	if isUwl && len(deployParams.uwlList.NameList) != 0 {
		if err := m.updateMetricsCollector(ctx, true, deployParams); err != nil {
			return err
		}
	} else {
		if err := m.deleteMetricsCollector(ctx, true); err != nil {
			return err
		}
	}

	return nil
}

func (m *MetricsCollector) generateDeployParams(ctx context.Context, req ctrl.Request) (*DeploymentParams, error) {
	list, uwlList, err := m.getMetricsAllowlist(ctx)
	if err != nil {
		return nil, err
	}

	endpointDeployment, err := m.getEndpointDeployment(ctx)
	if err != nil {
		return nil, err
	}

	deployParams := &DeploymentParams{
		allowlist:    list,
		forceRestart: req.Name == mtlsCertName || req.Name == mtlsCaName || req.Name == openshift.CaConfigmapName,
		nodeSelector: endpointDeployment.Spec.Template.Spec.NodeSelector,
		tolerations:  endpointDeployment.Spec.Template.Spec.Tolerations,
		uwlList:      uwlList,
	}

	// stash away proxy settings from endpoint deployment
	for _, container := range endpointDeployment.Spec.Template.Spec.Containers {
		if container.Name == "endpoint-observability-operator" {
			for _, env := range container.Env {
				switch env.Name {
				case "HTTP_PROXY":
					deployParams.proxyConfig.httpProxy = env.Value
				case "HTTPS_PROXY":
					deployParams.proxyConfig.httpsProxy = env.Value
				case "NO_PROXY":
					deployParams.proxyConfig.noProxy = env.Value
				case "HTTPS_PROXY_CA_BUNDLE":
					deployParams.proxyConfig.caBundle = env.Value
				}
			}
		}
	}

	return deployParams, nil
}

func (m *MetricsCollector) deleteMetricsCollector(ctx context.Context, isUWL bool) error {
	deployName := metricsCollectorName
	name := metricsCollector
	if isUWL {
		deployName = uwlMetricsCollectorName
		name = uwlMetricsCollector
	}

	foundDeploy := &appsv1.Deployment{}
	err := m.Client.Get(ctx, types.NamespacedName{Name: deployName, Namespace: m.Namespace}, foundDeploy)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get deployment %s/%s: %w", m.Namespace, deployName, err)
		}
	} else {
		m.Log.Info("Deleting metrics collector deployment", "name", deployName, "namespace", m.Namespace)
		err = m.Client.Delete(ctx, foundDeploy)
		if err != nil {
			return fmt.Errorf("failed to delete deployment %s/%s: %w", m.Namespace, deployName, err)
		}
	}

	foundSM := &monitoringv1.ServiceMonitor{}
	if err := m.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: m.Namespace}, foundSM); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get servicemonitor %s/%s: %w", m.Namespace, name, err)
		}
	} else {
		m.Log.Info("Deleting metrics collector servicemonitor", "name", name, "namespace", m.Namespace)
		if err := m.Client.Delete(ctx, foundSM); err != nil {
			return fmt.Errorf("failed to delete servicemonitor %s/%s: %w", m.Namespace, name, err)
		}
	}

	foundAlerts := &monitoringv1.PrometheusRule{}
	promRulesName := "acm-" + name + "-alerting-rules"
	if err := m.Client.Get(ctx, types.NamespacedName{Name: promRulesName, Namespace: m.Namespace}, foundAlerts); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get alerting rules %s/%s: %w", m.Namespace, promRulesName, err)
		}
	} else {
		m.Log.Info("Deleting metrics collector alerting rules", "name", promRulesName, "namespace", m.Namespace)
		if err := m.Client.Delete(ctx, foundAlerts); err != nil {
			return fmt.Errorf("failed to delete alerting rules %s/%s: %w", m.Namespace, promRulesName, err)
		}
	}

	foundService := &corev1.Service{}
	if err := m.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: m.Namespace}, foundService); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get service %s/%s: %w", m.Namespace, name, err)
		}
	} else {
		m.Log.Info("Deleting metrics collector service", "name", name, "namespace", m.Namespace)
		if err := m.Client.Delete(ctx, foundService); err != nil {
			return fmt.Errorf("failed to delete service %s/%s: %w", m.Namespace, name, err)
		}
	}

	return nil
}

func (m *MetricsCollector) updateMetricsCollector(ctx context.Context, isUWL bool, deployParams *DeploymentParams) error {
	if err := m.ensureService(ctx, isUWL); err != nil {
		return err
	}

	if err := m.ensureServiceMonitor(ctx, isUWL); err != nil {
		return err
	}

	if err := m.ensureAlertingRule(ctx, isUWL); err != nil {
		return err
	}

	if err := m.ensureDeployment(ctx, isUWL, deployParams); err != nil {
		return err
	}

	return nil
}

func (m *MetricsCollector) ensureService(ctx context.Context, isUWL bool) error {
	name := metricsCollector
	if isUWL {
		name = uwlMetricsCollector
	}

	desiredService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: m.Namespace,
			Labels: map[string]string{
				selectorKey: name,
			},
			Annotations: map[string]string{
				ownerLabelKey: ownerLabelValue,
				"service.beta.openshift.io/serving-cert-secret-name": name + "-kube-rbac-tls",
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				selectorKey: name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "metrics",
					Port:       8080,
					TargetPort: intstr.FromString("metrics"),
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	foundService := &corev1.Service{}
	err := m.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: m.Namespace}, foundService)
	if err != nil && errors.IsNotFound(err) {
		m.Log.Info("Creating Service", "name", name, "namespace", m.Namespace)
		if err := m.Client.Create(ctx, desiredService); err != nil {
			return fmt.Errorf("failed to create service %s/%s: %w", m.Namespace, name, err)
		}

		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get service %s/%s: %w", m.Namespace, name, err)
	}

	if !reflect.DeepEqual(desiredService.Spec, foundService.Spec) {
		foundService.Spec = desiredService.Spec
		m.Log.Info("Updating Service", "name", name, "namespace", m.Namespace)
		if err := m.Client.Update(ctx, foundService); err != nil {
			return fmt.Errorf("failed to update service %s/%s: %w", m.Namespace, name, err)
		}
	}

	return nil
}

// createServiceMonitor creates a ServiceMonitor for the metrics collector.
func (m *MetricsCollector) ensureServiceMonitor(ctx context.Context, isUWL bool) error {
	name := metricsCollector
	replace := "acm_metrics_collector_${1}"
	if isUWL {
		name = uwlMetricsCollector
		replace = "acm_uwl_metrics_collector_${1}"
	}

	desiredSm := &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: m.Namespace,
			Labels: map[string]string{
				selectorKey: name,
			},
			Annotations: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					selectorKey: name,
				},
			},
			NamespaceSelector: monitoringv1.NamespaceSelector{
				MatchNames: []string{m.Namespace},
			},
			Endpoints: []monitoringv1.Endpoint{
				{
					Port:   "metrics",
					Path:   "/metrics",
					Scheme: "http",
					MetricRelabelConfigs: []*monitoringv1.RelabelConfig{
						{
							Action:       "replace",
							Regex:        "(.+)",
							Replacement:  replace,
							SourceLabels: []string{"__name__"},
							TargetLabel:  "__name__",
						},
					},
				},
			},
		},
	}

	foundSm := &monitoringv1.ServiceMonitor{}
	err := m.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: m.Namespace}, foundSm)
	if err != nil && errors.IsNotFound(err) {
		m.Log.Info("Creating ServiceMonitor", "name", name, "namespace", m.Namespace)
		if err := m.Client.Create(ctx, desiredSm); err != nil {
			return fmt.Errorf("failed to create ServiceMonitor %s/%s: %w", m.Namespace, name, err)
		}

		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get ServiceMonitor %s/%s: %w", m.Namespace, name, err)
	}

	if !reflect.DeepEqual(desiredSm.Spec, foundSm.Spec) {
		foundSm.Spec = desiredSm.Spec
		m.Log.Info("Updating ServiceMonitor", "name", name, "namespace", m.Namespace)
		if err := m.Client.Update(ctx, foundSm); err != nil {
			return fmt.Errorf("failed to update ServiceMonitor %s/%s: %w", m.Namespace, name, err)
		}
	}

	return nil
}

func (m *MetricsCollector) ensureAlertingRule(ctx context.Context, isUWL bool) error {
	baseName := metricsCollector
	alert := "MetricsCollector"
	replace := "acm_metrics_collector_"
	if isUWL {
		baseName = uwlMetricsCollector
		alert = "UWLMetricsCollector"
		replace = "acm_uwl_metrics_collector_"
	}

	name := fmt.Sprintf("acm-%s-alerting-rules", baseName)

	desiredPromRule := &monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: m.Namespace,
		},
		Spec: monitoringv1.PrometheusRuleSpec{
			Groups: []monitoringv1.RuleGroup{
				{
					Name: baseName + "-rules",
					Rules: []monitoringv1.Rule{
						{
							Alert: "ACM" + alert + "FederationError",
							Annotations: map[string]string{
								"summary":     "Error federating from in-cluster Prometheus.",
								"description": "There are errors when federating from platform Prometheus",
							},
							Expr: intstr.FromString(`(sum by (status_code, type) (rate(` + replace + `federate_requests_total{status_code!~"2.*"}[10m]))) > 10`),
							For:  "10m",
							Labels: map[string]string{
								"severity": "critical",
							},
						},
						{
							Alert: "ACM" + alert + "ForwardRemoteWriteError",
							Annotations: map[string]string{
								"summary":     "Error forwarding to Hub Thanos.",
								"description": "There are errors when remote writing to Hub hub Thanos",
							},
							Expr: intstr.FromString(`(sum by (status_code, type) (rate(` + replace + `forward_write_requests_total{status_code!~"2.*"}[10m]))) > 10`),
							For:  "10m",
							Labels: map[string]string{
								"severity": "critical",
							},
						},
					},
				},
			},
		},
	}

	foundPromRule := &monitoringv1.PrometheusRule{}
	err := m.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: m.Namespace}, foundPromRule)
	if err != nil && errors.IsNotFound(err) {
		m.Log.Info("Creating PrometheusRule", "name", name, "namespace", m.Namespace)
		if err := m.Client.Create(ctx, desiredPromRule); err != nil {
			return fmt.Errorf("failed to create PrometheusRule %s/%s: %w", m.Namespace, name, err)
		}

		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get PrometheusRule %s/%s: %w", m.Namespace, name, err)
	}

	if !reflect.DeepEqual(desiredPromRule.Spec, foundPromRule.Spec) {
		foundPromRule.Spec = desiredPromRule.Spec
		m.Log.Info("Updating PrometheusRule", "name", name, "namespace", m.Namespace)
		if err := m.Client.Update(ctx, foundPromRule); err != nil {
			return fmt.Errorf("failed to update PrometheusRule %s/%s: %w", m.Namespace, name, err)
		}
	}

	return nil
}

func (m *MetricsCollector) ensureDeployment(ctx context.Context, isUWL bool, deployParams *DeploymentParams) error {
	secretName := metricsCollector
	if isUWL {
		secretName = uwlMetricsCollector
	}

	volumes := []corev1.Volume{
		{
			Name: "mtlscerts",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: mtlsCertName,
				},
			},
		},
		{
			Name: "mtlsca",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: mtlsCaName,
				},
			},
		},
	}

	if m.ClusterInfo.ClusterType != operatorconfig.OcpThreeClusterType {
		serviceCAOperatorGenerated := []corev1.Volume{
			{
				Name: "secret-kube-rbac-proxy-tls",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: secretName + "-kube-rbac-tls",
					},
				},
			},
			{
				Name: "secret-kube-rbac-proxy-metric",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: secretName + "-kube-rbac-proxy-metric",
					},
				},
			},
			{
				Name: "metrics-client-ca",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secretName + "-clientca-metric",
						},
					},
				},
			},
		}

		volumes = append(volumes, serviceCAOperatorGenerated...)
	}

	mounts := []corev1.VolumeMount{
		{
			Name:      "mtlscerts",
			MountPath: "/tlscerts/certs",
		},
		{
			Name:      "mtlsca",
			MountPath: "/tlscerts/ca",
		},
	}

	if m.ClusterInfo.ClusterID != "" {
		volumes = append(volumes, corev1.Volume{
			Name: caVolName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: openshift.CaConfigmapName,
					},
				},
			},
		})
		mounts = append(mounts, corev1.VolumeMount{
			Name:      caVolName,
			MountPath: caMounthPath,
		})
	}

	commands := m.getCommands(isUWL, deployParams)

	from := promURL
	if !m.ClusterInfo.InstallPrometheus {
		from = ocpPromURL
		if isUWL {
			from = uwlPromURL
		}
	}

	fromQuery := from
	name := metricsCollectorName
	if isUWL {
		fromQuery = uwlQueryURL
		name = uwlMetricsCollectorName
	}

	replicaCount := int32(0)
	if m.ObsAddonSpec.EnableMetrics || m.ClusterInfo.IsHubMetricsCollector {
		replicaCount = 1
	}

	desiredMetricsCollectorDep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: m.Namespace,
			Annotations: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicaCount,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					selectorKey: secretName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						ownerLabelKey: ownerLabelValue,
						operatorconfig.WorkloadPartitioningPodAnnotationKey: operatorconfig.WorkloadPodExpectedValueJSON,
					},
					Labels: map[string]string{
						selectorKey: secretName,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: m.ServiceAccountName,
					Containers: []corev1.Container{
						{
							Name:    metricsCollector,
							Image:   rendering.Images[operatorconfig.MetricsCollectorKey],
							Command: commands,
							Env: []corev1.EnvVar{
								{
									Name:  "FROM",
									Value: from,
								},
								{
									Name:  "FROM_QUERY",
									Value: fromQuery,
								},
								{
									Name:  "TO",
									Value: m.HubInfo.ObservatoriumAPIEndpoint,
								},
							},
							VolumeMounts:    mounts,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8080,
									Name:          "metrics",
								},
							},
						},
					},
					Volumes:      volumes,
					NodeSelector: deployParams.nodeSelector,
					Tolerations:  deployParams.tolerations,
				},
			},
		},
	}

	if deployParams.proxyConfig.httpProxy != "" || deployParams.proxyConfig.httpsProxy != "" || deployParams.proxyConfig.noProxy != "" {
		desiredMetricsCollectorDep.Spec.Template.Spec.Containers[0].Env = append(desiredMetricsCollectorDep.Spec.Template.Spec.Containers[0].Env,
			corev1.EnvVar{
				Name:  "HTTP_PROXY",
				Value: deployParams.proxyConfig.httpProxy,
			},
			corev1.EnvVar{
				Name:  "HTTPS_PROXY",
				Value: deployParams.proxyConfig.httpsProxy,
			},
			corev1.EnvVar{
				Name:  "NO_PROXY",
				Value: deployParams.proxyConfig.noProxy,
			})
	}
	if deployParams.proxyConfig.httpsProxy != "" && deployParams.proxyConfig.caBundle != "" {
		desiredMetricsCollectorDep.Spec.Template.Spec.Containers[0].Env = append(desiredMetricsCollectorDep.Spec.Template.Spec.Containers[0].Env,
			corev1.EnvVar{
				Name:  "HTTPS_PROXY_CA_BUNDLE",
				Value: deployParams.proxyConfig.caBundle,
			})
	}

	if m.ClusterInfo.IsHubMetricsCollector {
		//to avoid hub metrics collector from sending status
		desiredMetricsCollectorDep.Spec.Template.Spec.Containers[0].Env = append(desiredMetricsCollectorDep.Spec.Template.Spec.Containers[0].Env,
			corev1.EnvVar{
				Name:  "STANDALONE",
				Value: "true",
			})
	}

	privileged := false
	readOnlyRootFilesystem := true
	desiredMetricsCollectorDep.Spec.Template.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{
		Privileged:             &privileged,
		ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
	}

	if m.ObsAddonSpec.Resources != nil {
		desiredMetricsCollectorDep.Spec.Template.Spec.Containers[0].Resources = *m.ObsAddonSpec.Resources
	}

	foundMetricsCollectorDep := &appsv1.Deployment{}
	err := m.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: m.Namespace}, foundMetricsCollectorDep)
	if err != nil && errors.IsNotFound(err) {
		m.Log.Info("Creating Deployment", "name", name, "namespace", m.Namespace)
		if err := m.Client.Create(ctx, desiredMetricsCollectorDep); err != nil {
			return fmt.Errorf("failed to create Deployment %s/%s: %w", m.Namespace, name, err)
		}

		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get Deployment %s/%s: %w", m.Namespace, name, err)
	}

	isDifferent := !reflect.DeepEqual(desiredMetricsCollectorDep.Spec.Template.Spec, foundMetricsCollectorDep.Spec.Template.Spec) ||
		!reflect.DeepEqual(desiredMetricsCollectorDep.Spec.Replicas, foundMetricsCollectorDep.Spec.Replicas) ||
		deployParams.forceRestart
	if isDifferent {
		m.Log.Info("Updating Deployment", "name", name, "namespace", m.Namespace)
		if deployParams.forceRestart && foundMetricsCollectorDep.Status.ReadyReplicas != 0 {
			desiredMetricsCollectorDep.Spec.Template.ObjectMeta.Labels[restartLabel] = time.Now().Format("2006-1-2.1504")
		}

		desiredMetricsCollectorDep.ResourceVersion = foundMetricsCollectorDep.ResourceVersion

		if err := m.Client.Update(ctx, desiredMetricsCollectorDep); err != nil {
			return fmt.Errorf("failed to update Deployment %s/%s: %w", m.Namespace, name, err)
		}
	}

	return nil
}

func (m *MetricsCollector) getCommands(isUSW bool, deployParams *DeploymentParams) []string {
	interval := defaultInterval
	if m.ObsAddonSpec.Interval != 0 {
		interval = fmt.Sprintf("%ds", m.ObsAddonSpec.Interval)
	}

	evaluateInterval := "30s"
	if m.ObsAddonSpec.Interval < 30 {
		evaluateInterval = interval
	}

	caFile := caMounthPath + "/service-ca.crt"
	clusterID := m.ClusterInfo.ClusterID
	if clusterID == "" {
		clusterID = m.HubInfo.ClusterName
		// deprecated ca bundle, only used for ocp 3.11 env
		caFile = "/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt"
	}

	allowList := deployParams.allowlist
	if isUSW {
		allowList = deployParams.uwlList
	}

	commands := []string{
		"/usr/bin/metrics-collector",
		"--listen=:8080",
		"--from=$(FROM)",
		"--from-query=$(FROM_QUERY)",
		"--to-upload=$(TO)",
		"--to-upload-ca=/tlscerts/ca/ca.crt",
		"--to-upload-cert=/tlscerts/certs/tls.crt",
		"--to-upload-key=/tlscerts/certs/tls.key",
		"--interval=" + interval,
		"--evaluate-interval=" + evaluateInterval,
		"--limit-bytes=" + strconv.Itoa(limitBytes),
		fmt.Sprintf("--label=\"cluster=%s\"", m.HubInfo.ClusterName),
		fmt.Sprintf("--label=\"clusterID=%s\"", clusterID),
	}
	commands = append(commands, "--from-token-file=/var/run/secrets/kubernetes.io/serviceaccount/token")
	if !m.ClusterInfo.InstallPrometheus {
		commands = append(commands, "--from-ca-file="+caFile)
	}
	if m.ClusterInfo.ClusterType != operatorconfig.DefaultClusterType {
		commands = append(commands, fmt.Sprintf("--label=\"clusterType=%s\"", m.ClusterInfo.ClusterType))
	}

	dynamicMetricList := map[string]bool{}
	for _, group := range allowList.CollectRuleGroupList {
		if group.Selector.MatchExpression != nil {
			for _, expr := range group.Selector.MatchExpression {
				if !evaluateMatchExpression(expr, "clusterType", m.ClusterInfo.ClusterType) {
					continue
				}

				for _, rule := range group.CollectRuleList {
					matchList := []string{}
					for _, match := range rule.Metrics.MatchList {
						matchList = append(matchList, `"`+strings.ReplaceAll(match, `"`, `\"`)+`"`)
						if name := getNameInMatch(match); name != "" {
							dynamicMetricList[name] = false
						}
					}
					for _, name := range rule.Metrics.NameList {
						dynamicMetricList[name] = false
					}
					matchListStr := "[" + strings.Join(matchList, ",") + "]"
					nameListStr := `["` + strings.Join(rule.Metrics.NameList, `","`) + `"]`
					commands = append(
						commands,
						fmt.Sprintf("--collectrule={\"name\":\"%s\",\"expr\":\"%s\",\"for\":\"%s\",\"names\":%v,\"matches\":%v}",
							rule.Collect, rule.Expr, rule.For, nameListStr, matchListStr),
					)
				}
			}
		}
	}

	for _, metrics := range allowList.NameList {
		if _, ok := dynamicMetricList[metrics]; !ok {
			commands = append(commands, fmt.Sprintf("--match={__name__=\"%s\"}", metrics))
		}
	}
	for _, match := range allowList.MatchList {
		if name := getNameInMatch(match); name != "" {
			if _, ok := dynamicMetricList[name]; ok {
				continue
			}
		}
		commands = append(commands, fmt.Sprintf("--match={%s}", match))
	}

	renamekeys := make([]string, 0, len(allowList.RenameMap))
	for k := range allowList.RenameMap {
		renamekeys = append(renamekeys, k)
	}
	sort.Strings(renamekeys)
	for _, k := range renamekeys {
		commands = append(commands, fmt.Sprintf("--rename=\"%s=%s\"", k, allowList.RenameMap[k]))
	}
	for _, rule := range allowList.RecordingRuleList {
		commands = append(
			commands,
			fmt.Sprintf("--recordingrule={\"name\":\"%s\",\"query\":\"%s\"}", rule.Record, rule.Expr),
		)
	}
	return commands
}

func (m *MetricsCollector) getMetricsAllowlist(ctx context.Context) (*operatorconfig.MetricsAllowlist, *operatorconfig.MetricsAllowlist, error) {
	allowList := &operatorconfig.MetricsAllowlist{}
	userAllowList := &operatorconfig.MetricsAllowlist{}

	// get allowlist configmap
	cm := &corev1.ConfigMap{}
	err := m.Client.Get(ctx, types.NamespacedName{Name: operatorconfig.AllowlistConfigMapName,
		Namespace: m.Namespace}, cm)
	if err != nil {
		m.Log.Error(err, "Failed to get configmap", "name", operatorconfig.AllowlistConfigMapName, "namespace", m.Namespace)
	}

	if cm.Data != nil {
		configmapKey := operatorconfig.MetricsConfigMapKey
		if m.ClusterInfo.ClusterType == operatorconfig.OcpThreeClusterType {
			configmapKey = operatorconfig.MetricsOcp311ConfigMapKey
		}

		err = yaml.Unmarshal([]byte(cm.Data[configmapKey]), allowList)
		if err != nil {
			return allowList, userAllowList, fmt.Errorf("failed to unmarshal allowList data in configmap %s/%s: %w", cm.Namespace, cm.Name, err)
		}

		// get default user allowlist in configmap
		if uwlData, ok := cm.Data[operatorconfig.UwlMetricsConfigMapKey]; ok {
			err = yaml.Unmarshal([]byte(uwlData), userAllowList)
			if err != nil {
				return allowList, userAllowList, fmt.Errorf("failed to unmarshal user allowList data in configmap %s/%s: %w", cm.Namespace, cm.Name, err)
			}
		}
	}

	// get custom allowlist configmap in all namespaces
	cmList := &corev1.ConfigMapList{}
	cmNamespaces := []string{}
	err = m.Client.List(ctx, cmList, &client.ListOptions{})
	if err != nil {
		m.Log.Error(err, "Failed to list configmaps")
	}

	for _, allowlistCM := range cmList.Items {
		if allowlistCM.ObjectMeta.Name != operatorconfig.AllowlistCustomConfigMapName {
			continue
		}

		cmNamespaces = append(cmNamespaces, allowlistCM.ObjectMeta.Namespace)

		customAllowlist, _, customUwlAllowlist, err := util.ParseAllowlistConfigMap(allowlistCM)
		if err != nil {
			m.Log.Error(err, "Failed to parse data in configmap", "namespace", allowlistCM.ObjectMeta.Namespace, "name", allowlistCM.ObjectMeta.Name)
			continue
		}

		if allowlistCM.ObjectMeta.Namespace != m.Namespace {
			customUwlAllowlist = injectNamespaceLabel(customUwlAllowlist, allowlistCM.ObjectMeta.Namespace)
		}

		allowList, _, userAllowList = util.MergeAllowlist(allowList, customAllowlist, nil, userAllowList, customUwlAllowlist)
	}

	m.Log.Info("Merged allowLists from following namespaces", "namespaces", cmNamespaces)

	return allowList, userAllowList, nil
}

func (m *MetricsCollector) getEndpointDeployment(ctx context.Context) (*appsv1.Deployment, error) {
	ret := &appsv1.Deployment{}
	err := m.Client.Get(ctx, types.NamespacedName{Name: "endpoint-observability-operator", Namespace: m.Namespace}, ret)
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoint deployment %s/%s: %w", m.Namespace, "endpoint-observability-operator", err)
	}

	return ret, nil
}

func getNameInMatch(match string) string {
	r := regexp.MustCompile(`__name__="([^,]*)"`)
	m := r.FindAllStringSubmatch(match, -1)
	if m != nil {
		return m[0][1]
	}
	return ""
}

func (m *MetricsCollector) isUWLMonitoringEnabled(ctx context.Context) (bool, error) {
	sts := &appsv1.StatefulSet{}
	err := m.Client.Get(ctx, types.NamespacedName{Namespace: uwlNamespace, Name: uwlSts}, sts)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}

		return false, fmt.Errorf("failed to get uwl prometheus statefulset %s/%s: %w", uwlNamespace, uwlSts, err)
	}

	return true, nil
}

// for custom uwl allowlist:
// 1. only support "names" and "matches".
// 2. inject namespace label filter for all entries in the allowlist.
func injectNamespaceLabel(allowlist *operatorconfig.MetricsAllowlist,
	namespace string) *operatorconfig.MetricsAllowlist {
	updatedList := &operatorconfig.MetricsAllowlist{
		NameList:  []string{},
		MatchList: []string{},
	}
	for _, name := range allowlist.NameList {
		updatedList.MatchList = append(updatedList.MatchList,
			fmt.Sprintf("__name__=\"%s\",namespace=\"%s\"", name, namespace))
	}
	for _, match := range allowlist.MatchList {
		updatedList.MatchList = append(updatedList.MatchList, fmt.Sprintf("%s,namespace=\"%s\"", match, namespace))
	}
	return updatedList
}
