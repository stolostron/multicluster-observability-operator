// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package metricfamily

import (
	"errors"

	clientmodel "github.com/prometheus/client_model/go"
)

var (
	ErrUnsorted        = errors.New("metrics in provided family are not in increasing timestamp order")
	ErrNoTimestamp     = errors.New("metrics in provided family do not have a timestamp")
	ErrTimestampTooOld = errors.New("metrics in provided family have a timestamp that is too old, check clock skew")
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
