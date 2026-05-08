// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsworkload

import (
	"context"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing/rs-utility"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func EnsureRSWorkloadConfigMapExists(ctx context.Context, c client.Client) error {
	return rsutility.EnsureRSConfigMapExists(ctx, c, ConfigMapName, GetDefaultRSWorkloadConfig)
}

func GetDefaultRSWorkloadConfig() map[string]string {
	ruleConfig := rsutility.GetDefaultRSPrometheusRuleConfig()
	placement := rsutility.GetDefaultRSPlacement()
	return map[string]string{
		"prometheusRuleConfig":   rsutility.FormatYAML(ruleConfig),
		"placementConfiguration": rsutility.FormatYAML(placement),
	}
}

func GetRightSizingWorkloadConfigData(cm *corev1.ConfigMap) (rsutility.RSNamespaceConfigMapData, error) {
	return rsutility.GetRSConfigData(cm)
}

func GetWorkloadRSConfigMapPredicateFunc(ctx context.Context, c client.Client) predicate.Funcs {
	return rsutility.GetRSConfigMapPredicateFunc(ctx, c, ConfigMapName, ApplyRSWorkloadConfigMapChanges)
}

func ApplyRSWorkloadConfigMapChanges(ctx context.Context, c client.Client, configData rsutility.RSNamespaceConfigMapData) error {
	enabled := false
	mcoList := &mcov1beta2.MultiClusterObservabilityList{}
	if err := c.List(ctx, mcoList); err == nil && len(mcoList.Items) > 0 {
		mco := mcoList.Items[0]
		if mco.Spec.Capabilities != nil && mco.Spec.Capabilities.Platform != nil {
			enabled = mco.Spec.Capabilities.Platform.Analytics.WorkloadPodRightSizingRecommendation.Enabled
		}
	}

	prometheusRule, err := GeneratePrometheusRuleWithFeatures(configData, enabled, enabled)
	if err != nil {
		return err
	}

	if enabled {
		if err := rsutility.ApplyPrometheusRule(ctx, c, prometheusRule); err != nil {
			return err
		}
	} else {
		if err := rsutility.DeletePrometheusRule(ctx, c, PrometheusRuleName, rsutility.MonitoringNamespace); err != nil {
			return err
		}
	}

	if err := CreateOrUpdateWorkloadPrometheusRulePolicy(ctx, c, prometheusRule); err != nil {
		return err
	}
	if err := CreateUpdateWorkloadPlacement(ctx, c, configData.PlacementConfiguration); err != nil {
		return err
	}
	if err := CreateWorkloadPlacementBinding(ctx, c); err != nil {
		return err
	}
	log.Info("rs - workload configmap changes applied")
	return nil
}
