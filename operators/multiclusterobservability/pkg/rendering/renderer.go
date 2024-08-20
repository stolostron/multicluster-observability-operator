// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rendering

import (
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	obv1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/rendering/templates"
	rendererutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering"
	templatesutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering/templates"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logf.Log.WithName("renderer")

type MCORenderer struct {
	kubeClient            client.Client
	renderer              *rendererutil.Renderer
	cr                    *obv1beta2.MultiClusterObservability
	renderGrafanaFns      map[string]rendererutil.RenderFn
	renderAlertManagerFns map[string]rendererutil.RenderFn
	renderThanosFns       map[string]rendererutil.RenderFn
	renderProxyFns        map[string]rendererutil.RenderFn
	renderMCOAFns         map[string]rendererutil.RenderFn
}

func NewMCORenderer(multipleClusterMonitoring *obv1beta2.MultiClusterObservability, kubeClient client.Client) *MCORenderer {
	mcoRenderer := &MCORenderer{
		renderer:   rendererutil.NewRenderer(),
		cr:         multipleClusterMonitoring,
		kubeClient: kubeClient,
	}
	mcoRenderer.newGranfanaRenderer()
	mcoRenderer.newAlertManagerRenderer()
	mcoRenderer.newThanosRenderer()
	mcoRenderer.newProxyRenderer()
	mcoRenderer.newMCOARenderer()
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
		mcoconfig.GetCrLabelKey(): r.cr.Name,
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

	// load and render multicluster-observability-addon templates
	mcoaTemplates, err := templates.GetOrLoadMCOATemplates(templatesutil.GetTemplateRenderer())
	if err != nil {
		return nil, err
	}
	mcoaResources, err := r.renderMCOATemplates(mcoaTemplates, namespace, labels)
	if err != nil {
		return nil, err
	}
	resources = append(resources, mcoaResources...)

	for idx := range resources {
		if resources[idx].GetKind() == "Deployment" {
			obj := util.GetK8sObj(resources[idx].GetKind())
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(resources[idx].Object, obj)
			if err != nil {
				return nil, err
			}
			crLabelKey := mcoconfig.GetCrLabelKey()
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
				dep.Name = mcoconfig.GetOperandName(mcoconfig.ObservatoriumOperator)

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
