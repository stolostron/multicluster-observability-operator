// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rendering

import (
	"strconv"

	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	rendererutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
	"github.com/thanos-io/thanos/pkg/alert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/kustomize/api/resource"
	"sigs.k8s.io/yaml"
)

func (r *MCORenderer) newThanosRenderer() {
	r.renderThanosFns = map[string]rendererutil.RenderFn{
		"ServiceAccount":     r.renderer.RenderNamespace,
		"ConfigMap":          r.RenderThanosConfig,
		"ClusterRole":        r.renderer.RenderClusterRole,
		"ClusterRoleBinding": r.renderer.RenderClusterRoleBinding,
		"Secret":             r.renderer.RenderNamespace,
	}
}

func (r *MCORenderer) renderThanosTemplates(templates []*resource.Resource,
	namespace string, labels map[string]string,
) ([]*unstructured.Unstructured, error) {
	uobjs := []*unstructured.Unstructured{}
	for _, template := range templates {
		render, ok := r.renderThanosFns[template.GetKind()]
		if !ok {
			m, err := template.Map()
			if err != nil {
				return []*unstructured.Unstructured{}, err
			}
			uobjs = append(uobjs, &unstructured.Unstructured{Object: m})
			continue
		}
		uobj, err := render(template.DeepCopy(), namespace, labels)
		if err != nil {
			return []*unstructured.Unstructured{}, err
		}
		if uobj == nil {
			continue
		}
		uobjs = append(uobjs, uobj)
	}

	return uobjs, nil
}

func (r *MCORenderer) RenderThanosConfig(res *resource.Resource,
	namespace string, labels map[string]string,
) (*unstructured.Unstructured, error) {
	u, err := r.renderer.RenderNamespace(res, namespace, labels)
	if err != nil {
		return nil, err
	}
	if u.GetName() == "thanos-ruler-config" {
		obj := util.GetK8sObj(u.GetKind())
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj)
		if err != nil {
			return nil, err
		}
		cm := obj.(*corev1.ConfigMap)
		alertingConfig := &alert.AlertingConfig{}
		err = yaml.Unmarshal([]byte(cm.Data["config.yaml"]), alertingConfig)
		if err != nil {
			log.Error(err, "Failed to unmarshal data in configmap thanos-ruler-config")
			return nil, err
		}
		addr := []string{}
		replicas := mcoconfig.GetReplicas(mcoconfig.Alertmanager, r.cr.Spec.InstanceSize, r.cr.Spec.AdvancedConfig)
		for i := range *replicas {
			addr = append(addr, "observability-alertmanager-"+strconv.Itoa(int(i))+
				".alertmanager-operated.open-cluster-management-observability.svc:9095")
		}
		alertingConfig.Alertmanagers[0].EndpointsConfig.StaticAddresses = addr
		updateConfig, err := yaml.Marshal(alertingConfig)
		if err != nil {
			log.Error(err, "Failed to marshal data")
			return nil, err
		}
		cm.Data["config.yaml"] = string(updateConfig)

		unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			return nil, err
		}

		return &unstructured.Unstructured{Object: unstructuredObj}, nil
	}

	return u, nil
}
