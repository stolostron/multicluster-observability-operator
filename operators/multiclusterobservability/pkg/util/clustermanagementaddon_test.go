// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	"testing"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
)

func TestClusterManagmentAddon(t *testing.T) {
	s := scheme.Scheme
	addonv1alpha1.AddToScheme(s)
	routev1.AddToScheme(s)

	consoleRoute := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multicloud-console",
			Namespace: config.GetMCONamespace(),
		},
		Spec: routev1.RouteSpec{
			Host: "console",
		},
	}

	c := fake.NewClientBuilder().WithRuntimeObjects(consoleRoute).Build()
	_, err := CreateClusterManagementAddon(context.Background(), c)
	if err != nil {
		t.Fatalf("Failed to create clustermanagementaddon: (%v)", err)
	}
	_, err = CreateClusterManagementAddon(context.Background(), c)
	if err != nil {
		t.Fatalf("Failed to create clustermanagementaddon twice: (%v)", err)
	}
	addon := &addonv1alpha1.ClusterManagementAddOn{}
	err = c.Get(context.TODO(),
		types.NamespacedName{
			Name: ObservabilityController,
		},
		addon,
	)
	if err != nil {
		t.Fatalf("Failed to get clustermanagementaddon: (%v)", err)
	}
	if addon.Spec.AddOnConfiguration.CRDName != "observabilityaddons.observability.open-cluster-management.io" {
		t.Fatalf("Wrong CRD name included: %s", addon.Spec.AddOnConfiguration.CRDName)
	}
	if linkTxt, found := addon.ObjectMeta.Annotations["console.open-cluster-management.io/launch-link-text"]; found == false {
		t.Fatalf("No launch-link-text annotation included")
	} else {
		if linkTxt != "Grafana" {
			t.Fatalf("Wrong launch-link-text annotation: %s", linkTxt)
		}
	}

	if selfMgmt, found := addon.ObjectMeta.Annotations[addonv1alpha1.AddonLifecycleAnnotationKey]; found == false {
		t.Fatalf("no AddonLifecycle included")
	} else {
		if selfMgmt != addonv1alpha1.AddonLifecycleSelfManageAnnotationValue {
			t.Fatalf("Wrong AddonLifecycle annotation: %s", selfMgmt)
		}
	}

	err = DeleteClusterManagementAddon(context.Background(), c)
	if err != nil {
		t.Fatalf("Failed to delete clustermanagementaddon: (%v)", err)
	}
	err = c.Get(context.TODO(),
		types.NamespacedName{
			Name: ObservabilityController,
		},
		addon,
	)
	if err == nil || !errors.IsNotFound(err) {
		t.Fatalf("Failed to delete clustermanagementaddon: (%v)", err)
	}

	// Create a clustermanagement addon without the self-mgmt lifecycle annotation
	// This emulates an upgrade scenario where the addon is created without the self-mgmt annotation
	// and then we need to ensure it gets added during the update.
	clusterManagementAddon, err := newClusterManagementAddon(c)
	if err != nil {
		t.Fatalf("Failed to create new clustermanagementaddon: (%v)", err)
	}

	// Manually remove the self-management annotation before creating the object
	// This simulates an addon created without the annotation
	delete(clusterManagementAddon.ObjectMeta.Annotations, addonv1alpha1.AddonLifecycleAnnotationKey)

	if err := c.Create(context.TODO(), clusterManagementAddon); err != nil {
		t.Fatalf("Failed to create clustermanagementaddon: (%v)", err)
	}

	// Run the method under test, which should add the annotation
	_, err = CreateClusterManagementAddon(context.Background(), c)
	if err != nil {
		t.Fatalf("Failed to reconcile clustermanagementaddon: (%v)", err)
	}

	// Retrieve the updated object
	err = c.Get(context.TODO(),
		types.NamespacedName{
			Name: ObservabilityController,
		},
		addon,
	)
	if err != nil {
		t.Fatalf("Failed to get clustermanagementaddon: (%v)", err)
	}

	// Verify the annotation has been added
	if selfMgmt, found := addon.ObjectMeta.Annotations[addonv1alpha1.AddonLifecycleAnnotationKey]; found == false {
		t.Fatalf("Addon lifecycle annotation was not added as expected")
	} else {
		if selfMgmt != addonv1alpha1.AddonLifecycleSelfManageAnnotationValue {
			t.Fatalf("Wrong AddonLifecycle annotation: %s", selfMgmt)
		}
	}

	// delete it again for good measure
	err = DeleteClusterManagementAddon(context.Background(), c)
	if err != nil {
		t.Fatalf("Failed to delete clustermanagementaddon: (%v)", err)
	}
}

// TestSpecHashCalculation tests the spec hash calculation functionality
func TestSpecHashCalculation(t *testing.T) {
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
	if err != nil {
		t.Fatalf("Failed to calculate spec hash: %v", err)
	}
	if hash1 == "" {
		t.Fatal("Spec hash should not be empty")
	}

	// Calculate hash again with same config - should be identical
	hash2, err := calculateSpecHash(addonConfig)
	if err != nil {
		t.Fatalf("Failed to calculate spec hash second time: %v", err)
	}
	if hash1 != hash2 {
		t.Fatalf("Spec hashes should be identical: %s != %s", hash1, hash2)
	}

	// Test with nil config
	nilHash, err := calculateSpecHash(nil)
	if err != nil {
		t.Fatalf("Failed to calculate spec hash for nil config: %v", err)
	}
	if nilHash != "" {
		t.Fatal("Spec hash for nil config should be empty")
	}

	// Test hash changes when spec changes
	modifiedConfig := addonConfig.DeepCopy()
	modifiedConfig.Spec.ProxyConfig.HTTPProxy = "http://different-proxy.com"

	hash3, err := calculateSpecHash(modifiedConfig)
	if err != nil {
		t.Fatalf("Failed to calculate hash for modified config: %v", err)
	}
	if hash1 == hash3 {
		t.Fatal("Hash should change when spec changes")
	}

	// Test that metadata changes don't affect hash
	metadataOnlyChange := addonConfig.DeepCopy()
	metadataOnlyChange.Name = "different-name"
	metadataOnlyChange.Namespace = "different-namespace"
	metadataOnlyChange.Labels = map[string]string{"new": "label"}

	hash4, err := calculateSpecHash(metadataOnlyChange)
	if err != nil {
		t.Fatalf("Failed to calculate hash for metadata-only change: %v", err)
	}
	if hash1 != hash4 {
		t.Fatal("Hash should not change when only metadata changes")
	}
}

// TestUpdateSpecHashWithMissingDefaultConfigReference tests graceful handling of missing config references
func TestUpdateSpecHashWithMissingDefaultConfigReference(t *testing.T) {
	s := scheme.Scheme
	addonv1alpha1.AddToScheme(s)

	addonConfig := &addonv1alpha1.AddOnDeploymentConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-config",
			Namespace: "test-ns",
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

	c := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(addonConfig, testAddon).Build()
	ctx := context.Background()

	// Try to update spec hash - should not fail, should handle gracefully
	err := updateClusterManagementAddOnStatus(ctx, c, addonConfig)
	if err != nil {
		t.Fatalf("updateClusterManagementAddOnStatus should handle missing DefaultConfigReference gracefully: %v", err)
	}
}
