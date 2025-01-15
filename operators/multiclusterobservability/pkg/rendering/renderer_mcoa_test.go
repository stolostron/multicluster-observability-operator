// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rendering

import (
	"os"
	"path/filepath"
	"testing"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/rendering/templates"
	templatesutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering/templates"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
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

	uobj, err := renderer.renderMCOADeployment(dp, "test", map[string]string{"key": "value"})
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
						Collection: mcov1beta2.PlatformMetricsCollectionSpec{
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
						Collection: mcov1beta2.UserWorkloadMetricsCollectionSpec{
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
			MetricsHubHostname: "observability-hub",
		},
	}}

	uobj, err := renderer.renderAddonDeploymentConfig(aodc, "test", map[string]string{"key": "value"})
	assert.NoError(t, err)
	assert.NotNil(t, uobj)

	got := &addonv1alpha1.AddOnDeploymentConfig{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(uobj.Object, got)
	assert.NoError(t, err)

	clfV1 := mcoconfig.GetMCOASupportedCRDFQDN(mcoconfig.ClusterLogForwarderCRDName)
	otelV1beta1 := mcoconfig.GetMCOASupportedCRDFQDN(mcoconfig.OpenTelemetryCollectorCRDName)
	instrV1alpha1 := mcoconfig.GetMCOASupportedCRDFQDN(mcoconfig.InstrumentationCRDName)
	promV1alpha1 := mcoconfig.GetMCOASupportedCRDFQDN(mcoconfig.PrometheusAgentCRDName)

	assert.Len(t, got.Spec.CustomizedVariables, 8)
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1alpha1.CustomizedVariable{Name: namePlatformLogsCollection, Value: clfV1})
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1alpha1.CustomizedVariable{Name: nameUserWorkloadLogsCollection, Value: clfV1})
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1alpha1.CustomizedVariable{Name: nameUserWorkloadTracesCollection, Value: otelV1beta1})
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1alpha1.CustomizedVariable{Name: nameUserWorkloadInstrumentation, Value: instrV1alpha1})
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1alpha1.CustomizedVariable{Name: namePlatformMetricsCollection, Value: promV1alpha1})
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1alpha1.CustomizedVariable{Name: nameUserWorkloadMetricsCollection, Value: promV1alpha1})
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1alpha1.CustomizedVariable{Name: nameMetricsHubHostname, Value: "observability-hub"})
}

func TestMCORenderer_RenderClusterManagementAddOn(t *testing.T) {
	tests := []struct {
		name         string
		labels       map[string]string
		capabilities *mcov1beta2.CapabilitiesSpec
		expectConfig func(*testing.T, *addonv1alpha1.ClusterManagementAddOn)
	}{
		{
			name: "add metrics configs when platform is enabled",
			capabilities: &mcov1beta2.CapabilitiesSpec{
				Platform: &mcov1beta2.PlatformCapabilitiesSpec{
					Metrics: mcov1beta2.PlatformMetricsSpec{
						Collection: mcov1beta2.PlatformMetricsCollectionSpec{
							Enabled: true,
						},
					},
				},
			},
			expectConfig: func(t *testing.T, cma *addonv1alpha1.ClusterManagementAddOn) {
				assert.Len(t, cma.Spec.InstallStrategy.Placements[0].Configs, 3)
			},
		},
		{
			name: "add metrics configs when user workloads is enabled",
			capabilities: &mcov1beta2.CapabilitiesSpec{
				UserWorkloads: &mcov1beta2.UserWorkloadCapabilitiesSpec{
					Metrics: mcov1beta2.UserWorkloadMetricsSpec{
						Collection: mcov1beta2.UserWorkloadMetricsCollectionSpec{
							Enabled: true,
						},
					},
				},
			},
			expectConfig: func(t *testing.T, cma *addonv1alpha1.ClusterManagementAddOn) {
				assert.Len(t, cma.Spec.InstallStrategy.Placements[0].Configs, 1)
			},
		},
		{
			name: "add metrics configs when both platform and user workloads are enabled",
			capabilities: &mcov1beta2.CapabilitiesSpec{
				Platform: &mcov1beta2.PlatformCapabilitiesSpec{
					Metrics: mcov1beta2.PlatformMetricsSpec{
						Collection: mcov1beta2.PlatformMetricsCollectionSpec{
							Enabled: true,
						},
					},
				},
				UserWorkloads: &mcov1beta2.UserWorkloadCapabilitiesSpec{
					Metrics: mcov1beta2.UserWorkloadMetricsSpec{
						Collection: mcov1beta2.UserWorkloadMetricsCollectionSpec{
							Enabled: true,
						},
					},
				},
			},
			expectConfig: func(t *testing.T, cma *addonv1alpha1.ClusterManagementAddOn) {
				assert.Len(t, cma.Spec.InstallStrategy.Placements[0].Configs, 4)
			},
		},
		{
			name: "add logs configs when platform is enabled",
			capabilities: &mcov1beta2.CapabilitiesSpec{
				Platform: &mcov1beta2.PlatformCapabilitiesSpec{
					Logs: mcov1beta2.PlatformLogsSpec{
						Collection: mcov1beta2.PlatformLogsCollectionSpec{
							Enabled: true,
						},
					},
				},
			},
			expectConfig: func(t *testing.T, cma *addonv1alpha1.ClusterManagementAddOn) {
				assert.Len(t, cma.Spec.InstallStrategy.Placements[0].Configs, 1)
			},
		},
		{
			name: "add logs configs when user workloads is enabled",
			capabilities: &mcov1beta2.CapabilitiesSpec{
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
			expectConfig: func(t *testing.T, cma *addonv1alpha1.ClusterManagementAddOn) {
				assert.Len(t, cma.Spec.InstallStrategy.Placements[0].Configs, 0)
			},
		},
		{
			name: "add logs configs when both platform and user workloads are enabled",
			capabilities: &mcov1beta2.CapabilitiesSpec{
				Platform: &mcov1beta2.PlatformCapabilitiesSpec{
					Logs: mcov1beta2.PlatformLogsSpec{
						Collection: mcov1beta2.PlatformLogsCollectionSpec{
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
				},
			},
			expectConfig: func(t *testing.T, cma *addonv1alpha1.ClusterManagementAddOn) {
				assert.Len(t, cma.Spec.InstallStrategy.Placements[0].Configs, 1)
			},
		},
		{
			name: "add traces configs when user workloads is enabled",
			capabilities: &mcov1beta2.CapabilitiesSpec{
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
			expectConfig: func(t *testing.T, cma *addonv1alpha1.ClusterManagementAddOn) {
				assert.Len(t, cma.Spec.InstallStrategy.Placements[0].Configs, 2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &MCORenderer{
				cr: &mcov1beta2.MultiClusterObservability{
					Spec: mcov1beta2.MultiClusterObservabilitySpec{
						Capabilities: tt.capabilities,
					},
				},
			}

			// Add deployment config to reflect real input
			baseCMA := &addonv1alpha1.ClusterManagementAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"k1": "v1",
					},
				},
				Spec: addonv1alpha1.ClusterManagementAddOnSpec{
					InstallStrategy: addonv1alpha1.InstallStrategy{
						Placements: []addonv1alpha1.PlacementStrategy{
							{
								Configs: []addonv1alpha1.AddOnConfig{},
							},
						},
					},
				},
			}
			// to kustomize resource
			baseCMAUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(baseCMA)
			assert.NoError(t, err)
			kres := &kustomizeres.Resource{}
			err = runtime.DefaultUnstructuredConverter.FromUnstructured(baseCMAUnstructured, kres)
			assert.NoError(t, err)

			res, err := r.renderClusterManagementAddOn(kres, "ns", map[string]string{"k2": "v2"})
			assert.NoError(t, err)
			assert.NotNil(t, res)

			// check labels
			assert.Len(t, res.GetLabels(), 2)

			// check supportedConfigs
			cma := &addonapiv1alpha1.ClusterManagementAddOn{}
			err = runtime.DefaultUnstructuredConverter.FromUnstructured(res.Object, cma)
			assert.NoError(t, err)

			tt.expectConfig(t, cma)

			// Check duplicated supportedConfigs
			dups := make(map[string]struct{})
			for _, cfg := range cma.Spec.SupportedConfigs {
				key := cfg.ConfigGroupResource.Group + "/" + cfg.ConfigGroupResource.Resource
				if _, ok := dups[key]; ok {
					t.Errorf("duplicated supportedConfigs %s", key)
				}
				dups[key] = struct{}{}
			}
		})
	}
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
								Collection: mcov1beta2.PlatformMetricsCollectionSpec{
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
								Collection: mcov1beta2.UserWorkloadMetricsCollectionSpec{
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
								Collection: mcov1beta2.PlatformMetricsCollectionSpec{
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
								Collection: mcov1beta2.UserWorkloadMetricsCollectionSpec{
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

			uobjs, err := renderer.renderMCOATemplates(mcoaTemplates, "test", map[string]string{"key": "value"})
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
