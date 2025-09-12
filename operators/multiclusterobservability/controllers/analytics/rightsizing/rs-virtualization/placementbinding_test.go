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
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Note: CreateVirtualizationPlacementBinding is a thin wrapper around rsutility.CreateRSPlacementBinding
// that only adds package-specific constants. The core placement binding logic is extensively
// tested in rs-utility/placementbinding_test.go. This test focuses on verifying that the
// wrapper correctly uses the expected constants.

func TestCreateVirtualizationPlacementBinding_UsesCorrectConstants(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, policyv1.AddToScheme(scheme))

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	ctx := context.Background()

	err := CreateVirtualizationPlacementBinding(ctx, client)
	require.NoError(t, err)

	// Verify the PlacementBinding was created with the correct constants
	pb := &policyv1.PlacementBinding{}
	err = client.Get(ctx, types.NamespacedName{
		Name:      PlacementBindingName,
		Namespace: ComponentState.Namespace,
	}, pb)

	require.NoError(t, err)
	assert.Equal(t, PlacementBindingName, pb.Name, "Should use PlacementBindingName constant")
	assert.Equal(t, ComponentState.Namespace, pb.Namespace, "Should use Namespace variable")
	assert.Equal(t, PlacementName, pb.PlacementRef.Name, "Should use PlacementName constant")
	assert.Equal(t, PrometheusRulePolicyName, pb.Subjects[0].Name, "Should use PrometheusRulePolicyName constant")
}
