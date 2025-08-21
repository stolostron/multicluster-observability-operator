// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsnamespace

import (
	"context"

	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability/analytics/rs-utility"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// EnsureRSNamespaceConfigMapExists ensures that the ConfigMap exists, creating it if necessary
func EnsureRSNamespaceConfigMapExists(ctx context.Context, c client.Client) error {
	return rsutility.EnsureRSConfigMapExists(ctx, c, ConfigMapName, GetDefaultRSNamespaceConfig)
}

// GetDefaultRSNamespaceConfig returns default config data
func GetDefaultRSNamespaceConfig() map[string]string {
	// get default config data with PrometheusRule config and placement config
	ruleConfig := rsutility.GetDefaultRSPrometheusRuleConfig()
	placement := rsutility.GetDefaultRSPlacement()

	return map[string]string{
		"prometheusRuleConfig":   rsutility.FormatYAML(ruleConfig),
		"placementConfiguration": rsutility.FormatYAML(placement),
	}
}

// GetRightSizingConfigData extracts and unmarshals the data from the ConfigMap into RightSizingConfigData
func GetRightSizingConfigData(cm *corev1.ConfigMap) (rsutility.RSNamespaceConfigMapData, error) {
	return rsutility.GetRSConfigData(cm)
}

// Gets the namesapce rightsizing predicate function
func GetNamespaceRSConfigMapPredicateFunc(ctx context.Context, c client.Client) predicate.Funcs {
	return rsutility.GetRSConfigMapPredicateFunc(ctx, c, ConfigMapName, ApplyRSNamespaceConfigMapChanges)
}

// ApplyRSNamespaceConfigMapChanges updates PrometheusRule, Policy, Placement based on configmap changes
func ApplyRSNamespaceConfigMapChanges(ctx context.Context, c client.Client, configData rsutility.RSNamespaceConfigMapData) error {

	prometheusRule, err := GeneratePrometheusRule(configData)
	if err != nil {
		return err
	}

	err = CreateOrUpdatePrometheusRulePolicy(ctx, c, prometheusRule)
	if err != nil {
		return err
	}

	err = CreateUpdatePlacement(ctx, c, configData.PlacementConfiguration)
	if err != nil {
		return err
	}

	err = CreatePlacementBinding(ctx, c)
	if err != nil {
		return err
	}
	log.Info("rs - namespace configmap changes applied")

	return nil
}
