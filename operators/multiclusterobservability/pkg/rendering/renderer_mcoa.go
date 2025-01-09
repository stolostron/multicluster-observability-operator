// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rendering

import (
	"fmt"

	prometheusv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	prometheusalpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/kustomize/api/resource"

	"github.com/imdario/mergo"
	obv1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	rendererutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering"
)

const (
	cmaoKind = "ClusterManagementAddOn"

	// AODC CustomizedVariable Names
	namePlatformLogsCollection        = "platformLogsCollection"
	namePlatformMetricsCollection     = "platformMetricsCollection"
	nameUserWorkloadLogsCollection    = "userWorkloadLogsCollection"
	nameUserWorkloadTracesCollection  = "userWorkloadTracesCollection"
	nameUserWorkloadInstrumentation   = "userWorkloadInstrumentation"
	nameUserWorkloadMetricsCollection = "userWorkloadMetricsCollection"
	nameMetricsHubHostname            = "metricsHubHostname"
)

type MCOARendererOptions struct {
	DisableCMAORender  bool
	MetricsHubHostname string
}

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
		fmt.Sprintf("%s/%s",
			mcoconfig.DefaultImgRepository,
			mcoconfig.MultiClusterObservabilityAddonImgName,
		),
		mcoconfig.MultiClusterObservabilityAddonImgKey,
	)
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
			Replicas: mcoconfig.GetReplicas(
				mcoconfig.MultiClusterObservabilityAddon,
				r.cr.Spec.InstanceSize,
				r.cr.Spec.AdvancedConfig,
			),
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

	mcoaResources := mcoconfig.GetResources(
		mcoconfig.MultiClusterObservabilityAddon,
		r.cr.Spec.InstanceSize,
		r.cr.Spec.AdvancedConfig,
	)

	patchContainer := &corev1.Container{
		Image:           img,
		ImagePullPolicy: mcoconfig.GetImagePullPolicy(r.cr.Spec),
		Resources:       mcoaResources,
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

	cma := &addonapiv1alpha1.ClusterManagementAddOn{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, cma); err != nil {
		return nil, err
	}

	if r.cr.Spec.Capabilities != nil {
		if len(cma.Spec.InstallStrategy.Placements) != 1 {
			return nil, fmt.Errorf("expected exactly one placement, got %d", len(cma.Spec.InstallStrategy.Placements))
		}
		globalConfigs := []addonapiv1alpha1.AddOnConfig{}
		if r.cr.Spec.Capabilities.Platform != nil && r.cr.Spec.Capabilities.Platform.Metrics.Collection.Enabled {
			globalConfigs = append(globalConfigs, []addonapiv1alpha1.AddOnConfig{
				{
					ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
						Group:    prometheusalpha1.SchemeGroupVersion.Group,
						Resource: prometheusalpha1.PrometheusAgentName,
					},
					ConfigReferent: addonapiv1alpha1.ConfigReferent{
						Name:      "acm-platform-metrics-collector-default",
						Namespace: mcoconfig.GetDefaultNamespace(),
					},
				},
				{
					ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
						Group:    prometheusalpha1.SchemeGroupVersion.Group,
						Resource: prometheusalpha1.ScrapeConfigName,
					},
					ConfigReferent: addonapiv1alpha1.ConfigReferent{
						Name:      "platform-metrics-default",
						Namespace: mcoconfig.GetDefaultNamespace(),
					},
				},
				{
					ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
						Group:    prometheusv1.SchemeGroupVersion.Group,
						Resource: prometheusv1.PrometheusRuleName,
					},
					ConfigReferent: addonapiv1alpha1.ConfigReferent{
						Name:      "platform-rules-default",
						Namespace: mcoconfig.GetDefaultNamespace(),
					},
				},
			}...)
		}

		if r.cr.Spec.Capabilities.UserWorkloads != nil && r.cr.Spec.Capabilities.UserWorkloads.Metrics.Collection.Enabled {
			globalConfigs = append(globalConfigs, []addonapiv1alpha1.AddOnConfig{
				{
					ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
						Group:    prometheusalpha1.SchemeGroupVersion.Group,
						Resource: prometheusalpha1.PrometheusAgentName,
					},
					ConfigReferent: addonapiv1alpha1.ConfigReferent{
						Name:      "acm-user-workload-metrics-collector-default",
						Namespace: mcoconfig.GetDefaultNamespace(),
					},
				},
			}...)
		}

		if r.cr.Spec.Capabilities.Platform != nil && r.cr.Spec.Capabilities.Platform.Logs.Collection.Enabled {
			globalConfigs = append(globalConfigs, []addonapiv1alpha1.AddOnConfig{
				{
					ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
						Group:    "observability.openshift.io",
						Resource: "clusterlogforwarders",
					},
					ConfigReferent: addonapiv1alpha1.ConfigReferent{
						Name:      "instance",
						Namespace: mcoconfig.GetDefaultNamespace(),
					},
				},
			}...)
		}

		if r.cr.Spec.Capabilities.UserWorkloads != nil {
			if r.cr.Spec.Capabilities.UserWorkloads.Traces.Collection.Collector.Enabled {
				globalConfigs = append(globalConfigs, []addonapiv1alpha1.AddOnConfig{
					{
						ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
							Group:    "opentelemetry.io",
							Resource: "opentelemetrycollectors",
						},
						ConfigReferent: addonapiv1alpha1.ConfigReferent{
							Name:      "instance",
							Namespace: mcoconfig.GetDefaultNamespace(),
						},
					},
				}...)
			}

			if r.cr.Spec.Capabilities.UserWorkloads.Traces.Collection.Instrumentation.Enabled {
				globalConfigs = append(globalConfigs, []addonapiv1alpha1.AddOnConfig{
					{
						ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
							Group:    "opentelemetry.io",
							Resource: "instrumentations",
						},
						ConfigReferent: addonapiv1alpha1.ConfigReferent{
							Name:      "instance",
							Namespace: mcoconfig.GetDefaultNamespace(),
						},
					},
				}...)
			}
		}

		cma.Spec.InstallStrategy.Placements[0].Configs = append(cma.Spec.InstallStrategy.Placements[0].Configs, globalConfigs...)
	}

	u.Object, err = runtime.DefaultUnstructuredConverter.ToUnstructured(cma)
	if err != nil {
		return nil, err
	}

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
			if cs.Platform.Logs.Collection.Enabled {
				fqdn := mcoconfig.GetMCOASupportedCRDFQDN(mcoconfig.ClusterLogForwarderCRDName)
				appendCustomVar(aodc, namePlatformLogsCollection, fqdn)
			}
			if cs.Platform.Metrics.Collection.Enabled {
				fqdn := mcoconfig.GetMCOASupportedCRDFQDN(mcoconfig.PrometheusAgentCRDName)
				appendCustomVar(aodc, namePlatformMetricsCollection, fqdn)
			}
		}

		if cs.UserWorkloads != nil {
			if cs.UserWorkloads.Logs.Collection.ClusterLogForwarder.Enabled {
				fqdn := mcoconfig.GetMCOASupportedCRDFQDN(mcoconfig.ClusterLogForwarderCRDName)
				appendCustomVar(aodc, nameUserWorkloadLogsCollection, fqdn)
			}
			if cs.UserWorkloads.Metrics.Collection.Enabled {
				fqdn := mcoconfig.GetMCOASupportedCRDFQDN(mcoconfig.PrometheusAgentCRDName)
				appendCustomVar(aodc, nameUserWorkloadMetricsCollection, fqdn)
			}
			if cs.UserWorkloads.Traces.Collection.Collector.Enabled {
				fqdn := mcoconfig.GetMCOASupportedCRDFQDN(mcoconfig.OpenTelemetryCollectorCRDName)
				appendCustomVar(aodc, nameUserWorkloadTracesCollection, fqdn)
			}
			if cs.UserWorkloads.Traces.Collection.Instrumentation.Enabled {
				fqdn := mcoconfig.GetMCOASupportedCRDFQDN(mcoconfig.InstrumentationCRDName)
				appendCustomVar(aodc, nameUserWorkloadInstrumentation, fqdn)
			}
		}

		if (cs.Platform != nil && cs.Platform.Metrics.Collection.Enabled) ||
			(cs.UserWorkloads != nil && cs.UserWorkloads.Metrics.Collection.Enabled) {
			if r.rendererOptions == nil || r.rendererOptions.MCOAOptions.MetricsHubHostname == "" {
				return nil, fmt.Errorf("MetricsHubHostname is required when metrics collection is enabled")
			}
			appendCustomVar(aodc, nameMetricsHubHostname, r.rendererOptions.MCOAOptions.MetricsHubHostname)
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
		// Skip rendering the ClusterManagementAddOn resource if,
		// MCOA is enabled && the MCOA redering options disable it.
		// The goal is for MCO to create this resource but then allow
		// users to manage it.
		if MCOAEnabled(r.cr) && template.GetKind() == cmaoKind &&
			r.rendererOptions != nil && r.rendererOptions.MCOAOptions.DisableCMAORender {
			continue
		}

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

func MCOAEnabled(cr *obv1beta2.MultiClusterObservability) bool {
	if cr.Spec.Capabilities == nil {
		return false
	}
	mcoaEnabled := false
	if cr.Spec.Capabilities.Platform != nil {
		mcoaEnabled = mcoaEnabled || cr.Spec.Capabilities.Platform.Logs.Collection.Enabled || cr.Spec.Capabilities.Platform.Metrics.Collection.Enabled
	}
	if cr.Spec.Capabilities.UserWorkloads != nil {
		mcoaEnabled = mcoaEnabled || cr.Spec.Capabilities.UserWorkloads.Logs.Collection.ClusterLogForwarder.Enabled
		mcoaEnabled = mcoaEnabled || cr.Spec.Capabilities.UserWorkloads.Traces.Collection.Collector.Enabled
		mcoaEnabled = mcoaEnabled || cr.Spec.Capabilities.UserWorkloads.Traces.Collection.Instrumentation.Enabled
		mcoaEnabled = mcoaEnabled || cr.Spec.Capabilities.UserWorkloads.Metrics.Collection.Enabled
	}
	return mcoaEnabled
}

func MCOAPlatformMetricsEnabled(cr *obv1beta2.MultiClusterObservability) bool {
	if cr.Spec.Capabilities == nil {
		return false
	}

	if cr.Spec.Capabilities.Platform != nil && cr.Spec.Capabilities.Platform.Metrics.Collection.Enabled {
		return true
	}

	return false
}
