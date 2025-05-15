// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics

import (
	"context"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
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
) (*ctrl.Result, error) {
	log.Info("RS - Inside CreateRightSizingComponent")

	//  Get right-sizing namespace configuration
	isRightSizingNamespaceEnabled, newBinding := getRightSizingNamespaceConfig(mco)

	// Set to default namespace if not given
	if newBinding == "" {
		newBinding = rsDefaultNamespace
	}

	// Check if right-sizing namespace feature enabled or not
	// If disabled then cleanup related resources
	if !isRightSizingNamespaceEnabled {
		log.Info("RS - NamespaceRightSizing feature not enabled")
		// cleanupRSNamespaceResources(ctx, c, rsNamespace, false)
		rsNamespace = newBinding
		isEnabled = false
		return nil, nil
	}

	// Set the flag if namespaceBindingUpdated
	namespaceBindingUpdated := rsNamespace != newBinding && isEnabled

	// Set isEnabled flag which will be used for checking namespaceBindingUpdated condition next time
	isEnabled = true

	// Retrieve the existing namespace as existingNamespace and update the new namespace in rsNamespace
	// existingNamespace := rsNamespace
	rsNamespace = newBinding

	// Creating configmap with default values
	if err := EnsureRSNamespaceConfigMapExists(ctx, c); err != nil {
		return nil, err
	}

	if namespaceBindingUpdated {
		log.Info("RS - namespaceBindingUpdated")
		// // Clean up resources except config map to update NamespaceBinding
		// cleanupRSNamespaceResources(ctx, c, existingNamespace, true)

		// // Get configmap
		// cm := &corev1.ConfigMap{}
		// if err := c.Get(ctx, client.ObjectKey{Name: rsConfigMapName, Namespace: config.GetDefaultNamespace()}, cm); err != nil {
		// 	log.Error(err, "Failed to get RS ConfigMap")
		// 	return nil, err
		// }

		// // Get configmap data into specified structure
		// configData, err := GetRightSizingConfigData(cm)
		// if err != nil {
		// 	log.Error(err, "Failed to extract RightSizingConfigData")
		// 	return nil, err
		// }

		// // If NamespaceBinding has been updated apply the Policy Placement Placementbinding again
		// if err := applyRSNamespaceConfigMapChanges(ctx, c, configData); err != nil {
		// 	log.Error(err, "Failed to apply RS Namespace ConfigMap Changes")
		// 	return nil, err
		// }
	}

	log.Info("RS - CreateRightSizingComponent task completed")
	return nil, nil
}
