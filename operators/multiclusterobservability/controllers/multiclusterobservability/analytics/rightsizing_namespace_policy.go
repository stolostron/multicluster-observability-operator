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

// Utility to create pointer for bools
func pointerTo[T any](v T) *T {
	return &v
}

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
	log.Info("RS - Policy object created")

	errPolicy := c.Get(ctx, types.NamespacedName{
		Name:      policy.Name,
		Namespace: policy.Namespace,
	}, policy)

	if errPolicy != nil && !errors.IsNotFound(errPolicy) {
		log.Error(errPolicy, "RS - Error retrieving Policy")
		return errPolicy
	}

	// Add OwnerReference to PrometheusRule pointing to the Policy
	if policy.UID != "" {
		prometheusRule.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: policy.APIVersion,
				Kind:       policy.Kind,
				Name:       policy.Name,
				UID:        policy.UID,
				Controller: pointerTo(true),
			},
		}
	}

	// Convert PrometheusRule to unstructured JSON
	objMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&prometheusRule)
	if err != nil {
		log.Error(err, "error converting runtime.Object to unstructured")
		return err
	}
	promRuleJSON, err := json.Marshal(objMap)
	if err != nil {
		log.Error(err, "RS - Error marshaling PrometheusRule to JSON")
		return err
	}

	// Construct the ConfigurationPolicy
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
			PruneObjectBehavior: configpolicyv1.PruneObjectBehavior("None"), // ensures existing rules are not removed
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

	// Marshal ConfigurationPolicy into JSON and wrap it in a Policy
	configPolicyJSON, err := json.Marshal(configPolicy)
	if err != nil {
		log.Error(err, "RS - Error marshaling ConfigurationPolicy")
		return err
	}

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

	// Create or update the Policy
	if errors.IsNotFound(errPolicy) {
		log.Info("RS - PrometheusRulePolicy not found, creating new one",
			"namespace", rsNamespace, "name", rsPrometheusRulePolicyName)
		if err := c.Create(ctx, policy); err != nil {
			log.Error(err, "Failed to create PrometheusRulePolicy")
			return err
		}
		log.Info("RS - Created PrometheusRulePolicy")
	} else {
		log.Info("RS - PrometheusRulePolicy already exists, updating")
		if err := c.Update(ctx, policy); err != nil {
			log.Error(err, "Failed to update PrometheusRulePolicy")
			return err
		}
		log.Info("RS - PrometheusRulePolicy updated successfully")
	}

	return nil
}
