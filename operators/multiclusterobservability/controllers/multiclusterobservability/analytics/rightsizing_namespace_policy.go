// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics

import (
	"context"
	"encoding/json"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	configpolicyv1 "open-cluster-management.io/config-policy-controller/api/v1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Helps in creating or updating existing Policy for the PrometheusRule
func createOrUpdatePrometheusRulePolicy(
	ctx context.Context,
	c client.Client,
	prometheusRule monitoringv1.PrometheusRule) error {

	policy := &policyv1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rsPrometheusRulePolicyName,
			Namespace: rsNamespace,
		},
	}

	// Declare name, namespace in common log context and use it later everywhere
	logCtx := []any{"Namespace:", policy.Namespace, ", Name:", policy.Name}

	// Check if the policy exists
	errPolicy := c.Get(ctx, types.NamespacedName{
		Name:      policy.Name,
		Namespace: policy.Namespace,
	}, policy)
	if errPolicy != nil && !errors.IsNotFound(errPolicy) {
		log.Error(errPolicy, "RS - Error retrieving PrometheusRulePolicy", logCtx...)
		return errPolicy
	}

	// Convert PrometheusRule to unstructured JSON
	objMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&prometheusRule)
	if err != nil {
		log.Error(err, "RS - Error converting PrometheusRule to unstructured")
		return err
	}

	// Marshal the map back to JSON
	promRuleJSON, err := json.Marshal(objMap)
	if err != nil {
		log.Error(err, "RS - Error marshaling PrometheusRule")
		return err
	}

	// Define the ConfigurationPolicy
	configPolicy := configpolicyv1.ConfigurationPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy.open-cluster-management.io/v1",
			Kind:       "ConfigurationPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: rsPrometheusRulePolicyConfigName,
		},
		Spec: &configpolicyv1.ConfigurationPolicySpec{
			RemediationAction:   configpolicyv1.Inform,
			Severity:            "low",
			PruneObjectBehavior: configpolicyv1.PruneObjectBehavior("DeleteAll"),
			NamespaceSelector: configpolicyv1.Target{
				Include: []configpolicyv1.NonEmptyString{
					configpolicyv1.NonEmptyString(rsMonitoringNamespace),
				},
			},
			ObjectTemplates: []*configpolicyv1.ObjectTemplate{
				{
					ComplianceType: configpolicyv1.MustOnlyHave,
					ObjectDefinition: runtime.RawExtension{
						Raw: promRuleJSON,
					},
				},
			},
		},
	}

	// Marshal the ConfigurationPolicy to JSON
	configPolicyJSON, err := json.Marshal(configPolicy)
	if err != nil {
		log.Error(err, "RS - Error marshaling ConfigurationPolicy")
		return err
	}

	// Set Policy spec
	policy.Spec = policyv1.PolicySpec{
		RemediationAction: policyv1.Enforce,
		Disabled:          false,
		PolicyTemplates: []*policyv1.PolicyTemplate{
			{
				ObjectDefinition: runtime.RawExtension{
					Raw: configPolicyJSON,
				},
			},
		},
	}

	if errors.IsNotFound(errPolicy) {
		if err = c.Create(ctx, policy); err != nil {
			log.Error(err, "RS - Failed to create PrometheusRulePolicy", logCtx...)
			return err
		}
		log.Info("RS - Created PrometheusRulePolicy successfully", logCtx...)
	} else {
		if err := c.Update(ctx, policy); err != nil {
			log.Error(err, "RS - Failed to update PrometheusRulePolicy", logCtx...)
			return err
		}
		log.Info("RS - Updated PrometheusRulePolicy successfully", logCtx...)
	}

	return nil
}
