package metricfamily

import (
	"time"

	clientmodel "github.com/prometheus/client_model/go"
)

type dropExpiredSamples struct {
	min int64
}

func NewDropExpiredSamples(min time.Time) Transformer {
	return &dropExpiredSamples{
		min: min.Unix() * 1000,
	}
}

func (t *dropExpiredSamples) Transform(family *clientmodel.MetricFamily) (bool, error) {
	for i, m := range family.Metric {
		if m == nil {
			continue
		}
		if ts := m.GetTimestampMs(); ts < t.min {
			family.Metric[i] = nil
			continue
		}
	}
	return true, nil
}
