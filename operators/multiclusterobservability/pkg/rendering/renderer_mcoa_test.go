// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rendering

import (
	"os"
	"path/filepath"
	"testing"

	routev1 "github.com/openshift/api/route/v1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/rendering/templates"
	mcoutil "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	templatesutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering/templates"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	addonv1beta1 "open-cluster-management.io/api/addon/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	kustomizeres "sigs.k8s.io/kustomize/api/resource"
)

func TestRenderMCOADeployment(t *testing.T) {
	wd, err := os.Getwd()
	assert.NoError(t, err)
	templatesPath := filepath.Join(wd, "..", "..", "manifests")
	t.Setenv(templatesutil.TemplatesPathEnvVar, templatesPath)

	tmplRenderer := templatesutil.NewTemplateRenderer(templatesPath)
	mcoaTemplates, err := templates.GetOrLoadMCOATemplates(tmplRenderer)
	assert.NoError(t, err)

	var dp *kustomizeres.Resource
	for _, template := range mcoaTemplates {
		if template.GetKind() == "Deployment" {
			dp = template.DeepCopy()
			break
		}
	}

	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"cr-key": "cr-value",
			},
			Name: "multicluster-observability",
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			ImagePullPolicy: corev1.PullIfNotPresent,
			AdvancedConfig: &mcov1beta2.AdvancedConfig{
				MultiClusterObservabilityAddon: &mcov1beta2.CommonSpec{
					Replicas: ptr.To[int32](1),
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("256M"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:              resource.MustParse("200m"),
							corev1.ResourceMemory:           resource.MustParse("512M"),
							corev1.ResourceEphemeralStorage: resource.MustParse("4Gi"),
						},
					},
				},
			},
		},
	}

	renderer := &MCORenderer{cr: mco}

	uobj, err := renderer.renderMCOADeployment(t.Context(), dp, "test", map[string]string{"key": "value"})
	assert.NoError(t, err)
	assert.NotNil(t, uobj)

	got := &appsv1.Deployment{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(uobj.Object, got)
	assert.NoError(t, err)

	// Assert labels include the CR label key
	assert.Len(t, got.Labels, 4)
	assert.Contains(t, got.Labels, mcoconfig.GetCrLabelKey())
	assert.Len(t, got.Spec.Selector.MatchLabels, 2)
	assert.Contains(t, got.Spec.Selector.MatchLabels, mcoconfig.GetCrLabelKey())
	assert.Len(t, got.Spec.Template.ObjectMeta.Labels, 2)
	assert.Contains(t, got.Spec.Template.ObjectMeta.Labels, mcoconfig.GetCrLabelKey())

	// Assert pod spec values
	assert.Equal(t, mco.Spec.AdvancedConfig.MultiClusterObservabilityAddon.Replicas, got.Spec.Replicas)

	container := got.Spec.Template.Spec.Containers[0]
	assert.Contains(t, container.Image, mcoconfig.MultiClusterObservabilityAddonImgRepo)
	assert.Contains(t, container.Image, mcoconfig.MultiClusterObservabilityAddonImgName)
	assert.Contains(t, container.Image, mcoconfig.MultiClusterObservabilityAddonImgTagSuffix)
	assert.Equal(t, corev1.PullIfNotPresent, container.ImagePullPolicy)
	assert.Equal(t, *mco.Spec.AdvancedConfig.MultiClusterObservabilityAddon.Resources, container.Resources)
	assert.True(t, *container.SecurityContext.RunAsNonRoot)

	// Test with AddonManager resources and logVerbosity overrides
	mco.Spec.Capabilities = &mcov1beta2.CapabilitiesSpec{
		AddonManager: &mcov1beta2.AddonManagerSpec{
			LogVerbosity: ptr.To[int32](5),
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("300m"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
		},
	}
	uobj, err = renderer.renderMCOADeployment(t.Context(), dp, "test", map[string]string{"key": "value"})
	assert.NoError(t, err)
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(uobj.Object, got)
	assert.NoError(t, err)
	container = got.Spec.Template.Spec.Containers[0]
	assert.Equal(t, *mco.Spec.Capabilities.AddonManager.Resources, container.Resources)
	assert.Contains(t, container.Args, "--log-verbosity=5")
	assert.Contains(t, container.Args, "controller")
}

func TestRenderAddonDeploymentConfig(t *testing.T) {
	wd, err := os.Getwd()
	assert.NoError(t, err)
	templatesPath := filepath.Join(wd, "..", "..", "manifests")
	t.Setenv(templatesutil.TemplatesPathEnvVar, templatesPath)

	tmplRenderer := templatesutil.NewTemplateRenderer(templatesPath)
	mcoaTemplates, err := templates.GetOrLoadMCOATemplates(tmplRenderer)
	assert.NoError(t, err)

	var aodc *kustomizeres.Resource
	for _, template := range mcoaTemplates {
		if template.GetKind() == "AddOnDeploymentConfig" {
			aodc = template.DeepCopy()
			break
		}
	}

	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"cr-key": "cr-value",
			},
			Name: "multicluster-observability",
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			Capabilities: &mcov1beta2.CapabilitiesSpec{
				Platform: &mcov1beta2.PlatformCapabilitiesSpec{
					Logs: mcov1beta2.PlatformLogsSpec{
						Collection: mcov1beta2.PlatformLogsCollectionSpec{
							Enabled: true,
						},
					},
					Metrics: mcov1beta2.PlatformMetricsSpec{
						Default: mcov1beta2.PlatformMetricsDefaultSpec{
							Enabled: true,
						},
						UI: mcov1beta2.UIConfig{
							Enabled: true,
						},
					},
					Analytics: mcov1beta2.PlatformAnalyticsSpec{
						IncidentDetection: mcov1beta2.PlatformIncidentDetectionSpec{
							Enabled: true,
						},
					},
				},
				UserWorkloads: &mcov1beta2.UserWorkloadCapabilitiesSpec{
					Logs: mcov1beta2.UserWorkloadLogsSpec{
						Collection: mcov1beta2.UserWorkloadLogsCollectionSpec{
							ClusterLogForwarder: mcov1beta2.ClusterLogForwarderSpec{
								Enabled: true,
							},
						},
					},
					Metrics: mcov1beta2.UserWorkloadMetricsSpec{
						Default: mcov1beta2.UserWorkloadMetricsDefaultSpec{
							Enabled: true,
						},
					},
					Traces: mcov1beta2.UserWorkloadTracesSpec{
						Collection: mcov1beta2.OpenTelemetryCollectionSpec{
							Collector: mcov1beta2.OpenTelemetryCollectorSpec{
								Enabled: true,
							},
							Instrumentation: mcov1beta2.InstrumentationSpec{
								Enabled: true,
							},
						},
					},
				},
			},
		},
	}

	renderer := &MCORenderer{cr: mco, rendererOptions: &RendererOptions{
		MCOAOptions: MCOARendererOptions{
			MetricsHubHostname:             "observability-hub",
			MetricsHubAlertmanagerHostname: "alertmanager-hub",
		},
	}}

	uobj, err := renderer.renderAddonDeploymentConfig(t.Context(), aodc, "test", map[string]string{"key": "value"})
	assert.NoError(t, err)
	assert.NotNil(t, uobj)

	got := &addonv1beta1.AddOnDeploymentConfig{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(uobj.Object, got)
	assert.NoError(t, err)

	clfV1 := mcoconfig.GetMCOASupportedCRDFQDN(mcoconfig.ClusterLogForwarderCRDName)
	otelV1beta1 := mcoconfig.GetMCOASupportedCRDFQDN(mcoconfig.OpenTelemetryCollectorCRDName)
	instrV1alpha1 := mcoconfig.GetMCOASupportedCRDFQDN(mcoconfig.InstrumentationCRDName)
	promV1alpha1 := mcoconfig.GetMCOASupportedCRDFQDN(mcoconfig.PrometheusAgentCRDName)

	assert.Len(t, got.Spec.CustomizedVariables, 14)
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1beta1.CustomizedVariable{Name: nameUserWorkloadLogsCollection, Value: clfV1})
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1beta1.CustomizedVariable{Name: namePlatformIncidentDetection, Value: uipluginsCRDFQDN})
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1beta1.CustomizedVariable{Name: namePlatformLogsCollection, Value: clfV1})
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1beta1.CustomizedVariable{Name: nameUserWorkloadTracesCollection, Value: otelV1beta1})
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1beta1.CustomizedVariable{Name: nameUserWorkloadInstrumentation, Value: instrV1alpha1})
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1beta1.CustomizedVariable{Name: namePlatformMetricsCollection, Value: promV1alpha1})
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1beta1.CustomizedVariable{Name: namePlatformMetricsAlerts, Value: "disabled"})
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1beta1.CustomizedVariable{Name: nameUserWorkloadMetricsCollection, Value: promV1alpha1})
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1beta1.CustomizedVariable{Name: nameUserWorkloadMetricsAlerts, Value: "disabled"})
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1beta1.CustomizedVariable{Name: nameMetricsHubHostname, Value: "observability-hub"})
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1beta1.CustomizedVariable{Name: namePLatformMetricsUI, Value: uipluginsCRDFQDN})
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1beta1.CustomizedVariable{Name: mcoutil.ADCKeyRightSizingDelegated, Value: "false"})
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1beta1.CustomizedVariable{Name: mcoutil.ADCKeyPlatformNamespaceRightSizing, Value: "disabled"})
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1beta1.CustomizedVariable{Name: mcoutil.ADCKeyPlatformVirtualizationRightSizing, Value: "disabled"})
}

func TestRenderAddonDeploymentConfig_AlertsEnabled(t *testing.T) {
	wd, err := os.Getwd()
	assert.NoError(t, err)
	templatesPath := filepath.Join(wd, "..", "..", "manifests")
	t.Setenv(templatesutil.TemplatesPathEnvVar, templatesPath)

	tmplRenderer := templatesutil.NewTemplateRenderer(templatesPath)
	mcoaTemplates, err := templates.GetOrLoadMCOATemplates(tmplRenderer)
	assert.NoError(t, err)

	var aodc *kustomizeres.Resource
	for _, template := range mcoaTemplates {
		if template.GetKind() == "AddOnDeploymentConfig" {
			aodc = template.DeepCopy()
			break
		}
	}

	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{
			Name: "multicluster-observability",
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			Capabilities: &mcov1beta2.CapabilitiesSpec{
				Platform: &mcov1beta2.PlatformCapabilitiesSpec{
					Metrics: mcov1beta2.PlatformMetricsSpec{
						Default: mcov1beta2.PlatformMetricsDefaultSpec{
							Enabled: true,
						},
						Alerts: mcov1beta2.MetricsAlertsSpec{
							Enabled: true,
						},
					},
				},
				UserWorkloads: &mcov1beta2.UserWorkloadCapabilitiesSpec{
					Metrics: mcov1beta2.UserWorkloadMetricsSpec{
						Default: mcov1beta2.UserWorkloadMetricsDefaultSpec{
							Enabled: true,
						},
						Alerts: mcov1beta2.MetricsAlertsSpec{
							Enabled: true,
						},
					},
				},
			},
		},
	}

	renderer := &MCORenderer{cr: mco, rendererOptions: &RendererOptions{
		MCOAOptions: MCOARendererOptions{
			MetricsHubHostname:             "observability-hub",
			MetricsHubAlertmanagerHostname: "alertmanager-hub",
		},
	}}

	uobj, err := renderer.renderAddonDeploymentConfig(t.Context(), aodc, "test", map[string]string{"key": "value"})
	assert.NoError(t, err)
	assert.NotNil(t, uobj)

	got := &addonv1beta1.AddOnDeploymentConfig{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(uobj.Object, got)
	assert.NoError(t, err)

	assert.Contains(t, got.Spec.CustomizedVariables, addonv1beta1.CustomizedVariable{Name: namePlatformMetricsAlerts, Value: "enabled"})
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1beta1.CustomizedVariable{Name: nameUserWorkloadMetricsAlerts, Value: "enabled"})
}

func TestMCOAEnabled(t *testing.T) {
	tests := []struct {
		name     string
		cr       *mcov1beta2.MultiClusterObservability
		expected bool
	}{
		{
			name: "Capabilities not set",
			cr: &mcov1beta2.MultiClusterObservability{
				Spec: mcov1beta2.MultiClusterObservabilitySpec{
					Capabilities: nil,
				},
			},
			expected: false,
		},
		{
			name: "Platform logs collection enabled",
			cr: &mcov1beta2.MultiClusterObservability{
				Spec: mcov1beta2.MultiClusterObservabilitySpec{
					Capabilities: &mcov1beta2.CapabilitiesSpec{
						Platform: &mcov1beta2.PlatformCapabilitiesSpec{
							Logs: mcov1beta2.PlatformLogsSpec{
								Collection: mcov1beta2.PlatformLogsCollectionSpec{
									Enabled: true,
								},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "User workloads logs collection enabled",
			cr: &mcov1beta2.MultiClusterObservability{
				Spec: mcov1beta2.MultiClusterObservabilitySpec{
					Capabilities: &mcov1beta2.CapabilitiesSpec{
						UserWorkloads: &mcov1beta2.UserWorkloadCapabilitiesSpec{
							Logs: mcov1beta2.UserWorkloadLogsSpec{
								Collection: mcov1beta2.UserWorkloadLogsCollectionSpec{
									ClusterLogForwarder: mcov1beta2.ClusterLogForwarderSpec{
										Enabled: true,
									},
								},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "Platform metrics collection enabled",
			cr: &mcov1beta2.MultiClusterObservability{
				Spec: mcov1beta2.MultiClusterObservabilitySpec{
					Capabilities: &mcov1beta2.CapabilitiesSpec{
						Platform: &mcov1beta2.PlatformCapabilitiesSpec{
							Metrics: mcov1beta2.PlatformMetricsSpec{
								Default: mcov1beta2.PlatformMetricsDefaultSpec{
									Enabled: true,
								},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "User workloads metrics collection enabled",
			cr: &mcov1beta2.MultiClusterObservability{
				Spec: mcov1beta2.MultiClusterObservabilitySpec{
					Capabilities: &mcov1beta2.CapabilitiesSpec{
						UserWorkloads: &mcov1beta2.UserWorkloadCapabilitiesSpec{
							Metrics: mcov1beta2.UserWorkloadMetricsSpec{
								Default: mcov1beta2.UserWorkloadMetricsDefaultSpec{
									Enabled: true,
								},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "User workloads traces collection enabled",
			cr: &mcov1beta2.MultiClusterObservability{
				Spec: mcov1beta2.MultiClusterObservabilitySpec{
					Capabilities: &mcov1beta2.CapabilitiesSpec{
						UserWorkloads: &mcov1beta2.UserWorkloadCapabilitiesSpec{
							Traces: mcov1beta2.UserWorkloadTracesSpec{
								Collection: mcov1beta2.OpenTelemetryCollectionSpec{
									Collector: mcov1beta2.OpenTelemetryCollectorSpec{
										Enabled: true,
									},
									Instrumentation: mcov1beta2.InstrumentationSpec{
										Enabled: true,
									},
								},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "All capabilities disabled",
			cr: &mcov1beta2.MultiClusterObservability{
				Spec: mcov1beta2.MultiClusterObservabilitySpec{
					Capabilities: &mcov1beta2.CapabilitiesSpec{
						Platform: &mcov1beta2.PlatformCapabilitiesSpec{
							Logs: mcov1beta2.PlatformLogsSpec{
								Collection: mcov1beta2.PlatformLogsCollectionSpec{
									Enabled: false,
								},
							},
							Metrics: mcov1beta2.PlatformMetricsSpec{
								Default: mcov1beta2.PlatformMetricsDefaultSpec{
									Enabled: false,
								},
							},
						},
						UserWorkloads: &mcov1beta2.UserWorkloadCapabilitiesSpec{
							Logs: mcov1beta2.UserWorkloadLogsSpec{
								Collection: mcov1beta2.UserWorkloadLogsCollectionSpec{
									ClusterLogForwarder: mcov1beta2.ClusterLogForwarderSpec{
										Enabled: false,
									},
								},
							},
							Metrics: mcov1beta2.UserWorkloadMetricsSpec{
								Default: mcov1beta2.UserWorkloadMetricsDefaultSpec{
									Enabled: false,
								},
							},
							Traces: mcov1beta2.UserWorkloadTracesSpec{
								Collection: mcov1beta2.OpenTelemetryCollectionSpec{
									Collector: mcov1beta2.OpenTelemetryCollectorSpec{
										Enabled: false,
									},
									Instrumentation: mcov1beta2.InstrumentationSpec{
										Enabled: false,
									},
								},
							},
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MCOAEnabled(tt.cr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRenderMCOATemplates(t *testing.T) {
	wd, err := os.Getwd()
	assert.NoError(t, err)
	templatesPath := filepath.Join(wd, "..", "..", "manifests")
	t.Setenv(templatesutil.TemplatesPathEnvVar, templatesPath)

	tmplRenderer := templatesutil.NewTemplateRenderer(templatesPath)
	mcoaTemplates, err := templates.GetOrLoadMCOATemplates(tmplRenderer)
	assert.NoError(t, err)

	tests := []struct {
		name                          string
		capabilitiesEnabled           bool
		renderOptionDisableCMAORender bool
		expectClusterManagementAddOn  bool
	}{
		{
			name:                         "Capabilites disabled, ClusterManagementAddOn should be rendered to allow deletion",
			capabilitiesEnabled:          false,
			expectClusterManagementAddOn: true,
		},
		{
			name:                          "Capabilities enabled, ClusterManagementAddOn should be rendered by default",
			capabilitiesEnabled:           true,
			renderOptionDisableCMAORender: false,
			expectClusterManagementAddOn:  true,
		},
		{
			name:                          "Capabilities enabled, disableCMAORender ClusterManagementAddOn should not be rendered",
			capabilitiesEnabled:           true,
			renderOptionDisableCMAORender: true,
			expectClusterManagementAddOn:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mco := &mcov1beta2.MultiClusterObservability{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"cr-key": "cr-value",
					},
					Name: "multicluster-observability",
				},
				Spec: mcov1beta2.MultiClusterObservabilitySpec{
					Capabilities: &mcov1beta2.CapabilitiesSpec{
						Platform: &mcov1beta2.PlatformCapabilitiesSpec{
							Logs: mcov1beta2.PlatformLogsSpec{
								Collection: mcov1beta2.PlatformLogsCollectionSpec{
									Enabled: tt.capabilitiesEnabled,
								},
							},
						},
					},
				},
			}
			renderer := &MCORenderer{cr: mco}
			renderer.rendererOptions = &RendererOptions{
				MCOAOptions: MCOARendererOptions{
					DisableCMAORender: tt.renderOptionDisableCMAORender,
				},
			}

			uobjs, err := renderer.renderMCOATemplates(t.Context(), mcoaTemplates, "test", map[string]string{"key": "value"})
			assert.NoError(t, err)
			assert.NotNil(t, uobjs)

			foundClusterManagementAddOn := false
			for _, uobj := range uobjs {
				if uobj.GetKind() == "ClusterManagementAddOn" {
					foundClusterManagementAddOn = true
					break
				}
			}

			if tt.expectClusterManagementAddOn {
				assert.True(t, foundClusterManagementAddOn, "Expected ClusterManagementAddOn to be rendered")
			} else {
				assert.False(t, foundClusterManagementAddOn, "Expected ClusterManagementAddOn to not be rendered")
			}
		})
	}
}

// TestRenderClusterManagementAddOn verifies that the Grafana launch-link annotation
// is set only when platform metrics are enabled via MCOA capabilities.
func TestRenderClusterManagementAddOn(t *testing.T) {
	wd, err := os.Getwd()
	assert.NoError(t, err)
	templatesPath := filepath.Join(wd, "..", "..", "manifests")
	t.Setenv(templatesutil.TemplatesPathEnvVar, templatesPath)

	tmplRenderer := templatesutil.NewTemplateRenderer(templatesPath)
	mcoaTemplates, err := templates.GetOrLoadMCOATemplates(tmplRenderer)
	assert.NoError(t, err)

	var cmaTemplate *kustomizeres.Resource
	for _, template := range mcoaTemplates {
		if template.GetKind() == "ClusterManagementAddOn" {
			cmaTemplate = template.DeepCopy()
			break
		}
	}
	assert.NotNil(t, cmaTemplate, "ClusterManagementAddOn template not found")

	grafanaRoute := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcoconfig.GrafanaRouteName,
			Namespace: mcoconfig.GetDefaultNamespace(),
		},
		Spec: routev1.RouteSpec{
			Host: "grafana.apps.test-cluster.example.com",
		},
	}

	s := runtime.NewScheme()
	assert.NoError(t, routev1.AddToScheme(s))

	tests := []struct {
		name           string
		metricsEnabled bool
		expectLink     bool
	}{
		{
			name:           "Platform metrics enabled - should have Grafana link",
			metricsEnabled: true,
			expectLink:     true,
		},
		{
			name:           "Platform metrics disabled - should not have Grafana link",
			metricsEnabled: false,
			expectLink:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mco := &mcov1beta2.MultiClusterObservability{
				ObjectMeta: metav1.ObjectMeta{
					Name: "multicluster-observability",
				},
			}
			if tt.metricsEnabled {
				mco.Spec.Capabilities = &mcov1beta2.CapabilitiesSpec{
					Platform: &mcov1beta2.PlatformCapabilitiesSpec{
						Metrics: mcov1beta2.PlatformMetricsSpec{
							Default: mcov1beta2.PlatformMetricsDefaultSpec{
								Enabled: true,
							},
						},
					},
				}
			}

			fakeClient := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(grafanaRoute).Build()
			renderer := &MCORenderer{cr: mco, kubeClient: fakeClient}

			uobj, err := renderer.renderClusterManagementAddOn(t.Context(), cmaTemplate.DeepCopy(), "test", map[string]string{"key": "value"})
			assert.NoError(t, err)
			assert.NotNil(t, uobj)

			annotations := uobj.GetAnnotations()
			if tt.expectLink {
				assert.Contains(t, annotations, "console.open-cluster-management.io/launch-link")
				assert.Equal(t, "Grafana", annotations["console.open-cluster-management.io/launch-link-text"])
				assert.Contains(t, annotations["console.open-cluster-management.io/launch-link"], "grafana.apps.test-cluster.example.com")
				assert.Contains(t, annotations["console.open-cluster-management.io/launch-link"], grafanaMCOAHomeDashboardID)
			} else {
				assert.NotContains(t, annotations, "console.open-cluster-management.io/launch-link")
				assert.NotContains(t, annotations, "console.open-cluster-management.io/launch-link-text")
			}

			// Labels should always be set regardless of metrics
			assert.Contains(t, uobj.GetLabels(), "key")
		})
	}
}

// TestRenderClusterManagementAddOnNilCapabilities verifies that a nil Capabilities
// spec does not attempt a Grafana route lookup and produces no launch-link annotation.
func TestRenderClusterManagementAddOnNilCapabilities(t *testing.T) {
	wd, err := os.Getwd()
	assert.NoError(t, err)
	templatesPath := filepath.Join(wd, "..", "..", "manifests")
	t.Setenv(templatesutil.TemplatesPathEnvVar, templatesPath)

	tmplRenderer := templatesutil.NewTemplateRenderer(templatesPath)
	mcoaTemplates, err := templates.GetOrLoadMCOATemplates(tmplRenderer)
	assert.NoError(t, err)

	var cmaTemplate *kustomizeres.Resource
	for _, template := range mcoaTemplates {
		if template.GetKind() == "ClusterManagementAddOn" {
			cmaTemplate = template.DeepCopy()
			break
		}
	}

	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{Name: "multicluster-observability"},
	}

	// kubeClient intentionally nil: with nil Capabilities, no route fetch should occur
	renderer := &MCORenderer{cr: mco}
	uobj, err := renderer.renderClusterManagementAddOn(t.Context(), cmaTemplate.DeepCopy(), "test", map[string]string{"key": "value"})
	assert.NoError(t, err)
	assert.NotNil(t, uobj)
	assert.NotContains(t, uobj.GetAnnotations(), "console.open-cluster-management.io/launch-link")
}
