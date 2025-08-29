// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsutility

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCreatePlacementBinding_CreatesWhenNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = policyv1.AddToScheme(scheme)

	rsPlacementBindingName := "test-placement-binding"
	rsNamespace := "test-namespace"
	rsPlacementName := "test-placement"
	rsPrometheusRulePolicyName := "test-policy"

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	ctx := context.Background()

	err := CreateRSPlacementBinding(context.TODO(), client, rsPlacementBindingName, rsNamespace, rsPlacementName, rsPrometheusRulePolicyName)
	assert.NoError(t, err)

	// Validate that it was created
	pb := &policyv1.PlacementBinding{}
	err = client.Get(ctx, types.NamespacedName{
		Name:      rsPlacementBindingName,
		Namespace: rsNamespace,
	}, pb)
	assert.NoError(t, err)
	assert.Equal(t, rsPlacementName, pb.PlacementRef.Name)
	assert.Equal(t, "Placement", pb.PlacementRef.Kind)
	assert.Equal(t, "cluster.open-cluster-management.io", pb.PlacementRef.APIGroup)
	assert.Equal(t, rsPrometheusRulePolicyName, pb.Subjects[0].Name)
}

func TestCreatePlacementBinding_SkipsIfAlreadyExists(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = policyv1.AddToScheme(scheme)
	ctx := context.Background()

	rsPlacementBindingName := "test-placement-binding"
	rsNamespace := "test-namespace"
	rsPlacementName := "test-placement"
	rsPrometheusRulePolicyName := "test-policy"

	existing := &policyv1.PlacementBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rsPlacementBindingName,
			Namespace: rsNamespace,
		},
		PlacementRef: policyv1.PlacementSubject{
			Name:     rsPlacementName,
			APIGroup: "cluster.open-cluster-management.io",
			Kind:     "Placement",
		},
		Subjects: []policyv1.Subject{
			{
				Name:     rsPrometheusRulePolicyName,
				APIGroup: "policy.open-cluster-management.io",
				Kind:     "Policy",
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

	err := CreateRSPlacementBinding(context.TODO(), client, rsPlacementBindingName, rsNamespace, rsPlacementName, rsPrometheusRulePolicyName)
	assert.NoError(t, err)

	// Ensure it hasn't changed
	pb := &policyv1.PlacementBinding{}
	err = client.Get(ctx, types.NamespacedName{
		Name:      rsPlacementBindingName,
		Namespace: rsNamespace,
	}, pb)
	assert.NoError(t, err)
	assert.Equal(t, rsPlacementName, pb.PlacementRef.Name)
}
