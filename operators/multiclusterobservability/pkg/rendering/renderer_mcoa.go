// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rendering

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/kustomize/api/resource"

	"github.com/imdario/mergo"
	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	rendererutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering"
)

const (
	// AODC CustomizedVariable Names
	namePlatformLogsCollection       = "platformLogsCollection"
	nameUserWorkloadLogsCollection   = "userWorkloadLogsCollection"
	nameUserWorkloadTracesCollection = "userWorkloadTracesCollection"
	nameUserWorkloadInstrumentation  = "userWorkloadInstrumentation"

	// AODC CustomizedVariable Values
	clfV1         = "clusterlogforwarders.v1.logging.openshift.io"
	otelV1beta1   = "opentelemetrycollectors.v1beta1.opentelemetry.io"
	instrV1alpha1 = "instrumentations.v1alpha1.opentelemetry.io"
)

func (r *MCORenderer) newMCOARenderer() {
	r.renderMCOAFns = map[string]rendererutil.RenderFn{
		"AddOnDeploymentConfig":  r.renderAddonDeploymentConfig,
		"ClusterManagementAddOn": r.renderClusterManagementAddOn,
		"Deployment":             r.renderMCOADeployment,
		"ServiceAccount":         r.renderer.RenderNamespace,
		"ClusterRole":            r.renderer.RenderClusterRole,
		"ClusterRoleBinding":     r.renderer.RenderClusterRoleBinding,
	}
}

func (r *MCORenderer) renderMCOADeployment(
	res *resource.Resource,
	namespace string,
	labels map[string]string,
) (*unstructured.Unstructured, error) {
	u, err := r.renderer.RenderNamespace(res, namespace, labels)
	if err != nil {
		return nil, err
	}

	obj := &appsv1.Deployment{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj); err != nil {
		return nil, err
	}
	crLabelKey := mcoconfig.GetCrLabelKey()

	img := fmt.Sprintf("%s/%s:%s",
		mcoconfig.MultiClusterObservabilityAddonImgRepo,
		mcoconfig.MultiClusterObservabilityAddonImgName,
		mcoconfig.MultiClusterObservabilityAddonImgTagSuffix,
	)
	found, replacement := mcoconfig.ReplaceImage(
		r.cr.Annotations,
		mcoconfig.DefaultImgRepository,
		mcoconfig.MultiClusterObservabilityAddonImgKey)
	if found {
		img = replacement
	}

	patch := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				crLabelKey: r.cr.Name,
			},
			Name: mcoconfig.GetOperandName(mcoconfig.MultiClusterObservabilityAddon),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: mcoconfig.GetReplicas(mcoconfig.MultiClusterObservabilityAddon, r.cr.Spec.AdvancedConfig),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					crLabelKey: r.cr.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						crLabelKey: r.cr.Name,
					},
				},
				Spec: corev1.PodSpec{
					NodeSelector: r.cr.Spec.NodeSelector,
					Tolerations:  r.cr.Spec.Tolerations,
					ImagePullSecrets: []corev1.LocalObjectReference{
						{
							Name: mcoconfig.GetImagePullSecret(r.cr.Spec),
						},
					},
				},
			},
		},
	}

	if err := mergo.Merge(obj, patch); err != nil {
		return nil, err
	}

	patchContainer := &corev1.Container{
		Image:           img,
		ImagePullPolicy: mcoconfig.GetImagePullPolicy(r.cr.Spec),
		Resources:       mcoconfig.GetResources(mcoconfig.MultiClusterObservabilityAddon, r.cr.Spec.AdvancedConfig),
	}

	if err := mergo.Merge(&obj.Spec.Template.Spec.Containers[0], patchContainer, mergo.WithOverride); err != nil {
		return nil, err
	}

	uObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}

	return &unstructured.Unstructured{Object: uObj}, nil
}

func (r *MCORenderer) renderClusterManagementAddOn(
	res *resource.Resource,
	namespace string,
	labels map[string]string,
) (*unstructured.Unstructured, error) {
	m, err := res.Map()
	if err != nil {
		return nil, err
	}
	u := &unstructured.Unstructured{Object: m}

	cLabels := u.GetLabels()
	if cLabels == nil {
		cLabels = make(map[string]string)
	}
	for k, v := range labels {
		cLabels[k] = v
	}
	u.SetLabels(cLabels)

	return u, nil
}

func (r *MCORenderer) renderAddonDeploymentConfig(
	res *resource.Resource,
	namespace string,
	labels map[string]string,
) (*unstructured.Unstructured, error) {
	u, err := r.renderer.RenderNamespace(res, namespace, labels)
	if err != nil {
		return nil, err
	}

	if cs := r.cr.Spec.Capabilities; cs != nil {
		aodc := &addonapiv1alpha1.AddOnDeploymentConfig{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, aodc); err != nil {
			return nil, err
		}

		appendCustomVar := func(aodc *addonapiv1alpha1.AddOnDeploymentConfig, name, value string) {
			aodc.Spec.CustomizedVariables = append(
				aodc.Spec.CustomizedVariables,
				addonapiv1alpha1.CustomizedVariable{Name: name, Value: value},
			)
		}

		if cs.Platform != nil {
			if cs.Platform.Logs.Enabled {
				appendCustomVar(aodc, namePlatformLogsCollection, clfV1)
			}
		}

		if cs.UserWorkloads != nil {
			if cs.UserWorkloads.Logs.ClusterLogForwarder.Enabled {
				appendCustomVar(aodc, nameUserWorkloadLogsCollection, clfV1)
			}
			if cs.UserWorkloads.Traces.OpenTelemetryCollector.Enabled {
				appendCustomVar(aodc, nameUserWorkloadTracesCollection, otelV1beta1)
			}
			if cs.UserWorkloads.Traces.Instrumentation.Enabled {
				appendCustomVar(aodc, nameUserWorkloadInstrumentation, instrV1alpha1)
			}
		}

		u.Object, err = runtime.DefaultUnstructuredConverter.ToUnstructured(aodc)
		if err != nil {
			return nil, err
		}
	}

	cLabels := u.GetLabels()
	if cLabels == nil {
		cLabels = make(map[string]string)
	}
	for k, v := range labels {
		cLabels[k] = v
	}
	u.SetLabels(cLabels)

	return u, nil
}

func (r *MCORenderer) renderMCOATemplates(
	templates []*resource.Resource,
	namespace string,
	labels map[string]string,
) ([]*unstructured.Unstructured, error) {
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
