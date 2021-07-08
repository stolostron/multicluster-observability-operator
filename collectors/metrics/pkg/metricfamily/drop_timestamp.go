package metricfamily

import clientmodel "github.com/prometheus/client_model/go"

// DropTimestamp is a transformer that removes timestamps from metrics.
func DropTimestamp(family *clientmodel.MetricFamily) (bool, error) {
	if family == nil {
		return true, nil
	}

	for _, m := range family.Metric {
		if m == nil {
			continue
		}
		m.TimestampMs = nil
	}

	return true, nil
}
