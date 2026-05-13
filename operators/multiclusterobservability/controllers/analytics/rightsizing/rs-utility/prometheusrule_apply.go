// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsutility

import (
	"context"
	"fmt"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ApplyPrometheusRule ensures the given PrometheusRule exists on the hub cluster (create/update).
// This makes hub/right-sizing work even when policy propagation to local-cluster
// is disabled (e.g. hub self-management off).
func ApplyPrometheusRule(ctx context.Context, c client.Client, desired monitoringv1.PrometheusRule) error {
	existing := &monitoringv1.PrometheusRule{}
	key := types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}
	if err := c.Get(ctx, key, existing); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("rs - failed to get prometheusrule %s/%s: %w", desired.Namespace, desired.Name, err)
		}
		if err := c.Create(ctx, &desired); err != nil {
			if apierrors.IsAlreadyExists(err) {
				return ApplyPrometheusRule(ctx, c, desired)
			}
			return fmt.Errorf("rs - failed to create prometheusrule %s/%s: %w", desired.Namespace, desired.Name, err)
		}
		log.Info("rs - created prometheusrule on hub", "namespace", desired.Namespace, "name", desired.Name)
		return nil
	}

	desired.ResourceVersion = existing.ResourceVersion
	if err := c.Update(ctx, &desired); err != nil {
		return fmt.Errorf("rs - failed to update prometheusrule %s/%s: %w", desired.Namespace, desired.Name, err)
	}
	log.Info("rs - updated prometheusrule on hub", "namespace", desired.Namespace, "name", desired.Name)
	return nil
}

// DeletePrometheusRule deletes the PrometheusRule (best-effort).
func DeletePrometheusRule(ctx context.Context, c client.Client, name, namespace string) error {
	pr := &monitoringv1.PrometheusRule{}
	pr.Name = name
	pr.Namespace = namespace
	if err := c.Delete(ctx, pr); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("rs - failed to delete prometheusrule %s/%s: %w", namespace, name, err)
	}
	return nil
}
