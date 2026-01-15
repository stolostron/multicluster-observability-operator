// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsutility

import (
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("rs-utility")

// Common constants
const (
	DefaultRecommendationPercentage = 110
	MonitoringNamespace             = "openshift-monitoring"
	DefaultNamespace                = "open-cluster-management-global-set"
)

// RSLabelFilter represents label filtering criteria for right-sizing
type RSLabelFilter struct {
	LabelName         string   `yaml:"labelName"`
	InclusionCriteria []string `yaml:"inclusionCriteria,omitempty"`
	ExclusionCriteria []string `yaml:"exclusionCriteria,omitempty"`
}

// RSPrometheusRuleConfig represents the Prometheus rule configuration for right-sizing
type RSPrometheusRuleConfig struct {
	NamespaceFilterCriteria struct {
		InclusionCriteria []string `yaml:"inclusionCriteria"`
		ExclusionCriteria []string `yaml:"exclusionCriteria"`
	} `yaml:"namespaceFilterCriteria"`
	LabelFilterCriteria      []RSLabelFilter `yaml:"labelFilterCriteria"`
	RecommendationPercentage int             `yaml:"recommendationPercentage"`
}

// RSNamespaceConfigMapData represents the configmap data structure for right-sizing namespace
type RSNamespaceConfigMapData struct {
	PrometheusRuleConfig   RSPrometheusRuleConfig   `yaml:"prometheusRuleConfig"`
	PlacementConfiguration clusterv1beta1.Placement `yaml:"placementConfiguration"`
}
