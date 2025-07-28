// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsvirtualization

import (
	"context"
	"testing"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

func TestCreateOrUpdateVirtualizationPrometheusRulePolicy_CreatesNewPolicy(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, policyv1.AddToScheme(scheme))
	require.NoError(t, monitoringv1.AddToScheme(scheme))

	// Set up the namespace state
	originalNamespace := Namespace
	defer func() { Namespace = originalNamespace }() // Restore after test
	Namespace = "test-namespace"

	rule := monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PrometheusRuleName,
			Namespace: "openshift-monitoring",
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	err := CreateOrUpdateVirtualizationPrometheusRulePolicy(context.TODO(), client, rule)
	require.NoError(t, err)

	// Verify policy was created
	created := &policyv1.Policy{}
	err = client.Get(context.TODO(), types.NamespacedName{
		Name:      PrometheusRulePolicyName,
		Namespace: Namespace,
	}, created)

	require.NoError(t, err)
	assert.Equal(t, PrometheusRulePolicyName, created.Name)
	assert.Equal(t, Namespace, created.Namespace)
	assert.Len(t, created.Spec.PolicyTemplates, 1)
}

func TestCreateOrUpdateVirtualizationPrometheusRulePolicy_UpdatesExistingPolicy(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, policyv1.AddToScheme(scheme))
	require.NoError(t, monitoringv1.AddToScheme(scheme))

	// Set up the namespace state
	originalNamespace := Namespace
	defer func() { Namespace = originalNamespace }() // Restore after test
	Namespace = "test-namespace"

	// Create existing policy
	existingPolicy := &policyv1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PrometheusRulePolicyName,
			Namespace: Namespace,
		},
		Spec: policyv1.PolicySpec{
			PolicyTemplates: []*policyv1.PolicyTemplate{
				{
					ObjectDefinition: runtime.RawExtension{
						Raw: []byte(`{"kind": "PrometheusRule"}`),
					},
				},
			},
		},
	}

	rule := monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PrometheusRuleName,
			Namespace: "openshift-monitoring",
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingPolicy).Build()

	err := CreateOrUpdateVirtualizationPrometheusRulePolicy(context.TODO(), client, rule)
	require.NoError(t, err)

	// Verify policy was updated
	updated := &policyv1.Policy{}
	err = client.Get(context.TODO(), types.NamespacedName{
		Name:      PrometheusRulePolicyName,
		Namespace: Namespace,
	}, updated)

	require.NoError(t, err)
	assert.Equal(t, PrometheusRulePolicyName, updated.Name)
	assert.Len(t, updated.Spec.PolicyTemplates, 1)
}
