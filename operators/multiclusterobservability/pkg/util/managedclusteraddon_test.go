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
	addonframeworkutils "open-cluster-management.io/addon-framework/pkg/utils"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	addonv1beta1 "open-cluster-management.io/api/addon/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	name      = "test"
	namespace = "test"
)

func TestManagedClusterAddon(t *testing.T) {
	s := scheme.Scheme
	addonv1beta1.AddToScheme(s)
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
	addonv1beta1.AddToScheme(s)

	adc := &addonv1beta1.AddOnDeploymentConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-adc",
			Namespace: "test-ns",
		},
		Spec: addonv1beta1.AddOnDeploymentConfigSpec{
			CustomizedVariables: []addonv1beta1.CustomizedVariable{
				{Name: "key1", Value: "value1"},
			},
		},
	}

	expectedHash, err := addonframeworkutils.GetAddOnDeploymentConfigSpecHash(adc)
	if err != nil {
		t.Fatalf("Failed to compute expected spec hash: (%v)", err)
	}

	cma := &addonv1beta1.ClusterManagementAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name: ObservabilityController,
		},
		Spec: addonv1beta1.ClusterManagementAddOnSpec{
			DefaultConfigs: []addonv1beta1.AddOnConfig{
				{
					ConfigGroupResource: addonv1beta1.ConfigGroupResource{
						Group:    AddonGroup,
						Resource: AddonDeploymentConfigResource,
					},
					ConfigReferent: addonv1beta1.ConfigReferent{
						Name:      "test-adc",
						Namespace: "test-ns",
					},
				},
			},
		},
	}

	c := fake.NewClientBuilder().
		WithObjects(cma, adc).
		WithStatusSubresource(&addonv1alpha1.ManagedClusterAddOn{}).
		Build()

	firstAddon, err := CreateManagedClusterAddonCR(context.Background(), c, namespace, "testKey", "value")
	if err != nil {
		t.Fatalf("Failed to create managedclusteraddon: (%v)", err)
	}

	if firstAddon.Status.AddOnMeta.DisplayName != "Observability Controller" {
		t.Fatalf("Expected DisplayName to be set, got: %s", firstAddon.Status.AddOnMeta.DisplayName)
	}

	if len(firstAddon.Status.ConfigReferences) != 1 {
		t.Fatalf("Expected 1 ConfigReference, got: %d", len(firstAddon.Status.ConfigReferences))
	}
	if firstAddon.Status.ConfigReferences[0].DesiredConfig.SpecHash != expectedHash {
		t.Fatalf("Expected specHash %q, got: %q", expectedHash, firstAddon.Status.ConfigReferences[0].DesiredConfig.SpecHash)
	}

	// Second call should NOT modify status since SpecHash is already correct
	secondAddon, err := CreateManagedClusterAddonCR(context.Background(), c, namespace, "testKey", "value")
	if err != nil {
		t.Fatalf("Failed on second call to CreateManagedClusterAddonCR: (%v)", err)
	}

	if len(secondAddon.Status.ConfigReferences) != 1 {
		t.Fatalf("Expected 1 ConfigReference, got: %d", len(secondAddon.Status.ConfigReferences))
	}
	if secondAddon.Status.ConfigReferences[0].DesiredConfig.SpecHash != expectedHash {
		t.Fatalf("Expected specHash to remain %q, got: %q",
			expectedHash, secondAddon.Status.ConfigReferences[0].DesiredConfig.SpecHash)
	}
}

func TestManagedClusterAddonConfigReferencesInitializedWhenCMADefaultConfigAdded(t *testing.T) {
	s := scheme.Scheme
	addonv1alpha1.AddToScheme(s)
	c := fake.NewClientBuilder().
		WithStatusSubresource(&addonv1alpha1.ManagedClusterAddOn{}).
		WithStatusSubresource(&addonv1beta1.ClusterManagementAddOn{}).
		Build()

	// First: Create MCA without any CMA (no defaultConfig available)
	firstAddon, err := CreateManagedClusterAddonCR(context.Background(), c, namespace, "testKey", "value")
	if err != nil {
		t.Fatalf("Failed to create managedclusteraddon: (%v)", err)
	}

	if len(firstAddon.Status.ConfigReferences) != 0 {
		t.Fatalf("Expected 0 ConfigReferences, got: %d", len(firstAddon.Status.ConfigReferences))
	}

	// Second: Create CMA with defaultConfig AND the referenced ADC
	adc := &addonv1beta1.AddOnDeploymentConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-config",
			Namespace: "test-ns",
		},
		Spec: addonv1beta1.AddOnDeploymentConfigSpec{
			CustomizedVariables: []addonv1beta1.CustomizedVariable{
				{Name: "platform", Value: "enabled"},
			},
		},
	}
	if err := c.Create(context.Background(), adc); err != nil {
		t.Fatalf("Failed to create AddOnDeploymentConfig: (%v)", err)
	}

	expectedHash, err := addonframeworkutils.GetAddOnDeploymentConfigSpecHash(adc)
	if err != nil {
		t.Fatalf("Failed to compute expected spec hash: (%v)", err)
	}

	cma := &addonv1beta1.ClusterManagementAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name: ObservabilityController,
		},
		Spec: addonv1beta1.ClusterManagementAddOnSpec{
			DefaultConfigs: []addonv1beta1.AddOnConfig{
				{
					ConfigGroupResource: addonv1beta1.ConfigGroupResource{
						Group:    AddonGroup,
						Resource: AddonDeploymentConfigResource,
					},
					ConfigReferent: addonv1beta1.ConfigReferent{
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

	// Third: Reconcile — should initialize ConfigReferences with specHash
	secondAddon, err := CreateManagedClusterAddonCR(context.Background(), c, namespace, "testKey", "value")
	if err != nil {
		t.Fatalf("Failed on second call to CreateManagedClusterAddonCR: (%v)", err)
	}

	if len(secondAddon.Status.ConfigReferences) != 1 {
		t.Fatalf("Expected 1 ConfigReference after CMA defaultConfig added, got: %d", len(secondAddon.Status.ConfigReferences))
	}
	if secondAddon.Status.ConfigReferences[0].ConfigReferent.Name != "test-config" {
		t.Fatalf("Expected config name 'test-config', got: %s", secondAddon.Status.ConfigReferences[0].ConfigReferent.Name)
	}
	if secondAddon.Status.ConfigReferences[0].DesiredConfig.SpecHash != expectedHash {
		t.Fatalf("Expected specHash %q, got: %q",
			expectedHash, secondAddon.Status.ConfigReferences[0].DesiredConfig.SpecHash)
	}

	// Fourth: Call again — should NOT re-update (specHash already populated)
	thirdAddon, err := CreateManagedClusterAddonCR(context.Background(), c, namespace, "testKey", "value")
	if err != nil {
		t.Fatalf("Failed on third call to CreateManagedClusterAddonCR: (%v)", err)
	}

	if len(thirdAddon.Status.ConfigReferences) == 0 {
		t.Fatal("Expected ConfigReferences to be preserved, got empty slice")
	}
	if thirdAddon.Status.ConfigReferences[0].DesiredConfig.SpecHash != expectedHash {
		t.Fatalf("Expected specHash to be preserved as %q, got: %q",
			expectedHash, thirdAddon.Status.ConfigReferences[0].DesiredConfig.SpecHash)
	}
}

func TestManagedClusterAddonSpecHashUpdatedWhenADCChanges(t *testing.T) {
	s := scheme.Scheme
	addonv1alpha1.AddToScheme(s)

	adc := &addonv1beta1.AddOnDeploymentConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-config",
			Namespace: "test-ns",
		},
		Spec: addonv1beta1.AddOnDeploymentConfigSpec{
			CustomizedVariables: []addonv1beta1.CustomizedVariable{
				{Name: "key1", Value: "value1"},
			},
		},
	}

	originalHash, err := addonframeworkutils.GetAddOnDeploymentConfigSpecHash(adc)
	if err != nil {
		t.Fatalf("Failed to compute original spec hash: (%v)", err)
	}

	cma := &addonv1beta1.ClusterManagementAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name: ObservabilityController,
		},
		Spec: addonv1beta1.ClusterManagementAddOnSpec{
			DefaultConfigs: []addonv1beta1.AddOnConfig{
				{
					ConfigGroupResource: addonv1beta1.ConfigGroupResource{
						Group:    AddonGroup,
						Resource: AddonDeploymentConfigResource,
					},
					ConfigReferent: addonv1beta1.ConfigReferent{
						Name:      "test-config",
						Namespace: "test-ns",
					},
				},
			},
		},
	}

	c := fake.NewClientBuilder().
		WithObjects(cma, adc).
		WithStatusSubresource(&addonv1alpha1.ManagedClusterAddOn{}).
		Build()

	firstAddon, err := CreateManagedClusterAddonCR(context.Background(), c, namespace, "testKey", "value")
	if err != nil {
		t.Fatalf("Failed to create managedclusteraddon: (%v)", err)
	}

	if len(firstAddon.Status.ConfigReferences) == 0 {
		t.Fatal("Expected ConfigReferences to be initialized, got empty slice")
	}
	if firstAddon.Status.ConfigReferences[0].DesiredConfig.SpecHash != originalHash {
		t.Fatalf("Expected specHash %q, got: %q", originalHash, firstAddon.Status.ConfigReferences[0].DesiredConfig.SpecHash)
	}

	// Modify the ADC spec
	adc.Spec.CustomizedVariables = append(adc.Spec.CustomizedVariables,
		addonv1beta1.CustomizedVariable{Name: "key2", Value: "value2"})
	if err := c.Update(context.Background(), adc); err != nil {
		t.Fatalf("Failed to update ADC: (%v)", err)
	}

	updatedHash, err := addonframeworkutils.GetAddOnDeploymentConfigSpecHash(adc)
	if err != nil {
		t.Fatalf("Failed to compute updated spec hash: (%v)", err)
	}
	if updatedHash == originalHash {
		t.Fatal("Expected updated hash to differ from original")
	}

	// Reconcile again with a stale but non-empty MCA SpecHash.
	secondAddon, err := CreateManagedClusterAddonCR(context.Background(), c, namespace, "testKey", "value")
	if err != nil {
		t.Fatalf("Failed on re-init call: (%v)", err)
	}

	if len(secondAddon.Status.ConfigReferences) == 0 {
		t.Fatal("Expected ConfigReferences after re-init, got empty slice")
	}
	if secondAddon.Status.ConfigReferences[0].DesiredConfig.SpecHash != updatedHash {
		t.Fatalf("Expected updated specHash %q, got: %q",
			updatedHash, secondAddon.Status.ConfigReferences[0].DesiredConfig.SpecHash)
	}
}

func TestManagedClusterAddonSpecHashUpdatedWhenADCChangesAndStoredHashEmpty(t *testing.T) {
	s := scheme.Scheme
	addonv1alpha1.AddToScheme(s)

	adc := &addonv1beta1.AddOnDeploymentConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-config",
			Namespace: "test-ns",
		},
		Spec: addonv1beta1.AddOnDeploymentConfigSpec{
			CustomizedVariables: []addonv1beta1.CustomizedVariable{
				{Name: "key1", Value: "value1"},
			},
		},
	}

	originalHash, err := addonframeworkutils.GetAddOnDeploymentConfigSpecHash(adc)
	if err != nil {
		t.Fatalf("Failed to compute original spec hash: (%v)", err)
	}

	cma := &addonv1beta1.ClusterManagementAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name: ObservabilityController,
		},
		Spec: addonv1beta1.ClusterManagementAddOnSpec{
			DefaultConfigs: []addonv1beta1.AddOnConfig{
				{
					ConfigGroupResource: addonv1beta1.ConfigGroupResource{
						Group:    AddonGroup,
						Resource: AddonDeploymentConfigResource,
					},
					ConfigReferent: addonv1beta1.ConfigReferent{
						Name:      "test-config",
						Namespace: "test-ns",
					},
				},
			},
		},
	}

	c := fake.NewClientBuilder().
		WithObjects(cma, adc).
		WithStatusSubresource(&addonv1alpha1.ManagedClusterAddOn{}).
		Build()

	firstAddon, err := CreateManagedClusterAddonCR(context.Background(), c, namespace, "testKey", "value")
	if err != nil {
		t.Fatalf("Failed to create managedclusteraddon: (%v)", err)
	}

	if len(firstAddon.Status.ConfigReferences) == 0 {
		t.Fatal("Expected ConfigReferences to be initialized, got empty slice")
	}
	if firstAddon.Status.ConfigReferences[0].DesiredConfig.SpecHash != originalHash {
		t.Fatalf("Expected specHash %q, got: %q", originalHash, firstAddon.Status.ConfigReferences[0].DesiredConfig.SpecHash)
	}

	// Modify the ADC spec.
	adc.Spec.CustomizedVariables = append(adc.Spec.CustomizedVariables,
		addonv1beta1.CustomizedVariable{Name: "key2", Value: "value2"})
	if err := c.Update(context.Background(), adc); err != nil {
		t.Fatalf("Failed to update ADC: (%v)", err)
	}

	updatedHash, err := addonframeworkutils.GetAddOnDeploymentConfigSpecHash(adc)
	if err != nil {
		t.Fatalf("Failed to compute updated spec hash: (%v)", err)
	}
	if updatedHash == originalHash {
		t.Fatal("Expected updated hash to differ from original")
	}

	// Simulate that the MCA's SpecHash is now stale and empty.
	firstAddon.Status.ConfigReferences[0].DesiredConfig.SpecHash = ""
	if err := c.Status().Update(context.Background(), firstAddon); err != nil {
		t.Fatalf("Failed to clear specHash: (%v)", err)
	}

	secondAddon, err := CreateManagedClusterAddonCR(context.Background(), c, namespace, "testKey", "value")
	if err != nil {
		t.Fatalf("Failed on re-init call: (%v)", err)
	}

	if len(secondAddon.Status.ConfigReferences) == 0 {
		t.Fatal("Expected ConfigReferences after re-init, got empty slice")
	}
	if secondAddon.Status.ConfigReferences[0].DesiredConfig.SpecHash != updatedHash {
		t.Fatalf("Expected updated specHash %q, got: %q",
			updatedHash, secondAddon.Status.ConfigReferences[0].DesiredConfig.SpecHash)
	}
}
