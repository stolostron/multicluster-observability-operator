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
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Note: CreateOrUpdateVirtualizationPrometheusRulePolicy is a thin wrapper around rsutility.CreateOrUpdateRSPrometheusRulePolicy
// that only adds package-specific constants. The core Prometheus rule policy logic is extensively
// tested in rs-utility/policy_test.go. This test focuses on verifying that the
// wrapper correctly uses the expected constants.

func TestCreateOrUpdateVirtualizationPrometheusRulePolicy_UsesCorrectConstants(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, policyv1.AddToScheme(scheme))
	require.NoError(t, monitoringv1.AddToScheme(scheme))

	rule := monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PrometheusRuleName,
			Namespace: "openshift-monitoring",
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	err := CreateOrUpdateVirtualizationPrometheusRulePolicy(context.TODO(), client, rule)
	require.NoError(t, err)

	// Verify policy was created with the correct constants
	created := &policyv1.Policy{}
	err = client.Get(context.TODO(), types.NamespacedName{
		Name:      PrometheusRulePolicyName,
		Namespace: ComponentState.Namespace,
	}, created)

	require.NoError(t, err)
	assert.Equal(t, PrometheusRulePolicyName, created.Name, "Should use PrometheusRulePolicyName constant")
	assert.Equal(t, ComponentState.Namespace, created.Namespace, "Should use Namespace variable")
	assert.Len(t, created.Spec.PolicyTemplates, 1, "Should create policy with template")
}
