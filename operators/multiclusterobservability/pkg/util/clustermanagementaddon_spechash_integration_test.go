// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

//go:build integration

package util

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	testEnv   *envtest.Environment
	restCfg   *rest.Config
	k8sClient client.Client
)

func TestMain(m *testing.M) {
	opts := zap.Options{
		Development: true,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	var err error
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "controllers", "multiclusterobservability", "testdata", "crd"),
			filepath.Join("..", "..", "..", "..", "tests", "run-in-kind", "router"),
		},
		ControlPlaneStopTimeout: 5 * time.Minute,
	}

	restCfg, err = testEnv.Start()
	if err != nil {
		panic(err)
	}

	scheme := createBaseScheme()
	k8sClient, err = client.New(restCfg, client.Options{Scheme: scheme})
	if err != nil {
		panic(err)
	}

	code := m.Run()

	err = testEnv.Stop()
	if err != nil {
		panic(err)
	}

	os.Exit(code)
}

func createBaseScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = kubescheme.AddToScheme(scheme)
	_ = addonv1alpha1.AddToScheme(scheme)
	_ = routev1.AddToScheme(scheme)
	return scheme
}

// TestIntegrationSpecHashCalculation tests the spec hash calculation functionality
func TestIntegrationSpecHashCalculation(t *testing.T) {
	// Test basic hash calculation
	addonConfig := &addonv1alpha1.AddOnDeploymentConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-config",
			Namespace: "test-ns",
		},
		Spec: addonv1alpha1.AddOnDeploymentConfigSpec{
			ProxyConfig: addonv1alpha1.ProxyConfig{
				HTTPProxy:  "http://proxy.example.com",
				HTTPSProxy: "https://proxy.example.com",
				NoProxy:    "localhost,127.0.0.1",
			},
			NodePlacement: &addonv1alpha1.NodePlacement{
				NodeSelector: map[string]string{
					"kubernetes.io/os": "linux",
				},
			},
		},
	}

	hash1, err := calculateSpecHash(addonConfig)
	require.NoError(t, err)
	assert.NotEmpty(t, hash1, "Spec hash should not be empty")

	// Calculate hash again with same config - should be identical
	hash2, err := calculateSpecHash(addonConfig)
	require.NoError(t, err)
	assert.Equal(t, hash1, hash2, "Spec hashes should be identical for same config")

	// Test with nil config
	nilHash, err := calculateSpecHash(nil)
	require.NoError(t, err)
	assert.Empty(t, nilHash, "Spec hash for nil config should be empty")

	// Test hash changes when spec changes
	modifiedConfig := addonConfig.DeepCopy()
	modifiedConfig.Spec.ProxyConfig.HTTPProxy = "http://different-proxy.com"

	hash3, err := calculateSpecHash(modifiedConfig)
	require.NoError(t, err)
	assert.NotEqual(t, hash1, hash3, "Hash should change when spec changes")

	// Test that metadata changes don't affect hash
	metadataOnlyChange := addonConfig.DeepCopy()
	metadataOnlyChange.Name = "different-name"
	metadataOnlyChange.Namespace = "different-namespace"
	metadataOnlyChange.Labels = map[string]string{"new": "label"}

	hash4, err := calculateSpecHash(metadataOnlyChange)
	require.NoError(t, err)
	assert.Equal(t, hash1, hash4, "Hash should not change when only metadata changes")
}

// TestIntegrationClusterManagementAddonCreation tests that ClusterManagementAddon can be created successfully
func TestIntegrationClusterManagementAddonCreation(t *testing.T) {
	ctx := context.Background()
	testNamespace := "test-addon-creation"

	// Setup required resources
	resources := []client.Object{
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: config.GetMCONamespace(),
			},
		},
		&routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "grafana", // This must match config.GrafanaRouteName
				Namespace: config.GetMCONamespace(),
			},
			Spec: routev1.RouteSpec{
				Host: "grafana.example.com",
				To: routev1.RouteTargetReference{
					Kind: "Service",
					Name: "grafana",
				},
			},
		},
	}

	err := createResources(k8sClient, resources...)
	require.NoError(t, err)
	defer tearDownResources(t, k8sClient, resources...)

	// Create ClusterManagementAddon
	createdAddon, err := CreateClusterManagementAddon(ctx, k8sClient)
	require.NoError(t, err)
	require.NotNil(t, createdAddon)
	assert.Equal(t, ObservabilityController, createdAddon.Name)

	// Verify supported configs are set up
	require.NotEmpty(t, createdAddon.Spec.SupportedConfigs, "No supported configs found in ClusterManagementAddon")

	found := false
	for _, config := range createdAddon.Spec.SupportedConfigs {
		if config.ConfigGroupResource.Group == AddonGroup &&
			config.ConfigGroupResource.Resource == AddonDeploymentConfigResource {
			found = true
			break
		}
	}
	assert.True(t, found, "AddOnDeploymentConfig not found in supported configs")

	// Clean up the created addon
	defer func() {
		err := DeleteClusterManagementAddon(ctx, k8sClient)
		assert.NoError(t, err)
	}()
}

// TestIntegrationUpdateSpecHashFunction tests the UpdateClusterManagementAddOnSpecHash function
// Note: This test focuses on the function behavior rather than status persistence,
// since envtest has limitations with status subresource updates.
func TestIntegrationUpdateSpecHashFunction(t *testing.T) {
	ctx := context.Background()
	testNamespace := "test-spec-hash-function"

	// Create test AddOnDeploymentConfig
	addonConfig := &addonv1alpha1.AddOnDeploymentConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-addon-config",
			Namespace: testNamespace,
		},
		Spec: addonv1alpha1.AddOnDeploymentConfigSpec{
			ProxyConfig: addonv1alpha1.ProxyConfig{
				HTTPProxy:  "http://proxy.example.com",
				HTTPSProxy: "https://proxy.example.com",
				NoProxy:    "localhost,127.0.0.1",
			},
			NodePlacement: &addonv1alpha1.NodePlacement{
				NodeSelector: map[string]string{
					"kubernetes.io/os": "linux",
				},
			},
		},
	}

	// Create a basic ClusterManagementAddon without status (more realistic for envtest)
	existingAddon := &addonv1alpha1.ClusterManagementAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name: ObservabilityController,
		},
		Spec: addonv1alpha1.ClusterManagementAddOnSpec{
			InstallStrategy: addonv1alpha1.InstallStrategy{
				Type: addonv1alpha1.AddonInstallStrategyManual,
			},
			SupportedConfigs: []addonv1alpha1.ConfigMeta{
				{
					ConfigGroupResource: addonv1alpha1.ConfigGroupResource{
						Group:    AddonGroup,
						Resource: AddonDeploymentConfigResource,
					},
				},
			},
		},
	}

	resources := []client.Object{
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		},
		addonConfig,
		existingAddon,
	}

	err := createResources(k8sClient, resources...)
	require.NoError(t, err)
	defer tearDownResources(t, k8sClient, resources...)

	// Test that the function handles missing DefaultConfigReferences gracefully
	// This is the expected behavior when status isn't properly initialized
	err = UpdateClusterManagementAddOnSpecHash(ctx, k8sClient, addonConfig)
	assert.NoError(t, err, "UpdateClusterManagementAddOnSpecHash should handle missing DefaultConfigReferences gracefully")

	// Verify the addon still exists and wasn't corrupted
	addon := &addonv1alpha1.ClusterManagementAddOn{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: ObservabilityController}, addon)
	require.NoError(t, err)
	assert.Equal(t, ObservabilityController, addon.Name)
}

// TestIntegrationUpdateSpecHashWithMissingDefaultConfigReference tests graceful handling of missing config references
func TestIntegrationUpdateSpecHashWithMissingDefaultConfigReference(t *testing.T) {
	ctx := context.Background()
	testNamespace := "test-missing-config-ref"

	addonConfig := &addonv1alpha1.AddOnDeploymentConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-config",
			Namespace: testNamespace,
		},
		Spec: addonv1alpha1.AddOnDeploymentConfigSpec{
			ProxyConfig: addonv1alpha1.ProxyConfig{
				HTTPProxy: "http://test.com",
			},
		},
	}

	// Create the observability-controller ClusterManagementAddon without DefaultConfigReferences
	testAddon := &addonv1alpha1.ClusterManagementAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name: ObservabilityController,
		},
		Spec: addonv1alpha1.ClusterManagementAddOnSpec{
			InstallStrategy: addonv1alpha1.InstallStrategy{
				Type: addonv1alpha1.AddonInstallStrategyManual,
			},
			SupportedConfigs: []addonv1alpha1.ConfigMeta{
				{
					ConfigGroupResource: addonv1alpha1.ConfigGroupResource{
						Group:    AddonGroup,
						Resource: AddonDeploymentConfigResource,
					},
				},
			},
		},
	}

	resources := []client.Object{
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		},
		addonConfig,
		testAddon,
	}

	err := createResources(k8sClient, resources...)
	require.NoError(t, err)
	defer tearDownResources(t, k8sClient, resources...)

	// Try to update spec hash - should not fail, should handle gracefully
	err = updateClusterManagementAddOnStatus(ctx, k8sClient, addonConfig)
	assert.NoError(t, err, "updateClusterManagementAddOnStatus should handle missing DefaultConfigReference gracefully")
}

// Helper functions

func createResources(k8sClient client.Client, resources ...client.Object) error {
	for _, resource := range resources {
		if err := k8sClient.Create(context.Background(), resource); err != nil {
			return err
		}
	}
	return nil
}

func tearDownResources(t *testing.T, k8sClient client.Client, resources ...client.Object) {
	for _, resource := range resources {
		err := k8sClient.Delete(context.Background(), resource)
		if err != nil {
			t.Logf("Failed to delete resource %s/%s: %v", resource.GetNamespace(), resource.GetName(), err)
		}
	}
}
