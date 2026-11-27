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
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stolostron/multicluster-observability-operator/metrics-inspector/pkg/hll"
	"github.com/stolostron/multicluster-observability-operator/metrics-inspector/pkg/protoparser"
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

	// Verify streams without an explicit interval inherit ParseConfigWithInterval / DefaultInterval (15m).
	defaultIntervalYAML := `
streams:
  - group_by: [cluster]
`
	cfgDefault, err := ParseConfig([]byte(defaultIntervalYAML))
	if err != nil {
		t.Fatalf("unexpected ParseConfig error for omitted interval: %v", err)
	}
	if cfgDefault.Streams[0].Interval != DefaultInterval {
		t.Fatalf("expected default interval %s, got %s", DefaultInterval, cfgDefault.Streams[0].Interval)
	}

	cfgCustom, err := ParseConfigWithInterval([]byte(defaultIntervalYAML), 20*time.Minute)
	if err != nil {
		t.Fatalf("unexpected ParseConfigWithInterval error: %v", err)
	}
	if cfgCustom.Streams[0].Interval != 20*time.Minute {
		t.Fatalf("expected custom interval 20m, got %s", cfgCustom.Streams[0].Interval)
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
	rep1 := est.EvaluateAt(t0)
	if len(rep1.Groups) != 2 {
		t.Fatalf("expected 2 groups in window 1, got %d", len(rep1.Groups))
	}

	// Rotate all 4 buckets once (curr -> prev, new empty curr).
	for _, b := range est.buckets {
		b.rotate()
	}

	// Window 2: insert 100 more series (100..199) for cluster-1; nothing for cluster-2.
	est.InsertMany(makeBatch("cluster-1", 100, 100))

	rep2 := est.EvaluateAt(t0.Add(15 * time.Minute))
	// Both cluster-1 (prev 100 + curr 100 = ~200) and cluster-2 (prev 50) should still be present!
	if len(rep2.Groups) != 2 {
		t.Fatalf("expected 2 groups after 1st rotation (prev+curr), got %d", len(rep2.Groups))
	}
	for _, g := range rep2.Groups {
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
	rep3 := est.EvaluateAt(t0.Add(30 * time.Minute))
	// cluster-2 has now expired; cluster-1 retains only Window 2's 100 series.
	if len(rep3.Groups) != 1 {
		t.Fatalf("expected 1 group after 2nd rotation (cluster-2 expired), got %d", len(rep3.Groups))
	}
	if rep3.Groups[0].Values[0] != "cluster-1" || rep3.Groups[0].Estimate < 90 || rep3.Groups[0].Estimate > 110 {
		t.Fatalf("expected cluster-1 estimate ~100 after 2nd rotation, got %+v", rep3.Groups[0])
	}

	// Rotate a third time with no inserts: all groups expire.
	for _, b := range est.buckets {
		b.rotate()
	}
	rep4 := est.EvaluateAt(t0.Add(45 * time.Minute))
	if len(rep4.Groups) != 0 || rep4.GroupSize != 0 {
		t.Fatalf("expected 0 groups and GroupSize=0 after 3rd rotation, got groups=%d size=%d", len(rep4.Groups), rep4.GroupSize)
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

	rep := est.EvaluateAt(time.Now())
	if len(rep.Groups) != 10 {
		t.Fatalf("expected active groups to be capped at GroupLimit=10, got %d", len(rep.Groups))
	}
	// GroupSize includes both the 10 tracked groups and the HLL estimate of rejected groups (~40).
	if rep.GroupSize < 35 || rep.GroupSize > 65 {
		t.Fatalf("expected GroupSize (tracked + rejected HLL) ~50, got %d", rep.GroupSize)
	}

	// Rotate twice so all groups and rejected sketches age out, then insert 5 new groups.
	for _, b := range est.buckets {
		b.rotate()
		b.rotate()
	}
	est.InsertMany(tss[:5])
	repAfter := est.EvaluateAt(time.Now().Add(time.Hour))
	if len(repAfter.Groups) != 5 {
		t.Fatalf("expected 5 groups after rotation recovery, got %d", len(repAfter.Groups))
	}
	if repAfter.GroupSize != 5 {
		t.Fatalf("expected GroupSize=5 after rotation recovery, got %d", repAfter.GroupSize)
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

	rep := est.EvaluateAt(time.Now())
	byLabel := make(map[string]uint64)
	for _, g := range rep.Groups {
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
	rep := est.EvaluateAt(now)
	if rep.GlobalEstimate != 1 {
		t.Fatalf("expected GlobalEstimate=1 after filter matching, got %d", rep.GlobalEstimate)
	}

	var buf bytes.Buffer
	if err := rep.WritePrometheusText(&buf); err != nil {
		t.Fatalf("WritePrometheusText failed: %v", err)
	}
	if !strings.Contains(buf.String(), `cardinality_estimate{filter="{__name__=~\"apiserver_.*\", verb!=\"WATCH\", cluster=\"hub\"}",group_by_keys="__global__",interval="1h0m0s"} 1`) {
		t.Fatalf("unexpected Prometheus output for global filtered stream:\n%s", buf.String())
	}
}

func TestStreamReportLabelEscaping(t *testing.T) {
	rep := StreamReport{
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
	if err := rep.WritePrometheusText(&buf); err != nil {
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

	// Empty baseline or empty current window -> 0 churn (turnover requires active population in both)
	sEmpty := hll.MustNewByPrecision(14)
	defer sEmpty.Release()
	if r := CalculateChurnRatio(s1, sEmpty); r != 0 {
		t.Fatalf("expected 0 churn when previous window is empty, got %.4f", r)
	}
	if r := CalculateChurnRatio(sEmpty, s1); r != 0 {
		t.Fatalf("expected 0 churn when current window is empty, got %.4f", r)
	}
	if r := CalculateChurnRatio(sEmpty, sEmpty); r != 0 {
		t.Fatalf("expected 0 churn when both windows are empty, got %.4f", r)
	}
	if r := CalculateChurnRatio(nil, s1); r != 0 {
		t.Fatalf("expected 0 churn for nil sketch, got %.4f", r)
	}

	// Verify scratch sketch yields identical results without allocations.
	scratch := hll.MustNewByPrecision(14)
	defer scratch.Release()
	if r := CalculateChurnRatio(s1, s2, scratch); r > 0.02 {
		t.Fatalf("expected ~0 churn with scratch, got %.4f", r)
	}
	if r := CalculateChurnRatio(s1, s3, scratch); math.Abs(r-1.0) > 0.05 {
		t.Fatalf("expected ~1.0 churn with scratch, got %.4f", r)
	}
}

func TestChurnTrackerMaxEntriesAndCleanup(t *testing.T) {
	interval := 15 * time.Minute
	ct := newChurnTracker(interval, 2)
	now := time.Now()

	s := hll.MustNewByPrecision(14)
	defer s.Release()
	s.InsertHash(12345)

	scratch := hll.MustNewByPrecision(14)
	defer scratch.Release()

	// Insert key 1 & 2 (within limit 2)
	ct.updateAndRate(1, s, scratch, now)
	ct.updateAndRate(2, s, scratch, now)

	ct.mu.Lock()
	if len(ct.entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(ct.entries))
	}
	ct.mu.Unlock()

	// Insert key 3 (exceeds maxEntries = 2) -> rejected
	rate := ct.updateAndRate(3, s, scratch, now)
	if rate != 0 {
		t.Fatalf("expected 0 churn for rejected key 3, got %f", rate)
	}
	ct.mu.Lock()
	if len(ct.entries) != 2 {
		t.Fatalf("expected still 2 entries after rejection, got %d", len(ct.entries))
	}
	ct.mu.Unlock()

	// Advance time past interval -> cleanup should evict entries
	ct.cleanup(now.Add(interval + time.Second))
	ct.mu.Lock()
	if len(ct.entries) != 0 {
		t.Fatalf("expected 0 entries after cleanup past interval, got %d", len(ct.entries))
	}
	ct.mu.Unlock()

	// Repopulate and test releaseAll
	ct.updateAndRate(10, s, scratch, now)
	ct.updateAndRate(20, s, scratch, now)
	ct.releaseAll()
	ct.mu.Lock()
	if len(ct.entries) != 0 {
		t.Fatalf("expected 0 entries after releaseAll, got %d", len(ct.entries))
	}
	ct.mu.Unlock()
}

func TestEngineIngestAndEvaluate(t *testing.T) {
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
	reports := eng.Evaluate(now)
	if len(reports) != 2 {
		t.Fatalf("expected 2 reports, got %d", len(reports))
	}

	// Stream 0: [cluster] -> 1 group ("cluster-a") with ~55 series
	if len(reports[0].Groups) != 1 {
		t.Fatalf("expected 1 cluster group, got %d", len(reports[0].Groups))
	}
	if reports[0].Groups[0].Estimate < 52 || reports[0].Groups[0].Estimate > 58 {
		t.Fatalf("expected cluster-a estimate ~55, got %d", reports[0].Groups[0].Estimate)
	}

	// Stream 1: [cluster, namespace] -> ns-low dropped by min_cardinality=10, only ns-high emitted
	if len(reports[1].Groups) != 1 {
		t.Fatalf("expected 1 namespace group after min_cardinality filter, got %d", len(reports[1].Groups))
	}
	if reports[1].DroppedCount != 1 {
		t.Fatalf("expected DroppedCount=1, got %d", reports[1].DroppedCount)
	}
	if !slices.Equal(reports[1].Groups[0].Values, []string{"cluster-a", "ns-high"}) {
		t.Fatalf("unexpected group values: %v", reports[1].Groups[0].Values)
	}

	// Verify Remote Write TimeSeries label sorting
	tsList := reports[1].ToTimeSeries(now.UnixMilli())
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

func TestParseConfigWithDefaults(t *testing.T) {
	yamlWithoutMaxItems := []byte(`
streams:
  - group_by: [cluster]
`)
	cfg, err := ParseConfigWithDefaults(yamlWithoutMaxItems, DefaultInterval, 50_000_000)
	if err != nil {
		t.Fatalf("ParseConfigWithDefaults failed: %v", err)
	}
	if cfg.MaxItemsPerInterval != 50_000_000 {
		t.Fatalf("expected fallback MaxItemsPerInterval=50000000, got %d", cfg.MaxItemsPerInterval)
	}

	yamlWithExplicitMaxItems := []byte(`
max_items_per_interval: 20000000
streams:
  - group_by: [cluster]
`)
	cfgExplicit, err := ParseConfigWithDefaults(yamlWithExplicitMaxItems, DefaultInterval, 50_000_000)
	if err != nil {
		t.Fatalf("ParseConfigWithDefaults failed: %v", err)
	}
	if cfgExplicit.MaxItemsPerInterval != 20_000_000 {
		t.Fatalf("expected explicit MaxItemsPerInterval=20000000 to take precedence, got %d", cfgExplicit.MaxItemsPerInterval)
	}
}

func TestEngineContinuousIngestNoSawtoothDrop(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
max_items_per_interval: 100000
streams:
  - interval: 15m
    group_by: [cluster]
    hll_precision: 14
    calculate_churn: false
    group_limit: 100
    min_cardinality: 0
`))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}
	eng, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer eng.Stop()

	const totalSeries = 5400
	batch := make([]protoparser.TimeSeries, totalSeries)
	for i := range totalSeries {
		name := fmt.Sprintf("metric_%d", i)
		batch[i] = protoparser.TimeSeries{
			Hash: xxhash.Sum64([]byte("local-cluster|" + name)),
			Labels: []prompb.Label{
				{Name: "__name__", Value: name},
				{Name: "cluster", Value: "local-cluster"},
			},
		}
	}

	p := eng.pipelines[0]
	est := p.estimators[0]

	// Simulate 45 minutes of 30s scrapes (90 steps of 30s), advancing both the
	// Deduplicator shard rotations (every 22.5s -> 1 shard per 22.5s) and Estimator
	// bucket rotations (every 30s -> 1 bucket per 30s, so each bucket rotates every 7.5m).
	var (
		dedupShardIdx int
		estBucketIdx  int
		nextDedupRot  = 22500 * time.Millisecond // 15m / (4 * 10) = 22.5s
		nextEstRot    = 30 * time.Second         // 15m / (2 * 15) = 30s
	)

	now := time.Now()
	for step := range 90 {
		elapsed := time.Duration(step) * 30 * time.Second

		for nextDedupRot <= elapsed {
			p.dedup.resetShard(dedupShardIdx)
			dedupShardIdx = (dedupShardIdx + 1) % dedupBucketsCount
			nextDedupRot += 22500 * time.Millisecond
		}
		for nextEstRot <= elapsed {
			est.buckets[estBucketIdx].rotate()
			estBucketIdx = (estBucketIdx + 1) % len(est.buckets)
			nextEstRot += 30 * time.Second
		}

		eng.Ingest(batch)

		reports := eng.Evaluate(now.Add(elapsed))
		if len(reports) != 1 || len(reports[0].Groups) != 1 {
			t.Fatalf("step %d (%s): expected 1 cluster group, got %+v", step, elapsed, reports)
		}
		got := reports[0].Groups[0].Estimate
		// With p=14 (~0.8% standard error), 5400 series should stay tightly within [5250, 5550]
		// across all rotations and never sawtooth down to ~2700.
		if got < 5250 || got > 5550 {
			t.Fatalf("step %d (%s): expected steady cardinality ~5400, got %d", step, elapsed, got)
		}
	}
}

func TestChurnExcludeRegexValidation(t *testing.T) {
	yamlInvalid := `
churn_exclude_regex: "["
streams:
  - group_by: [cluster]
`
	_, err := ParseConfig([]byte(yamlInvalid))
	if err == nil {
		t.Fatalf("expected error parsing invalid churn_exclude_regex, got nil")
	}
	if !strings.Contains(err.Error(), "invalid churn_exclude_regex") {
		t.Fatalf("expected error to mention 'invalid churn_exclude_regex', got: %v", err)
	}

	cfg := Config{
		ChurnExcludeRegex: `*invalid`,
		Streams: []StreamConfig{
			{GroupBy: []string{"cluster"}},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected Validate() to fail on invalid regex, got nil")
	}

	yamlInvalidStream := `
churn_exclude_regex: "^valid_prefix_"
streams:
  - group_by: [cluster]
    churn_exclude_regex: "["
`
	_, err = ParseConfig([]byte(yamlInvalidStream))
	if err == nil {
		t.Fatalf("expected error parsing invalid per-stream churn_exclude_regex, got nil")
	}
	if !strings.Contains(err.Error(), "invalid stream[0]") || !strings.Contains(err.Error(), "invalid churn_exclude_regex") {
		t.Fatalf("expected error to mention 'invalid stream[0]' and 'invalid churn_exclude_regex', got: %v", err)
	}
}

func TestChurnExcludeRegexPrecedence(t *testing.T) {
	yamlWithout := []byte(`
streams:
  - group_by: [cluster]
`)
	cfgFallback, err := ParseConfigWithAllDefaults(yamlWithout, ConfigDefaults{
		ChurnExcludeRegex: "^acm_rs[:_]",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfgFallback.ChurnExcludeRegex != "^acm_rs[:_]" {
		t.Fatalf("expected ChurnExcludeRegex to fall back to CLI default, got %q", cfgFallback.ChurnExcludeRegex)
	}
	if len(cfgFallback.Streams) != 1 || cfgFallback.Streams[0].ChurnExcludeRe() == nil {
		t.Fatalf("expected StreamConfig to have compiled regex from default")
	}
	if !cfgFallback.Streams[0].ChurnExcludeRe().MatchString("acm_rs:test") {
		t.Fatalf("expected compiled regex to match 'acm_rs:test'")
	}

	yamlWith := []byte(`
churn_exclude_regex: "^custom_prefix_"
streams:
  - group_by: [cluster]
`)
	cfgExplicit, err := ParseConfigWithAllDefaults(yamlWith, ConfigDefaults{
		ChurnExcludeRegex: "^acm_rs[:_]",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfgExplicit.ChurnExcludeRegex != "^custom_prefix_" {
		t.Fatalf("expected explicit YAML churn_exclude_regex to take precedence, got %q", cfgExplicit.ChurnExcludeRegex)
	}
	if len(cfgExplicit.Streams) != 1 || cfgExplicit.Streams[0].ChurnExcludeRe() == nil {
		t.Fatalf("expected StreamConfig to have compiled regex from YAML")
	}
	if !cfgExplicit.Streams[0].ChurnExcludeRe().MatchString("custom_prefix_test") {
		t.Fatalf("expected compiled regex to match 'custom_prefix_test'")
	}
	if cfgExplicit.Streams[0].ChurnExcludeRe().MatchString("acm_rs:test") {
		t.Fatalf("expected CLI default to be overridden and not match 'acm_rs:test'")
	}

	yamlStreamOverride := []byte(`
churn_exclude_regex: "^top_level_"
streams:
  - group_by: [cluster]
  - group_by: [cluster, __name__]
    churn_exclude_regex: "^stream_override_"
`)
	cfgStreamOverride, err := ParseConfigWithAllDefaults(yamlStreamOverride, ConfigDefaults{
		ChurnExcludeRegex: "^acm_rs[:_]",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfgStreamOverride.Streams) != 2 {
		t.Fatalf("expected 2 streams, got %d", len(cfgStreamOverride.Streams))
	}
	s0Re := cfgStreamOverride.Streams[0].ChurnExcludeRe()
	if s0Re == nil || !s0Re.MatchString("top_level_metric") || s0Re.MatchString("stream_override_metric") {
		t.Fatalf("stream 0 should match top_level_metric and not stream_override_metric")
	}
	s1Re := cfgStreamOverride.Streams[1].ChurnExcludeRe()
	if s1Re == nil || !s1Re.MatchString("stream_override_metric") || s1Re.MatchString("top_level_metric") {
		t.Fatalf("stream 1 should match stream_override_metric and not top_level_metric")
	}
}

func TestChurnExcludeRegexEvaluation(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
max_items_per_interval: 100000
churn_exclude_regex: "^acm_rs[:_]"
streams:
  - interval: 15m
    group_by: [__name__]
    hll_precision: 12
    calculate_churn: true
  - interval: 15m
    group_by: [cluster, __name__]
    hll_precision: 12
    calculate_churn: true
  - interval: 15m
    group_by: [cluster]
    hll_precision: 12
    calculate_churn: true
`))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	eng, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer eng.Stop()

	// Ingest 3 distinct metrics
	// 1. acm_rs:namespace:cpu_recommendation (matches ^acm_rs[:_])
	// 2. acm_rs_vm:namespace:cpu_usage (matches ^acm_rs[:_])
	// 3. container_cpu_working_set_bytes (does not match)
	batch := []protoparser.TimeSeries{
		{
			Hash: xxhash.Sum64([]byte("local-cluster|acm_rs:namespace:cpu_recommendation")),
			Labels: []prompb.Label{
				{Name: "__name__", Value: "acm_rs:namespace:cpu_recommendation"},
				{Name: "cluster", Value: "local-cluster"},
			},
		},
		{
			Hash: xxhash.Sum64([]byte("local-cluster|acm_rs_vm:namespace:cpu_usage")),
			Labels: []prompb.Label{
				{Name: "__name__", Value: "acm_rs_vm:namespace:cpu_usage"},
				{Name: "cluster", Value: "local-cluster"},
			},
		},
		{
			Hash: xxhash.Sum64([]byte("local-cluster|container_cpu_working_set_bytes")),
			Labels: []prompb.Label{
				{Name: "__name__", Value: "container_cpu_working_set_bytes"},
				{Name: "cluster", Value: "local-cluster"},
			},
		},
	}

	now := time.Now()
	eng.Ingest(batch)

	// First evaluation creates baseline windows
	reports := eng.Evaluate(now)
	if len(reports) != 3 {
		t.Fatalf("expected 3 reports, got %d", len(reports))
	}

	// Advance time to populate churn calculation
	now2 := now.Add(15 * time.Minute)
	batch2 := []protoparser.TimeSeries{
		{
			Hash: xxhash.Sum64([]byte("local-cluster|acm_rs:namespace:cpu_recommendation|v2")),
			Labels: []prompb.Label{
				{Name: "__name__", Value: "acm_rs:namespace:cpu_recommendation"},
				{Name: "cluster", Value: "local-cluster"},
				{Name: "pod", Value: "pod-2"},
			},
		},
		{
			Hash: xxhash.Sum64([]byte("local-cluster|container_cpu_working_set_bytes|v2")),
			Labels: []prompb.Label{
				{Name: "__name__", Value: "container_cpu_working_set_bytes"},
				{Name: "cluster", Value: "local-cluster"},
				{Name: "pod", Value: "pod-2"},
			},
		},
	}
	eng.Ingest(batch2)
	reports2 := eng.Evaluate(now2)

	// Check Stream 0: [__name__]
	r0 := reports2[0]
	if len(r0.Groups) != 3 {
		t.Fatalf("stream 0 [__name__]: expected 3 groups, got %d", len(r0.Groups))
	}
	for _, g := range r0.Groups {
		name := g.Values[0]
		if g.Estimate == 0 {
			t.Fatalf("expected non-zero cardinality estimate for %s", name)
		}
		if strings.HasPrefix(name, "acm_rs:") || strings.HasPrefix(name, "acm_rs_vm:") {
			if !g.SkipChurn {
				t.Fatalf("expected SkipChurn=true for %s", name)
			}
		} else if name == "container_cpu_working_set_bytes" {
			if g.SkipChurn {
				t.Fatalf("expected SkipChurn=false for %s", name)
			}
		}
	}
	s0 := r0.collectSeries()
	var churnNames0 []string
	for _, pt := range s0 {
		if pt.labels.Get(labels.MetricName) == MetricCardinalityChurn {
			churnNames0 = append(churnNames0, pt.labels.Get("by__name__"))
		}
	}
	if len(churnNames0) != 1 || churnNames0[0] != "container_cpu_working_set_bytes" {
		t.Fatalf("stream 0: expected churn emitted only for container_cpu_working_set_bytes, got %v", churnNames0)
	}

	// Verify internal churnTracker for Stream 0 only contains non-excluded metrics
	checkTracker := func() {
		ct0 := eng.pipelines[0].estimators[0].churn
		ct0.mu.Lock()
		defer ct0.mu.Unlock()
		if len(ct0.entries) != 1 {
			t.Fatalf("stream 0 churnTracker: expected exactly 1 entry, got %d", len(ct0.entries))
		}
		expectedKey := xxhash.Sum64([]byte("\x00container_cpu_working_set_bytes"))
		if _, ok := ct0.entries[expectedKey]; !ok {
			t.Fatalf("stream 0 churnTracker: expected entry for container_cpu_working_set_bytes (key %d)", expectedKey)
		}
		excludedKey1 := xxhash.Sum64([]byte("\x00acm_rs:namespace:cpu_recommendation"))
		if _, ok := ct0.entries[excludedKey1]; ok {
			t.Fatalf("stream 0 churnTracker: unexpected entry for acm_rs:namespace:cpu_recommendation")
		}
		excludedKey2 := xxhash.Sum64([]byte("\x00acm_rs_vm:namespace:cpu_usage"))
		if _, ok := ct0.entries[excludedKey2]; ok {
			t.Fatalf("stream 0 churnTracker: unexpected entry for acm_rs_vm:namespace:cpu_usage")
		}
	}
	checkTracker()

	// Check Stream 1: [cluster, __name__]
	r1 := reports2[1]
	for _, g := range r1.Groups {
		name := g.Values[1]
		if g.Estimate == 0 {
			t.Fatalf("expected non-zero cardinality estimate for %v", g.Values)
		}
		if strings.HasPrefix(name, "acm_rs:") || strings.HasPrefix(name, "acm_rs_vm:") {
			if !g.SkipChurn {
				t.Fatalf("expected SkipChurn=true for %v", g.Values)
			}
		} else if name == "container_cpu_working_set_bytes" {
			if g.SkipChurn {
				t.Fatalf("expected SkipChurn=false for %v", g.Values)
			}
		}
	}
	s1 := r1.collectSeries()
	var churnNames1 []string
	for _, pt := range s1 {
		if pt.labels.Get(labels.MetricName) == MetricCardinalityChurn {
			churnNames1 = append(churnNames1, pt.labels.Get("by__name__"))
		}
	}
	if len(churnNames1) != 1 || churnNames1[0] != "container_cpu_working_set_bytes" {
		t.Fatalf("stream 1: expected churn emitted only for container_cpu_working_set_bytes, got %v", churnNames1)
	}

	// Check Stream 2: [cluster] (not grouped by __name__)
	r2 := reports2[2]
	if len(r2.Groups) != 1 {
		t.Fatalf("stream 2 [cluster]: expected 1 group, got %d", len(r2.Groups))
	}
	if r2.Groups[0].SkipChurn {
		t.Fatalf("stream 2 [cluster]: expected SkipChurn=false")
	}
	s2 := r2.collectSeries()
	hasClusterChurn := false
	for _, pt := range s2 {
		if pt.labels.Get(labels.MetricName) == MetricCardinalityChurn {
			hasClusterChurn = true
			if pt.labels.Get("by_cluster") != "local-cluster" {
				t.Fatalf("stream 2: unexpected by_cluster label: %s", pt.labels.Get("by_cluster"))
			}
		}
	}
	if !hasClusterChurn {
		t.Fatalf("stream 2 [cluster]: expected cardinality_churn_ratio to be emitted")
	}
}
