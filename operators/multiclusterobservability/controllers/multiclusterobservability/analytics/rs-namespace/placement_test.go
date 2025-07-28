// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsnamespace

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
)

// Note: CreateUpdatePlacement is a thin wrapper around rsutility.CreateUpdateRSPlacement
// that only adds package-specific constants. The core placement logic is extensively
// tested in rs-utility/placement_test.go. This test focuses on verifying that the
// wrapper correctly uses the expected constants.

func TestCreateUpdatePlacement_UsesCorrectConstants(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clusterv1beta1.AddToScheme(scheme))

	placementSpec := clusterv1beta1.PlacementSpec{
		NumberOfClusters: &[]int32{1}[0],
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	err := CreateUpdatePlacement(context.TODO(), client, clusterv1beta1.Placement{Spec: placementSpec})
	require.NoError(t, err)

	// Verify placement was created with the correct name and namespace from constants
	placement := &clusterv1beta1.Placement{}
	err = client.Get(context.TODO(), types.NamespacedName{
		Name:      PlacementName,
		Namespace: ComponentState.Namespace,
	}, placement)

	require.NoError(t, err)
	assert.Equal(t, PlacementName, placement.Name, "Should use PlacementName constant")
	assert.Equal(t, ComponentState.Namespace, placement.Namespace, "Should use Namespace constant")
}

func TestCreateUpdatePlacement_UsesCorrectConstants(t *testing.T) {
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

	err := CreateUpdatePlacement(context.TODO(), client, clusterv1beta1.Placement{Spec: placementSpec})
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
