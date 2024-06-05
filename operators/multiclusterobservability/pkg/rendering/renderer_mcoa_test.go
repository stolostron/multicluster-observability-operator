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
	"k8s.io/utils/pointer"
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
					Replicas: pointer.Int32(1),
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("256M"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("512M"),
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
					Logs: mcov1beta2.LogsCollectionSpec{
						Enabled: true,
					},
				},
				UserWorkloads: &mcov1beta2.UserWorkloadCapabilitiesSpec{
					Logs: mcov1beta2.UserWorkloadLogsSpec{
						ClusterLogForwarder: mcov1beta2.ClusterLogForwarderSpec{
							Enabled: true,
						},
					},
					Traces: mcov1beta2.UserWorkloadTracesSpec{
						Instrumentation: mcov1beta2.InstrumentationSpec{
							Enabled: true,
						},
						OpenTelemetryCollector: mcov1beta2.OpenTelemetryCollectorSpec{
							Enabled: true,
						},
					},
				},
			},
		},
	}
	renderer := &MCORenderer{cr: mco}

	uobj, err := renderer.renderAddonDeploymentConfig(aodc, "test", map[string]string{"key": "value"})
	assert.NoError(t, err)
	assert.NotNil(t, uobj)

	got := &addonv1alpha1.AddOnDeploymentConfig{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(uobj.Object, got)
	assert.NoError(t, err)

	assert.Len(t, got.Spec.CustomizedVariables, 5)
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1alpha1.CustomizedVariable{Name: namePlatformLogsCollection, Value: clfV1})
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1alpha1.CustomizedVariable{Name: nameUserWorkloadLogsCollection, Value: clfV1})
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1alpha1.CustomizedVariable{Name: nameUserWorkloadTracesCollection, Value: otelV1beta1})
	assert.Contains(t, got.Spec.CustomizedVariables, addonv1alpha1.CustomizedVariable{Name: nameUserWorkloadInstrumentation, Value: instrV1alpha1})
}
