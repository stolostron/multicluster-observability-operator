package analytics

import (
	"context"
	"encoding/json"
	"fmt"
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

func modifyComplianceTypeIfPolicyExists(c client.Client) error {
	policy := &policyv1.Policy{}
	err := c.Get(context.TODO(), types.NamespacedName{
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
			if objTemplate.ComplianceType == "MustOnlyHave" {
				objTemplate.ComplianceType = "MustNotHave"
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
	err = c.Update(context.TODO(), policy)
	if err != nil {
		log.Error(err, "Failed to update the modified policy")
		return err
	}

	log.Info("Successfully updated ComplianceType in policy")

	// Wait for 1 seconds
	time.Sleep(1 * time.Second)

	return nil
}

func createOrUpdatePrometheusRulePolicy(c client.Client, prometheusRule monitoringv1.PrometheusRule) error {

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

	errPolicy := c.Get(context.TODO(), types.NamespacedName{
		Name:      policy.Name,
		Namespace: policy.Namespace,
	}, policy)

	if errPolicy != nil && !errors.IsNotFound(errPolicy) {
		fmt.Println("Error retriving Policy:", errPolicy)
		return errPolicy
	}

	// Marshal the PrometheusRule object into JSON
	promRuleJSON, err := addAPIVersionAndKind(prometheusRule, "monitoring.coreos.com/v1", "PrometheusRule")
	if err != nil {
		fmt.Println("Error marshaling ConfigurationPolicy:", err)
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
			RemediationAction: "inform",
			Severity:          "low",
			NamespaceSelector: configpolicyv1.Target{
				Include: []configpolicyv1.NonEmptyString{
					configpolicyv1.NonEmptyString(rsMonitoringNamespace),
				},
			},
			ObjectTemplates: []*configpolicyv1.ObjectTemplate{
				{
					ComplianceType: "MustOnlyHave",
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
		fmt.Println("Error marshaling ConfigurationPolicy:", err)
		return err
	}

	policy.Spec = policyv1.PolicySpec{
		RemediationAction: "enforce",
		Disabled:          false,
		PolicyTemplates: []*policyv1.PolicyTemplate{
			{
				ObjectDefinition: runtime.RawExtension{
					Raw: configPolicyJSON,
				},
				// ObjectDefinition: runtime.RawExtension{
				// 	Object: &prometheusRule, // Use marshaled YAML of PrometheusRule
				// },
			},
		},
	}

	if errors.IsNotFound(errPolicy) {

		log.Info("RS - PrometheusRulePolicy not found, creating a new one",
			"Namespace", rsNamespace,
			"Name", rsPrometheusRulePolicyName,
		)
		if client.IgnoreNotFound(errPolicy) != nil {
			log.Error(errPolicy, "RS - Unable to fetch PrometheusRulePolicy")
			return errPolicy
		}

		if err = c.Create(context.TODO(), policy); err != nil {
			log.Error(err, "Failed to create PrometheusRulePolicy")
			return err
		}
		log.Info("RS - Created PrometheusRulePolicy completed", "Policy", rsPrometheusRulePolicyName)
	} else {
		log.Info("RS - PrometheusRulePolicy already exists, updating data",
			"Namespace", rsNamespace,
			"Name", rsPrometheusRulePolicyName,
		)

		if err = c.Update(context.TODO(), policy); err != nil {
			log.Error(err, "Failed to update PrometheusRulePolicy")
			return err
		}
		log.Info("RS - PrometheusRulePolicy updated successfully", "Policy", rsPrometheusRulePolicyName)

	}

	return nil
}
