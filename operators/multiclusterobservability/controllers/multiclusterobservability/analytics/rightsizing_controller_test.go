// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	analyticsctrl "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability/analytics"

	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

func TestCreateRightSizingComponent_WhenEnabled(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = mcov1beta2.AddToScheme(scheme)
	_ = policyv1.AddToScheme(scheme)

	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{
			Name: "observability",
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			Capabilities: &mcov1beta2.CapabilitiesSpec{
				Platform: &mcov1beta2.PlatformCapabilitiesSpec{
					Analytics: &mcov1beta2.PlatformAnalyticsSpec{
						NamespaceRightSizingRecommendation: &mcov1beta2.NamespaceRightSizingRecommendationSpec{
							Enabled:          true,
							NamespaceBinding: "custom-ns",
						},
					},
				},
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mco).
		Build()

	result, err := analyticsctrl.CreateRightSizingComponent(context.TODO(), client, scheme, mco, nil)

	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestCreateRightSizingComponent_WhenDisabled(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = mcov1beta2.AddToScheme(scheme)
	_ = policyv1.AddToScheme(scheme)

	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{
			Name: "observability",
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			Capabilities: &mcov1beta2.CapabilitiesSpec{
				Platform: &mcov1beta2.PlatformCapabilitiesSpec{
					Analytics: &mcov1beta2.PlatformAnalyticsSpec{
						NamespaceRightSizingRecommendation: &mcov1beta2.NamespaceRightSizingRecommendationSpec{
							Enabled: false,
						},
					},
				},
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mco).
		Build()

	result, err := analyticsctrl.CreateRightSizingComponent(context.TODO(), client, scheme, mco, nil)

	require.NoError(t, err)
	assert.Nil(t, result)
}
