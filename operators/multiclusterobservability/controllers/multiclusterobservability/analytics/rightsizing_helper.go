// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics

import (
	"gopkg.in/yaml.v2"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
)

// FormatYAML converts a Go data structure to a YAML-formatted string
func FormatYAML[T RSPrometheusRuleConfig | clusterv1beta1.Placement](data T) string {
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		log.Error(err, "Error marshaling data to YAML: %v")
		return ""
	}
	return string(yamlData)
}
