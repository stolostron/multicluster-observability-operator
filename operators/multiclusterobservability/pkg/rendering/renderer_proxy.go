// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rendering

import (
	"fmt"
	"strings"

	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	rendererutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/kustomize/api/resource"
)

func (r *MCORenderer) newProxyRenderer() {
	r.renderProxyFns = map[string]rendererutil.RenderFn{
		"Deployment":            r.renderProxyDeployment,
		"Service":               r.renderer.RenderNamespace,
		"ServiceAccount":        r.renderer.RenderNamespace,
		"ConfigMap":             r.renderer.RenderNamespace,
		"ClusterRole":           r.renderer.RenderClusterRole,
		"ClusterRoleBinding":    r.renderer.RenderClusterRoleBinding,
		"Secret":                r.renderProxySecret,
		"Role":                  r.renderer.RenderNamespace,
		"RoleBinding":           r.renderer.RenderNamespace,
		"Ingress":               r.renderer.RenderNamespace,
		"PersistentVolumeClaim": r.renderer.RenderNamespace,
	}
}

func (r *MCORenderer) renderProxyDeployment(res *resource.Resource,
	namespace string, labels map[string]string,
) (*unstructured.Unstructured, error) {
	u, err := r.renderer.RenderDeployments(res, namespace, labels)
	if err != nil {
		return nil, err
	}
	obj := util.GetK8sObj(u.GetKind())
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj)
	if err != nil {
		return nil, err
	}

	crLabelKey := mcoconfig.GetCrLabelKey()
	dep := obj.(*v1.Deployment)
	dep.Labels[crLabelKey] = r.cr.Name
	dep.Spec.Selector.MatchLabels[crLabelKey] = r.cr.Name
	dep.Spec.Template.Labels[crLabelKey] = r.cr.Name
	dep.Name = mcoconfig.GetOperandName(mcoconfig.RBACQueryProxy)
	dep.Spec.Replicas = mcoconfig.GetReplicas(mcoconfig.RBACQueryProxy, r.cr.Spec.InstanceSize, r.cr.Spec.AdvancedConfig)

	spec := &dep.Spec.Template.Spec
	imagePullPolicy := mcoconfig.GetImagePullPolicy(r.cr.Spec)
	spec.Containers[0].ImagePullPolicy = imagePullPolicy
	args0 := spec.Containers[0].Args
	for idx := range args0 {
		args0[idx] = strings.Replace(args0[idx], "{{MCO_NAMESPACE}}", mcoconfig.GetDefaultNamespace(), 1)
		args0[idx] = strings.Replace(
			args0[idx],
			"{{OBSERVATORIUM_NAME}}",
			mcoconfig.GetOperandName(mcoconfig.Observatorium),
			1,
		)
	}
	queryTimeout := mcoconfig.GetGrafanaQueryTimeout()
	args0 = append(args0, fmt.Sprintf("--proxy-timeout=%s", queryTimeout))
	spec.Containers[0].Args = args0
	spec.Containers[0].Resources = mcoconfig.GetResources(mcoconfig.RBACQueryProxy, r.cr.Spec.InstanceSize, r.cr.Spec.AdvancedConfig)

	spec.Containers[1].ImagePullPolicy = imagePullPolicy
	args1 := spec.Containers[1].Args
	for idx := range args1 {
		args1[idx] = strings.Replace(args1[idx], "{{MCO_NAMESPACE}}", mcoconfig.GetDefaultNamespace(), 1)
	}
	spec.Containers[1].Args = args1
	spec.NodeSelector = r.cr.Spec.NodeSelector
	spec.Tolerations = r.cr.Spec.Tolerations
	spec.ImagePullSecrets = []corev1.LocalObjectReference{
		{Name: mcoconfig.GetImagePullSecret(r.cr.Spec)},
	}

	spec.Containers[0].Image = mcoconfig.DefaultImgRepository + "/" + mcoconfig.RBACQueryProxyImgName +
		":" + mcoconfig.DefaultImgTagSuffix
	// replace the proxy image
	found, image := mcoconfig.ReplaceImage(
		r.cr.Annotations,
		spec.Containers[0].Image,
		mcoconfig.RBACQueryProxyKey)
	if found {
		spec.Containers[0].Image = image
	}

	// If we're on OCP and has imagestreams, we always want the oauth image
	// from the imagestream, and fail the reconcile if we don't find it.
	// If we're on non-OCP (tests) we use the base template image
	found, image = mcoconfig.GetOauthProxyImage(r.imageClient)
	if found {
		spec.Containers[1].Image = image
	} else if r.HasImagestream() {
		return nil, fmt.Errorf("failed to get OAuth image for alertmanager")
	}

	for idx := range spec.Volumes {
		if spec.Volumes[idx].Name == "ca-certs" {
			spec.Volumes[idx].Secret.SecretName = mcoconfig.ServerCACerts
		}
		if spec.Volumes[idx].Name == "client-certs" {
			spec.Volumes[idx].Secret.SecretName = mcoconfig.GrafanaCerts
		}
	}

	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}

	return &unstructured.Unstructured{Object: unstructuredObj}, nil
}

func (r *MCORenderer) renderProxySecret(res *resource.Resource,
	namespace string, labels map[string]string,
) (*unstructured.Unstructured, error) {
	u, err := r.renderer.RenderNamespace(res, namespace, labels)
	if err != nil {
		return nil, err
	}

	// #nosec G101 -- Not a hardcoded credential.
	if u.GetName() == "rbac-proxy-cookie-secret" {
		obj := util.GetK8sObj(u.GetKind())
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj)
		if err != nil {
			return nil, err
		}
		srt := obj.(*corev1.Secret)
		p, err := util.GeneratePassword(16)
		if err != nil {
			return nil, err
		}
		srt.Data["session_secret"] = []byte(p)
		unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			return nil, err
		}
		return &unstructured.Unstructured{Object: unstructuredObj}, nil
	}

	return u, nil
}

func (r *MCORenderer) renderProxyTemplates(templates []*resource.Resource,
	namespace string, labels map[string]string,
) ([]*unstructured.Unstructured, error) {
	uobjs := []*unstructured.Unstructured{}
	for _, template := range templates {
		render, ok := r.renderProxyFns[template.GetKind()]
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
