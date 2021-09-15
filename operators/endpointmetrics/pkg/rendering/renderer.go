// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package rendering

import (
	"context"
	"fmt"
	"os"
	"strings"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/open-cluster-management/multicluster-observability-operator/operators/endpointmetrics/pkg/rendering/templates"
	operatorconfig "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/config"
	rendererutil "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/rendering"
	templatesutil "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/rendering/templates"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/util"
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

func Render(r *rendererutil.Renderer, c runtimeclient.Client, hubInfo *operatorconfig.HubInfo) ([]*unstructured.Unstructured, error) {

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
		if resources[idx].GetKind() == "StatefulSet" && resources[idx].GetName() == "prometheus-k8s" {
			obj := util.GetK8sObj(resources[idx].GetKind())
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(resources[idx].Object, obj)
			if err != nil {
				return nil, err
			}
			sts := obj.(*v1.StatefulSet)
			spec := &sts.Spec.Template.Spec
			spec.Containers[0].Image = Images[operatorconfig.PrometheusKey]
			spec.Containers[1].Image = Images[operatorconfig.KubeRbacProxyKey]
			spec.Containers[2].Image = Images[operatorconfig.ConfigmapReloaderKey]
			spec.ImagePullSecrets = []corev1.LocalObjectReference{
				{Name: os.Getenv(operatorconfig.PullSecret)},
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
		if resources[idx].GetKind() == "ConfigMap" && resources[idx].GetName() == "prometheus-k8s-config" {
			obj := util.GetK8sObj(resources[idx].GetKind())
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(resources[idx].Object, obj)
			if err != nil {
				return nil, err
			}
			cm := obj.(*corev1.ConfigMap)
			promConfig, exists := cm.Data["prometheus.yaml"]
			if !exists {
				return nil, fmt.Errorf("no key 'prometheus.yaml' found in the configmap: %s/%s", cm.GetNamespace(), cm.GetName())
			}
			// replace the hub alertmanager address
			hubAmEp := strings.TrimLeft(hubInfo.AlertmanagerEndpoint, "https://")
			promConfig = strings.ReplaceAll(promConfig, "_ALERTMANAGER_ENDPOINT_", hubAmEp)
			// replace the cluster ID with clusterName in hubInfo
			promConfig = strings.ReplaceAll(promConfig, "_CLUSTERID_", hubInfo.ClusterName)

			// replace the disabled metrics
			disabledMetricsSt, err := getDisabledMetrics(c)
			if err != nil {
				return nil, err
			}
			if disabledMetricsSt != "" {
				cm.Data["prometheus.yaml"] = strings.ReplaceAll(promConfig, "_DISABLED_METRICS_", disabledMetricsSt)
			}

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
