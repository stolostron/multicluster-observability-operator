// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsvirtualization

import (
	"context"
	"fmt"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateOrUpdateVirtualizationPrometheusRulePolicy creates or updates the PrometheusRule policy for virtualization
func CreateOrUpdateVirtualizationPrometheusRulePolicy(
	ctx context.Context,
	c client.Client,
	prometheusRule monitoringv1.PrometheusRule,
) error {
	policy := &policyv1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PrometheusRulePolicyName,
			Namespace: Namespace,
		},
	}

	// Declare name, namespace in common log context and use it later everywhere
	logCtx := []any{"Namespace:", policy.Namespace, ", Name:", policy.Name}

	err := c.Get(ctx, client.ObjectKey{Name: policy.Name, Namespace: policy.Namespace}, policy)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new policy
			policy.Spec.PolicyTemplates = []*policyv1.PolicyTemplate{
				{
					ObjectDefinition: runtime.RawExtension{
						Object: &prometheusRule,
					},
				},
			}

			if err := c.Create(ctx, policy); err != nil {
				return fmt.Errorf("rs - failed to create policy: %w", err)
			}
			log.Info("rs - policy created successfully", logCtx...)
			return nil
		}
		return fmt.Errorf("rs - failed to fetch policy: %w", err)
	}

	// Update existing policy
	policy.Spec.PolicyTemplates = []*policyv1.PolicyTemplate{
		{
			ObjectDefinition: runtime.RawExtension{
				Object: &prometheusRule,
			},
		},
	}

	if err := c.Update(ctx, policy); err != nil {
		return fmt.Errorf("rs - failed to update policy: %w", err)
	}

	log.Info("rs - policy updated successfully", logCtx...)
	return nil
}
