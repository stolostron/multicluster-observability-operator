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
	name      = "test"
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

func TestManagedClusterAddonStatusNotUpdatedOnSubsequentCalls(t *testing.T) {
	s := scheme.Scheme
	addonv1alpha1.AddToScheme(s)
	c := fake.NewClientBuilder().WithStatusSubresource(&addonv1alpha1.ManagedClusterAddOn{}).Build()

	// First call - creates MCA and initializes status
	firstAddon, err := CreateManagedClusterAddonCR(context.Background(), c, namespace, "testKey", "value")
	if err != nil {
		t.Fatalf("Failed to create managedclusteraddon: (%v)", err)
	}

	// Verify initial status was set
	if firstAddon.Status.AddOnMeta.DisplayName != "Observability Controller" {
		t.Fatalf("Expected DisplayName to be set, got: %s", firstAddon.Status.AddOnMeta.DisplayName)
	}

	// Simulate addon-framework populating specHash by updating status
	firstAddon.Status.ConfigReferences = []addonv1alpha1.ConfigReference{
		{
			ConfigGroupResource: addonv1alpha1.ConfigGroupResource{
				Group:    AddonGroup,
				Resource: AddonDeploymentConfigResource,
			},
			DesiredConfig: &addonv1alpha1.ConfigSpecHash{
				SpecHash: "test-spec-hash-from-framework",
			},
		},
	}
	if err := c.Status().Update(context.Background(), firstAddon); err != nil {
		t.Fatalf("Failed to update status: (%v)", err)
	}

	// Second call - should NOT modify status
	secondAddon, err := CreateManagedClusterAddonCR(context.Background(), c, namespace, "testKey", "value")
	if err != nil {
		t.Fatalf("Failed on second call to CreateManagedClusterAddonCR: (%v)", err)
	}

	// Verify specHash was not overwritten
	if len(secondAddon.Status.ConfigReferences) != 1 {
		t.Fatalf("Expected 1 ConfigReference, got: %d", len(secondAddon.Status.ConfigReferences))
	}
	if secondAddon.Status.ConfigReferences[0].DesiredConfig.SpecHash != "test-spec-hash-from-framework" {
		t.Fatalf("Expected specHash to remain 'test-spec-hash-from-framework', got: %s",
			secondAddon.Status.ConfigReferences[0].DesiredConfig.SpecHash)
	}
}

func TestManagedClusterAddonConfigReferencesInitializedWhenCMADefaultConfigAdded(t *testing.T) {
	s := scheme.Scheme
	addonv1alpha1.AddToScheme(s)
	c := fake.NewClientBuilder().
		WithStatusSubresource(&addonv1alpha1.ManagedClusterAddOn{}).
		WithStatusSubresource(&addonv1alpha1.ClusterManagementAddOn{}).
		Build()

	// First: Create MCA without any CMA (no defaultConfig available)
	firstAddon, err := CreateManagedClusterAddonCR(context.Background(), c, namespace, "testKey", "value")
	if err != nil {
		t.Fatalf("Failed to create managedclusteraddon: (%v)", err)
	}

	// Verify no ConfigReferences were initialized (no CMA defaultConfig)
	if len(firstAddon.Status.ConfigReferences) != 0 {
		t.Fatalf("Expected 0 ConfigReferences, got: %d", len(firstAddon.Status.ConfigReferences))
	}

	// Second: Create CMA with defaultConfig (simulating e2e test adding it)
	cma := &addonv1alpha1.ClusterManagementAddOn{
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
					DefaultConfig: &addonv1alpha1.ConfigReferent{
						Name:      "test-config",
						Namespace: "test-ns",
					},
				},
			},
		},
	}
	if err := c.Create(context.Background(), cma); err != nil {
		t.Fatalf("Failed to create ClusterManagementAddOn: (%v)", err)
	}

	// Third: Call CreateManagedClusterAddonCR again (simulating reconcile after CMA update)
	// This should initialize ConfigReferences now that CMA has defaultConfig
	secondAddon, err := CreateManagedClusterAddonCR(context.Background(), c, namespace, "testKey", "value")
	if err != nil {
		t.Fatalf("Failed on second call to CreateManagedClusterAddonCR: (%v)", err)
	}

	// Verify ConfigReferences were initialized
	if len(secondAddon.Status.ConfigReferences) != 1 {
		t.Fatalf("Expected 1 ConfigReference after CMA defaultConfig added, got: %d", len(secondAddon.Status.ConfigReferences))
	}
	if secondAddon.Status.ConfigReferences[0].ConfigReferent.Name != "test-config" {
		t.Fatalf("Expected config name 'test-config', got: %s", secondAddon.Status.ConfigReferences[0].ConfigReferent.Name)
	}

	// Fourth: Simulate addon-framework populating specHash
	secondAddon.Status.ConfigReferences[0].DesiredConfig = &addonv1alpha1.ConfigSpecHash{
		SpecHash: "framework-populated-hash",
	}
	if err := c.Status().Update(context.Background(), secondAddon); err != nil {
		t.Fatalf("Failed to update status with specHash: (%v)", err)
	}

	// Fifth: Call CreateManagedClusterAddonCR again - should NOT overwrite specHash
	thirdAddon, err := CreateManagedClusterAddonCR(context.Background(), c, namespace, "testKey", "value")
	if err != nil {
		t.Fatalf("Failed on third call to CreateManagedClusterAddonCR: (%v)", err)
	}

	// Verify specHash was preserved
	if thirdAddon.Status.ConfigReferences[0].DesiredConfig.SpecHash != "framework-populated-hash" {
		t.Fatalf("Expected specHash to be preserved as 'framework-populated-hash', got: %s",
			thirdAddon.Status.ConfigReferences[0].DesiredConfig.SpecHash)
	}
}
