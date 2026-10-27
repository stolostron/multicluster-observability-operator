// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package estimator

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

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

// Snapshot represents a point-in-time evaluation of a single Estimator stream.
type Snapshot struct {
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

// ToTimeSeries converts the Snapshot into Prometheus Remote Write TimeSeries with
// lexicographically sorted label names as required by Thanos Receive.
func (s *Snapshot) ToTimeSeries(timestampMs int64) []prompb.TimeSeries {
	intervalStr := s.Interval.String()

	if len(s.GroupBy) == 0 {
		out := make([]prompb.TimeSeries, 0, 2)
		out = append(out, newTimeSeries(
			[]prompb.Label{
				{Name: "__name__", Value: MetricCardinalityEstimate},
				{Name: "filter", Value: s.Filter},
				{Name: "group_by_keys", Value: globalKeyword},
				{Name: "interval", Value: intervalStr},
			},
			float64(s.GlobalEstimate),
			timestampMs,
		))
		if s.CalculateChurn {
			out = append(out, newTimeSeries(
				[]prompb.Label{
					{Name: "__name__", Value: MetricCardinalityChurn},
					{Name: "filter", Value: s.Filter},
					{Name: "group_by_keys", Value: globalKeyword},
					{Name: "interval", Value: intervalStr},
				},
				s.GlobalChurn,
				timestampMs,
			))
		}
		return out
	}

	groupByKeys := strings.Join(s.GroupBy, ",")
	byLabelNames := make([]string, len(s.GroupBy))
	for i, k := range s.GroupBy {
		byLabelNames[i] = ByLabelName(k)
	}

	capacity := len(s.Groups) + 3
	if s.CalculateChurn {
		capacity += len(s.Groups)
	}
	out := make([]prompb.TimeSeries, 0, capacity)

	for _, g := range s.Groups {
		groupValues := strings.Join(g.Values, ",")

		estLabels := make([]prompb.Label, 0, 5+len(s.GroupBy))
		estLabels = append(estLabels,
			prompb.Label{Name: "__name__", Value: MetricCardinalityEstimate},
			prompb.Label{Name: "filter", Value: s.Filter},
			prompb.Label{Name: "group_by_keys", Value: groupByKeys},
			prompb.Label{Name: "group_by_values", Value: groupValues},
			prompb.Label{Name: "interval", Value: intervalStr},
		)
		for i, val := range g.Values {
			estLabels = append(estLabels, prompb.Label{Name: byLabelNames[i], Value: val})
		}
		out = append(out, newTimeSeries(estLabels, float64(g.Estimate), timestampMs))

		if s.CalculateChurn {
			churnLabels := make([]prompb.Label, 0, 5+len(s.GroupBy))
			churnLabels = append(churnLabels,
				prompb.Label{Name: "__name__", Value: MetricCardinalityChurn},
				prompb.Label{Name: "filter", Value: s.Filter},
				prompb.Label{Name: "group_by_keys", Value: groupByKeys},
				prompb.Label{Name: "group_by_values", Value: groupValues},
				prompb.Label{Name: "interval", Value: intervalStr},
			)
			for i, val := range g.Values {
				churnLabels = append(churnLabels, prompb.Label{Name: byLabelNames[i], Value: val})
			}
			out = append(out, newTimeSeries(churnLabels, g.ChurnRatio, timestampMs))
		}
	}

	// Emit group size metadata series: cardinality_estimate{group_by_keys="__group__", group_by_values="<keys>"}
	out = append(out, newTimeSeries(
		[]prompb.Label{
			{Name: "__name__", Value: MetricCardinalityEstimate},
			{Name: "filter", Value: s.Filter},
			{Name: "group_by_keys", Value: groupKeyword},
			{Name: "group_by_values", Value: groupByKeys},
			{Name: "interval", Value: intervalStr},
		},
		float64(s.GroupSize),
		timestampMs,
	))

	// Emit group limit series
	out = append(out, newTimeSeries(
		[]prompb.Label{
			{Name: "__name__", Value: MetricGroupLimit},
			{Name: "filter", Value: s.Filter},
			{Name: "group_by_keys", Value: groupKeyword},
			{Name: "group_by_values", Value: groupByKeys},
			{Name: "interval", Value: intervalStr},
		},
		float64(s.GroupLimit),
		timestampMs,
	))

	if s.DroppedCount > 0 {
		out = append(out, newTimeSeries(
			[]prompb.Label{
				{Name: "__name__", Value: MetricEstimatesDropped},
				{Name: "filter", Value: s.Filter},
				{Name: "group_by_keys", Value: groupByKeys},
				{Name: "interval", Value: intervalStr},
			},
			float64(s.DroppedCount),
			timestampMs,
		))
	}

	return out
}

func newTimeSeries(lbls []prompb.Label, val float64, timestampMs int64) prompb.TimeSeries {
	slices.SortFunc(lbls, func(a, b prompb.Label) int {
		return cmp.Compare(a.Name, b.Name)
	})
	return prompb.TimeSeries{
		Labels: lbls,
		Samples: []prompb.Sample{
			{
				Value:     val,
				Timestamp: timestampMs,
			},
		},
	}
}

// WritePrometheusText formats the Snapshot in Prometheus text exposition format.
func (s *Snapshot) WritePrometheusText(w io.Writer) error {
	var b strings.Builder
	intervalQ := strconv.Quote(s.Interval.String())
	filterQ := strconv.Quote(s.Filter)

	if len(s.GroupBy) == 0 {
		fmt.Fprintf(&b, "%s{interval=%s,filter=%s,group_by_keys=%q} %d\n",
			MetricCardinalityEstimate, intervalQ, filterQ, globalKeyword, s.GlobalEstimate)
		if s.CalculateChurn {
			fmt.Fprintf(&b, "%s{interval=%s,filter=%s,group_by_keys=%q} %.4f\n",
				MetricCardinalityChurn, intervalQ, filterQ, globalKeyword, s.GlobalChurn)
		}
		_, err := io.WriteString(w, b.String())
		return err
	}

	groupByKeys := strings.Join(s.GroupBy, ",")
	groupByKeysQ := strconv.Quote(groupByKeys)
	byLabelNames := make([]string, len(s.GroupBy))
	for i, k := range s.GroupBy {
		byLabelNames[i] = ByLabelName(k)
	}

	for _, g := range s.Groups {
		groupValuesQ := strconv.Quote(strings.Join(g.Values, ","))
		b.Reset()
		fmt.Fprintf(&b, "%s{interval=%s,filter=%s,group_by_keys=%s,group_by_values=%s",
			MetricCardinalityEstimate, intervalQ, filterQ, groupByKeysQ, groupValuesQ)
		for i, val := range g.Values {
			fmt.Fprintf(&b, ",%s=%s", byLabelNames[i], strconv.Quote(val))
		}
		fmt.Fprintf(&b, "} %d\n", g.Estimate)

		if s.CalculateChurn {
			fmt.Fprintf(&b, "%s{interval=%s,filter=%s,group_by_keys=%s,group_by_values=%s",
				MetricCardinalityChurn, intervalQ, filterQ, groupByKeysQ, groupValuesQ)
			for i, val := range g.Values {
				fmt.Fprintf(&b, ",%s=%s", byLabelNames[i], strconv.Quote(val))
			}
			fmt.Fprintf(&b, "} %.4f\n", g.ChurnRatio)
		}

		if _, err := io.WriteString(w, b.String()); err != nil {
			return err
		}
	}

	b.Reset()
	fmt.Fprintf(&b, "%s{interval=%s,filter=%s,group_by_keys=%q,group_by_values=%s} %d\n",
		MetricCardinalityEstimate, intervalQ, filterQ, groupKeyword, groupByKeysQ, s.GroupSize)
	fmt.Fprintf(&b, "%s{interval=%s,filter=%s,group_by_keys=%q,group_by_values=%s} %d\n",
		MetricGroupLimit, intervalQ, filterQ, groupKeyword, groupByKeysQ, s.GroupLimit)
	if s.DroppedCount > 0 {
		fmt.Fprintf(&b, "%s{interval=%s,filter=%s,group_by_keys=%s} %d\n",
			MetricEstimatesDropped, intervalQ, filterQ, groupByKeysQ, s.DroppedCount)
	}
	_, err := io.WriteString(w, b.String())
	return err
}
