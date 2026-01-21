// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsutility

import (
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	"sigs.k8s.io/yaml"
)

// FormatYAML converts a Go data structure to a YAML-formatted string
func FormatYAML[T RSPrometheusRuleConfig | clusterv1beta1.Placement](data T) string {
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		log.Error(err, "rs - error marshaling data to yaml: %v")
		return ""
	}
	return string(yamlData)
}

// GetDefaultRSPrometheusRuleConfig creates a default prometheus rule configuration for right-sizing
func GetDefaultRSPrometheusRuleConfig() RSPrometheusRuleConfig {
	var ruleConfig RSPrometheusRuleConfig
	ruleConfig.NamespaceFilterCriteria.ExclusionCriteria = []string{"openshift.*"}
	ruleConfig.RecommendationPercentage = DefaultRecommendationPercentage
	return ruleConfig
}

// IsPlatformFeatureConfigured checks if the Platform feature is enabled
func IsPlatformFeatureConfigured(mco *mcov1beta2.MultiClusterObservability) bool {
	return mco.Spec.Capabilities != nil &&
		mco.Spec.Capabilities.Platform != nil
}
