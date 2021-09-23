// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package rendering

import (
	"fmt"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/kustomize/v3/pkg/resource"

	obv1beta2 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	mcoconfig "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/rendering/templates"
	rendererutil "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/rendering"
	templatesutil "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/rendering/templates"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/util"
)

var log = logf.Log.WithName("renderer")

type MCORenderer struct {
	renderer              *rendererutil.Renderer
	cr                    *obv1beta2.MultiClusterObservability
	renderGrafanaFns      map[string]rendererutil.RenderFn
	renderAlertManagerFns map[string]rendererutil.RenderFn
	renderThanosFns       map[string]rendererutil.RenderFn
	renderProxyFns        map[string]rendererutil.RenderFn
}

func NewMCORenderer(multipleClusterMonitoring *obv1beta2.MultiClusterObservability) *MCORenderer {
	mcoRenderer := &MCORenderer{
		renderer: rendererutil.NewRenderer(),
		cr:       multipleClusterMonitoring,
	}
	mcoRenderer.newGranfanaRenderer()
	mcoRenderer.newAlertManagerRenderer()
	mcoRenderer.newThanosRenderer()
	mcoRenderer.newProxyRenderer()
	return mcoRenderer
}

func (r *MCORenderer) Render() ([]*unstructured.Unstructured, error) {
	// load and render generic templates
	genericTemplates, err := templates.GetOrLoadGenericTemplates(templatesutil.GetTemplateRenderer())
	if err != nil {
		return nil, err
	}
	namespace := mcoconfig.GetDefaultNamespace()
	labels := map[string]string{
		config.GetCrLabelKey(): r.cr.Name,
	}
	resources, err := r.renderer.RenderTemplates(genericTemplates, namespace, labels)
	if err != nil {
		return nil, err
	}

	// load and render grafana templates
	grafanaTemplates, err := templates.GetOrLoadGrafanaTemplates(templatesutil.GetTemplateRenderer())
	if err != nil {
		return nil, err
	}
	grafanaResources, err := r.renderGrafanaTemplates(grafanaTemplates, namespace, labels)
	if err != nil {
		return nil, err
	}
	resources = append(resources, grafanaResources...)

	//load and render alertmanager templates
	alertTemplates, err := templates.GetOrLoadAlertManagerTemplates(templatesutil.GetTemplateRenderer())
	if err != nil {
		return nil, err
	}
	alertResources, err := r.renderAlertManagerTemplates(alertTemplates, namespace, labels)
	if err != nil {
		return nil, err
	}
	resources = append(resources, alertResources...)

	// load and render thanos templates
	thanosTemplates, err := templates.GetOrLoadThanosTemplates(templatesutil.GetTemplateRenderer())
	if err != nil {
		return nil, err
	}
	thanosResources, err := r.renderThanosTemplates(thanosTemplates, namespace, labels)
	if err != nil {
		return nil, err
	}
	resources = append(resources, thanosResources...)

	// load and render proxy templates
	proxyTemplates, err := templates.GetOrLoadProxyTemplates(templatesutil.GetTemplateRenderer())
	if err != nil {
		return nil, err
	}
	proxyResources, err := r.renderProxyTemplates(proxyTemplates, namespace, labels)
	if err != nil {
		return nil, err
	}
	resources = append(resources, proxyResources...)

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

			spec := &dep.Spec.Template.Spec
			spec.Containers[0].ImagePullPolicy = mcoconfig.GetImagePullPolicy(r.cr.Spec)
			spec.NodeSelector = r.cr.Spec.NodeSelector
			spec.Tolerations = r.cr.Spec.Tolerations
			spec.ImagePullSecrets = []corev1.LocalObjectReference{
				{Name: mcoconfig.GetImagePullSecret(r.cr.Spec)},
			}

			switch resources[idx].GetName() {

			case "observatorium-operator":
				spec.Containers[0].Image = mcoconfig.DefaultImgRepository + "/" +
					mcoconfig.ObservatoriumOperatorImgName + ":" + mcoconfig.DefaultImgTagSuffix

				found, image := mcoconfig.ReplaceImage(r.cr.Annotations, spec.Containers[0].Image,
					mcoconfig.ObservatoriumOperatorImgKey)
				if found {
					spec.Containers[0].Image = image
				}
				dep.Name = mcoconfig.GetOperandName(config.ObservatoriumOperator)

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

func (r *MCORenderer) renderMutatingWebhookConfiguration(res *resource.Resource) (*unstructured.Unstructured, error) {
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
