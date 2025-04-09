// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cloudflare/cfssl/log"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	configpolicyv1 "open-cluster-management.io/config-policy-controller/api/v1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func modifyComplianceTypeIfPolicyExists(ctx context.Context, c client.Client) error {
	policy := &policyv1.Policy{}
	err := c.Get(ctx, types.NamespacedName{
		Name:      rsPrometheusRulePolicyName,
		Namespace: rsNamespace,
	}, policy)

	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Policy does not exist. Skipping update.")
			return nil
		}
		log.Error(err, "Error retrieving the policy")
		return err
	}

	// Unmarshal the inner ConfigurationPolicy
	for _, pt := range policy.Spec.PolicyTemplates {
		var configPolicy configpolicyv1.ConfigurationPolicy
		err := json.Unmarshal(pt.ObjectDefinition.Raw, &configPolicy)
		if err != nil {
			log.Error(err, "Failed to unmarshal ConfigurationPolicy from PolicyTemplate")
			return err
		}

		// Change ComplianceType if it's "MustOnlyHave"
		changed := false
		for _, objTemplate := range configPolicy.Spec.ObjectTemplates {
			if objTemplate.ComplianceType == configpolicyv1.MustOnlyHave {
				objTemplate.ComplianceType = configpolicyv1.MustNotHave
				changed = true
			}
		}

		if changed {
			// Marshal the modified ConfigurationPolicy back into JSON
			modifiedRaw, err := json.Marshal(configPolicy)
			if err != nil {
				log.Error(err, "Failed to marshal modified ConfigurationPolicy")
				return err
			}

			pt.ObjectDefinition = runtime.RawExtension{Raw: modifiedRaw}
		}
	}

	// Update the modified policy
	err = c.Update(ctx, policy)
	if err != nil {
		log.Error(err, "Failed to update the modified policy")
		return err
	}

	log.Info("Successfully updated ComplianceType in policy")

	// Wait for 5 seconds
	time.Sleep(5 * time.Second)

	return nil
}

// Helps in creating or updating existing Policy for the PrometheusRule
func createOrUpdatePrometheusRulePolicy(
	ctx context.Context,
	c client.Client,
	prometheusRule monitoringv1.PrometheusRule) error {

	policy := &policyv1.Policy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy.open-cluster-management.io/v1",
			Kind:       "Policy",
		},
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

	// Marshal the PrometheusRule object into JSON
	promRuleJSON, err := AddAPIVersionAndKind(prometheusRule, "monitoring.coreos.com/v1", "PrometheusRule")
	if err != nil {
		log.Error(err, "RS - Error marshaling ConfigurationPolicy")
	}

	// Define the ConfigurationPolicy object
	configPolicy := configpolicyv1.ConfigurationPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy.open-cluster-management.io/v1",
			Kind:       "ConfigurationPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: rsPrometheusRulePolicyConfigName,
		},
		Spec: &configpolicyv1.ConfigurationPolicySpec{
			RemediationAction: configpolicyv1.Inform,
			Severity:          "low",
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

	// Marshal the ConfigurationPolicy object into JSON
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

	if errors.IsNotFound(errPolicy) {

		log.Info("RS - PrometheusRulePolicy not found, creating a new one",
			" Namespace:", rsNamespace,
			" Name:", rsPrometheusRulePolicyName,
		)
		if client.IgnoreNotFound(errPolicy) != nil {
			log.Error(errPolicy, "RS - Unable to fetch PrometheusRulePolicy")
			return errPolicy
		}

		if err = c.Create(ctx, policy); err != nil {
			log.Error(err, "Failed to create PrometheusRulePolicy")
			return err
		}
		log.Info("RS - Created PrometheusRulePolicy completed", " Name:", rsPrometheusRulePolicyName)
	} else {
		log.Info("RS - PrometheusRulePolicy already exists, updating data",
			" Name:", rsPrometheusRulePolicyName,
			" Namespace:", rsNamespace,
		)

		if err = c.Update(ctx, policy); err != nil {
			log.Error(err, "Failed to update PrometheusRulePolicy")
			return err
		}
		log.Info("RS - PrometheusRulePolicy updated successfully", "Policy", rsPrometheusRulePolicyName)

	}

	return nil
}
