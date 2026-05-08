// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsgpu

import (
	"context"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing/rs-utility"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	PlacementBindingName     = "rs-gpu-policyset-binding"
	PlacementName            = "rs-gpu-placement"
	PrometheusRulePolicyName = "rs-gpu-prom-rules-policy"
	PrometheusRuleName       = "acm-rs-gpu-prometheus-rules"
	ConfigMapName            = "rs-gpu-config"
)

var (
	log = logf.Log.WithName("rs-gpu")

	componentConfig = rsutility.ComponentConfig{
		ComponentType:            rsutility.ComponentTypeGPU,
		ConfigMapName:            ConfigMapName,
		PlacementName:            PlacementName,
		PlacementBindingName:     PlacementBindingName,
		PrometheusRulePolicyName: PrometheusRulePolicyName,
		PrometheusRuleName:       PrometheusRuleName,
		DefaultNamespace:         rsutility.DefaultNamespace,
		GetDefaultConfigFunc:     GetDefaultRSGPUConfig,
		ApplyChangesFunc:         ApplyRSGPUConfigMapChanges,
	}

	ComponentState = &rsutility.ComponentState{
		Namespace: rsutility.DefaultNamespace,
		Enabled:   false,
	}
)

func HandleRightSizing(ctx context.Context, c client.Client, mco *mcov1beta2.MultiClusterObservability) error {
	log.V(1).Info("rs - handling gpu right-sizing")
	return rsutility.HandleComponentRightSizing(ctx, c, mco, componentConfig, ComponentState)
}

func CleanupRSGPUResources(ctx context.Context, c client.Client, namespace string, bindingUpdated bool) error {
	return rsutility.CleanupComponentResources(ctx, c, componentConfig, namespace, bindingUpdated)
}
