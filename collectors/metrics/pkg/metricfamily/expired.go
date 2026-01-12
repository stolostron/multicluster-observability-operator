// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package metricfamily

import (
	"time"

	clientmodel "github.com/prometheus/client_model/go"
)

type dropExpiredSamples struct {
	min int64
}

func NewDropExpiredSamples(minTime time.Time) Transformer {
	return &dropExpiredSamples{
		min: minTime.Unix() * 1000,
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
