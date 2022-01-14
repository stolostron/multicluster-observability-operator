package metricfamily

import (
	"fmt"

	clientmodel "github.com/prometheus/client_model/go"
)

var (
	ErrUnsorted        = fmt.Errorf("metrics in provided family are not in increasing timestamp order")
	ErrNoTimestamp     = fmt.Errorf("metrics in provided family do not have a timestamp")
	ErrTimestampTooOld = fmt.Errorf("metrics in provided family have a timestamp that is too old, check clock skew")
)

type errorOnUnsorted struct {
	timestamp int64
	require   bool
}

func NewErrorOnUnsorted(requireTimestamp bool) Transformer {
	return &errorOnUnsorted{
		require: requireTimestamp,
	}
}

func (t *errorOnUnsorted) Transform(family *clientmodel.MetricFamily) (bool, error) {
	t.timestamp = 0
	for _, m := range family.Metric {
		if m == nil {
			continue
		}
		var ts int64
		if m.TimestampMs != nil {
			ts = *m.TimestampMs
		} else if t.require {
			return false, ErrNoTimestamp
		}
		if ts < t.timestamp {
			return false, ErrUnsorted
		}
		t.timestamp = ts
	}
	return true, nil
}
