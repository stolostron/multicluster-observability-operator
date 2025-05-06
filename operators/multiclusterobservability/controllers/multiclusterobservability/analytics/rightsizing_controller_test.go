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

	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

func newTestMCO(binding string, enabled bool) *mcov1beta2.MultiClusterObservability {
	return &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{Name: "observability"},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			Capabilities: &mcov1beta2.CapabilitiesSpec{
				Platform: &mcov1beta2.PlatformCapabilitiesSpec{
					Analytics: mcov1beta2.PlatformAnalyticsSpec{
						NamespaceRightSizingRecommendation: mcov1beta2.NamespaceRightSizingRecommendationSpec{
							Enabled:          enabled,
							NamespaceBinding: binding,
						},
					},
				},
			},
		},
	}
}

func TestCreateRightSizingComponent_WhenEnabled_WithNamespaceChange(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = mcov1beta2.AddToScheme(scheme)
	_ = policyv1.AddToScheme(scheme)
	_ = clusterv1beta1.AddToScheme(scheme)

	mco := newTestMCO("custom-ns", true)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rs-namespace-config",
			Namespace: "custom-ns",
		},
		Data: map[string]string{
			"config.yaml": `
prometheusRuleConfig:
  namespaceFilterCriteria:
    inclusionCriteria: ["ns1"]
    exclusionCriteria: []
  labelFilterCriteria: []
  recommendationPercentage: 110
placementConfiguration:
  predicates: []
`,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mco, configMap).
		Build()

	result, err := analyticsctrl.CreateRightSizingComponent(context.TODO(), k8sClient, scheme, mco, nil)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestCreateRightSizingComponent_WhenEnabled_NoChange(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = mcov1beta2.AddToScheme(scheme)
	_ = policyv1.AddToScheme(scheme)
	_ = clusterv1beta1.AddToScheme(scheme)

	mco := newTestMCO("open-cluster-management-global-set", true)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mco).
		Build()

	result, err := analyticsctrl.CreateRightSizingComponent(context.TODO(), k8sClient, scheme, mco, nil)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestCreateRightSizingComponent_WhenDisabled(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = mcov1beta2.AddToScheme(scheme)
	_ = policyv1.AddToScheme(scheme)
	_ = clusterv1beta1.AddToScheme(scheme)

	mco := newTestMCO("", false)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mco).
		Build()

	result, err := analyticsctrl.CreateRightSizingComponent(context.TODO(), k8sClient, scheme, mco, nil)
	require.NoError(t, err)
	assert.Nil(t, result)
}
