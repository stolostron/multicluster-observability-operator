// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rendering

import (
	"context"
	"os"
	"path"
	"strings"
	"testing"

	ocinfrav1 "github.com/openshift/api/config/v1"
	imagev1 "github.com/openshift/api/image/v1"
	fakeimageclient "github.com/openshift/client-go/image/clientset/versioned/fake"
	fakeimagev1client "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1/fake"
	"github.com/openshift/library-go/pkg/crypto"
	mcoshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	templatesutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering/templates"
	obsv1alpha1 "github.com/stolostron/observatorium-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRender(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir %v", err)
	}
	templatesPath := path.Join(path.Dir(path.Dir(wd)), "manifests")
	t.Setenv(templatesutil.TemplatesPathEnvVar, templatesPath)

	mchcr := &mcov1beta2.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "test"},
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
	s := runtime.NewScheme()
	corev1.AddToScheme(s)
	obsv1alpha1.AddToScheme(s)
	kubeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(clientCa).Build()

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

	renderer := NewMCORenderer(mchcr, kubeClient, imageClient)
	_, err = renderer.Render()
	if err != nil {
		t.Fatalf("failed to render MultiClusterObservability: %v", err)
	}
}

func TestTLSProfilePropagation(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir %v", err)
	}
	templatesPath := path.Join(path.Dir(path.Dir(wd)), "manifests")
	t.Setenv(templatesutil.TemplatesPathEnvVar, templatesPath)

	mchcr := &mcov1beta2.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "test"},
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
	s := runtime.NewScheme()
	corev1.AddToScheme(s)
	obsv1alpha1.AddToScheme(s)
	testClient := fake.NewClientBuilder().WithScheme(s).WithObjects(clientCa).Build()
	err = config.SetOperandNames(testClient)
	if err != nil {
		t.Fatal(err)
	}
	imageClient := &fakeimagev1client.FakeImageV1{Fake: &(fakeimageclient.NewSimpleClientset().Fake)}
	_, err = imageClient.ImageStreams("openshift").Create(context.Background(),
		&imagev1.ImageStream{
			ObjectMeta: metav1.ObjectMeta{Name: "oauth-proxy", Namespace: "openshift"},
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

	tests := []struct {
		name          string
		profile       *ocinfrav1.TLSSecurityProfile
		expectedMin   string
		expectedCiph  []string
		expectedProxy string
	}{
		{
			name:          "nil profile",
			profile:       nil,
			expectedMin:   "VersionTLS12",
			expectedCiph:  crypto.OpenSSLToIANACipherSuites(ocinfrav1.TLSProfiles[ocinfrav1.TLSProfileIntermediateType].Ciphers),
			expectedProxy: "TLSv1.2",
		},
		{
			name: "old profile",
			profile: &ocinfrav1.TLSSecurityProfile{
				Type: ocinfrav1.TLSProfileOldType,
			},
			expectedMin:   "VersionTLS10",
			expectedCiph:  crypto.OpenSSLToIANACipherSuites(ocinfrav1.TLSProfiles[ocinfrav1.TLSProfileOldType].Ciphers),
			expectedProxy: "TLSv1.0",
		},
		{
			name: "intermediate profile",
			profile: &ocinfrav1.TLSSecurityProfile{
				Type: ocinfrav1.TLSProfileIntermediateType,
			},
			expectedMin:   "VersionTLS12",
			expectedCiph:  crypto.OpenSSLToIANACipherSuites(ocinfrav1.TLSProfiles[ocinfrav1.TLSProfileIntermediateType].Ciphers),
			expectedProxy: "TLSv1.2",
		},
		{
			// The modern profile uses TLS 1.3, which requires no cipher suites because TLS 1.3 cipher suites
			// are not configurable in Go and are handled automatically.
			name: "modern profile",
			profile: &ocinfrav1.TLSSecurityProfile{
				Type: ocinfrav1.TLSProfileModernType,
			},
			expectedMin:   "VersionTLS13",
			expectedCiph:  nil,
			expectedProxy: "TLSv1.3",
		},
		{
			name: "custom profile",
			profile: &ocinfrav1.TLSSecurityProfile{
				Type: ocinfrav1.TLSProfileCustomType,
				Custom: &ocinfrav1.CustomTLSProfile{
					TLSProfileSpec: ocinfrav1.TLSProfileSpec{
						MinTLSVersion: ocinfrav1.VersionTLS11,
						Ciphers:       []string{"AES128-SHA"}, // OpenSSL format
					},
				},
			},
			expectedMin:   "VersionTLS11",
			expectedCiph:  []string{"TLS_RSA_WITH_AES_128_CBC_SHA"}, // IANA format
			expectedProxy: "TLSv1.1",
		},
		{
			name: "custom profile with nil custom spec falls back to intermediate",
			profile: &ocinfrav1.TLSSecurityProfile{
				Type: ocinfrav1.TLSProfileCustomType,
			},
			expectedMin:   "VersionTLS12",
			expectedCiph:  crypto.OpenSSLToIANACipherSuites(ocinfrav1.TLSProfiles[ocinfrav1.TLSProfileIntermediateType].Ciphers),
			expectedProxy: "TLSv1.2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			renderer := NewMCORenderer(mchcr, testClient, imageClient)
			renderer.WithRendererOptions(&RendererOptions{
				TLSProfile: tt.profile,
				MCOAOptions: MCOARendererOptions{
					MetricsHubHostname:             "test",
					MetricsHubAlertmanagerHostname: "test",
				},
			})
			objs, err := renderer.Render()
			assert.NoError(t, err)

			var foundProxy, foundGrafana, foundAlertmanager bool

			for _, obj := range objs {
				if obj.GetKind() == "Deployment" && obj.GetName() == config.GetOperandName(config.RBACQueryProxy) {
					foundProxy = true
					var dep appsv1.Deployment
					err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &dep)
					assert.NoError(t, err)

					args1 := dep.Spec.Template.Spec.Containers[1].Args
					assert.Contains(t, args1, "--tls-min-version="+tt.expectedProxy)

					if len(tt.expectedCiph) > 0 {
						cipherArg := "--tls-cipher-suites=" + strings.Join(tt.expectedCiph, ",")
						assert.Contains(t, args1, cipherArg)
					} else {
						for _, arg := range args1 {
							assert.NotContains(t, arg, "--tls-cipher-suites=")
						}
					}
				}
				if obj.GetKind() == "Deployment" && obj.GetName() == config.GetOperandName(config.Grafana) {
					foundGrafana = true
					var dep appsv1.Deployment
					err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &dep)
					assert.NoError(t, err)

					args2 := dep.Spec.Template.Spec.Containers[2].Args
					assert.Contains(t, args2, "--tls-min-version="+tt.expectedProxy)

					if len(tt.expectedCiph) > 0 {
						cipherArg := "--tls-cipher-suites=" + strings.Join(tt.expectedCiph, ",")
						assert.Contains(t, args2, cipherArg)
					} else {
						for _, arg := range args2 {
							assert.NotContains(t, arg, "--tls-cipher-suites=")
						}
					}
				}
				if obj.GetKind() == "StatefulSet" && obj.GetName() == config.GetOperandName(config.Alertmanager) {
					foundAlertmanager = true
					var sts appsv1.StatefulSet
					err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &sts)
					assert.NoError(t, err)

					args2 := sts.Spec.Template.Spec.Containers[2].Args
					assert.Contains(t, args2, "--tls-min-version="+tt.expectedProxy)

					args3 := sts.Spec.Template.Spec.Containers[3].Args
					assert.Contains(t, args3, "--tls-min-version="+tt.expectedMin)

					if len(tt.expectedCiph) > 0 {
						cipherArg := "--tls-cipher-suites=" + strings.Join(tt.expectedCiph, ",")
						assert.Contains(t, args2, cipherArg)
						assert.Contains(t, args3, cipherArg)
					} else {
						for _, arg := range args2 {
							assert.NotContains(t, arg, "--tls-cipher-suites=")
						}
						for _, arg := range args3 {
							assert.NotContains(t, arg, "--tls-cipher-suites=")
						}
					}
				}
			}

			assert.True(t, foundProxy, "RBAC Query Proxy deployment not found")
			assert.True(t, foundGrafana, "Grafana deployment not found")
			assert.True(t, foundAlertmanager, "Alertmanager statefulset not found")
		})
	}
}

func TestGetOauthProxyFromImageStreams(t *testing.T) {
	imageClient := &fakeimagev1client.FakeImageV1{Fake: &(fakeimageclient.NewSimpleClientset().Fake)}
	_, err := imageClient.ImageStreams(config.OauthProxyImageStreamNamespace).Create(context.Background(),
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
