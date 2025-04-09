// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

const (
	mockRsPlacementBindingName     = "rs-policyset-binding"
	mockRsPlacementName            = "rs-placement"
	mockRsPrometheusRulePolicyName = "rs-prom-rules-policy"
	mockRsConfigMapName            = "rs-namespace-config"
	mockRsNamespace                = "open-cluster-management-observability"
)

func TestIsRightSizingNamespaceEnabled(t *testing.T) {
	enabled := isRightSizingNamespaceEnabled(&mcov1beta2.MultiClusterObservability{
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			Capabilities: &mcov1beta2.CapabilitiesSpec{
				Platform: &mcov1beta2.PlatformCapabilitiesSpec{
					Analytics: &mcov1beta2.PlatformAnalyticsSpec{
						NamespaceRightSizingRecommendation: &mcov1beta2.NamespaceRightSizingRecommendationSpec{
							Enabled: true,
						},
					},
				},
			},
		},
	})
	assert.True(t, enabled)

	disabled := isRightSizingNamespaceEnabled(&mcov1beta2.MultiClusterObservability{})
	assert.False(t, disabled)
}

func TestCleanupRSNamespaceResources(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = policyv1.AddToScheme(scheme)
	_ = clusterv1beta1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&policyv1.PlacementBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mockRsPlacementBindingName,
				Namespace: mockRsNamespace,
			},
		},
		&clusterv1beta1.Placement{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mockRsPlacementName,
				Namespace: mockRsNamespace,
			},
		},
		&policyv1.Policy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mockRsPrometheusRulePolicyName,
				Namespace: mockRsNamespace,
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mockRsConfigMapName,
				Namespace: mockRsNamespace,
			},
		},
	).Build()

	cleanupRSNamespaceResources(context.TODO(), c, mockRsNamespace)

	// Verify that resources were deleted
	for _, obj := range []client.Object{
		&policyv1.PlacementBinding{ObjectMeta: metav1.ObjectMeta{Name: mockRsPlacementBindingName, Namespace: mockRsNamespace}},
		&clusterv1beta1.Placement{ObjectMeta: metav1.ObjectMeta{Name: mockRsPlacementName, Namespace: mockRsNamespace}},
		&policyv1.Policy{ObjectMeta: metav1.ObjectMeta{Name: mockRsPrometheusRulePolicyName, Namespace: mockRsNamespace}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: mockRsConfigMapName, Namespace: mockRsNamespace}},
	} {
		err := c.Get(context.TODO(), client.ObjectKeyFromObject(obj), obj)
		assert.Error(t, err) // Expect NotFound errors
	}
}
