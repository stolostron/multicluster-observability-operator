// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package rendering

import (
	"fmt"
	"os"
	"strings"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/open-cluster-management/multicluster-observability-operator/operators/endpointmetrics/pkg/rendering/templates"
	operatorconfig "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/config"
	rendererutil "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/rendering"
	templatesutil "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/rendering/templates"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/util"
)

var (
	namespace = os.Getenv("WATCH_NAMESPACE")
	log       = logf.Log.WithName("renderer")
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
			cm.Data["prometheus.yaml"] = strings.ReplaceAll(promConfig, "_CLUSTERID_", hubInfo.ClusterName)

			unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
			if err != nil {
				return nil, err
			}
			resources[idx].Object = unstructuredObj
		}
	}

	return resources, nil
}
