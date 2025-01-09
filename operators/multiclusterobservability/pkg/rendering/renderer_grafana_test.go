package rendering

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	obv1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/rendering/templates"
	rendererutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering"
	templatesutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering/templates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestRenderGrafana(t *testing.T) {
	wd, err := os.Getwd()
	assert.NoError(t, err)
	templatesPath := filepath.Join(wd, "..", "..", "manifests")
	tmplRenderer := templatesutil.NewTemplateRenderer(templatesPath)
	grafanaTemplates, err := templates.GetOrLoadGrafanaTemplates(tmplRenderer)
	require.NoError(t, err)

	testCases := map[string]struct {
		mco    obv1beta2.MultiClusterObservability
		expect func(*testing.T, []*unstructured.Unstructured)
	}{
		"MCOA for metrics is enabled": {
			mco: obv1beta2.MultiClusterObservability{
				Spec: obv1beta2.MultiClusterObservabilitySpec{
					Capabilities: &obv1beta2.CapabilitiesSpec{
						Platform: &obv1beta2.PlatformCapabilitiesSpec{
							Metrics: obv1beta2.PlatformMetricsSpec{
								Collection: obv1beta2.PlatformMetricsCollectionSpec{
									Enabled: true,
								},
							},
						},
					},
				},
			},
			expect: func(t *testing.T, res []*unstructured.Unstructured) {
				assert.Len(t, res, 44)
			},
		},
		"MCOA for metrics is disabled": {
			mco: obv1beta2.MultiClusterObservability{},
			expect: func(t *testing.T, res []*unstructured.Unstructured) {
				assert.Len(t, res, 40)
				for _, r := range res {
					assert.NotContains(t, r.GetName(), "nexus")
				}
			},
		},
	}

	for tcName, tc := range testCases {

		t.Run(tcName, func(t *testing.T) {
			mcoRenderer := &MCORenderer{
				renderer: rendererutil.NewRenderer(),
				cr:       &tc.mco,
			}

			mcoRenderer.newGranfanaRenderer()
			namespace := "myns"
			grafanaResources, err := mcoRenderer.renderGrafanaTemplates(grafanaTemplates, namespace, nil)
			require.NoError(t, err)
			for _, r := range grafanaResources {
				if r.GetKind() == "ClusterRole" {
					continue
				}
				assert.Equal(t, namespace, r.GetNamespace(), fmt.Sprintf(" resource %s/%s", r.GetKind(), r.GetName()))
			}

			tc.expect(t, grafanaResources)
		})
	}
}
