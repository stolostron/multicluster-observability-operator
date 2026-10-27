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
