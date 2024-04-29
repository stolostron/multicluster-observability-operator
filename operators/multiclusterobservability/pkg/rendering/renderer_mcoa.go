// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rendering

import (
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/kustomize/api/resource"

	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	rendererutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
)

func (r *MCORenderer) newMCOARenderer() {
	r.renderMCOAFns = map[string]rendererutil.RenderFn{
		"Deployment":             r.renderMCOADeployment,
		"ServiceAccount":         r.renderer.RenderNamespace,
		"ClusterRole":            r.renderer.RenderClusterRole,
		"ClusterRoleBinding":     r.renderer.RenderClusterRoleBinding,
		"ClusterManagementAddOn": r.renderer.RenderClusterManagementAddOn,
	}
}

func (r *MCORenderer) renderMCOADeployment(res *resource.Resource,
	namespace string, labels map[string]string) (*unstructured.Unstructured, error) {
	u, err := r.renderer.RenderNamespace(res, namespace, labels)
	if err != nil {
		return nil, err
	}
	obj := util.GetK8sObj(u.GetKind())
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj)
	if err != nil {
		return nil, err
	}
	crLabelKey := mcoconfig.GetCrLabelKey()
	dep := obj.(*v1.StatefulSet)
	dep.ObjectMeta.Labels[crLabelKey] = r.cr.Name
	dep.Spec.Selector.MatchLabels[crLabelKey] = r.cr.Name
	dep.Spec.Template.ObjectMeta.Labels[crLabelKey] = r.cr.Name
	dep.Name = mcoconfig.GetOperandName(mcoconfig.MultiClusterObservabilityAddon)
	dep.Spec.Replicas = mcoconfig.GetReplicas(mcoconfig.MultiClusterObservabilityAddon, r.cr.Spec.AdvancedConfig)

	spec := &dep.Spec.Template.Spec

	imagePullPolicy := mcoconfig.GetImagePullPolicy(r.cr.Spec)
	spec.Containers[0].ImagePullPolicy = imagePullPolicy
	spec.Containers[0].Resources = mcoconfig.GetResources(mcoconfig.MultiClusterObservabilityAddon, r.cr.Spec.AdvancedConfig)
	spec.NodeSelector = r.cr.Spec.NodeSelector
	spec.Tolerations = r.cr.Spec.Tolerations
	spec.ImagePullSecrets = []corev1.LocalObjectReference{
		{Name: mcoconfig.GetImagePullSecret(r.cr.Spec)},
	}

	spec.Containers[0].Image = mcoconfig.DefaultImgRepository + "/" + mcoconfig.MCOAImgName +
		":" + mcoconfig.DefaultImgTagSuffix
	found, image := mcoconfig.ReplaceImage(
		r.cr.Annotations,
		mcoconfig.DefaultImgRepository+"/"+mcoconfig.MCOAImgName,
		mcoconfig.MultiClusterObservabilityAddon)
	if found {
		spec.Containers[0].Image = image
	}

	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}

	return &unstructured.Unstructured{Object: unstructuredObj}, nil
}

func (r *MCORenderer) renderMCOATemplates(templates []*resource.Resource,
	namespace string, labels map[string]string) ([]*unstructured.Unstructured, error) {
	uobjs := []*unstructured.Unstructured{}
	for _, template := range templates {
		render, ok := r.renderMCOAFns[template.GetKind()]
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
