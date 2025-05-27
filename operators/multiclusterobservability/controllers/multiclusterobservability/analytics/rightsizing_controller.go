// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics

import (
	"context"
	"fmt"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	corev1 "k8s.io/api/core/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	rsPolicySetName                   = "rs-policyset"
	rsPlacementName                   = "rs-placement"
	rsPlacementBindingName            = "rs-policyset-binding"
	rsPrometheusRulePolicyName        = "rs-prom-rules-policy"
	rsPrometheusRulePolicyConfigName  = "rs-prometheus-rules-policy-config"
	rsPrometheusRuleName              = "acm-rs-namespace-prometheus-rules"
	rsConfigMapName                   = "rs-namespace-config"
	rsDefaultNamespace                = "open-cluster-management-global-set"
	rsMonitoringNamespace             = "openshift-monitoring"
	rsDefaultRecommendationPercentage = 110
)

var (
	rsNamespace = rsDefaultNamespace
	isEnabled   = false
	log         = logf.Log.WithName("analytics")
)

type RSLabelFilter struct {
	LabelName         string   `yaml:"labelName"`
	InclusionCriteria []string `yaml:"inclusionCriteria,omitempty"`
	ExclusionCriteria []string `yaml:"exclusionCriteria,omitempty"`
}

type RSPrometheusRuleConfig struct {
	NamespaceFilterCriteria struct {
		InclusionCriteria []string `yaml:"inclusionCriteria"`
		ExclusionCriteria []string `yaml:"exclusionCriteria"`
	} `yaml:"namespaceFilterCriteria"`
	LabelFilterCriteria      []RSLabelFilter `yaml:"labelFilterCriteria"`
	RecommendationPercentage int             `yaml:"recommendationPercentage"`
}

type RSNamespaceConfigMapData struct {
	PrometheusRuleConfig   RSPrometheusRuleConfig   `yaml:"prometheusRuleConfig"`
	PlacementConfiguration clusterv1beta1.Placement `yaml:"placementConfiguration"`
}

func CreateRightSizingComponent(
	ctx context.Context,
	c client.Client,
	mco *mcov1beta2.MultiClusterObservability,
) error {
	log.V(1).Info("rs - inside create rs component")

	//  Get right-sizing namespace configuration
	isRightSizingNamespaceEnabled, newBinding := getRightSizingNamespaceConfig(mco)

	// Set to default namespace if not given
	if newBinding == "" {
		newBinding = rsDefaultNamespace
	}

	// Check if right-sizing namespace feature enabled or not
	// If disabled then cleanup related resources
	if !isRightSizingNamespaceEnabled {
		log.V(1).Info("rs - namespace rs feature not enabled")
		cleanupRSNamespaceResources(ctx, c, rsNamespace, false)
		rsNamespace = newBinding
		isEnabled = false
		return nil
	}

	// Set the flag if namespaceBindingUpdated
	namespaceBindingUpdated := rsNamespace != newBinding && isEnabled

	// Set isEnabled flag which will be used for checking namespaceBindingUpdated condition next time
	isEnabled = true

	// Retrieve the existing namespace as existingNamespace and update the new namespace in rsNamespace
	existingNamespace := rsNamespace
	rsNamespace = newBinding

	// Creating configmap with default values
	if err := EnsureRSNamespaceConfigMapExists(ctx, c); err != nil {
		return err
	}

	if namespaceBindingUpdated {
		// Clean up resources except config map to update NamespaceBinding
		cleanupRSNamespaceResources(ctx, c, existingNamespace, true)

		// Get configmap
		cm := &corev1.ConfigMap{}
		if err := c.Get(ctx, client.ObjectKey{Name: rsConfigMapName, Namespace: config.GetDefaultNamespace()}, cm); err != nil {
			return fmt.Errorf("rs - failed to get existing configmap: %w", err)
		}

		// Get configmap data into specified structure
		configData, err := GetRightSizingConfigData(cm)
		if err != nil {
			return fmt.Errorf("rs - failed to extract config data: %w", err)
		}

		// If NamespaceBinding has been updated apply the Policy Placement Placementbinding again
		if err := applyRSNamespaceConfigMapChanges(ctx, c, configData); err != nil {
			return fmt.Errorf("rs - failed to apply configmap changes: %w", err)
		}
	}

	log.Info("rs - create component task completed")
	return nil
}
