// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package rendering

import (
	"fmt"
	"strconv"
	"strings"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/kustomize/v3/pkg/resource"

	obv1beta2 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta2"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
	mcoconfig "github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/rendering/patching"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/rendering/templates"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/util"
)

const (
	metadataErr = "failed to find metadata field"

	nsUpdateAnnoKey = "update-namespace"
)

var log = logf.Log.WithName("renderer")

type renderFn func(*resource.Resource) (*unstructured.Unstructured, error)

type Renderer struct {
	cr                    *obv1beta2.MultiClusterObservability
	renderFns             map[string]renderFn
	renderGrafanaFns      map[string]renderFn
	renderAlertManagerFns map[string]renderFn
	renderThanosFns       map[string]renderFn
}

func NewRenderer(multipleClusterMonitoring *obv1beta2.MultiClusterObservability) *Renderer {
	renderer := &Renderer{cr: multipleClusterMonitoring}
	renderer.renderFns = map[string]renderFn{
		"Deployment":            renderer.renderDeployments,
		"Service":               renderer.renderNamespace,
		"ServiceAccount":        renderer.renderNamespace,
		"ConfigMap":             renderer.renderNamespace,
		"ClusterRole":           renderer.renderClusterRole,
		"ClusterRoleBinding":    renderer.renderClusterRoleBinding,
		"Secret":                renderer.renderNamespace,
		"Role":                  renderer.renderNamespace,
		"RoleBinding":           renderer.renderNamespace,
		"Ingress":               renderer.renderNamespace,
		"PersistentVolumeClaim": renderer.renderNamespace,
	}
	renderer.newGranfanaRenderer()
	renderer.newAlertManagerRenderer()
	renderer.newThanosRenderer()
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

	//render alertmanager templates
	alertTemplates, err := templates.GetTemplateRenderer().GetAlertManagerTemplates(r.cr)
	if err != nil {
		return nil, err
	}
	alertResources, err := r.renderAlertManagerTemplates(alertTemplates)
	if err != nil {
		return nil, err
	}
	resources = append(resources, alertResources...)

	//render thanos templates
	thanosTemplates, err := templates.GetTemplateRenderer().GetThanosTemplates(r.cr)
	if err != nil {
		return nil, err
	}
	thanosResources, err := r.renderThanosTemplates(thanosTemplates)
	if err != nil {
		return nil, err
	}
	resources = append(resources, thanosResources...)

	for idx := range resources {
		if resources[idx].GetKind() == "Deployment" {
			obj := util.GetK8sObj(resources[idx].GetKind())
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(resources[idx].Object, obj)
			if err != nil {
				return nil, err
			}
			crLabelKey := config.GetCrLabelKey()
			dep := obj.(*v1.Deployment)
			dep.ObjectMeta.Labels[crLabelKey] = r.cr.Name
			dep.Spec.Selector.MatchLabels[crLabelKey] = r.cr.Name
			dep.Spec.Template.ObjectMeta.Labels[crLabelKey] = r.cr.Name
			dep.Name = mcoconfig.GetObjectPrefix() + "-" + dep.Name

			spec := &dep.Spec.Template.Spec
			spec.Containers[0].ImagePullPolicy = mcoconfig.GetImagePullPolicy(r.cr.Spec)
			spec.NodeSelector = r.cr.Spec.NodeSelector
			spec.Tolerations = r.cr.Spec.Tolerations
			spec.ImagePullSecrets = []corev1.LocalObjectReference{
				{Name: mcoconfig.GetImagePullSecret(r.cr.Spec)},
			}

			switch resources[idx].GetName() {

			case "grafana":
				found, image := mcoconfig.ReplaceImage(r.cr.Annotations, mcoconfig.GrafanaImgRepo, mcoconfig.GrafanaImgName)
				if found {
					spec.Containers[0].Image = image
				}
				found, image = mcoconfig.ReplaceImage(r.cr.Annotations, spec.Containers[1].Image,
					mcoconfig.GrafanaDashboardLoaderKey)
				if found {
					spec.Containers[1].Image = image
				}

			case "observatorium-operator":
				found, image := mcoconfig.ReplaceImage(r.cr.Annotations, spec.Containers[0].Image,
					mcoconfig.ObservatoriumOperatorImgName)
				if found {
					spec.Containers[0].Image = image
				}

			case "rbac-query-proxy":
				dep.Spec.Replicas = config.GetReplicas(config.RBACQueryProxy, r.cr.Spec.AdvancedConfig)
				updateProxySpec(spec, r.cr)
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

func updateProxySpec(spec *corev1.PodSpec, mco *obv1beta2.MultiClusterObservability) {
	found, image := mcoconfig.ReplaceImage(mco.Annotations, spec.Containers[0].Image,
		mcoconfig.RbacQueryProxyKey)
	if found {
		spec.Containers[0].Image = image
	}

	args := spec.Containers[0].Args
	for idx := range args {
		args[idx] = strings.Replace(args[idx], "{{MCO_NAMESPACE}}", mcoconfig.GetDefaultNamespace(), 1)
	}
	for idx := range spec.Volumes {
		if spec.Volumes[idx].Name == "ca-certs" {
			spec.Volumes[idx].Secret.SecretName = mcoconfig.ServerCerts
		}
		if spec.Volumes[idx].Name == "client-certs" {
			spec.Volumes[idx].Secret.SecretName = mcoconfig.GrafanaCerts
		}
	}
	spec.Containers[0].Resources = mcoconfig.GetResources(mcoconfig.RBACQueryProxy, mco.Spec.AdvancedConfig)
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

	res.SetNamespace(mcoconfig.GetDefaultNamespace())
	u := &unstructured.Unstructured{Object: res.Map()}
	return u, nil
}

func (r *Renderer) renderNamespace(res *resource.Resource) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{Object: res.Map()}
	if UpdateNamespace(u) {
		res.SetNamespace(mcoconfig.GetDefaultNamespace())
	}

	return u, nil
}

func (r *Renderer) renderClusterRole(res *resource.Resource) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{Object: res.Map()}

	labels := u.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	crLabelKey := config.GetCrLabelKey()
	labels[crLabelKey] = r.cr.Name
	u.SetLabels(labels)

	return u, nil
}

func (r *Renderer) renderClusterRoleBinding(res *resource.Resource) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{Object: res.Map()}

	labels := u.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	crLabelKey := config.GetCrLabelKey()
	labels[crLabelKey] = r.cr.Name
	u.SetLabels(labels)

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
		subject["namespace"] = mcoconfig.GetDefaultNamespace()
	}

	return u, nil
}

// UpdateNamespace checks for annotiation to update NS
func UpdateNamespace(u *unstructured.Unstructured) bool {
	annotations := u.GetAnnotations()
	v, ok := annotations[nsUpdateAnnoKey]
	if !ok {
		return true
	}
	ret, _ := strconv.ParseBool(v)
	return ret
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

	service["namespace"] = mcoconfig.GetDefaultNamespace()
	return u, nil
}
