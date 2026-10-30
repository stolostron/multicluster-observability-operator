// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package collector

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/prometheus/prometheus/promql/parser"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	sigsyaml "sigs.k8s.io/yaml"
)

func TestDefaultAllowlistRecordingRuleQueries(t *testing.T) {
	data, err := os.ReadFile("../../../multiclusterobservability/manifests/base/config/metrics_allowlist.yaml")
	require.NoError(t, err)
	var cm corev1.ConfigMap
	require.NoError(t, sigsyaml.Unmarshal(data, &cm))
	var allowlist operatorconfig.MetricsAllowlist
	require.NoError(t, yaml.Unmarshal([]byte(cm.Data[operatorconfig.MetricsConfigMapKey]), &allowlist))
	require.NotEmpty(t, allowlist.RecordingRuleList)

	collector := &MetricsCollector{
		ObsAddon: &oav1beta1.ObservabilityAddon{},
		HubInfo:  &operatorconfig.HubInfo{},
	}
	count := 0
	foundTrend := false
	for _, arg := range collector.getCommands(false, &deploymentParams{allowlist: &allowlist}) {
		encoded, ok := strings.CutPrefix(arg, "--recordingrule=")
		if !ok {
			continue
		}
		count++
		// The collector decodes JSON before sending the query to Prometheus.
		// Parsing the YAML expression directly misses escaping errors on that path.
		var rule struct{ Name, Query string }
		require.NoError(t, json.Unmarshal([]byte(encoded), &rule))
		t.Run(rule.Name, func(t *testing.T) {
			expr, err := parser.ParseExpr(rule.Query)
			require.NoError(t, err)
			if rule.Name != "sli:apiserver_request_duration_seconds:trend:1m" {
				return
			}
			for _, selectors := range parser.ExtractSelectors(expr) {
				for _, matcher := range selectors {
					if matcher.Name != "le" {
						continue
					}
					foundTrend = true
					for _, value := range []string{"1", "1.0"} {
						require.True(t, matcher.Matches(value), "must include the one-second bucket %q", value)
					}
					for _, value := range []string{"0.1", "10", "100", "+Inf"} {
						require.False(t, matcher.Matches(value), "must exclude other buckets: %q", value)
					}
				}
			}
		})
	}
	require.Equal(t, len(allowlist.RecordingRuleList), count)
	require.True(t, foundTrend, "the API server latency rule must select the one-second buckets")
}
