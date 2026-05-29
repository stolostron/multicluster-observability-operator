// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rendering

import (
	"fmt"
	"os"
	"path"
	"testing"

	imagev1 "github.com/openshift/api/image/v1"
	fakeimageclient "github.com/openshift/client-go/image/clientset/versioned/fake"
	fakeimagev1client "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1/fake"
	mcoshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	templatesutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering/templates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRender(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir %v", err)
	}
	templatesPath := path.Join(path.Dir(path.Dir(wd)), "manifests")
	os.Setenv(templatesutil.TemplatesPathEnvVar, templatesPath)
	defer os.Unsetenv(templatesutil.TemplatesPathEnvVar)

	mchcr := &mcov1beta2.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			ImagePullPolicy: "IfNotPresent",
			ImagePullSecret: "test",
			StorageConfig: &mcov1beta2.StorageConfig{
				MetricObjectStorage: &mcoshared.PreConfiguredStorage{
					Key:  "test",
					Name: "test",
				},
				StorageClass:            "gp2",
				AlertmanagerStorageSize: "1Gi",
				CompactStorageSize:      "1Gi",
				RuleStorageSize:         "1Gi",
				ReceiveStorageSize:      "1Gi",
				StoreStorageSize:        "1Gi",
			},
		},
	}

	clientCa := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "extension-apiserver-authentication",
			Namespace: "kube-system",
		},
		Data: map[string]string{
			"client-ca-file": "test",
		},
	}
	kubeClient := fake.NewClientBuilder().WithObjects(clientCa).Build()

	imageClient := &fakeimagev1client.FakeImageV1{Fake: &(fakeimageclient.NewSimpleClientset().Fake)}
	_, err = imageClient.ImageStreams(config.OauthProxyImageStreamNamespace).Create(t.Context(),
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

	renderer := NewMCORenderer(mchcr, kubeClient, imageClient)
	_, err = renderer.Render(t.Context())
	if err != nil {
		t.Fatalf("failed to render MultiClusterObservability: %v", err)
	}
}

func TestGetOauthProxyFromImageStreams(t *testing.T) {
	imageClient := &fakeimagev1client.FakeImageV1{Fake: &(fakeimageclient.NewSimpleClientset().Fake)}
	_, err := imageClient.ImageStreams(config.OauthProxyImageStreamNamespace).Create(t.Context(),
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
	found, oauthProxyImage := config.GetOauthProxyImage(imageClient)
	if !found {
		t.Fatal("Failed to get oauth proxy image")
	}
	assert.Equal(t, "quay.io/openshift-release-dev/ocp-v4.0-art-dev", oauthProxyImage)
}

func TestMCOAGrafanaResources(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	templatesPath := path.Join(path.Dir(path.Dir(wd)), "manifests")
	t.Setenv(templatesutil.TemplatesPathEnvVar, templatesPath)

	kubeClient := fake.NewClientBuilder().Build()

	testCases := map[string]struct {
		mco    mcov1beta2.MultiClusterObservability
		expect func(*testing.T, []*unstructured.Unstructured)
	}{
		"MCOA for metrics is enabled": {
			mco: mcov1beta2.MultiClusterObservability{
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
			expect: func(t *testing.T, resources []*unstructured.Unstructured) {
				assert.Empty(t, resources)
			},
		},
		"MCOA for metrics is disabled": {
			mco: mcov1beta2.MultiClusterObservability{},
			expect: func(t *testing.T, resources []*unstructured.Unstructured) {
				assert.NotEmpty(t, resources)

				allowedKinds := []string{"ScrapeConfig", "PrometheusRule"}
				kindCounts := map[string]int{}
				for _, resource := range resources {
					kind := resource.GetKind()
					assert.Contains(t, allowedKinds, kind)
					kindCounts[kind]++
				}

				assert.Equal(t, 7, kindCounts["ScrapeConfig"])
				assert.Equal(t, 3, kindCounts["PrometheusRule"])
			},
		},
	}

	for tcName, tc := range testCases {
		t.Run(tcName, func(t *testing.T) {
			renderer := NewMCORenderer(&tc.mco, kubeClient, nil)

			namespace := "myns"
			resources, err := renderer.MCOAGrafanaResources(t.Context(), namespace, nil)
			require.NoError(t, err)
			for _, resource := range resources {
				assert.Equal(t, namespace, resource.GetNamespace(),
					fmt.Sprintf("resource %s/%s", resource.GetKind(), resource.GetName()))
			}

			tc.expect(t, resources)
		})
	}
}
