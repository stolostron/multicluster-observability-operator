// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rendering

import (
	"fmt"
	"maps"
	"net/url"

	"github.com/imdario/mergo"
	obv1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	rendererutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/kustomize/api/resource"
)

const (
	cmaoKind = "ClusterManagementAddOn"

	// AODC CustomizedVariable Names
	namePlatformLogsCollection        = "platformLogsCollection"
	namePlatformIncidentDetection     = "platformIncidentDetection"
	uipluginsCRDFQDN                  = "uiplugins.v1alpha1.observability.openshift.io"
	namePlatformMetricsCollection     = "platformMetricsCollection"
	nameUserWorkloadLogsCollection    = "userWorkloadLogsCollection"
	nameUserWorkloadTracesCollection  = "userWorkloadTracesCollection"
	nameUserWorkloadInstrumentation   = "userWorkloadInstrumentation"
	nameUserWorkloadMetricsCollection = "userWorkloadMetricsCollection"
	nameMetricsHubHostname            = "metricsHubHostname"
	nameMetricsAlertManagerHostname   = "metricsAlertManagerHostname"
	namePLatformMetricsUI             = "platformMetricsUI"

	grafanaMCOAHomeDashboardID = "89eaec849a6e4837a619fb0540c22b13"
	grafanaLink                = "/d/" + grafanaMCOAHomeDashboardID + "/acm-clusters-overview"
)

type MCOARendererOptions struct {
	DisableCMAORender              bool
	MetricsHubHostname             string
	MetricsHubAlertmanagerHostname string
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

	if r.cr.Spec.Capabilities != nil && r.cr.Spec.Capabilities.AddonManager != nil {
		if r.cr.Spec.Capabilities.AddonManager.Resources != nil {
			mcoaResources = *r.cr.Spec.Capabilities.AddonManager.Resources
		}
	}

	patchContainer := &corev1.Container{
		Image:           img,
		ImagePullPolicy: mcoconfig.GetImagePullPolicy(r.cr.Spec),
		Resources:       mcoaResources,
	}

	if r.cr.Spec.Capabilities != nil && r.cr.Spec.Capabilities.AddonManager != nil {
		if r.cr.Spec.Capabilities.AddonManager.LogVerbosity != nil {
			patchContainer.Args = append(patchContainer.Args, fmt.Sprintf("--log-verbosity=%d", *r.cr.Spec.Capabilities.AddonManager.LogVerbosity))
		}
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

	// Add grafana link annotation
	host, err := mcoconfig.GetRouteHost(r.kubeClient, mcoconfig.GrafanaRouteName, mcoconfig.GetDefaultNamespace())
	if err != nil {
		return nil, fmt.Errorf("failed to get host route: %w", err)
	}
	grafanaUrl := url.URL{
		Scheme: "https",
		Host:   host,
		Path:   grafanaLink,
	}
	annotations := maps.Clone(u.GetAnnotations())
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations["console.open-cluster-management.io/launch-link"] = grafanaUrl.String()
	annotations["console.open-cluster-management.io/launch-link-text"] = "Grafana"
	u.SetAnnotations(annotations)

	cLabels := u.GetLabels()
	if cLabels == nil {
		cLabels = make(map[string]string)
	}
	maps.Copy(cLabels, labels)
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
			if cs.Platform.Logs.Collection.Enabled {
				fqdn := mcoconfig.GetMCOASupportedCRDFQDN(mcoconfig.ClusterLogForwarderCRDName)
				appendCustomVar(aodc, namePlatformLogsCollection, fqdn)
			}
			if cs.Platform.Metrics.Default.Enabled {
				fqdn := mcoconfig.GetMCOASupportedCRDFQDN(mcoconfig.PrometheusAgentCRDName)
				appendCustomVar(aodc, namePlatformMetricsCollection, fqdn)
				if cs.Platform.Metrics.UI.Enabled {
					appendCustomVar(aodc, namePLatformMetricsUI, uipluginsCRDFQDN)
				}
			}
			if cs.Platform.Analytics.IncidentDetection.Enabled {
				appendCustomVar(aodc, namePlatformIncidentDetection, uipluginsCRDFQDN)
			}
		}

		if cs.UserWorkloads != nil {
			if cs.UserWorkloads.Logs.Collection.ClusterLogForwarder.Enabled {
				fqdn := mcoconfig.GetMCOASupportedCRDFQDN(mcoconfig.ClusterLogForwarderCRDName)
				appendCustomVar(aodc, nameUserWorkloadLogsCollection, fqdn)
			}
			if cs.UserWorkloads.Metrics.Default.Enabled {
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

		if (cs.Platform != nil && cs.Platform.Metrics.Default.Enabled) ||
			(cs.UserWorkloads != nil && cs.UserWorkloads.Metrics.Default.Enabled) {
			if r.rendererOptions == nil {
				return nil, fmt.Errorf("rendererOptions is nil")
			}
			metricsHubHostname := r.rendererOptions.MCOAOptions.MetricsHubHostname
			metricsHubAlertmanagerHostname := r.rendererOptions.MCOAOptions.MetricsHubAlertmanagerHostname
			if metricsHubHostname == "" || metricsHubAlertmanagerHostname == "" {
				return nil, fmt.Errorf("MetricsHubHostname (%q) and MetricsHubAlertmanagerHostname (%q) are required when metrics collection is enabled",
					metricsHubHostname, metricsHubAlertmanagerHostname)
			}
			appendCustomVar(aodc, nameMetricsHubHostname, metricsHubHostname)
			appendCustomVar(aodc, nameMetricsAlertManagerHostname, metricsHubAlertmanagerHostname)
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
	maps.Copy(cLabels, labels)
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
		mcoaEnabled = mcoaEnabled ||
			cr.Spec.Capabilities.Platform.Logs.Collection.Enabled ||
			cr.Spec.Capabilities.Platform.Metrics.Default.Enabled ||
			cr.Spec.Capabilities.Platform.Analytics.IncidentDetection.Enabled
	}
	if cr.Spec.Capabilities.UserWorkloads != nil {
		mcoaEnabled = mcoaEnabled || cr.Spec.Capabilities.UserWorkloads.Logs.Collection.ClusterLogForwarder.Enabled
		mcoaEnabled = mcoaEnabled || cr.Spec.Capabilities.UserWorkloads.Traces.Collection.Collector.Enabled
		mcoaEnabled = mcoaEnabled || cr.Spec.Capabilities.UserWorkloads.Traces.Collection.Instrumentation.Enabled
		mcoaEnabled = mcoaEnabled || cr.Spec.Capabilities.UserWorkloads.Metrics.Default.Enabled
	}
	return mcoaEnabled
}

func MCOAPlatformMetricsEnabled(cr *obv1beta2.MultiClusterObservability) bool {
	if cr.Spec.Capabilities == nil {
		return false
	}

	if cr.Spec.Capabilities.Platform != nil && cr.Spec.Capabilities.Platform.Metrics.Default.Enabled {
		return true
	}

	return false
}
