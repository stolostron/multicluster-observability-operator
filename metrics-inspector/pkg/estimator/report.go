// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package estimator

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
)

const (
	MetricCardinalityEstimate = "cardinality_estimate"
	MetricCardinalityChurn    = "cardinality_churn_ratio"
	MetricGroupLimit          = "cardinality_estimator_group_limit"
	MetricEstimatesDropped    = "cardinality_estimator_estimates_dropped"
)

// GroupEstimate holds the evaluated cardinality and churn ratio for a single group_by tuple.
type GroupEstimate struct {
	Values     []string
	Estimate   uint64
	ChurnRatio float64
}

// StreamReport represents a point-in-time evaluation of a single Estimator stream.
type StreamReport struct {
	Interval       time.Duration
	Filter         string
	GroupBy        []string
	GroupLimit     int64
	GroupSize      int64
	DroppedCount   uint64
	CalculateChurn bool

	// Populated when len(GroupBy) == 0.
	GlobalEstimate uint64
	GlobalChurn    float64

	// Populated when len(GroupBy) > 0 (already filtered by MinCardinality).
	Groups []GroupEstimate
}

// ByLabelName returns the Prometheus label name used for a group_by key.
func ByLabelName(key string) string {
	switch key {
	case "__name__":
		return "by__name__"
	case "__label__":
		return "by__label__"
	default:
		return "by_" + key
	}
}

type seriesPoint struct {
	labels labels.Labels
	value  float64
	isInt  bool
	intVal int64
}

func (r *StreamReport) collectSeries() []seriesPoint {
	intervalStr := r.Interval.String()
	b := labels.NewBuilder(labels.EmptyLabels())

	if len(r.GroupBy) == 0 {
		out := make([]seriesPoint, 0, 2)
		b.Reset(labels.EmptyLabels())
		b.Set(labels.MetricName, MetricCardinalityEstimate)
		b.Set("filter", r.Filter)
		b.Set("group_by_keys", globalKeyword)
		b.Set("interval", intervalStr)
		out = append(out, seriesPoint{
			labels: b.Labels(),
			value:  float64(r.GlobalEstimate),
			isInt:  true,
			intVal: int64(r.GlobalEstimate), //nolint:gosec // HLL estimate is bounded well below math.MaxInt64
		})

		if r.CalculateChurn {
			b.Set(labels.MetricName, MetricCardinalityChurn)
			out = append(out, seriesPoint{
				labels: b.Labels(),
				value:  r.GlobalChurn,
			})
		}
		return out
	}

	groupByKeys := strings.Join(r.GroupBy, ",")
	byLabelNames := make([]string, len(r.GroupBy))
	for i, k := range r.GroupBy {
		byLabelNames[i] = ByLabelName(k)
	}

	capacity := len(r.Groups) + 3
	if r.CalculateChurn {
		capacity += len(r.Groups)
	}
	out := make([]seriesPoint, 0, capacity)

	for _, g := range r.Groups {
		groupValues := strings.Join(g.Values, ",")

		b.Reset(labels.EmptyLabels())
		b.Set(labels.MetricName, MetricCardinalityEstimate)
		b.Set("filter", r.Filter)
		b.Set("group_by_keys", groupByKeys)
		b.Set("group_by_values", groupValues)
		b.Set("interval", intervalStr)
		for i, val := range g.Values {
			b.Set(byLabelNames[i], val)
		}
		out = append(out, seriesPoint{
			labels: b.Labels(),
			value:  float64(g.Estimate),
			isInt:  true,
			intVal: int64(g.Estimate), //nolint:gosec // HLL estimate is bounded well below math.MaxInt64
		})

		if r.CalculateChurn {
			b.Set(labels.MetricName, MetricCardinalityChurn)
			out = append(out, seriesPoint{
				labels: b.Labels(),
				value:  g.ChurnRatio,
			})
		}
	}

	// Emit group size metadata series: cardinality_estimate{group_by_keys="__group__", group_by_values="<keys>"}
	b.Reset(labels.EmptyLabels())
	b.Set(labels.MetricName, MetricCardinalityEstimate)
	b.Set("filter", r.Filter)
	b.Set("group_by_keys", groupKeyword)
	b.Set("group_by_values", groupByKeys)
	b.Set("interval", intervalStr)
	out = append(out, seriesPoint{
		labels: b.Labels(),
		value:  float64(r.GroupSize),
		isInt:  true,
		intVal: r.GroupSize,
	})

	// Emit group limit series
	b.Set(labels.MetricName, MetricGroupLimit)
	out = append(out, seriesPoint{
		labels: b.Labels(),
		value:  float64(r.GroupLimit),
		isInt:  true,
		intVal: r.GroupLimit,
	})

	if r.DroppedCount > 0 {
		b.Reset(labels.EmptyLabels())
		b.Set(labels.MetricName, MetricEstimatesDropped)
		b.Set("filter", r.Filter)
		b.Set("group_by_keys", groupByKeys)
		b.Set("interval", intervalStr)
		out = append(out, seriesPoint{
			labels: b.Labels(),
			value:  float64(r.DroppedCount),
			isInt:  true,
			intVal: int64(r.DroppedCount), //nolint:gosec // dropped group count fits in int64
		})
	}

	return out
}

// ToTimeSeries converts the StreamReport into Prometheus Remote Write TimeSeries using
// upstream Prometheus labels.Builder and prompb.FromLabels to guarantee canonical
// label sorting, deduplication, and empty-label omission.
func (r *StreamReport) ToTimeSeries(timestampMs int64) []prompb.TimeSeries {
	points := r.collectSeries()
	out := make([]prompb.TimeSeries, len(points))
	for i, pt := range points {
		out[i] = prompb.TimeSeries{
			Labels: prompb.FromLabels(pt.labels, nil),
			Samples: []prompb.Sample{
				{
					Value:     pt.value,
					Timestamp: timestampMs,
				},
			},
		}
	}
	return out
}

// WritePrometheusText formats the StreamReport in Prometheus text exposition format
// from the canonical labels.Labels set.
func (r *StreamReport) WritePrometheusText(w io.Writer) error {
	points := r.collectSeries()
	var b strings.Builder
	for _, pt := range points {
		b.Reset()
		b.WriteString(pt.labels.Get(labels.MetricName))
		b.WriteByte('{')
		first := true
		pt.labels.Range(func(l labels.Label) {
			if l.Name == labels.MetricName {
				return
			}
			if !first {
				b.WriteByte(',')
			}
			first = false
			b.WriteString(l.Name)
			b.WriteByte('=')
			b.WriteString(strconv.Quote(l.Value))
		})
		b.WriteString("} ")
		if pt.isInt {
			fmt.Fprintf(&b, "%d\n", pt.intVal)
		} else {
			fmt.Fprintf(&b, "%.4f\n", pt.value)
		}
		if _, err := io.WriteString(w, b.String()); err != nil {
			return err
		}
	}
	return nil
}
