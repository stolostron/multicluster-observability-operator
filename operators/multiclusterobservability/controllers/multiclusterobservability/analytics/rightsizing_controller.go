// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics

import (
	"context"

	"github.com/cloudflare/cfssl/log"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

var rsNamespace = rsDefaultNamespace

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
	scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability,
	mgr ctrl.Manager,
) (*ctrl.Result, error) {
	log.Info("RS - Inside CreateRightSizingComponent")

	if !isRightSizingNamespaceEnabled(mco) {
		log.Info("RS - NamespaceRightSizing feature not enabled")
		cleanupRSNamespaceResources(ctx, c, rsNamespace, false)
		return nil, nil
	}

	// Check if NamespaceBinding has been updated or not if not available set to default
	newBinding := mco.Spec.Capabilities.Platform.Analytics.NamespaceRightSizingRecommendation.NamespaceBinding
	if newBinding == "" {
		newBinding = rsDefaultNamespace
	}

	// Set the flag if namespaceBindingUpdated
	namespaceBindingUpdated := rsNamespace != newBinding

	// Retrieve the existing namespace as existingNamespace and update the new namespace in rsNamespace
	existingNamespace := rsNamespace
	rsNamespace = newBinding

	// Creating configmap with default values
	if err := EnsureRSNamespaceConfigMapExists(ctx, c); err != nil {
		return nil, err
	}

	if namespaceBindingUpdated {
		// Clean up resources except config map to update NamespaceBinding
		cleanupRSNamespaceResources(ctx, c, existingNamespace, true)

		// Get configmap
		cm := &corev1.ConfigMap{}
		if err := c.Get(ctx, client.ObjectKey{Name: rsConfigMapName, Namespace: config.GetDefaultNamespace()}, cm); err != nil {
			log.Error(err, "Failed to get RS ConfigMap")
			return nil, err
		}

		// Get configmap data into specified structure
		configData, err := GetRightSizingConfigData(cm)
		if err != nil {
			log.Error(err, "Failed to extract RightSizingConfigData")
			return nil, err
		}

		// If NamespaceBinding has been updated apply the Policy Placement Placementbinding again
		if err := applyRSNamespaceConfigMapChanges(ctx, c, configData); err != nil {
			log.Error(err, "Failed to apply RS Namespace ConfigMap Changes")
			return nil, err
		}
	}

	log.Info("RS - CreateRightSizingComponent task completed")
	return nil, nil
}
