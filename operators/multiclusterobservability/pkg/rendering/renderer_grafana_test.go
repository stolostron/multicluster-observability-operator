package rendering

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	imagev1 "github.com/openshift/api/image/v1"
	fakeimageclient "github.com/openshift/client-go/image/clientset/versioned/fake"
	fakeimagev1client "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1/fake"
	obv1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/rendering/templates"
	rendererutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering"
	templatesutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering/templates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestRenderGrafana(t *testing.T) {
	wd, err := os.Getwd()
	assert.NoError(t, err)
	templatesPath := filepath.Join(wd, "..", "..", "manifests")
	tmplRenderer := templatesutil.NewTemplateRenderer(templatesPath)
	grafanaTemplates, err := templates.GetOrLoadGrafanaTemplates(tmplRenderer)
	require.NoError(t, err)

	imageClient := &fakeimagev1client.FakeImageV1{Fake: &(fakeimageclient.NewSimpleClientset().Fake)}
	_, err = imageClient.ImageStreams(config.OauthProxyImageStreamNamespace).Create(context.Background(),
		&imagev1.ImageStream{
			ObjectMeta: metav1.ObjectMeta{
				Name:      config.OauthProxyImageStreamName,
				Namespace: config.OauthProxyImageStreamNamespace,
			},
			Spec: imagev1.ImageStreamSpec{
				Tags: []imagev1.TagReference{
					{
						Name: "v4.4",
						From: &corev1.ObjectReference{
							Kind: "DockerImage",
							Name: "quay.io/openshift-release-dev/ocp-v4.0-art-dev",
						},
					},
				},
			},
		}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

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
				renderer:    rendererutil.NewRenderer(),
				cr:          &tc.mco,
				imageClient: imageClient,
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
