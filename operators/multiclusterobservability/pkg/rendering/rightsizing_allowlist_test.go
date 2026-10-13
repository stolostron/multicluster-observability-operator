// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rendering

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

// Legacy collection must forward the same profiled metrics as MCOA federation.
// Intermediate :5m series stay on the source cluster, avoiding extra cardinality.
func TestWorkloadAndPodMetricsInLegacyAllowlist(t *testing.T) {
	data, err := os.ReadFile("../../manifests/base/config/metrics_allowlist.yaml")
	require.NoError(t, err)
	var cm corev1.ConfigMap
	require.NoError(t, yaml.Unmarshal(data, &cm))
	var allowlist struct {
		Names []string `json:"names"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(cm.Data["metrics_list.yaml"]), &allowlist))
	require.Contains(t, allowlist.Names, "acm_rs:pod:info", "pods without resource metrics must reach the dashboard")
	for _, level := range []string{"workload", "pod"} {
		for _, metric := range []string{
			"cpu_request", "cpu_limit", "cpu_usage", "cpu_recommendation",
			"memory_request", "memory_limit", "memory_usage", "memory_recommendation",
		} {
			name := "acm_rs:" + level + ":" + metric
			count := 0
			for _, entry := range allowlist.Names {
				if entry == name {
					count++
				}
			}
			require.Equal(t, 1, count, "%s must be forwarded exactly once", name)
			require.NotContains(t, allowlist.Names, name+":5m")
		}
	}
}
