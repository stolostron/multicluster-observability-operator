// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package estimator

import (
	"bytes"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stolostron/multicluster-observability-operator/estimators/cardinality/pkg/hll"
	"github.com/stolostron/multicluster-observability-operator/estimators/cardinality/pkg/protoparser"
)

func TestParseConfig(t *testing.T) {
	yamlData := `
max_items_per_interval: 100000
streams:
  - interval: 15m
    group_by: [cluster, __name__]
    hll_precision: 12
    calculate_churn: false
    group_limit: 5000
    min_cardinality: 25
  - interval: 15m
    filter: '{__name__=~"container_.*", namespace!="kube-system"}'
    group_by: [__label__, cluster]
    calculate_churn: true
`
	cfg, err := ParseConfig([]byte(yamlData))
	if err != nil {
		t.Fatalf("unexpected ParseConfig error: %v", err)
	}
	if len(cfg.Streams) != 2 {
		t.Fatalf("expected 2 streams, got %d", len(cfg.Streams))
	}
	// Verify order is preserved for [cluster, __name__]
	if !slices.Equal(cfg.Streams[0].GroupBy, []string{"cluster", "__name__"}) {
		t.Fatalf("expected [cluster, __name__], got %v", cfg.Streams[0].GroupBy)
	}
	// Verify __label__ is normalized to the end
	if !slices.Equal(cfg.Streams[1].GroupBy, []string{"cluster", "__label__"}) {
		t.Fatalf("expected [cluster, __label__], got %v", cfg.Streams[1].GroupBy)
	}
	if !cfg.HasLabelKeyword() {
		t.Fatalf("expected HasLabelKeyword() to be true")
	}
}

func TestDeduplicator(t *testing.T) {
	d := NewDeduplicator(time.Minute, 10000)
	defer d.Stop()

	for i := range uint64(500) {
		fp := xxhash.Sum64(fmt.Appendf(nil, "series-%d", i))
		if !d.CheckAndAdd(fp) {
			t.Fatalf("expected first CheckAndAdd(%d) to return true", i)
		}
		if d.CheckAndAdd(fp) {
			t.Fatalf("expected second CheckAndAdd(%d) to return false (duplicate)", i)
		}
	}
}

func TestDeduplicatorShardRotation(t *testing.T) {
	d := NewDeduplicator(time.Hour, 100000)
	defer d.Stop()

	// Insert 100 items, each routed to shard h % 10.
	hashes := make([]uint64, 100)
	for i := range hashes {
		h := xxhash.Sum64(fmt.Appendf(nil, "rot-series-%d", i))
		hashes[i] = h
		if !d.CheckAndAdd(h) {
			t.Fatalf("expected initial insert %d to pass through", i)
		}
	}

	// Reset only shard 0.
	d.resetShard(0)

	for i, h := range hashes {
		got := d.CheckAndAdd(h)
		if h%dedupBucketsCount == 0 {
			if !got {
				t.Fatalf("expected item %d in rotated shard 0 to be accepted again", i)
			}
		} else {
			if got {
				t.Fatalf("expected item %d in unrotated shard %d to still be deduplicated", i, h%dedupBucketsCount)
			}
		}
	}
}

func TestDeduplicatorSaturationPassThrough(t *testing.T) {
	// Configure maxItemsPerInterval = 100 -> bucketMaxSize = 10 per shard.
	d := NewDeduplicator(time.Hour, 100)
	defer d.Stop()

	// Saturate shard 0 with 10 unique hashes where h % 10 == 0.
	var shard0Hashes []uint64
	for i := uint64(0); len(shard0Hashes) < 10; i++ {
		h := xxhash.Sum64(fmt.Appendf(nil, "sat-%d", i))
		if h%dedupBucketsCount != 0 {
			continue
		}
		shard0Hashes = append(shard0Hashes, h)
		if !d.CheckAndAdd(h) {
			t.Fatalf("expected pre-saturation insert to return true")
		}
	}

	// Once shard 0 has reached bucketMaxSize (10), even previously seen hashes in shard 0
	// must pass through (return true) rather than risking false-positive drops on new series.
	for _, h := range shard0Hashes {
		if !d.CheckAndAdd(h) {
			t.Fatalf("expected saturated shard 0 to switch to pass-through (return true)")
		}
	}

	// Resetting shard 0 restores normal deduplication.
	d.resetShard(0)
	first := shard0Hashes[0]
	if !d.CheckAndAdd(first) {
		t.Fatalf("expected first insert after shard reset to return true")
	}
	if d.CheckAndAdd(first) {
		t.Fatalf("expected second insert after shard reset to be deduplicated (return false)")
	}
}

func TestEstimatorRotationsAndExpiry(t *testing.T) {
	est, err := NewEstimator(StreamConfig{
		Interval:       time.Hour, // long interval so we control rotation deterministically
		GroupBy:        []string{"cluster"},
		HLLPrecision:   12,
		GroupLimit:     100,
		Buckets:        4,
		CalculateChurn: true,
	})
	if err != nil {
		t.Fatalf("NewEstimator failed: %v", err)
	}
	defer est.Stop()

	makeBatch := func(cluster string, start, count int) []protoparser.TimeSeries {
		out := make([]protoparser.TimeSeries, 0, count)
		for i := start; i < start+count; i++ {
			name := fmt.Sprintf("m_%d", i)
			out = append(out, protoparser.TimeSeries{
				Hash: xxhash.Sum64([]byte(cluster + "|" + name)),
				Labels: []prompb.Label{
					{Name: "__name__", Value: name},
					{Name: "cluster", Value: cluster},
				},
			})
		}
		return out
	}

	// Window 1: insert 100 series for cluster-1 and 50 for cluster-2.
	est.InsertMany(makeBatch("cluster-1", 0, 100))
	est.InsertMany(makeBatch("cluster-2", 0, 50))

	t0 := time.Now()
	snap1 := est.SnapshotAt(t0)
	if len(snap1.Groups) != 2 {
		t.Fatalf("expected 2 groups in window 1, got %d", len(snap1.Groups))
	}

	// Rotate all 4 buckets once (curr -> prev, new empty curr).
	for _, b := range est.buckets {
		b.rotate()
	}

	// Window 2: insert 100 more series (100..199) for cluster-1; nothing for cluster-2.
	est.InsertMany(makeBatch("cluster-1", 100, 100))

	snap2 := est.SnapshotAt(t0.Add(15 * time.Minute))
	// Both cluster-1 (prev 100 + curr 100 = ~200) and cluster-2 (prev 50) should still be present!
	if len(snap2.Groups) != 2 {
		t.Fatalf("expected 2 groups after 1st rotation (prev+curr), got %d", len(snap2.Groups))
	}
	for _, g := range snap2.Groups {
		switch g.Values[0] {
		case "cluster-1":
			if g.Estimate < 185 || g.Estimate > 215 {
				t.Fatalf("expected cluster-1 merged estimate ~200, got %d", g.Estimate)
			}
		case "cluster-2":
			if g.Estimate < 45 || g.Estimate > 55 {
				t.Fatalf("expected cluster-2 prev estimate ~50, got %d", g.Estimate)
			}
		}
	}

	// Rotate all 4 buckets a second time: Window 1 expires completely.
	for _, b := range est.buckets {
		b.rotate()
	}
	snap3 := est.SnapshotAt(t0.Add(30 * time.Minute))
	// cluster-2 has now expired; cluster-1 retains only Window 2's 100 series.
	if len(snap3.Groups) != 1 {
		t.Fatalf("expected 1 group after 2nd rotation (cluster-2 expired), got %d", len(snap3.Groups))
	}
	if snap3.Groups[0].Values[0] != "cluster-1" || snap3.Groups[0].Estimate < 90 || snap3.Groups[0].Estimate > 110 {
		t.Fatalf("expected cluster-1 estimate ~100 after 2nd rotation, got %+v", snap3.Groups[0])
	}

	// Rotate a third time with no inserts: all groups expire.
	for _, b := range est.buckets {
		b.rotate()
	}
	snap4 := est.SnapshotAt(t0.Add(45 * time.Minute))
	if len(snap4.Groups) != 0 || snap4.GroupSize != 0 {
		t.Fatalf("expected 0 groups and GroupSize=0 after 3rd rotation, got groups=%d size=%d", len(snap4.Groups), snap4.GroupSize)
	}
}

func TestEstimatorGroupLimitAndOverflowAging(t *testing.T) {
	est, err := NewEstimator(StreamConfig{
		Interval:     time.Hour,
		GroupBy:      []string{"__name__"},
		HLLPrecision: 10,
		GroupLimit:   10, // cap at 10 tracked groups
		Buckets:      4,
	})
	if err != nil {
		t.Fatalf("NewEstimator failed: %v", err)
	}
	defer est.Stop()

	// Insert 50 distinct metric names (exceeding GroupLimit = 10).
	var tss []protoparser.TimeSeries
	for i := range 50 {
		name := fmt.Sprintf("metric_overflow_%d", i)
		tss = append(tss, protoparser.TimeSeries{
			Hash:   xxhash.Sum64([]byte(name)),
			Labels: []prompb.Label{{Name: "__name__", Value: name}},
		})
	}
	est.InsertMany(tss)

	snap := est.SnapshotAt(time.Now())
	if len(snap.Groups) != 10 {
		t.Fatalf("expected active groups to be capped at GroupLimit=10, got %d", len(snap.Groups))
	}
	// GroupSize includes both the 10 tracked groups and the HLL estimate of rejected groups (~40).
	if snap.GroupSize < 35 || snap.GroupSize > 65 {
		t.Fatalf("expected GroupSize (tracked + rejected HLL) ~50, got %d", snap.GroupSize)
	}

	// Rotate twice so all groups and rejected sketches age out, then insert 5 new groups.
	for _, b := range est.buckets {
		b.rotate()
		b.rotate()
	}
	est.InsertMany(tss[:5])
	snapAfter := est.SnapshotAt(time.Now().Add(time.Hour))
	if len(snapAfter.Groups) != 5 {
		t.Fatalf("expected 5 groups after rotation recovery, got %d", len(snapAfter.Groups))
	}
	if snapAfter.GroupSize != 5 {
		t.Fatalf("expected GroupSize=5 after rotation recovery, got %d", snapAfter.GroupSize)
	}
}

func TestEstimatorLabelKeywordWithWireParser(t *testing.T) {
	est, err := NewEstimator(StreamConfig{
		Interval:     time.Hour,
		GroupBy:      []string{"cluster", "__name__", "__label__"},
		HLLPrecision: 12,
		GroupLimit:   100,
		Buckets:      4,
	})
	if err != nil {
		t.Fatalf("NewEstimator failed: %v", err)
	}
	defer est.Stop()

	// Build a protobuf WriteRequest with 20 pod values and 2 namespace values on the same metric.
	var p prompb.WriteRequest
	for i := range 20 {
		p.Timeseries = append(p.Timeseries, prompb.TimeSeries{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "kube_pod_info"},
				{Name: "cluster", Value: "local-cluster"},
				{Name: "namespace", Value: fmt.Sprintf("ns-%d", i%2)},
				{Name: "pod", Value: fmt.Sprintf("pod-%d", i)},
			},
			Samples: []prompb.Sample{{Value: 1, Timestamp: 1000}},
		})
	}
	raw, err := p.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var wr protoparser.WriteRequest
	if err := wr.UnmarshalWriteRequest(raw, true); err != nil {
		t.Fatalf("UnmarshalWriteRequest failed: %v", err)
	}
	est.InsertMany(wr.Timeseries)

	snap := est.SnapshotAt(time.Now())
	byLabel := make(map[string]uint64)
	for _, g := range snap.Groups {
		if len(g.Values) != 3 {
			t.Fatalf("expected 3 group values [cluster, __name__, __label__], got %v", g.Values)
		}
		byLabel[g.Values[2]] = g.Estimate
	}

	if byLabel["pod"] < 18 || byLabel["pod"] > 22 {
		t.Fatalf("expected __label__=pod cardinality ~20, got %d", byLabel["pod"])
	}
	if byLabel["namespace"] != 2 {
		t.Fatalf("expected __label__=namespace cardinality 2, got %d", byLabel["namespace"])
	}
	if byLabel["cluster"] != 1 {
		t.Fatalf("expected __label__=cluster cardinality 1, got %d", byLabel["cluster"])
	}
}

func TestEstimatorFilterAndGlobalStream(t *testing.T) {
	est, err := NewEstimator(StreamConfig{
		Interval:       time.Hour,
		Filter:         `{__name__=~"apiserver_.*", verb!="WATCH", cluster="hub"}`,
		GroupBy:        nil, // global stream (no group_by)
		HLLPrecision:   12,
		Buckets:        4,
		CalculateChurn: true,
	})
	if err != nil {
		t.Fatalf("NewEstimator failed: %v", err)
	}
	defer est.Stop()

	tss := []protoparser.TimeSeries{
		{
			Hash: 101,
			Labels: []prompb.Label{
				{Name: "__name__", Value: "apiserver_request_total"},
				{Name: "cluster", Value: "hub"},
				{Name: "verb", Value: "GET"},
			},
		},
		{
			Hash: 102,
			Labels: []prompb.Label{
				{Name: "__name__", Value: "apiserver_request_total"},
				{Name: "cluster", Value: "hub"},
				{Name: "verb", Value: "WATCH"}, // excluded by verb!="WATCH"
			},
		},
		{
			Hash: 103,
			Labels: []prompb.Label{
				{Name: "__name__", Value: "node_cpu_seconds_total"}, // excluded by __name__=~"apiserver_.*"
				{Name: "cluster", Value: "hub"},
			},
		},
	}
	est.InsertMany(tss)

	now := time.Now()
	snap := est.SnapshotAt(now)
	if snap.GlobalEstimate != 1 {
		t.Fatalf("expected GlobalEstimate=1 after filter matching, got %d", snap.GlobalEstimate)
	}

	var buf bytes.Buffer
	if err := snap.WritePrometheusText(&buf); err != nil {
		t.Fatalf("WritePrometheusText failed: %v", err)
	}
	if !strings.Contains(buf.String(), `cardinality_estimate{interval="1h0m0s",filter="{__name__=~\"apiserver_.*\", verb!=\"WATCH\", cluster=\"hub\"}",group_by_keys="__global__"} 1`) {
		t.Fatalf("unexpected Prometheus output for global filtered stream:\n%s", buf.String())
	}
}

func TestSnapshotLabelEscaping(t *testing.T) {
	snap := Snapshot{
		Interval:       15 * time.Minute,
		GroupBy:        []string{"cluster", "reason"},
		GroupLimit:     100,
		GroupSize:      1,
		CalculateChurn: true,
		Groups: []GroupEstimate{
			{
				Values:     []string{"cluster-1", "line1\n\"quoted\"\\slash"},
				Estimate:   42,
				ChurnRatio: 0.125,
			},
		},
	}

	var buf bytes.Buffer
	if err := snap.WritePrometheusText(&buf); err != nil {
		t.Fatalf("WritePrometheusText failed: %v", err)
	}
	out := buf.String()
	expected := `by_cluster="cluster-1",by_reason="line1\n\"quoted\"\\slash"`
	if !strings.Contains(out, expected) {
		t.Fatalf("expected escaped label %s in:\n%s", expected, out)
	}
}

func TestCalculateChurnRatio(t *testing.T) {
	s1 := hll.MustNewByPrecision(14)
	s2 := hll.MustNewByPrecision(14)
	defer s1.Release()
	defer s2.Release()

	// Identical sets -> 0 churn
	for i := range 2000 {
		h := xxhash.Sum64(fmt.Appendf(nil, "item-%d", i))
		s1.InsertHash(h)
		s2.InsertHash(h)
	}
	if r := CalculateChurnRatio(s1, s2); r > 0.02 {
		t.Fatalf("expected ~0 churn for identical sketches, got %.4f", r)
	}

	// Disjoint sets -> ~1.0 churn
	s3 := hll.MustNewByPrecision(14)
	defer s3.Release()
	for i := 2000; i < 4000; i++ {
		h := xxhash.Sum64(fmt.Appendf(nil, "item-%d", i))
		s3.InsertHash(h)
	}
	if r := CalculateChurnRatio(s1, s3); math.Abs(r-1.0) > 0.05 {
		t.Fatalf("expected ~1.0 churn for disjoint sketches, got %.4f", r)
	}
}

func TestEngineIngestAndSnapshots(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
max_items_per_interval: 100000
streams:
  - interval: 15m
    group_by: [cluster]
    hll_precision: 14
    calculate_churn: true
    group_limit: 100
    min_cardinality: 0
  - interval: 15m
    group_by: [cluster, namespace]
    hll_precision: 12
    calculate_churn: true
    group_limit: 100
    min_cardinality: 10
`))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	eng, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer eng.Stop()

	var batch []protoparser.TimeSeries
	// 50 series in cluster-a / ns-high (above min_cardinality=10)
	for i := range 50 {
		name := fmt.Sprintf("metric_%d", i)
		fp := xxhash.Sum64([]byte("cluster-a|ns-high|" + name))
		batch = append(batch, protoparser.TimeSeries{
			Hash: fp,
			Labels: []prompb.Label{
				{Name: "__name__", Value: name},
				{Name: "cluster", Value: "cluster-a"},
				{Name: "namespace", Value: "ns-high"},
			},
		})
	}
	// 5 series in cluster-a / ns-low (below min_cardinality=10)
	for i := range 5 {
		name := fmt.Sprintf("small_%d", i)
		fp := xxhash.Sum64([]byte("cluster-a|ns-low|" + name))
		batch = append(batch, protoparser.TimeSeries{
			Hash: fp,
			Labels: []prompb.Label{
				{Name: "__name__", Value: name},
				{Name: "cluster", Value: "cluster-a"},
				{Name: "namespace", Value: "ns-low"},
			},
		})
	}

	// Ingest twice to verify Bloom filter deduplication
	eng.Ingest(batch)
	eng.Ingest(batch)

	if got := eng.IngestedSeriesTotal(); got != 110 {
		t.Fatalf("expected IngestedSeriesTotal=110, got %d", got)
	}
	if got := eng.DeduplicatedSeriesTotal(); got != 55 {
		t.Fatalf("expected DeduplicatedSeriesTotal=55, got %d", got)
	}

	now := time.Now()
	snaps := eng.Snapshots(now)
	if len(snaps) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snaps))
	}

	// Stream 0: [cluster] -> 1 group ("cluster-a") with ~55 series
	if len(snaps[0].Groups) != 1 {
		t.Fatalf("expected 1 cluster group, got %d", len(snaps[0].Groups))
	}
	if snaps[0].Groups[0].Estimate < 52 || snaps[0].Groups[0].Estimate > 58 {
		t.Fatalf("expected cluster-a estimate ~55, got %d", snaps[0].Groups[0].Estimate)
	}

	// Stream 1: [cluster, namespace] -> ns-low dropped by min_cardinality=10, only ns-high emitted
	if len(snaps[1].Groups) != 1 {
		t.Fatalf("expected 1 namespace group after min_cardinality filter, got %d", len(snaps[1].Groups))
	}
	if snaps[1].DroppedCount != 1 {
		t.Fatalf("expected DroppedCount=1, got %d", snaps[1].DroppedCount)
	}
	if !slices.Equal(snaps[1].Groups[0].Values, []string{"cluster-a", "ns-high"}) {
		t.Fatalf("unexpected group values: %v", snaps[1].Groups[0].Values)
	}

	// Verify Remote Write TimeSeries label sorting
	tsList := snaps[1].ToTimeSeries(now.UnixMilli())
	if len(tsList) == 0 {
		t.Fatalf("expected non-empty TimeSeries slice")
	}
	for _, ts := range tsList {
		for i := 1; i < len(ts.Labels); i++ {
			if ts.Labels[i-1].Name >= ts.Labels[i].Name {
				t.Fatalf("TimeSeries labels not strictly sorted: %v", ts.Labels)
			}
		}
	}

	// Verify Prometheus text exposition output
	var buf bytes.Buffer
	if err := eng.WritePrometheusText(&buf, now); err != nil {
		t.Fatalf("WritePrometheusText failed: %v", err)
	}
	text := buf.String()
	if !strings.Contains(text, `by_cluster="cluster-a",by_namespace="ns-high"`) {
		t.Fatalf("expected Prometheus text to contain cluster-a/ns-high series, got:\n%s", text)
	}
}
