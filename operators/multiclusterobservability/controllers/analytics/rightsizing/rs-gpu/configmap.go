// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsgpu

import (
	"context"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing/rs-utility"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func EnsureRSGPUConfigMapExists(ctx context.Context, c client.Client) error {
	return rsutility.EnsureRSConfigMapExists(ctx, c, ConfigMapName, GetDefaultRSGPUConfig)
}

func GetDefaultRSGPUConfig() map[string]string {
	ruleConfig := rsutility.GetDefaultRSPrometheusRuleConfig()
	placement := rsutility.GetDefaultRSPlacement()
	return map[string]string{
		"prometheusRuleConfig":   rsutility.FormatYAML(ruleConfig),
		"placementConfiguration": rsutility.FormatYAML(placement),
	}
}

func GetRightSizingGPUConfigData(cm *corev1.ConfigMap) (rsutility.RSNamespaceConfigMapData, error) {
	return rsutility.GetRSConfigData(cm)
}

func GetGPURSConfigMapPredicateFunc(ctx context.Context, c client.Client) predicate.Funcs {
	return rsutility.GetRSConfigMapPredicateFunc(ctx, c, ConfigMapName, ApplyRSGPUConfigMapChanges)
}

func ApplyRSGPUConfigMapChanges(ctx context.Context, c client.Client, configData rsutility.RSNamespaceConfigMapData) error {
	// Prevent duplicate definition of the shared mapping rule (acm_rs:pod_workload:relabel:5m)
	// when rs-workload is enabled alongside rs-gpu.
	workloadOrPodEnabled := false
	enabled := false
	mcoList := &mcov1beta2.MultiClusterObservabilityList{}
	if err := c.List(ctx, mcoList); err == nil && len(mcoList.Items) > 0 {
		mco := mcoList.Items[0]
		if mco.Spec.Capabilities != nil && mco.Spec.Capabilities.Platform != nil {
			workloadOrPodEnabled = mco.Spec.Capabilities.Platform.Analytics.WorkloadPodRightSizingRecommendation.Enabled
			enabled = mco.Spec.Capabilities.Platform.Analytics.GPURightSizingRecommendation.Enabled
		}
	}

	prometheusRule, err := GeneratePrometheusRuleWithMapping(configData, !workloadOrPodEnabled)
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

	if err := CreateOrUpdateGPUPrometheusRulePolicy(ctx, c, prometheusRule); err != nil {
		return err
	}
	if err := CreateUpdateGPUPlacement(ctx, c, configData.PlacementConfiguration); err != nil {
		return err
	}
	if err := CreateGPUPlacementBinding(ctx, c); err != nil {
		return err
	}

	log.Info("rs - gpu configmap changes applied")
	return nil
}
