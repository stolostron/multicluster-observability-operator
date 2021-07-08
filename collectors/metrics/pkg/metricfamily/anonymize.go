package metricfamily

import (
	"crypto/sha256"
	"encoding/base64"

	clientmodel "github.com/prometheus/client_model/go"
)

type AnonymizeMetrics struct {
	salt     string
	global   map[string]struct{}
	byMetric map[string]map[string]struct{}
}

// NewMetricsAnonymizer hashes label values on the incoming metrics using a cryptographic hash.
// Because the cardinality of most label values is low, only a portion of the hash is returned.
// To prevent rainbow tables from being used to recover the label value, each client should use
// a salt value. Because label values are expected to remain stable over many sessions, the salt
// must also be stable over the same time period. The salt should not be shared with the remote
// agent. This type is not thread-safe.
func NewMetricsAnonymizer(salt string, labels []string, metricsLabels map[string][]string) *AnonymizeMetrics {
	global := make(map[string]struct{})
	for _, label := range labels {
		global[label] = struct{}{}
	}
	byMetric := make(map[string]map[string]struct{})
	for name, labels := range metricsLabels {
		l := make(map[string]struct{})
		for _, label := range labels {
			l[label] = struct{}{}
		}
		byMetric[name] = l
	}
	return &AnonymizeMetrics{
		salt:     salt,
		global:   global,
		byMetric: byMetric,
	}
}

func (a *AnonymizeMetrics) Transform(family *clientmodel.MetricFamily) (bool, error) {
	if family == nil {
		return false, nil
	}
	if set, ok := a.byMetric[family.GetName()]; ok {
		transformMetricLabelValues(a.salt, family.Metric, a.global, set)
	} else {
		transformMetricLabelValues(a.salt, family.Metric, a.global)
	}
	return true, nil
}

func transformMetricLabelValues(salt string, metrics []*clientmodel.Metric, sets ...map[string]struct{}) {
	for _, m := range metrics {
		if m == nil {
			continue
		}
		for _, pair := range m.Label {
			if pair.Value == nil || *pair.Value == "" {
				continue
			}
			name := pair.GetName()
			for _, set := range sets {
				_, ok := set[name]
				if !ok {
					continue
				}
				v := secureValueHash(salt, pair.GetValue())
				pair.Value = &v
				break
			}
		}
	}
}

// secureValueHash hashes the input value for moderately low cardinality (< 1 million unique inputs)
// and converts it to a base64 string suitable for use as a label value in Prometheus.
func secureValueHash(salt, value string) string {
	hash := sha256.Sum256([]byte(salt + value))
	return base64.RawURLEncoding.EncodeToString(hash[:9])
}
