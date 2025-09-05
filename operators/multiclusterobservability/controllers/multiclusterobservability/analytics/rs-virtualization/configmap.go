// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsvirtualization

import (
	"context"

	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability/analytics/rs-utility"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// EnsureRSVirtualizationConfigMapExists ensures that the ConfigMap exists, creating it if necessary
func EnsureRSVirtualizationConfigMapExists(ctx context.Context, c client.Client) error {
	return rsutility.EnsureRSConfigMapExists(ctx, c, ConfigMapName, GetDefaultRSVirtualizationConfig)
}

func GetDefaultRSVirtualizationConfig() map[string]string {
	// get default config data with PrometheusRule config and placement config
	ruleConfig := rsutility.GetDefaultRSPrometheusRuleConfig()
	placement := rsutility.GetDefaultRSPlacement()

	return map[string]string{
		"prometheusRuleConfig":   rsutility.FormatYAML(ruleConfig),
		"placementConfiguration": rsutility.FormatYAML(placement),
	}
}

// GetRightSizingVirtualizationConfigData extracts and unmarshals the data from the ConfigMap into RSVirtualizationConfigMapData
func GetRightSizingVirtualizationConfigData(cm *corev1.ConfigMap) (rsutility.RSNamespaceConfigMapData, error) {
	return rsutility.GetRSConfigData(cm)
}

func GetVirtualizationRSConfigMapPredicateFunc(ctx context.Context, c client.Client) predicate.Funcs {
	return rsutility.GetRSConfigMapPredicateFunc(ctx, c, ConfigMapName, ApplyRSVirtualizationConfigMapChanges)
}

func ApplyRSVirtualizationConfigMapChanges(ctx context.Context, c client.Client, configData rsutility.RSNamespaceConfigMapData) error {

	prometheusRule, err := GeneratePrometheusRule(configData)
	if err != nil {
		return err
	}

	err = CreateOrUpdateVirtualizationPrometheusRulePolicy(ctx, c, prometheusRule)
	if err != nil {
		return err
	}

	err = CreateUpdateVirtualizationPlacement(ctx, c, configData.PlacementConfiguration)
	if err != nil {
		return err
	}

	err = CreateVirtualizationPlacementBinding(ctx, c)
	if err != nil {
		return err
	}
	log.Info("rs - virtualization configmap changes applied")

	return nil
}
