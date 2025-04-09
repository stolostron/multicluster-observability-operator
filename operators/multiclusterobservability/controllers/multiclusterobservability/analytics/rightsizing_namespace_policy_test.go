// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics

import (
	"context"
	"encoding/json"
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

func TestModifyComplianceTypeIfPolicyExists_ChangesMustOnlyHave(t *testing.T) {
	scheme := initScheme()

	configPolicy := configpolicyv1.ConfigurationPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cp"},
		Spec: &configpolicyv1.ConfigurationPolicySpec{
			ObjectTemplates: []*configpolicyv1.ObjectTemplate{
				{ComplianceType: configpolicyv1.MustOnlyHave},
			},
		},
	}
	rawCP, _ := json.Marshal(configPolicy)

	existingPolicy := &policyv1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rsPrometheusRulePolicyName,
			Namespace: rsNamespace,
		},
		Spec: policyv1.PolicySpec{
			PolicyTemplates: []*policyv1.PolicyTemplate{
				{ObjectDefinition: runtime.RawExtension{Raw: rawCP}},
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingPolicy).Build()
	err := modifyComplianceTypeIfPolicyExists(context.TODO(), client)
	assert.NoError(t, err)

	updated := &policyv1.Policy{}
	err = client.Get(context.TODO(), types.NamespacedName{
		Name:      rsPrometheusRulePolicyName,
		Namespace: rsNamespace,
	}, updated)
	assert.NoError(t, err)

	var updatedCP configpolicyv1.ConfigurationPolicy
	err = json.Unmarshal(updated.Spec.PolicyTemplates[0].ObjectDefinition.Raw, &updatedCP)
	assert.NoError(t, err)
	assert.Equal(t, configpolicyv1.MustNotHave, updatedCP.Spec.ObjectTemplates[0].ComplianceType)
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
	assert.Equal(t, "Policy", created.Kind)
	assert.Equal(t, rsPrometheusRulePolicyName, created.Name)
	assert.Equal(t, policyv1.Enforce, created.Spec.RemediationAction)
}
