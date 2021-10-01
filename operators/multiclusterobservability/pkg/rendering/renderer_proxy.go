// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package rendering

import (
	"strings"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/kustomize/v3/pkg/resource"

	"github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	mcoconfig "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	rendererutil "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/rendering"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/util"
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
	namespace string, labels map[string]string) (*unstructured.Unstructured, error) {
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
	dep.ObjectMeta.Labels[crLabelKey] = r.cr.Name
	dep.Spec.Selector.MatchLabels[crLabelKey] = r.cr.Name
	dep.Spec.Template.ObjectMeta.Labels[crLabelKey] = r.cr.Name
	dep.Name = mcoconfig.GetOperandName(config.RBACQueryProxy)
	dep.Spec.Replicas = config.GetReplicas(config.RBACQueryProxy, r.cr.Spec.AdvancedConfig)

	spec := &dep.Spec.Template.Spec
	spec.Containers[0].ImagePullPolicy = mcoconfig.GetImagePullPolicy(r.cr.Spec)
	args0 := spec.Containers[0].Args
	for idx := range args0 {
		args0[idx] = strings.Replace(args0[idx], "{{MCO_NAMESPACE}}", mcoconfig.GetDefaultNamespace(), 1)
		args0[idx] = strings.Replace(args0[idx], "{{OBSERVATORIUM_NAME}}", mcoconfig.GetOperandName(mcoconfig.Observatorium), 1)
	}
	spec.Containers[0].Args = args0
	spec.Containers[0].Resources = mcoconfig.GetResources(mcoconfig.RBACQueryProxy, r.cr.Spec.AdvancedConfig)

	spec.Containers[1].ImagePullPolicy = mcoconfig.GetImagePullPolicy(r.cr.Spec)
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

	spec.Containers[0].Image = config.DefaultImgRepository + "/" + config.RBACQueryProxyImgName +
		":" + config.DefaultImgTagSuffix
	//replace the proxy image
	found, image := mcoconfig.ReplaceImage(
		r.cr.Annotations,
		spec.Containers[0].Image,
		mcoconfig.RBACQueryProxyKey)
	if found {
		spec.Containers[0].Image = image
	}

	// the oauth-proxy image only exists in mch-image-manifest configmap
	// pass nil annotation to make sure oauth-proxy overrided from mch-image-manifest
	found, image = mcoconfig.ReplaceImage(nil, mcoconfig.OauthProxyImgRepo,
		mcoconfig.OauthProxyKey)
	if found {
		spec.Containers[1].Image = image
	}

	for idx := range spec.Volumes {
		if spec.Volumes[idx].Name == "ca-certs" {
			spec.Volumes[idx].Secret.SecretName = mcoconfig.ServerCerts
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
	namespace string, labels map[string]string) (*unstructured.Unstructured, error) {
	u, err := r.renderer.RenderNamespace(res, namespace, labels)
	if err != nil {
		return nil, err
	}

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
	namespace string, labels map[string]string) ([]*unstructured.Unstructured, error) {
	uobjs := []*unstructured.Unstructured{}
	for _, template := range templates {
		render, ok := r.renderProxyFns[template.GetKind()]
		if !ok {
			uobjs = append(uobjs, &unstructured.Unstructured{Object: template.Map()})
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
