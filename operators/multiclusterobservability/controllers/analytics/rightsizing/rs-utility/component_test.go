// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsutility

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func setupRSUtilityScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, clusterv1beta1.AddToScheme(scheme))
	require.NoError(t, policyv1.AddToScheme(scheme))
	return scheme
}

func rsLabeledMeta(name, namespace string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
		Labels:    map[string]string{RSManagedByLabel: RSManagedByValue},
	}
}

// TestCleanupLegacyPolicyResourcesByLabel_PreservesConfigMap verifies that the migration
// path deletes Policy/PlacementBinding/Placement resources but keeps ConfigMaps intact.
// ConfigMaps are preserved because MCOA reuses them for per-cluster configuration.
func TestCleanupLegacyPolicyResourcesByLabel_PreservesConfigMap(t *testing.T) {
	scheme := setupRSUtilityScheme(t)

	ns := DefaultNamespace
	policy := &policyv1.Policy{ObjectMeta: rsLabeledMeta("rs-prom-rules-policy", ns)}
	pb := &policyv1.PlacementBinding{ObjectMeta: rsLabeledMeta("rs-policyset-binding", ns)}
	placement := &clusterv1beta1.Placement{ObjectMeta: rsLabeledMeta("rs-placement", "")}
	cm := &corev1.ConfigMap{ObjectMeta: rsLabeledMeta("rs-namespace-config", ns)}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(policy, pb, placement, cm).Build()

	require.NoError(t, CleanupLegacyPolicyResourcesByLabel(context.TODO(), c))

	// Policy, PlacementBinding, Placement must be deleted.
	err := c.Get(context.TODO(), types.NamespacedName{Name: policy.Name, Namespace: ns}, &policyv1.Policy{})
	require.True(t, apierrors.IsNotFound(err), "Policy must be deleted by legacy cleanup")

	err = c.Get(context.TODO(), types.NamespacedName{Name: pb.Name, Namespace: ns}, &policyv1.PlacementBinding{})
	require.True(t, apierrors.IsNotFound(err), "PlacementBinding must be deleted by legacy cleanup")

	err = c.Get(context.TODO(), types.NamespacedName{Name: placement.Name}, &clusterv1beta1.Placement{})
	require.True(t, apierrors.IsNotFound(err), "Placement must be deleted by legacy cleanup")

	// ConfigMap must be preserved — MCOA reuses it.
	err = c.Get(context.TODO(), types.NamespacedName{Name: cm.Name, Namespace: ns}, &corev1.ConfigMap{})
	require.NoError(t, err, "ConfigMap must NOT be deleted by legacy cleanup (MCOA reuses it)")
}

// TestCleanupRSResourcesByLabel_DeletesConfigMap verifies that full MCO deletion cleanup
// removes ConfigMaps in addition to Policy/PlacementBinding/Placement resources.
func TestCleanupRSResourcesByLabel_DeletesConfigMap(t *testing.T) {
	scheme := setupRSUtilityScheme(t)

	ns := DefaultNamespace
	policy := &policyv1.Policy{ObjectMeta: rsLabeledMeta("rs-prom-rules-policy", ns)}
	placement := &clusterv1beta1.Placement{ObjectMeta: rsLabeledMeta("rs-placement", "")}
	cm := &corev1.ConfigMap{ObjectMeta: rsLabeledMeta("rs-namespace-config", ns)}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(policy, placement, cm).Build()

	require.NoError(t, CleanupRSResourcesByLabel(context.TODO(), c))

	// All resources including ConfigMap must be deleted.
	err := c.Get(context.TODO(), types.NamespacedName{Name: policy.Name, Namespace: ns}, &policyv1.Policy{})
	require.True(t, apierrors.IsNotFound(err), "Policy must be deleted by full RS cleanup")

	err = c.Get(context.TODO(), types.NamespacedName{Name: placement.Name}, &clusterv1beta1.Placement{})
	require.True(t, apierrors.IsNotFound(err), "Placement must be deleted by full RS cleanup")

	err = c.Get(context.TODO(), types.NamespacedName{Name: cm.Name, Namespace: ns}, &corev1.ConfigMap{})
	require.True(t, apierrors.IsNotFound(err), "ConfigMap must be deleted by full RS cleanup (MCO CR gone)")
}
