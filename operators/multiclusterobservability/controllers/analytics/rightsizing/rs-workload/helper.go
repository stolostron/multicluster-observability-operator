// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsworkload

import (
	"context"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing/rs-utility"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// NOTE: governance-policy-propagator admission enforces (namespace + name) length <= 62.
	// With the default namespaceBinding "open-cluster-management-global-set" (34 chars),
	// keep names <= 28 chars.
	PlacementBindingName     = "rs-wl-policyset-binding"
	PlacementName            = "rs-wl-placement"
	PrometheusRulePolicyName = "rs-wl-prom-rules-policy"
	PrometheusRuleName       = "acm-rs-workload-prometheus-rules"
	ConfigMapName            = "rs-workload-config"
)

var (
	log = logf.Log.WithName("rs-workload")

	componentConfig = rsutility.ComponentConfig{
		ComponentType:            rsutility.ComponentTypeWorkload,
		ConfigMapName:            ConfigMapName,
		PlacementName:            PlacementName,
		PlacementBindingName:     PlacementBindingName,
		PrometheusRulePolicyName: PrometheusRulePolicyName,
		PrometheusRuleName:       PrometheusRuleName,
		DefaultNamespace:         rsutility.DefaultNamespace,
		GetDefaultConfigFunc:     GetDefaultRSWorkloadConfig,
		ApplyChangesFunc:         ApplyRSWorkloadConfigMapChanges,
	}

	ComponentState = &rsutility.ComponentState{
		Namespace: rsutility.DefaultNamespace,
		Enabled:   false,
	}
)

func HandleRightSizing(ctx context.Context, c client.Client, mco *mcov1beta2.MultiClusterObservability) error {
	log.V(1).Info("rs - handling workload+pod right-sizing")
	return rsutility.HandleComponentRightSizing(ctx, c, mco, componentConfig, ComponentState)
}

func CleanupRSWorkloadResources(ctx context.Context, c client.Client, namespace string, bindingUpdated bool) error {
	return rsutility.CleanupComponentResources(ctx, c, componentConfig, namespace, bindingUpdated)
}
