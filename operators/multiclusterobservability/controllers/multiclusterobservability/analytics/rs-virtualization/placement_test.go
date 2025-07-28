// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsvirtualization

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
)

func TestCreateUpdateVirtualizationPlacement_CreatesNew(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clusterv1beta1.AddToScheme(scheme))

	// Set up the namespace state
	originalNamespace := Namespace
	defer func() { Namespace = originalNamespace }() // Restore after test
	Namespace = "test-namespace"

	placementSpec := clusterv1beta1.PlacementSpec{
		NumberOfClusters: &[]int32{2}[0],
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	err := CreateUpdateVirtualizationPlacement(context.TODO(), client, clusterv1beta1.Placement{Spec: placementSpec})
	require.NoError(t, err)

	// Verify placement was created
	placement := &clusterv1beta1.Placement{}
	err = client.Get(context.TODO(), types.NamespacedName{
		Name:      PlacementName,
		Namespace: Namespace,
	}, placement)

	require.NoError(t, err)
	assert.Equal(t, PlacementName, placement.Name)
	assert.Equal(t, Namespace, placement.Namespace)
	assert.Equal(t, int32(2), *placement.Spec.NumberOfClusters)
}

func TestCreateUpdateVirtualizationPlacement_UpdatesExisting(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clusterv1beta1.AddToScheme(scheme))

	// Set up the namespace state
	originalNamespace := Namespace
	defer func() { Namespace = originalNamespace }() // Restore after test
	Namespace = "test-namespace"

	// Create initial placement
	existingPlacement := &clusterv1beta1.Placement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PlacementName,
			Namespace: Namespace,
		},
		Spec: clusterv1beta1.PlacementSpec{
			NumberOfClusters: &[]int32{1}[0],
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existingPlacement).
		Build()

	// Update with new spec
	newSpec := clusterv1beta1.PlacementSpec{
		NumberOfClusters: &[]int32{3}[0],
	}

	err := CreateUpdateVirtualizationPlacement(context.TODO(), client, clusterv1beta1.Placement{Spec: newSpec})
	require.NoError(t, err)

	// Verify placement was updated
	placement := &clusterv1beta1.Placement{}
	err = client.Get(context.TODO(), types.NamespacedName{
		Name:      PlacementName,
		Namespace: Namespace,
	}, placement)

	require.NoError(t, err)
	assert.Equal(t, int32(3), *placement.Spec.NumberOfClusters)
}

func TestCreateUpdateVirtualizationPlacement_UsesCorrectConstants(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clusterv1beta1.AddToScheme(scheme))

	// Set up the namespace state
	originalNamespace := Namespace
	defer func() { Namespace = originalNamespace }() // Restore after test
	Namespace = "test-namespace"

	placementSpec := clusterv1beta1.PlacementSpec{
		NumberOfClusters: &[]int32{1}[0],
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	err := CreateUpdateVirtualizationPlacement(context.TODO(), client, clusterv1beta1.Placement{Spec: placementSpec})
	require.NoError(t, err)

	// Verify placement was created with the correct name and namespace from constants
	placement := &clusterv1beta1.Placement{}
	err = client.Get(context.TODO(), types.NamespacedName{
		Name:      PlacementName,
		Namespace: Namespace,
	}, placement)

	require.NoError(t, err)
	assert.Equal(t, PlacementName, placement.Name, "Should use PlacementName constant")
	assert.Equal(t, Namespace, placement.Namespace, "Should use Namespace constant")
}
