// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsnamespace

import (
	"context"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability/analytics/rs-utility"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// Namespace-specific resource names
	PlacementBindingName     = "rs-policyset-binding"
	PlacementName            = "rs-placement"
	PrometheusRulePolicyName = "rs-prom-rules-policy"
	PrometheusRuleName       = "acm-rs-namespace-prometheus-rules"
	ConfigMapName            = "rs-namespace-config"
)

var (
	// State variables - exported for testing
	Namespace = rsutility.DefaultNamespace
	Enabled   = false

	log = logf.Log.WithName("rs-namespace")

	// Component configuration
	componentConfig = rsutility.ComponentConfig{
		ComponentType:            rsutility.ComponentTypeNamespace,
		ConfigMapName:            ConfigMapName,
		PlacementName:            PlacementName,
		PlacementBindingName:     PlacementBindingName,
		PrometheusRulePolicyName: PrometheusRulePolicyName,
		DefaultNamespace:         rsutility.DefaultNamespace,
		GetDefaultConfigFunc:     GetDefaultRSNamespaceConfig,
		ApplyChangesFunc:         ApplyRSNamespaceConfigMapChanges,
	}

	// Component state
	componentState = &rsutility.ComponentState{
		Namespace: rsutility.DefaultNamespace,
		Enabled:   false,
	}
)

// HandleRightSizing handles the namespace right-sizing functionality
func HandleRightSizing(ctx context.Context, c client.Client, mco *mcov1beta2.MultiClusterObservability) error {
	log.V(1).Info("rs - handling namespace right-sizing")

	// Sync global state with component state
	componentState.Namespace = Namespace
	componentState.Enabled = Enabled

	// Use generic component handler
	err := rsutility.HandleComponentRightSizing(ctx, c, mco, componentConfig, componentState)

	// Sync component state back to global state for backward compatibility
	Namespace = componentState.Namespace
	Enabled = componentState.Enabled

	return err
}

// GetRightSizingNamespaceConfig gets the namespace right-sizing configuration
func GetRightSizingNamespaceConfig(mco *mcov1beta2.MultiClusterObservability) (bool, string) {
	enabled, binding, err := rsutility.GetComponentConfig(mco, rsutility.ComponentTypeNamespace)
	if err != nil {
		log.Error(err, "rs - failed to get namespace right-sizing config")
		return false, ""
	}
	return enabled, binding
}

// CleanupRSNamespaceResources cleans up the resources created for namespace right-sizing
func CleanupRSNamespaceResources(ctx context.Context, c client.Client, namespace string, bindingUpdated bool) {
	log.V(1).Info("rs - cleaning up namespace resources if exist")
	rsutility.CleanupComponentResources(ctx, c, componentConfig, namespace, bindingUpdated)
}
