package metricfamily

import clientmodel "github.com/prometheus/client_model/go"

type DropUnsorted struct {
	timestamp int64
}

func (o *DropUnsorted) Transform(family *clientmodel.MetricFamily) (bool, error) {
	for i, m := range family.Metric {
		if m == nil {
			continue
		}
		var ts int64
		if m.TimestampMs != nil {
			ts = *m.TimestampMs
		}
		if ts < o.timestamp {
			family.Metric[i] = nil
			continue
		}
		o.timestamp = ts
	}
	o.timestamp = 0
	return true, nil
}
