// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package rendering

import (
	"context"
	"fmt"
	"os"
	"strings"

	prometheusv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/rendering/templates"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	rendererutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering"
	templatesutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering/templates"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
)

const (
	metricsConfigMapKey = "metrics_list.yaml"
)

var (
	namespace       = os.Getenv("WATCH_NAMESPACE")
	log             = logf.Log.WithName("renderer")
	disabledMetrics = []string{
		"apiserver_admission_controller_admission_duration_seconds_bucket",
		"apiserver_flowcontrol_priority_level_request_count_watermarks_bucket",
		"apiserver_response_sizes_bucket",
		"apiserver_watch_events_sizes_bucket",
		"container_memory_failures_total",
		"cluster_quantile:apiserver_request_duration_seconds:histogram_quantile",
		"etcd_request_duration_seconds_bucket",
		"kubelet_http_requests_duration_seconds_bucket",
		"kubelet_runtime_operations_duration_seconds_bucket",
		"rest_client_request_duration_seconds_bucket",
		"storage_operation_duration_seconds_bucket",
	}
)

var Images = map[string]string{}

func Render(
	r *rendererutil.Renderer,
	c runtimeclient.Client,
	hubInfo *operatorconfig.HubInfo,
) ([]*unstructured.Unstructured, error) {

	genericTemplates, err := templates.GetTemplates(templatesutil.GetTemplateRenderer())
	if err != nil {
		return nil, err
	}
	resources, err := r.RenderTemplates(genericTemplates, namespace, map[string]string{})
	if err != nil {
		return nil, err
	}
	for idx := range resources {
		if resources[idx].GetKind() == "Deployment" && resources[idx].GetName() == "kube-state-metrics" {
			obj := util.GetK8sObj(resources[idx].GetKind())
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(resources[idx].Object, obj)
			if err != nil {
				return nil, err
			}
			dep := obj.(*v1.Deployment)
			spec := &dep.Spec.Template.Spec
			spec.Containers[0].Image = Images[operatorconfig.KubeStateMetricsKey]
			spec.Containers[1].Image = Images[operatorconfig.KubeRbacProxyKey]
			spec.Containers[2].Image = Images[operatorconfig.KubeRbacProxyKey]
			spec.ImagePullSecrets = []corev1.LocalObjectReference{
				{Name: os.Getenv(operatorconfig.PullSecret)},
			}

			unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
			if err != nil {
				return nil, err
			}
			resources[idx].Object = unstructuredObj
		}
		if resources[idx].GetKind() == "Deployment" && resources[idx].GetName() == "prometheus-operator" {
			obj := util.GetK8sObj(resources[idx].GetKind())
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(resources[idx].Object, obj)
			if err != nil {
				return nil, err
			}
			dep := obj.(*v1.Deployment)
			spec := &dep.Spec.Template.Spec
			spec.Containers[0].Image = Images[operatorconfig.PrometheusOperatorKey]
			spec.ImagePullSecrets = []corev1.LocalObjectReference{
				{Name: os.Getenv(operatorconfig.PullSecret)},
			}
			args := spec.Containers[0].Args
			for idx := range args {
				args[idx] = strings.Replace(args[idx], "{{NAMESPACE}}", namespace, 1)
				args[idx] = strings.Replace(args[idx], "{{PROM_CONFIGMAP_RELOADER_IMG}}", Images[operatorconfig.PrometheusConfigmapReloaderKey], 1)
			}
			spec.Containers[0].Args = args

			unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
			if err != nil {
				return nil, err
			}
			resources[idx].Object = unstructuredObj
		}
		if resources[idx].GetKind() == "Prometheus" && resources[idx].GetName() == "k8s" {
			obj := util.GetK8sObj(resources[idx].GetKind())
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(resources[idx].Object, obj)
			if err != nil {
				return nil, err
			}
			prom := obj.(*prometheusv1.Prometheus)
			spec := &prom.Spec
			image := Images[operatorconfig.PrometheusKey]
			spec.Image = &image
			spec.Containers[0].Image = Images[operatorconfig.KubeRbacProxyKey]
			spec.ImagePullSecrets = []corev1.LocalObjectReference{
				{Name: os.Getenv(operatorconfig.PullSecret)},
			}
			spec.ExternalLabels[operatorconfig.ClusterLabelKeyForAlerts] = hubInfo.ClusterName
			if hubInfo.AlertmanagerEndpoint == "" {
				log.Info("setting AdditionalAlertManagerConfigs to nil, deleting secrets")
				spec.AdditionalAlertManagerConfigs = nil
				spec.Secrets = []string{}
			} else {
				log.Info("restoring AdditionalAlertManagerConfigs, secrets")
				spec.AdditionalAlertManagerConfigs = &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "prometheus-alertmanager",
					},
					Key: "alertmanager.yaml",
				}
				spec.Secrets = []string{"hub-alertmanager-router-ca", "observability-alertmanager-accessor"}
			}
			unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
			if err != nil {
				return nil, err
			}
			resources[idx].Object = unstructuredObj
		}
		if resources[idx].GetKind() == "DaemonSet" && resources[idx].GetName() == "node-exporter" {
			obj := util.GetK8sObj(resources[idx].GetKind())
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(resources[idx].Object, obj)
			if err != nil {
				return nil, err
			}
			ds := obj.(*v1.DaemonSet)
			spec := &ds.Spec.Template.Spec
			spec.Containers[0].Image = Images[operatorconfig.NodeExporterKey]
			spec.Containers[1].Image = Images[operatorconfig.KubeRbacProxyKey]
			spec.ImagePullSecrets = []corev1.LocalObjectReference{
				{Name: os.Getenv(operatorconfig.PullSecret)},
			}

			unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
			if err != nil {
				return nil, err
			}
			resources[idx].Object = unstructuredObj
		}
		if resources[idx].GetKind() == "Secret" && resources[idx].GetName() == "prometheus-scrape-targets " {
			obj := util.GetK8sObj(resources[idx].GetKind())
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(resources[idx].Object, obj)
			if err != nil {
				return nil, err
			}
			s := obj.(*corev1.Secret)
			promConfig, exists := s.StringData["scrape-targets.yaml"]
			if !exists {
				return nil, fmt.Errorf(
					"no key 'scrape-targets.yaml' found in the secret: %s/%s",
					s.GetNamespace(),
					s.GetName(),
				)
			}

			// replace the disabled metrics
			disabledMetricsSt, err := getDisabledMetrics(c)
			if err != nil {
				return nil, err
			}
			if disabledMetricsSt != "" {
				s.StringData["scrape-targets.yaml"] = strings.ReplaceAll(promConfig, "_DISABLED_METRICS_", disabledMetricsSt)
			}

			unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
			if err != nil {
				return nil, err
			}
			resources[idx].Object = unstructuredObj
		}
		if resources[idx].GetKind() == "Secret" && resources[idx].GetName() == "prometheus-alertmanager" {
			obj := util.GetK8sObj(resources[idx].GetKind())
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(resources[idx].Object, obj)
			if err != nil {
				return nil, err
			}
			s := obj.(*corev1.Secret)
			amConfig, exists := s.StringData["alertmanager.yaml"]
			if !exists {
				return nil, fmt.Errorf(
					"no key 'alertmanager.yaml' found in the configmap: %s/%s",
					s.GetNamespace(),
					s.GetName(),
				)
			}
			// replace the hub alertmanager address. Address will be set to null when alerts are disabled
			hubAmEp := strings.TrimLeft(hubInfo.AlertmanagerEndpoint, "https://")
			amConfig = strings.ReplaceAll(amConfig, "_ALERTMANAGER_ENDPOINT_", hubAmEp)
			s.StringData["alertmanager.yaml"] = amConfig

			unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
			if err != nil {
				return nil, err
			}
			resources[idx].Object = unstructuredObj
		}
	}

	return resources, nil
}

func getDisabledMetrics(c runtimeclient.Client) (string, error) {
	cm := &corev1.ConfigMap{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: operatorconfig.AllowlistConfigMapName,
		Namespace: namespace}, cm)
	if err != nil {
		return "", err
	}
	metricsList := []string{}
	for _, m := range disabledMetrics {
		if !strings.Contains(cm.Data[metricsConfigMapKey], m) {
			metricsList = append(metricsList, m)
		}
	}
	return strings.Join(metricsList, "|"), nil
}
