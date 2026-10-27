// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package estimator

import (
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/prometheus/prompb"
	"github.com/stolostron/multicluster-observability-operator/estimators/cardinality/pkg/protoparser"
)

const prodStreamsYAML = `max_items_per_interval: 15000000
streams:
  - interval: 10m
    group_by: [cluster]
    hll_precision: 14
    calculate_churn: true
    group_limit: 5000
    min_cardinality: 0
  - interval: 10m
    group_by: [__name__]
    hll_precision: 14
    calculate_churn: true
    group_limit: 10000
    min_cardinality: 0
  - interval: 10m
    group_by: [cluster, namespace]
    hll_precision: 12
    calculate_churn: true
    group_limit: 500000
    min_cardinality: 25
  - interval: 10m
    group_by: [cluster, __name__]
    hll_precision: 12
    calculate_churn: false
    group_limit: 650000
    min_cardinality: 25
`

func buildSyntheticWritePayload(b *testing.B, numSeries int) []byte {
	b.Helper()
	var wr prompb.WriteRequest
	wr.Timeseries = make([]prompb.TimeSeries, numSeries)
	for i := range numSeries {
		wr.Timeseries[i] = prompb.TimeSeries{
			Labels: []prompb.Label{
				{Name: "__name__", Value: fmt.Sprintf("container_memory_working_set_bytes_%d", i%250)},
				{Name: "cluster", Value: fmt.Sprintf("managed-cluster-%04d", i%2600)},
				{Name: "namespace", Value: fmt.Sprintf("openshift-workload-%03d", i%150)},
				{Name: "pod", Value: fmt.Sprintf("workload-pod-%05d", i)},
			},
			Samples: []prompb.Sample{{Value: float64(i), Timestamp: 1700000000000}},
		}
	}
	raw, err := wr.Marshal()
	if err != nil {
		b.Fatalf("Marshal failed: %v", err)
	}
	return raw
}

func BenchmarkUnmarshalWriteRequest2000Series(b *testing.B) {
	raw := buildSyntheticWritePayload(b, 2000)
	var wr protoparser.WriteRequest

	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for range b.N {
		wr.Reset()
		if err := wr.UnmarshalWriteRequest(raw, false); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEngineIngestSteadyStateDuplicates(b *testing.B) {
	cfg, err := ParseConfig([]byte(prodStreamsYAML))
	if err != nil {
		b.Fatal(err)
	}
	eng, err := NewEngine(cfg)
	if err != nil {
		b.Fatal(err)
	}
	defer eng.Stop()

	raw := buildSyntheticWritePayload(b, 2000)
	var wr protoparser.WriteRequest
	if err := wr.UnmarshalWriteRequest(raw, false); err != nil {
		b.Fatal(err)
	}
	// Prime the Bloom filter (simulating first scrape in the 10m window).
	eng.Ingest(wr.Timeseries)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		eng.Ingest(wr.Timeseries)
	}
	b.ReportMetric(float64(len(wr.Timeseries)*b.N)/b.Elapsed().Seconds(), "series/s")
}

func BenchmarkEngineIngestColdStartUniqueSeries(b *testing.B) {
	cfg, err := ParseConfig([]byte(prodStreamsYAML))
	if err != nil {
		b.Fatal(err)
	}
	eng, err := NewEngine(cfg)
	if err != nil {
		b.Fatal(err)
	}
	defer eng.Stop()

	raw := buildSyntheticWritePayload(b, 2000)
	var wr protoparser.WriteRequest
	if err := wr.UnmarshalWriteRequest(raw, false); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		// Mix 64-bit golden-ratio prime into both halves of Hash so every iteration
		// exercises full Bloom filter miss + all 4 HLL streams.
		mix := uint64(i+1) * 0x9e3779b97f4a7c15 //nolint:gosec // benchmark hash mixer
		for j := range wr.Timeseries {
			wr.Timeseries[j].Hash ^= mix
		}
		eng.Ingest(wr.Timeseries)
	}
	b.ReportMetric(float64(len(wr.Timeseries)*b.N)/b.Elapsed().Seconds(), "series/s")
}

func BenchmarkEngineSnapshots(b *testing.B) {
	cfg, err := ParseConfig([]byte(prodStreamsYAML))
	if err != nil {
		b.Fatal(err)
	}
	eng, err := NewEngine(cfg)
	if err != nil {
		b.Fatal(err)
	}
	defer eng.Stop()

	raw := buildSyntheticWritePayload(b, 10000)
	var wr protoparser.WriteRequest
	if err := wr.UnmarshalWriteRequest(raw, false); err != nil {
		b.Fatal(err)
	}
	eng.Ingest(wr.Timeseries)
	now := time.Now()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		snaps := eng.Snapshots(now)
		for i := range snaps {
			_ = snaps[i].ToTimeSeries(now.UnixMilli())
		}
	}
}
