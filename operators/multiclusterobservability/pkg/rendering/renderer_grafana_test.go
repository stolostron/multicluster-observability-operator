// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rendering

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
								Default: obv1beta2.PlatformMetricsDefaultSpec{
									Enabled: true,
								},
							},
						},
					},
				},
			},
			expect: func(t *testing.T, templates []*unstructured.Unstructured) {
				assert.Greater(t, len(templates), 1)
				deprecatedCount := 0
				homeDashboardsCount := 0

				// Check that the deprecated field is added
				for _, template := range templates {
					if template.GetKind() != "ConfigMap" {
						continue
					}

					templateJson, err := template.MarshalJSON()
					assert.NoError(t, err)

					templateStr := string(templateJson)
					isDeprecated := strings.Contains(templateStr, "DEPRECATED")

					// Ensure mcoa dashboards are not the ones marked as deprecated
					annotations := template.GetAnnotations()
					assert.False(t, isDeprecated && strings.Contains(annotations[dashboardFolderAnnotationKey], "MCOA"))

					if isDeprecated {
						deprecatedCount++
					}

					// Ensure home dashboard is set for mcoa and disabled for the standard dashboard
					labels := template.GetLabels()
					if _, ok := labels[homeDashboardUIDLabelKey]; ok {
						homeDashboardsCount++
						isHome, ok := annotations[setHomeDashboardAnnotationKey]
						if strings.Contains(templateStr, "MCOA") {
							assert.True(t, ok)
							assert.Equal(t, "true", isHome)
						} else {
							assert.True(t, ok)
							assert.Equal(t, "false", isHome)
						}
					}
				}

				assert.NotZero(t, deprecatedCount)
				assert.Equal(t, 2, homeDashboardsCount)
			},
		},
		"MCOA for metrics is disabled": {
			mco: obv1beta2.MultiClusterObservability{},
			expect: func(t *testing.T, templates []*unstructured.Unstructured) {
				assert.Greater(t, len(templates), 1)
				forbiddenTypes := []string{"ScrapeConfig", "PrometheusRule"}
				homeDashboardsCount := 0
				// Check that the deprecated string is never added
				for _, template := range templates {
					assert.NotContains(t, forbiddenTypes, template.GetKind())

					if template.GetKind() != "ConfigMap" {
						continue
					}

					templateJson, err := template.MarshalJSON()
					assert.NoError(t, err)

					templateStr := string(templateJson)
					assert.NotContains(t, templateStr, "DEPRECATED")

					// Ensure home dashboard is disabled for mcoa and set for the standard dashboard
					annotations := template.GetAnnotations()
					labels := template.GetLabels()
					if _, ok := labels[homeDashboardUIDLabelKey]; ok {
						homeDashboardsCount++
						isHome, ok := annotations[setHomeDashboardAnnotationKey]
						if strings.Contains(templateStr, "MCOA") {
							assert.True(t, ok)
							assert.Equal(t, "false", isHome)
						} else {
							assert.True(t, ok)
							assert.Equal(t, "true", isHome)
						}
					}
				}
				assert.Equal(t, 2, homeDashboardsCount)
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
