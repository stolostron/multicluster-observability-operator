// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsutility

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
)

func TestFormatYAML_ValidData(t *testing.T) {
	input := RSPrometheusRuleConfig{
		NamespaceFilterCriteria: struct {
			InclusionCriteria []string `json:"inclusionCriteria"`
			ExclusionCriteria []string `json:"exclusionCriteria"`
		}{
			InclusionCriteria: []string{"ns1", "ns2"},
			ExclusionCriteria: []string{"ns3"},
		},
		LabelFilterCriteria:      []RSLabelFilter{},
		RecommendationPercentage: 80,
	}

	output := FormatYAML(input)
	assert.Contains(t, output, "inclusionCriteria:")
	assert.Contains(t, output, "recommendationPercentage: 80")
}

func TestFormatYAML_WithPlacement(t *testing.T) {
	labelSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{
			"environment": "prod",
		},
	}

	placement := clusterv1beta1.Placement{
		Spec: clusterv1beta1.PlacementSpec{
			Predicates: []clusterv1beta1.ClusterPredicate{
				{
					RequiredClusterSelector: clusterv1beta1.ClusterSelector{
						LabelSelector: labelSelector,
					},
				},
			},
		},
	}

	output := FormatYAML(placement)
	assert.Contains(t, output, "predicates:")
	assert.Contains(t, output, "labelselector:")
	assert.Contains(t, output, "matchlabels:")
	assert.Contains(t, output, "environment: prod")
}

func TestGetDefaultRSPrometheusRuleConfig(t *testing.T) {
	config := GetDefaultRSPrometheusRuleConfig()

	assert.Equal(t, DefaultRecommendationPercentage, config.RecommendationPercentage)
	assert.Equal(t, 110, config.RecommendationPercentage)
	assert.Contains(t, config.NamespaceFilterCriteria.ExclusionCriteria, "openshift.*")
	assert.Empty(t, config.NamespaceFilterCriteria.InclusionCriteria)
	assert.Empty(t, config.LabelFilterCriteria)
}
