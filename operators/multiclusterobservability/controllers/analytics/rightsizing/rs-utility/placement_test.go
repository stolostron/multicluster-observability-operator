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
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetDefaultRSPlacement(t *testing.T) {
	placement := GetDefaultRSPlacement()

	assert.Empty(t, placement.Spec.Predicates)
	assert.Len(t, placement.Spec.Tolerations, 2)

	// Check tolerations
	tolerations := placement.Spec.Tolerations
	assert.Equal(t, "cluster.open-cluster-management.io/unreachable", tolerations[0].Key)
	assert.Equal(t, clusterv1beta1.TolerationOpExists, tolerations[0].Operator)
	assert.Equal(t, "cluster.open-cluster-management.io/unavailable", tolerations[1].Key)
	assert.Equal(t, clusterv1beta1.TolerationOpExists, tolerations[1].Operator)
}

func TestCreateUpdatePlacement_CreatesNew(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clusterv1beta1.AddToScheme(scheme)

	rsPlacementName := "test-placement"
	rsNamespace := "test-namespace"

	placementSpec := clusterv1beta1.PlacementSpec{
		Tolerations: []clusterv1beta1.Toleration{
			{
				Key:      "unreachable",
				Operator: clusterv1beta1.TolerationOpExists,
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	err := CreateUpdateRSPlacement(context.TODO(), client, rsPlacementName, rsNamespace, clusterv1beta1.Placement{Spec: placementSpec})
	assert.NoError(t, err)

	created := &clusterv1beta1.Placement{}
	err = client.Get(context.TODO(), types.NamespacedName{
		Name:      rsPlacementName,
		Namespace: rsNamespace,
	}, created)
	assert.NoError(t, err)
	assert.Equal(t, placementSpec.Tolerations[0].Key, created.Spec.Tolerations[0].Key)
}

func TestCreateUpdatePlacement_UpdatesExisting(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clusterv1beta1.AddToScheme(scheme)

	rsPlacementName := "test-placement"
	rsNamespace := "test-namespace"

	existing := &clusterv1beta1.Placement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rsPlacementName,
			Namespace: rsNamespace,
		},
		Spec: clusterv1beta1.PlacementSpec{},
	}

	newSpec := clusterv1beta1.PlacementSpec{
		Tolerations: []clusterv1beta1.Toleration{
			{
				Key:      "maintenance",
				Operator: clusterv1beta1.TolerationOpExists,
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

	err := CreateUpdateRSPlacement(context.TODO(), client, rsPlacementName, rsNamespace, clusterv1beta1.Placement{Spec: newSpec})
	assert.NoError(t, err)

	updated := &clusterv1beta1.Placement{}
	err = client.Get(context.TODO(), types.NamespacedName{
		Name:      rsPlacementName,
		Namespace: rsNamespace,
	}, updated)
	assert.NoError(t, err)
	assert.Equal(t, "maintenance", updated.Spec.Tolerations[0].Key)
}
