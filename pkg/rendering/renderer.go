// Copyright (c) 2020 Red Hat, Inc.

package rendering

import (
	"fmt"
	"strconv"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/kustomize/v3/pkg/resource"

	monitoringv1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	mcoconfig "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/rendering/patching"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/rendering/templates"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/util"
)

const (
	metadataErr = "failed to find metadata field"

	nsUpdateAnnoKey = "update-namespace"
	crLabelKey      = "observability.open-cluster-management.io/name"
)

var log = logf.Log.WithName("renderer")

type renderFn func(*resource.Resource) (*unstructured.Unstructured, error)

type Renderer struct {
	cr               *monitoringv1.MultiClusterObservability
	renderFns        map[string]renderFn
	renderGrafanaFns map[string]renderFn
	renderMinioFns   map[string]renderFn
}

func NewRenderer(multipleClusterMonitoring *monitoringv1.MultiClusterObservability) *Renderer {
	renderer := &Renderer{cr: multipleClusterMonitoring}
	renderer.renderFns = map[string]renderFn{
		"Deployment":            renderer.renderDeployments,
		"Service":               renderer.renderNamespace,
		"ServiceAccount":        renderer.renderNamespace,
		"ConfigMap":             renderer.renderNamespace,
		"ClusterRoleBinding":    renderer.renderClusterRoleBinding,
		"Secret":                renderer.renderNamespace,
		"Role":                  renderer.renderNamespace,
		"RoleBinding":           renderer.renderNamespace,
		"Ingress":               renderer.renderNamespace,
		"PersistentVolumeClaim": renderer.renderNamespace,
	}
	renderer.newGranfanaRenderer()
	renderer.newMinioRenderer()
	return renderer
}

func (r *Renderer) Render(c runtimeclient.Client) ([]*unstructured.Unstructured, error) {

	genericTemplates, err := templates.GetTemplateRenderer().GetTemplates(r.cr)
	if err != nil {
		return nil, err
	}
	resources, err := r.renderTemplates(genericTemplates)
	if err != nil {
		return nil, err
	}

	// render grafana templates
	grafanaTemplates, err := templates.GetTemplateRenderer().GetGrafanaTemplates(r.cr)
	if err != nil {
		return nil, err
	}
	grafanaResources, err := r.renderGrafanaTemplates(grafanaTemplates)
	if err != nil {
		return nil, err
	}
	resources = append(resources, grafanaResources...)

	// render grafana templates
	minioTemplates, err := templates.GetTemplateRenderer().GetMinioTemplates(r.cr)
	if err != nil {
		return nil, err
	}
	minioResources, err := r.renderMinioTemplates(minioTemplates)
	if err != nil {
		return nil, err
	}

	resources = append(resources, minioResources...)
	for idx, _ := range resources {
		if resources[idx].GetKind() == "PersistentVolumeClaim" {
			obj := util.GetK8sObj(resources[idx].GetKind())
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(resources[idx].Object, obj)
			if err != nil {
				return nil, err
			}

			spec := &obj.(*corev1.PersistentVolumeClaim).Spec
			storageClass := r.cr.Spec.StorageClass
			spec.StorageClassName = &storageClass
			spec.Resources.Requests[corev1.ResourceStorage] = k8sresource.MustParse(r.cr.Spec.StorageSize.String())
			unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
			if err != nil {
				return nil, err
			}

			resources[idx].Object = unstructuredObj
		}

		if resources[idx].GetKind() == "Deployment" {
			obj := util.GetK8sObj(resources[idx].GetKind())
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(resources[idx].Object, obj)
			if err != nil {
				return nil, err
			}
			dep := obj.(*v1.Deployment)
			dep.ObjectMeta.Labels[crLabelKey] = r.cr.Name
			dep.Spec.Selector.MatchLabels[crLabelKey] = r.cr.Name
			dep.Spec.Template.ObjectMeta.Labels[crLabelKey] = r.cr.Name

			spec := &dep.Spec.Template.Spec
			spec.Containers[0].ImagePullPolicy = r.cr.Spec.ImagePullPolicy
			spec.NodeSelector = r.cr.Spec.NodeSelector
			spec.ImagePullSecrets = []corev1.LocalObjectReference{
				{Name: r.cr.Spec.ImagePullSecret},
			}
			grafanaImgRepo := mcoconfig.GrafanaImgRepo
			grafanaImgTagSuffix := mcoconfig.GrafanaImgTagSuffix
			minioImgRepo := mcoconfig.MinioImgRepo
			minioImgTagSuffix := mcoconfig.MinioImgTagSuffix
			observatoriumImgRepo := mcoconfig.ObservatoriumImgRepo
			observatoriumImgTagSuffix := mcoconfig.ObservatoriumImgTagSuffix
			if mcoconfig.IsNeededReplacement(r.cr.Annotations) {
				imgRepo := util.GetAnnotation(r.cr, mcoconfig.AnnotationKeyImageRepository)
				imgVersion := util.GetAnnotation(r.cr, mcoconfig.AnnotationKeyImageTagSuffix)
				if imgVersion == "" {
					imgVersion = mcoconfig.DefaultImgTagSuffix
				}
				grafanaImgRepo = imgRepo
				grafanaImgTagSuffix = imgVersion
				minioImgRepo = imgRepo
				minioImgTagSuffix = imgVersion
				observatoriumImgRepo = imgRepo
				observatoriumImgTagSuffix = imgVersion
			}

			switch resources[idx].GetName() {

			case "grafana":
				spec.Containers[0].Image = grafanaImgRepo + "/grafana:" + grafanaImgTagSuffix

			case "minio":
				spec.Containers[0].Image = minioImgRepo + "/minio:" + minioImgTagSuffix
				updateMinioSpecEnv(spec)

			case "observatorium-operator":
				spec.Containers[0].Image = observatoriumImgRepo + "/observatorium-operator:" + observatoriumImgTagSuffix

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

func updateMinioSpecEnv(spec *corev1.PodSpec) {
	for i, env := range spec.Containers[0].Env {
		if env.Name == "MINIO_ACCESS_KEY" {
			spec.Containers[0].Env[i].Value = mcoconfig.DefaultObjStorageAccesskey
		}

		if env.Name == "MINIO_SECRET_KEY" {
			spec.Containers[0].Env[i].Value = mcoconfig.DefaultObjStorageSecretkey
		}
	}
}

func (r *Renderer) renderTemplates(templates []*resource.Resource) ([]*unstructured.Unstructured, error) {
	uobjs := []*unstructured.Unstructured{}
	for _, template := range templates {
		render, ok := r.renderFns[template.GetKind()]
		if !ok {
			uobjs = append(uobjs, &unstructured.Unstructured{Object: template.Map()})
			continue
		}
		uobj, err := render(template.DeepCopy())
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

func (r *Renderer) renderDeployments(res *resource.Resource) (*unstructured.Unstructured, error) {
	err := patching.ApplyGlobalPatches(res, r.cr)
	if err != nil {
		return nil, err
	}

	res.SetNamespace(r.cr.Namespace)
	u := &unstructured.Unstructured{Object: res.Map()}
	return u, nil
}

func (r *Renderer) renderNamespace(res *resource.Resource) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{Object: res.Map()}
	if UpdateNamespace(u) {
		res.SetNamespace(r.cr.Namespace)
	}

	return u, nil
}

func (r *Renderer) renderClusterRoleBinding(res *resource.Resource) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{Object: res.Map()}

	subjects, ok := u.Object["subjects"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to find clusterrolebinding subjects field")
	}
	subject := subjects[0].(map[string]interface{})
	kind := subject["kind"]
	if kind == "Group" {
		return u, nil
	}

	if UpdateNamespace(u) {
		subject["namespace"] = r.cr.Namespace
	}

	return u, nil
}

// UpdateNamespace checks for annotiation to update NS
func UpdateNamespace(u *unstructured.Unstructured) bool {
	metadata, ok := u.Object["metadata"].(map[string]interface{})
	updateNamespace := true
	if ok {
		annotations, ok := metadata["annotations"].(map[string]interface{})
		if ok && annotations != nil {
			if annotations[nsUpdateAnnoKey] != nil && annotations[nsUpdateAnnoKey].(string) != "" {
				updateNamespace, _ = strconv.ParseBool(annotations[nsUpdateAnnoKey].(string))
			}
		}
	}
	return updateNamespace
}

func (r *Renderer) renderMutatingWebhookConfiguration(res *resource.Resource) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{Object: res.Map()}
	webooks, ok := u.Object["webhooks"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to find webhooks spec field")
	}
	webhook := webooks[0].(map[string]interface{})
	clientConfig := webhook["clientConfig"].(map[string]interface{})
	service := clientConfig["service"].(map[string]interface{})

	service["namespace"] = r.cr.Namespace
	return u, nil
}
