// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package observabilityendpoint

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
	restartLabel = "cert/time-restarted"
)

var (
	ocpPromURL  = "https://prometheus-k8s.openshift-monitoring.svc:9091"
	uwlPromURL  = "https://prometheus-user-workload.openshift-user-workload-monitoring.svc:9092"
	uwlQueryURL = "https://thanos-querier.openshift-monitoring.svc:9091"
	promURL     = "https://prometheus-k8s:9091"
)

type CollectorParams struct {
	isUWL        bool
	clusterID    string
	clusterType  string
	obsAddonSpec oashared.ObservabilityAddonSpec
	hubInfo      operatorconfig.HubInfo
	allowlist    operatorconfig.MetricsAllowlist
	nodeSelector map[string]string
	tolerations  []corev1.Toleration
	httpProxy    string
	httpsProxy   string
	noProxy      string
	CABundle     string
	replicaCount int32
}

func getCommands(params CollectorParams) []string {
	interval := fmt.Sprint(params.obsAddonSpec.Interval) + "s"
	if fmt.Sprint(params.obsAddonSpec.Interval) == "" {
		interval = defaultInterval
	}
	evaluateInterval := "30s"
	if params.obsAddonSpec.Interval < 30 {
		evaluateInterval = interval
	}
	caFile := caMounthPath + "/service-ca.crt"
	clusterID := params.clusterID
	if params.clusterID == "" {
		clusterID = params.hubInfo.ClusterName
		// deprecated ca bundle, only used for ocp 3.11 env
		caFile = "/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt"
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
		fmt.Sprintf("--label=\"cluster=%s\"", params.hubInfo.ClusterName),
		fmt.Sprintf("--label=\"clusterID=%s\"", clusterID),
	}
	commands = append(commands, "--from-token-file=/var/run/secrets/kubernetes.io/serviceaccount/token")
	if !installPrometheus {
		commands = append(commands, "--from-ca-file="+caFile)
	}
	if params.clusterType != defaultClusterType {
		commands = append(commands, fmt.Sprintf("--label=\"clusterType=%s\"", params.clusterType))
	}

	dynamicMetricList := map[string]bool{}
	for _, group := range params.allowlist.CollectRuleGroupList {
		if group.Selector.MatchExpression != nil {
			for _, expr := range group.Selector.MatchExpression {
				if isHubMetricsCollector {
					if !evluateMatchExpression(expr, clusterID, params.clusterType, params.hubInfo,
						params.allowlist, params.nodeSelector, params.tolerations, params.replicaCount) {
						continue
					}
				} else if !evluateMatchExpression(expr, clusterID, params.clusterType, params.obsAddonSpec, params.hubInfo,
					params.allowlist, params.nodeSelector, params.tolerations, params.replicaCount) {
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

	for _, metrics := range params.allowlist.NameList {
		if _, ok := dynamicMetricList[metrics]; !ok {
			commands = append(commands, fmt.Sprintf("--match={__name__=\"%s\"}", metrics))
		}
	}
	for _, match := range params.allowlist.MatchList {
		if name := getNameInMatch(match); name != "" {
			if _, ok := dynamicMetricList[name]; ok {
				continue
			}
		}
		commands = append(commands, fmt.Sprintf("--match={%s}", match))
	}

	renamekeys := make([]string, 0, len(params.allowlist.RenameMap))
	for k := range params.allowlist.RenameMap {
		renamekeys = append(renamekeys, k)
	}
	sort.Strings(renamekeys)
	for _, k := range renamekeys {
		commands = append(commands, fmt.Sprintf("--rename=\"%s=%s\"", k, params.allowlist.RenameMap[k]))
	}
	for _, rule := range params.allowlist.RecordingRuleList {
		commands = append(
			commands,
			fmt.Sprintf("--recordingrule={\"name\":\"%s\",\"query\":\"%s\"}", rule.Record, rule.Expr),
		)
	}
	return commands
}

func createDeployment(params CollectorParams) *appsv1.Deployment {
	secretName := metricsCollector
	if params.isUWL {
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

	if params.clusterType != ocpThreeClusterType {
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
	if params.clusterID != "" {
		volumes = append(volumes, corev1.Volume{
			Name: caVolName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: caConfigmapName,
					},
				},
			},
		})
		mounts = append(mounts, corev1.VolumeMount{
			Name:      caVolName,
			MountPath: caMounthPath,
		})
	}

	commands := getCommands(params)

	from := promURL
	if !installPrometheus {
		from = ocpPromURL
		if params.isUWL {
			from = uwlPromURL
		}
	}
	fromQuery := from
	if params.isUWL {
		fromQuery = uwlQueryURL
	}
	name := metricsCollectorName
	if params.isUWL {
		name = uwlMetricsCollectorName
	}
	metricsCollectorDep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(params.replicaCount),
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
					ServiceAccountName: serviceAccountName,
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
									Value: params.hubInfo.ObservatoriumAPIEndpoint,
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
					NodeSelector: params.nodeSelector,
					Tolerations:  params.tolerations,
				},
			},
		},
	}

	if params.httpProxy != "" || params.httpsProxy != "" || params.noProxy != "" {
		metricsCollectorDep.Spec.Template.Spec.Containers[0].Env = append(metricsCollectorDep.Spec.Template.Spec.Containers[0].Env,
			corev1.EnvVar{
				Name:  "HTTP_PROXY",
				Value: params.httpProxy,
			},
			corev1.EnvVar{
				Name:  "HTTPS_PROXY",
				Value: params.httpsProxy,
			},
			corev1.EnvVar{
				Name:  "NO_PROXY",
				Value: params.noProxy,
			})
	}
	if params.httpsProxy != "" && params.CABundle != "" {
		metricsCollectorDep.Spec.Template.Spec.Containers[0].Env = append(metricsCollectorDep.Spec.Template.Spec.Containers[0].Env,
			corev1.EnvVar{
				Name:  "HTTPS_PROXY_CA_BUNDLE",
				Value: params.CABundle,
			})
	}

	if isHubMetricsCollector {
		// to avoid hub metrics collector from sending status
		metricsCollectorDep.Spec.Template.Spec.Containers[0].Env = append(metricsCollectorDep.Spec.Template.Spec.Containers[0].Env,
			corev1.EnvVar{
				Name:  "STANDALONE",
				Value: "true",
			})
	}

	privileged := false
	readOnlyRootFilesystem := true

	metricsCollectorDep.Spec.Template.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{
		Privileged:             &privileged,
		ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
	}

	if params.obsAddonSpec.Resources != nil {
		metricsCollectorDep.Spec.Template.Spec.Containers[0].Resources = *params.obsAddonSpec.Resources
	}
	return metricsCollectorDep
}

func createService(params CollectorParams) *corev1.Service {
	name := metricsCollector
	if params.isUWL {
		name = uwlMetricsCollector
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
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
}

func createAlertingRule(params CollectorParams) *monitoringv1.PrometheusRule {
	name := metricsCollector
	alert := "MetricsCollector"
	replace := "acm_metrics_collector_"
	if params.isUWL {
		name = uwlMetricsCollector
		alert = "UWLMetricsCollector"
		replace = "acm_uwl_metrics_collector_"
	}

	return &monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "acm-" + name + "-alerting-rules",
			Namespace: namespace,
		},
		Spec: monitoringv1.PrometheusRuleSpec{
			Groups: []monitoringv1.RuleGroup{
				{
					Name: name + "-rules",
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
}

// createServiceMonitor creates a ServiceMonitor for the metrics collector.
func createServiceMonitor(params CollectorParams) *monitoringv1.ServiceMonitor {
	name := metricsCollector
	replace := "acm_metrics_collector_${1}"
	if params.isUWL {
		name = uwlMetricsCollector
		replace = "acm_uwl_metrics_collector_${1}"
	}

	return &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
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
				MatchNames: []string{namespace},
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
}

func updateMetricsCollectors(ctx context.Context, c client.Client, obsAddonSpec oashared.ObservabilityAddonSpec,
	hubInfo operatorconfig.HubInfo, clusterID string, clusterType string,
	replicaCount int32, forceRestart bool) (bool, error) {

	list, uwlList, err := getMetricsAllowlist(ctx, c, clusterType)
	if err != nil {
		return false, err
	}
	endpointDeployment := getEndpointDeployment(ctx, c)
	params := CollectorParams{
		isUWL:        false,
		clusterID:    clusterID,
		clusterType:  clusterType,
		obsAddonSpec: obsAddonSpec,
		hubInfo:      hubInfo,
		allowlist:    list,
		replicaCount: replicaCount,
		nodeSelector: endpointDeployment.Spec.Template.Spec.NodeSelector,
		tolerations:  endpointDeployment.Spec.Template.Spec.Tolerations,
	}

	// stash away proxy settings from endpoint deployment
	for _, container := range endpointDeployment.Spec.Template.Spec.Containers {
		if container.Name == "endpoint-observability-operator" {
			for _, env := range container.Env {
				if env.Name == "HTTP_PROXY" {
					params.httpProxy = env.Value
				} else if env.Name == "HTTPS_PROXY" {
					params.httpsProxy = env.Value
				} else if env.Name == "NO_PROXY" {
					params.noProxy = env.Value
				} else if env.Name == "HTTPS_PROXY_CA_BUNDLE" {
					params.CABundle = env.Value
				}
			}
		}
	}

	result, err := updateMetricsCollector(ctx, c, params, forceRestart)
	if err != nil || !result {
		return result, err
	}
	isUwl, err := isUWLMonitoringEnabled(ctx, c)
	if err != nil {
		return result, err
	}
	if isUwl && len(uwlList.NameList) != 0 {
		params.isUWL = true
		params.allowlist = uwlList
		result, err = updateMetricsCollector(ctx, c, params, forceRestart)
	} else {
		err = deleteMetricsCollector(ctx, c, uwlMetricsCollectorName)
		if err != nil {
			return false, err
		}
	}
	return result, err
}

func updateMetricsCollector(ctx context.Context, c client.Client, params CollectorParams,
	forceRestart bool) (bool, error) {
	name := metricsCollectorName
	resourceName := metricsCollector
	if params.isUWL {
		resourceName = uwlMetricsCollector
		name = uwlMetricsCollectorName
	}

	desiredService := createService(params)
	retryErr := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		foundService := &corev1.Service{}
		err := c.Get(ctx, types.NamespacedName{Name: metricsCollector, Namespace: namespace}, foundService)
		if err != nil && errors.IsNotFound(err) {
			log.Info("Creating Service", "name", metricsCollector, "namespace", namespace)
			if err := c.Create(ctx, desiredService); err != nil {
				return fmt.Errorf("failed to create service %s/%s: %w", namespace, metricsCollector, err)
			}

			return nil
		} else if err != nil {
			return fmt.Errorf("failed to get service %s/%s: %w", namespace, metricsCollector, err)
		}

		if !equality.Semantic.DeepDerivative(desiredService.Spec, foundService.Spec) {
			log.Info("Updating Service", "name", metricsCollector, "namespace", namespace)

			foundService.Spec = desiredService.Spec
			if err := c.Update(ctx, foundService); err != nil {
				return fmt.Errorf("failed to update service %s/%s: %w", namespace, metricsCollector, err)
			}
		}

		return nil
	})

	if retryErr != nil {
		return false, retryErr
	}

	desiredSm := createServiceMonitor(params)
	retryErr = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		foundSm := &monitoringv1.ServiceMonitor{}
		err := c.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: namespace}, foundSm)
		if err != nil && errors.IsNotFound(err) {
			log.Info("Creating ServiceMonitor", "name", resourceName, "namespace", namespace)
			if err := c.Create(ctx, desiredSm); err != nil {
				return fmt.Errorf("failed to create ServiceMonitor %s/%s: %w", namespace, resourceName, err)
			}

			return nil
		} else if err != nil {
			return fmt.Errorf("failed to get ServiceMonitor %s/%s: %w", namespace, resourceName, err)
		}

		if !equality.Semantic.DeepDerivative(desiredSm.Spec, foundSm.Spec) {
			log.Info("Updating ServiceMonitor", "name", resourceName, "namespace", namespace)

			foundSm.Spec = desiredSm.Spec
			if err := c.Update(ctx, foundSm); err != nil {
				return fmt.Errorf("failed to update ServiceMonitor %s/%s: %w", namespace, resourceName, err)
			}
		}

		return nil
	})

	if retryErr != nil {
		return false, retryErr
	}

	promRuleName := "acm-" + resourceName + "-alerting-rules"
	desiredPromRule := createAlertingRule(params)
	retryErr = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		foundPromRule := &monitoringv1.PrometheusRule{}
		err := c.Get(ctx, types.NamespacedName{Name: promRuleName, Namespace: namespace}, foundPromRule)
		if err != nil && errors.IsNotFound(err) {
			log.Info("Creating PrometheusRule", "name", promRuleName, "namespace", namespace)
			if err := c.Create(ctx, desiredPromRule); err != nil {
				return fmt.Errorf("failed to create PrometheusRule %s/%s: %w", namespace, promRuleName, err)
			}

			return nil
		} else if err != nil {
			return fmt.Errorf("failed to get PrometheusRule %s/%s: %w", namespace, promRuleName, err)
		}

		if !equality.Semantic.DeepDerivative(desiredPromRule.Spec, foundPromRule.Spec) {
			log.Info("Updating PrometheusRule", "name", promRuleName, "namespace", namespace)

			foundPromRule.Spec = desiredPromRule.Spec
			if err := c.Update(ctx, foundPromRule); err != nil {
				return fmt.Errorf("failed to update PrometheusRule %s/%s: %w", namespace, promRuleName, err)
			}
		}

		return nil
	})

	if retryErr != nil {
		return false, retryErr
	}

	desiredMetricsCollectorDep := createDeployment(params)
	retryErr = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		foundMetricsCollectorDep := &appsv1.Deployment{}
		err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, foundMetricsCollectorDep)
		if err != nil && errors.IsNotFound(err) {
			log.Info("Creating Deployment", "name", name, "namespace", namespace)
			if err := c.Create(ctx, desiredMetricsCollectorDep); err != nil {
				return fmt.Errorf("failed to create Deployment %s/%s: %w", namespace, name, err)
			}
		} else if err != nil {
			return fmt.Errorf("failed to get Deployment %s/%s: %w", namespace, name, err)
		}

		isDifferentSpec := !equality.Semantic.DeepDerivative(desiredMetricsCollectorDep.Spec.Template.Spec, foundMetricsCollectorDep.Spec.Template.Spec)
		isDifferentReplicas := !equality.Semantic.DeepEqual(desiredMetricsCollectorDep.Spec.Replicas, foundMetricsCollectorDep.Spec.Replicas)
		if isDifferentSpec || isDifferentReplicas || forceRestart {
			log.Info("Updating Deployment", "name", name, "namespace", namespace, "isDifferentSpec", isDifferentSpec, "isDifferentReplicas", isDifferentReplicas, "forceRestart", forceRestart)
			if forceRestart && foundMetricsCollectorDep.Status.ReadyReplicas != 0 {
				desiredMetricsCollectorDep.Spec.Template.ObjectMeta.Labels[restartLabel] = time.Now().Format("2006-1-2.1504")
			}

			desiredMetricsCollectorDep.ResourceVersion = foundMetricsCollectorDep.ResourceVersion

			if err := c.Update(ctx, desiredMetricsCollectorDep); err != nil {
				return fmt.Errorf("failed to update Deployment %s/%s: %w", namespace, name, err)
			}

			return nil
		}

		return nil
	})

	if retryErr != nil {
		return false, retryErr
	}

	return true, nil
}

func deleteMetricsCollector(ctx context.Context, c client.Client, name string) error {
	found := &appsv1.Deployment{}
	err := c.Get(ctx, types.NamespacedName{Name: name,
		Namespace: namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("The metrics collector deployment does not exist", "name", name)
			return nil
		}
		log.Error(err, "Failed to check the metrics collector deployment", "name", name)
		return err
	}
	err = c.Delete(ctx, found)
	if err != nil {
		log.Error(err, "Failed to delete the metrics collector deployment", "name", name)
		return err
	}
	log.Info("metrics collector deployment deleted", "name", name)

	foundSM := &monitoringv1.ServiceMonitor{}
	if err := c.Get(ctx, types.NamespacedName{Name: strings.TrimSuffix(name, "-deployment"),
		Namespace: namespace}, foundSM); err != nil {
		if errors.IsNotFound(err) {
			log.Info("The metrics collector servicemonitor does not exist", "name", strings.TrimSuffix(name, "-deployment"))
			return nil
		}
		log.Error(err, "Failed to check the metrics collector servicemonitor", "name", strings.TrimSuffix(name, "-deployment"))
		return err
	}
	if err := c.Delete(ctx, foundSM); err != nil {
		log.Error(err, "Failed to delete the metrics collector servicemonitor", "name", strings.TrimSuffix(name, "-deployment"))
		return err
	}
	log.Info("metrics collector servicemonitor deleted", "name", strings.TrimSuffix(name, "-deployment"))

	foundAlerts := &monitoringv1.PrometheusRule{}
	if err := c.Get(ctx, types.NamespacedName{Name: "acm-" + strings.TrimSuffix(name, "-deployment") + "-alerting-rules",
		Namespace: namespace}, foundAlerts); err != nil {
		if errors.IsNotFound(err) {
			log.Info("The metrics collector alerting rules does not exist", "name", "acm-"+strings.TrimSuffix(name, "-deployment")+"-alerting-rules")
			return nil
		}
		log.Error(err, "Failed to check the metrics collector alerting rules", "name", "acm-"+strings.TrimSuffix(name, "-deployment")+"-alerting-rules")
		return err
	}
	if err := c.Delete(ctx, foundAlerts); err != nil {
		log.Error(err, "Failed to delete the metrics collector alerting rules", "name", "acm-"+strings.TrimSuffix(name, "-deployment")+"-alerting-rules")
		return err
	}
	log.Info("metrics collector alerting rules deleted", "name", "acm-"+strings.TrimSuffix(name, "-deployment")+"-alerting-rules")

	foundService := &corev1.Service{}
	if err := c.Get(ctx, types.NamespacedName{Name: strings.TrimSuffix(name, "-deployment"),
		Namespace: namespace}, foundService); err != nil {
		if errors.IsNotFound(err) {
			log.Info("The metrics collector service does not exist", "name", strings.TrimSuffix(name, "-deployment"))
			return nil
		}
		log.Error(err, "Failed to check the metrics collector service", "name", strings.TrimSuffix(name, "-deployment"))
		return err
	}
	if err := c.Delete(ctx, foundService); err != nil {
		log.Error(err, "Failed to delete the metrics collector service", "name", strings.TrimSuffix(name, "-deployment"))
		return err
	}
	log.Info("metrics collector service deleted", "name", strings.TrimSuffix(name, "-deployment"))

	return nil
}

func int32Ptr(i int32) *int32 { return &i }

func getMetricsAllowlist(ctx context.Context, c client.Client,
	clusterType string) (operatorconfig.MetricsAllowlist, operatorconfig.MetricsAllowlist, error) {
	l := &operatorconfig.MetricsAllowlist{}
	ul := &operatorconfig.MetricsAllowlist{}
	cm := &corev1.ConfigMap{}
	err := c.Get(ctx, types.NamespacedName{Name: operatorconfig.AllowlistConfigMapName,
		Namespace: namespace}, cm)
	if err != nil {
		log.Error(err, "Failed to get configmap")
	} else {
		if cm.Data != nil {
			configmapKey := operatorconfig.MetricsConfigMapKey
			if clusterType == ocpThreeClusterType {
				configmapKey = operatorconfig.MetricsOcp311ConfigMapKey
			}
			err = yaml.Unmarshal([]byte(cm.Data[configmapKey]), l)
			if err != nil {
				log.Error(err, "Failed to unmarshal data in configmap")
				return *l, *ul, err
			}
			if uwlData, ok := cm.Data[operatorconfig.UwlMetricsConfigMapKey]; ok {
				err = yaml.Unmarshal([]byte(uwlData), ul)
				if err != nil {
					log.Error(err, "Failed to unmarshal uwl data in configmap")
					return *l, *ul, err
				}
			}
		}
	}

	cmList := &corev1.ConfigMapList{}
	_ = c.List(ctx, cmList, &client.ListOptions{})
	for _, allowlistCM := range cmList.Items {
		if allowlistCM.ObjectMeta.Name == operatorconfig.AllowlistCustomConfigMapName {
			log.Info("Parse custom allowlist configmap", "namespace", allowlistCM.ObjectMeta.Namespace,
				"name", allowlistCM.ObjectMeta.Name)
			customAllowlist, _, customUwlAllowlist, err := util.ParseAllowlistConfigMap(allowlistCM)
			if err != nil {
				log.Error(err, "Failed to parse data in configmap", "namespace", allowlistCM.ObjectMeta.Namespace,
					"name", allowlistCM.ObjectMeta.Name)
			}
			if allowlistCM.ObjectMeta.Namespace != namespace {
				customUwlAllowlist = injectNamespaceLabel(customUwlAllowlist, allowlistCM.ObjectMeta.Namespace)
			}
			l, _, ul = util.MergeAllowlist(l, customAllowlist, nil, ul, customUwlAllowlist)
		}
	}

	return *l, *ul, nil
}

func getEndpointDeployment(ctx context.Context, c client.Client) appsv1.Deployment {
	d := &appsv1.Deployment{}
	err := c.Get(ctx, types.NamespacedName{Name: "endpoint-observability-operator", Namespace: namespace}, d)
	if err != nil {
		log.Error(err, "Failed to get deployment")
	}
	return *d
}

func getNameInMatch(match string) string {
	r := regexp.MustCompile(`__name__="([^,]*)"`)
	m := r.FindAllStringSubmatch(match, -1)
	if m != nil {
		return m[0][1]
	}
	return ""
}

func isUWLMonitoringEnabled(ctx context.Context, c client.Client) (bool, error) {
	sts := &appsv1.StatefulSet{}
	err := c.Get(ctx, types.NamespacedName{Namespace: uwlNamespace, Name: uwlSts}, sts)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "Failed to get uwl prometheus statefulset")
			return false, err
		} else {
			return false, nil
		}
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
