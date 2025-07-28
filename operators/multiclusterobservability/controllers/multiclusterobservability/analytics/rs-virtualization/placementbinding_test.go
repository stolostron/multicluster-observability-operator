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
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

func TestCreateVirtualizationPlacementBinding_CreatesWhenNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, policyv1.AddToScheme(scheme))

	// Set up the namespace state
	originalNamespace := Namespace
	defer func() { Namespace = originalNamespace }() // Restore after test
	Namespace = "test-namespace"

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	ctx := context.Background()

	err := CreateVirtualizationPlacementBinding(ctx, client)
	require.NoError(t, err)

	// Verify the PlacementBinding was created
	pb := &policyv1.PlacementBinding{}
	err = client.Get(ctx, types.NamespacedName{
		Name:      PlacementBindingName,
		Namespace: Namespace,
	}, pb)

	require.NoError(t, err)
	assert.Equal(t, PlacementBindingName, pb.Name)
	assert.Equal(t, Namespace, pb.Namespace)
	assert.Equal(t, PlacementName, pb.PlacementRef.Name)
	assert.Equal(t, PrometheusRulePolicyName, pb.Subjects[0].Name)
}

func TestCreateVirtualizationPlacementBinding_SkipsIfAlreadyExists(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, policyv1.AddToScheme(scheme))

	// Set up the namespace state
	originalNamespace := Namespace
	defer func() { Namespace = originalNamespace }() // Restore after test
	Namespace = "test-namespace"
	ctx := context.Background()

	// Create an existing PlacementBinding
	existing := &policyv1.PlacementBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PlacementBindingName,
			Namespace: Namespace,
		},
		PlacementRef: policyv1.PlacementSubject{
			Name:     PlacementName,
			Kind:     "Placement",
			APIGroup: "cluster.open-cluster-management.io",
		},
		Subjects: []policyv1.Subject{
			{
				Name:     PrometheusRulePolicyName,
				Kind:     "Policy",
				APIGroup: "policy.open-cluster-management.io",
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

	err := CreateVirtualizationPlacementBinding(ctx, client)
	require.NoError(t, err)

	// Verify no duplicate was created (should still be just the original)
	pb := &policyv1.PlacementBinding{}
	err = client.Get(ctx, types.NamespacedName{
		Name:      PlacementBindingName,
		Namespace: Namespace,
	}, pb)

	require.NoError(t, err)
	assert.Equal(t, PlacementBindingName, pb.Name)
}
