// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	"testing"

	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
)

const (
	// name      = "test"
	namespace = "test"
)

func TestManagedClusterAddon(t *testing.T) {
	s := scheme.Scheme
	addonv1alpha1.AddToScheme(s)
	c := fake.NewClientBuilder().WithStatusSubresource(&addonv1alpha1.ManagedClusterAddOn{}).Build()
	_, err := CreateManagedClusterAddonCR(context.Background(), c, namespace, "testKey", "value")
	if err != nil {
		t.Fatalf("Failed to create managedclusteraddon: (%v)", err)
	}
	addon := &addonv1alpha1.ManagedClusterAddOn{}
	err = c.Get(context.TODO(),
		types.NamespacedName{
			Name:      config.ManagedClusterAddonName,
			Namespace: namespace,
		},
		addon,
	)
	if err != nil {
		t.Fatalf("Failed to get managedclusteraddon: (%v)", err)
	}
}

// TestUpdateManagedClusterAddOnSpecHash tests the MCA spec hash update functionality
func TestUpdateManagedClusterAddOnSpecHash(t *testing.T) {
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

	// Create ManagedClusterAddOn
	mca := &addonv1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.ManagedClusterAddonName,
			Namespace: namespace,
		},
		Spec: addonv1alpha1.ManagedClusterAddOnSpec{
			InstallNamespace: "test-install-ns",
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&addonv1alpha1.ManagedClusterAddOn{}).WithRuntimeObjects(addonConfig, mca).Build()
	ctx := context.Background()

	// Update spec hash
	err := UpdateManagedClusterAddOnSpecHash(ctx, c, namespace, addonConfig)
	if err != nil {
		t.Fatalf("UpdateManagedClusterAddOnSpecHash failed: %v", err)
	}

	// Verify the status was updated
	updatedMCA := &addonv1alpha1.ManagedClusterAddOn{}
	err = c.Get(ctx, types.NamespacedName{Name: config.ManagedClusterAddonName, Namespace: namespace}, updatedMCA)
	if err != nil {
		t.Fatalf("Failed to get updated ManagedClusterAddOn: %v", err)
	}

	// Check that ConfigReferences was created
	if len(updatedMCA.Status.ConfigReferences) == 0 {
		t.Fatal("ConfigReferences should not be empty after spec hash update")
	}

	// Find the AddOnDeploymentConfig reference
	var foundRef *addonv1alpha1.ConfigReference
	for i, ref := range updatedMCA.Status.ConfigReferences {
		if ref.ConfigGroupResource.Group == AddonGroup && ref.ConfigGroupResource.Resource == AddonDeploymentConfigResource {
			foundRef = &updatedMCA.Status.ConfigReferences[i]
			break
		}
	}

	if foundRef == nil {
		t.Fatal("AddOnDeploymentConfig ConfigReference not found in status")
	}

	// Verify the spec hash is set
	if foundRef.DesiredConfig == nil || foundRef.DesiredConfig.SpecHash == "" {
		t.Fatal("SpecHash should be set in DesiredConfig")
	}

	// Verify the config referent matches
	if foundRef.ConfigReferent.Name != addonConfig.Name || foundRef.ConfigReferent.Namespace != addonConfig.Namespace {
		t.Fatalf("ConfigReferent mismatch: got %s/%s, want %s/%s",
			foundRef.ConfigReferent.Namespace, foundRef.ConfigReferent.Name,
			addonConfig.Namespace, addonConfig.Name)
	}

	// Calculate expected hash and verify it matches
	expectedHash, err := CalculateAddOnDeploymentConfigSpecHash(addonConfig)
	if err != nil {
		t.Fatalf("Failed to calculate expected hash: %v", err)
	}
	if foundRef.DesiredConfig.SpecHash != expectedHash {
		t.Fatalf("SpecHash mismatch: got %s, want %s", foundRef.DesiredConfig.SpecHash, expectedHash)
	}
}

// TestUpdateManagedClusterAddOnSpecHashWithExistingRef tests updating an existing ConfigReference
func TestUpdateManagedClusterAddOnSpecHashWithExistingRef(t *testing.T) {
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

	// Create ManagedClusterAddOn with existing ConfigReference
	mca := &addonv1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.ManagedClusterAddonName,
			Namespace: namespace,
		},
		Spec: addonv1alpha1.ManagedClusterAddOnSpec{
			InstallNamespace: "test-install-ns",
		},
		Status: addonv1alpha1.ManagedClusterAddOnStatus{
			ConfigReferences: []addonv1alpha1.ConfigReference{
				{
					ConfigGroupResource: addonv1alpha1.ConfigGroupResource{
						Group:    AddonGroup,
						Resource: AddonDeploymentConfigResource,
					},
					ConfigReferent: addonv1alpha1.ConfigReferent{
						Name:      "old-config",
						Namespace: "old-ns",
					},
					DesiredConfig: &addonv1alpha1.ConfigSpecHash{
						SpecHash: "old-hash",
					},
				},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&addonv1alpha1.ManagedClusterAddOn{}).WithRuntimeObjects(addonConfig, mca).Build()
	ctx := context.Background()

	// Update spec hash
	err := UpdateManagedClusterAddOnSpecHash(ctx, c, namespace, addonConfig)
	if err != nil {
		t.Fatalf("UpdateManagedClusterAddOnSpecHash failed: %v", err)
	}

	// Verify the status was updated
	updatedMCA := &addonv1alpha1.ManagedClusterAddOn{}
	err = c.Get(ctx, types.NamespacedName{Name: config.ManagedClusterAddonName, Namespace: namespace}, updatedMCA)
	if err != nil {
		t.Fatalf("Failed to get updated ManagedClusterAddOn: %v", err)
	}

	// Should still have only one ConfigReference
	if len(updatedMCA.Status.ConfigReferences) != 1 {
		t.Fatalf("Expected 1 ConfigReference, got %d", len(updatedMCA.Status.ConfigReferences))
	}

	ref := updatedMCA.Status.ConfigReferences[0]

	// Verify the config referent was updated
	if ref.ConfigReferent.Name != addonConfig.Name || ref.ConfigReferent.Namespace != addonConfig.Namespace {
		t.Fatalf("ConfigReferent should be updated: got %s/%s, want %s/%s",
			ref.ConfigReferent.Namespace, ref.ConfigReferent.Name,
			addonConfig.Namespace, addonConfig.Name)
	}

	// Verify the hash was updated
	expectedHash, _ := CalculateAddOnDeploymentConfigSpecHash(addonConfig)
	if ref.DesiredConfig.SpecHash != expectedHash {
		t.Fatalf("SpecHash should be updated: got %s, want %s", ref.DesiredConfig.SpecHash, expectedHash)
	}
	if ref.DesiredConfig.SpecHash == "old-hash" {
		t.Fatal("SpecHash should have changed from old-hash")
	}
}

// TestUpdateManagedClusterAddOnSpecHashNilConfig tests handling of nil config
func TestUpdateManagedClusterAddOnSpecHashNilConfig(t *testing.T) {
	s := scheme.Scheme
	addonv1alpha1.AddToScheme(s)

	mca := &addonv1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.ManagedClusterAddonName,
			Namespace: namespace,
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&addonv1alpha1.ManagedClusterAddOn{}).WithRuntimeObjects(mca).Build()
	ctx := context.Background()

	// Should not error with nil config
	err := UpdateManagedClusterAddOnSpecHash(ctx, c, namespace, nil)
	if err != nil {
		t.Fatalf("UpdateManagedClusterAddOnSpecHash should handle nil config gracefully: %v", err)
	}
}
