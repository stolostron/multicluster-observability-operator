// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	configpolicyv1 "open-cluster-management.io/config-policy-controller/api/v1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

// initScheme initializes and registers all required types into scheme
func initScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = policyv1.AddToScheme(s)
	_ = configpolicyv1.AddToScheme(s)
	_ = monitoringv1.AddToScheme(s)
	return s
}

func TestCreateOrUpdatePrometheusRulePolicy_CreatesNewPolicy(t *testing.T) {
	scheme := initScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	rule := monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rule",
			Namespace: rsMonitoringNamespace,
		},
	}

	err := createOrUpdatePrometheusRulePolicy(context.TODO(), client, rule)
	assert.NoError(t, err)

	created := &policyv1.Policy{}
	err = client.Get(context.TODO(), types.NamespacedName{
		Name:      rsPrometheusRulePolicyName,
		Namespace: rsNamespace,
	}, created)
	assert.NoError(t, err)

	// Manually set Kind for assertion because fake client does not auto-populate it
	created.TypeMeta = metav1.TypeMeta{
		Kind:       "Policy",
		APIVersion: "policy.open-cluster-management.io/v1",
	}

	assert.Equal(t, "Policy", created.Kind)
	assert.Equal(t, rsPrometheusRulePolicyName, created.Name)
	assert.Equal(t, policyv1.Enforce, created.Spec.RemediationAction)
}
