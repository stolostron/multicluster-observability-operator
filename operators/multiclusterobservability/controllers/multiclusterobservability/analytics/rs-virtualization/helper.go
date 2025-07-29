// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsvirtualization

import (
	"context"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability/analytics/rs-utility"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// Virtualization-specific resource names
	PlacementBindingName     = "rs-virt-policyset-binding"
	PlacementName            = "rs-virt-placement"
	PrometheusRulePolicyName = "rs-virt-prom-rules-policy"
	PrometheusRuleName       = "acm-rs-virt-prometheus-rules"
	ConfigMapName            = "rs-virt-config"
)

var (
	// State variables - exported for testing
	Namespace = rsutility.DefaultNamespace
	Enabled   = false

	log = logf.Log.WithName("rs-virtualization")

	// Component configuration
	componentConfig = rsutility.ComponentConfig{
		ComponentType:            rsutility.ComponentTypeVirtualization,
		ConfigMapName:            ConfigMapName,
		PlacementName:            PlacementName,
		PlacementBindingName:     PlacementBindingName,
		PrometheusRulePolicyName: PrometheusRulePolicyName,
		DefaultNamespace:         rsutility.DefaultNamespace,
		GetDefaultConfigFunc:     GetDefaultRSVirtualizationConfig,
		ApplyChangesFunc:         ApplyRSVirtualizationConfigMapChanges,
	}

	// Component state
	componentState = &rsutility.ComponentState{
		Namespace: rsutility.DefaultNamespace,
		Enabled:   false,
	}
)

// HandleRightSizing handles the virtualization right-sizing functionality
func HandleRightSizing(ctx context.Context, c client.Client, mco *mcov1beta2.MultiClusterObservability) error {
	log.V(1).Info("rs - handling virtualization right-sizing")

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

// GetRightSizingVirtualizationConfig gets the virtualization right-sizing configuration
func GetRightSizingVirtualizationConfig(mco *mcov1beta2.MultiClusterObservability) (bool, string) {
	enabled, binding, err := rsutility.GetComponentConfig(mco, rsutility.ComponentTypeVirtualization)
	if err != nil {
		log.Error(err, "rs - failed to get virtualization right-sizing config")
		return false, ""
	}
	return enabled, binding
}

// CleanupRSVirtualizationResources cleans up the resources created for virtualization right-sizing
func CleanupRSVirtualizationResources(ctx context.Context, c client.Client, namespace string, bindingUpdated bool) {
	log.V(1).Info("rs - cleaning up virtualization resources if exist")
	rsutility.CleanupComponentResources(ctx, c, componentConfig, namespace, bindingUpdated)
}
