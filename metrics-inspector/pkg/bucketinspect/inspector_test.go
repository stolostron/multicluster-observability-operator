// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package bucketinspect

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thanos-io/objstore"
)

type iterCallRecord struct {
	dir     string
	options []objstore.IterOption
}

type spyBucketReader struct {
	*objstore.InMemBucket
	mu          sync.Mutex
	iterErr     error
	getCalls    map[string]int
	existsCalls map[string]int
	iterCalls   []iterCallRecord
}

func newSpyBucketReader() *spyBucketReader {
	return &spyBucketReader{
		InMemBucket: objstore.NewInMemBucket(),
		getCalls:    make(map[string]int),
		existsCalls: make(map[string]int),
	}
}

func (s *spyBucketReader) put(key, content string) {
	if err := s.Upload(context.Background(), key, strings.NewReader(content)); err != nil {
		panic(err)
	}
}

func (s *spyBucketReader) remove(key string) {
	if err := s.Delete(context.Background(), key); err != nil {
		panic(err)
	}
}

func (s *spyBucketReader) Iter(ctx context.Context, dir string, f func(string) error, options ...objstore.IterOption) error {
	s.mu.Lock()
	err := s.iterErr
	s.iterCalls = append(s.iterCalls, iterCallRecord{dir: dir, options: options})
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return s.InMemBucket.Iter(ctx, dir, f, options...)
}

func (s *spyBucketReader) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	s.mu.Lock()
	s.getCalls[name]++
	s.mu.Unlock()
	return s.InMemBucket.Get(ctx, name)
}

func (s *spyBucketReader) Exists(ctx context.Context, name string) (bool, error) {
	s.mu.Lock()
	s.existsCalls[name]++
	s.mu.Unlock()
	return s.InMemBucket.Exists(ctx, name)
}

func TestParseDurationSeconds(t *testing.T) {
	tests := []struct {
		in      string
		want    float64
		wantErr bool
	}{
		{in: "365d", want: 365 * 86400},
		{in: "14d", want: 14 * 86400},
		{in: "24h", want: 86400},
		{in: "0s", want: 0},
		{in: "0d", want: 0},
		{in: "", want: 0},
		{in: "invalid", wantErr: true},
	}

	for _, tc := range tests {
		got, err := ParseDurationSeconds(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseDurationSeconds(%q) expected error, got nil", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseDurationSeconds(%q) unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseDurationSeconds(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestNewBucketValidation(t *testing.T) {
	tmpDir := t.TempDir()
	validFS := fmt.Sprintf(`
type: FILESYSTEM
config:
  directory: %s
`, tmpDir)
	bkt, err := NewBucket([]byte(validFS))
	if err != nil {
		t.Fatalf("expected valid FILESYSTEM bucket config, got: %v", err)
	}
	_ = bkt.Close()

	if _, err := NewBucket([]byte(`type: S3`)); err == nil {
		t.Errorf("expected error for S3 config without bucket")
	}
	if _, err := NewBucket([]byte(`type: UNKNOWN`)); err == nil {
		t.Errorf("expected error for unknown provider type")
	}
}

func TestInspectorIncrementalCacheAndMetrics(t *testing.T) {
	ctx := context.Background()
	reader := newSpyBucketReader()

	now := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	nowMs := now.UnixMilli()

	// Block 1: Active receive raw block (2h duration, 1 day ago) - Compacted level 3 with higher series union (9999)
	ulid1 := "01K6B9RVTWK2B1VSQ3R34BMHP7"
	min1 := nowMs - 24*3600*1000
	max1 := min1 + 2*3600*1000
	meta1 := fmt.Sprintf(`{
		"ulid": %q,
		"minTime": %d,
		"maxTime": %d,
		"stats": {"numSamples": 100000, "numSeries": 9999, "numChunks": 2000},
		"compaction": {"level": 3},
		"thanos": {
			"labels": {"receive": "true", "tenant_id": "tenant-a"},
			"downsample": {"resolution": 0},
			"source": "compactor",
			"files": [
				{"rel_path": "chunks/000001", "size_bytes": 50000},
				{"rel_path": "index", "size_bytes": 10000},
				{"rel_path": "meta.json", "size_bytes": 500}
			]
		}
	}`, ulid1, min1, max1)

	// Block 2: Overlapping HA replica receive raw block covering the exact same [min1, max1] window!
	ulid2 := "01K6B9S9BM9FDJQP4H6B20V05N"
	meta2 := fmt.Sprintf(`{
		"ulid": %q,
		"minTime": %d,
		"maxTime": %d,
		"stats": {"numSamples": 100000, "numSeries": 1250, "numChunks": 2000},
		"compaction": {"level": 1},
		"thanos": {
			"labels": {"receive_replica": "receive-1", "tenant_id": "tenant-a"},
			"downsample": {"resolution": 0},
			"source": "receive",
			"files": [
				{"rel_path": "chunks/000001", "size_bytes": 50000},
				{"rel_path": "index", "size_bytes": 10000},
				{"rel_path": "meta.json", "size_bytes": 500}
			]
		}
	}`, ulid2, min1, max1)

	// Block 3: 5m downsampled ruler block (20 days ago, 10h duration)
	ulid3 := "01K6GWHVWRJWMP7DFXPGKBWG19"
	min3 := nowMs - 20*24*3600*1000
	max3 := min3 + 10*3600*1000
	meta3 := fmt.Sprintf(`{
		"ulid": %q,
		"minTime": %d,
		"maxTime": %d,
		"stats": {"numSamples": 5000, "numSeries": 80, "numChunks": 100},
		"compaction": {"level": 2},
		"thanos": {
			"labels": {"rule_replica": "thanos-rule-0", "tenant_id": "tenant-a"},
			"downsample": {"resolution": 300000},
			"source": "compactor",
			"files": [
				{"rel_path": "chunks/000001", "size_bytes": 4000},
				{"rel_path": "index", "size_bytes": 1000}
			]
		}
	}`, ulid3, min3, max3)

	reader.put(ulid1+"/meta.json", meta1)
	reader.put(ulid2+"/meta.json", meta2)
	reader.put(ulid3+"/meta.json", meta3)

	ins := NewInspector(reader, InspectorConfig{
		SyncInterval: 15 * time.Minute,
		Retention: RetentionConfig{
			RawSeconds: 14 * 86400,
			FiveMinSec: 180 * 86400,
			OneHourSec: 365 * 86400,
		},
	})

	// First Sync: should fetch all 3 blocks.
	if err := ins.Sync(ctx); err != nil {
		t.Fatalf("first Sync failed: %v", err)
	}
	if ins.CachedBlocks() != 3 {
		t.Fatalf("CachedBlocks = %d, want 3", ins.CachedBlocks())
	}
	for _, u := range []string{ulid1, ulid2, ulid3} {
		if got := reader.getCalls[u+"/meta.json"]; got != 1 {
			t.Errorf("expected 1 Get call for %s, got %d", u, got)
		}
	}

	var buf bytes.Buffer
	ins.WritePrometheusText(&buf, now)
	out := buf.String()

	// Verify pruned diagnostic primitives are NOT emitted.
	for _, unwanted := range []string{"thanos_bucket_spanned_seconds", "thanos_bucket_samples", "thanos_bucket_chunks", "thanos_bucket_recent_block_series", "thanos_bucket_downsample_storage_ratio"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("did not expect metric %q in output, but found it", unwanted)
		}
	}

	// Verify total storage sums both blocks (60500 + 60500 = 121000).
	wantStorage := `thanos_bucket_storage_bytes{age_tier="0_to_7d",marked_for_deletion="false",producer="receive",resolution="raw",tenant_id="tenant-a"} 121000`
	if !strings.Contains(out, wantStorage) {
		t.Errorf("expected storage 121000, got output:\n%s", out)
	}

	// Verify configured retention metric (14d = 1209600s = 1.2096e+06 in Prometheus text format).
	if !strings.Contains(out, `thanos_bucket_configured_retention_seconds{resolution="raw"} 1.2096e+06`) {
		t.Errorf("expected raw retention 1.2096e+06s, got output:\n%s", out)
	}

	// Second Sync:
	// - Mark ulid2 for deletion via deletion-mark.json
	// - Remove ulid3 completely
	// - Add new ulid4
	reader.put(ulid2+"/deletion-mark.json", `{"id": "`+ulid2+`"}`)
	reader.remove(ulid3 + "/meta.json")

	ulid4 := "01K6SH3C9W5S7Z1N69B423WDTC"
	min4 := max1 // Contiguous after [min1, max1] for another 2h
	max4 := min4 + 2*3600*1000
	meta4 := fmt.Sprintf(`{
		"ulid": %q,
		"minTime": %d,
		"maxTime": %d,
		"stats": {"numSamples": 80000, "numSeries": 1300, "numChunks": 1600},
		"compaction": {"level": 1},
		"thanos": {
			"labels": {"receive": "true", "tenant_id": "tenant-a"},
			"downsample": {"resolution": 0},
			"source": "receive",
			"files": [
				{"rel_path": "chunks/000001", "size_bytes": 40000},
				{"rel_path": "index", "size_bytes": 8000},
				{"rel_path": "meta.json", "size_bytes": 500}
			]
		}
	}`, ulid4, min4, max4)
	reader.put(ulid4+"/meta.json", meta4)

	if err := ins.Sync(ctx); err != nil {
		t.Fatalf("second Sync failed: %v", err)
	}

	// Verify ulid1 and ulid2 were NOT re-downloaded (incremental cache hit!).
	if got := reader.getCalls[ulid1+"/meta.json"]; got != 1 {
		t.Errorf("expected ulid1 meta.json Get count to remain 1, got %d", got)
	}
	if got := reader.getCalls[ulid2+"/meta.json"]; got != 1 {
		t.Errorf("expected ulid2 meta.json Get count to remain 1, got %d", got)
	}
	if got := reader.getCalls[ulid4+"/meta.json"]; got != 1 {
		t.Errorf("expected ulid4 meta.json Get count to be 1, got %d", got)
	}

	buf.Reset()
	ins.WritePrometheusText(&buf, now)
	out2 := buf.String()

	// ulid2 is now marked_for_deletion="true", while ulid1 + ulid4 are active and contiguous.
	// ulid2 is marked for deletion: 50000 chunks + 10000 index + 500 meta = 60500 bytes.
	wantDelStorage := `thanos_bucket_storage_bytes{age_tier="0_to_7d",marked_for_deletion="true",producer="receive",resolution="raw",tenant_id="tenant-a"} 60500`
	if !strings.Contains(out2, wantDelStorage) {
		t.Errorf("expected marked_for_deletion=true storage bytes 60500, got:\n%s", out2)
	}

	// Third Sync: Simulate transient object store failure; verify error counter increments and cache is preserved.
	reader.mu.Lock()
	reader.iterErr = errors.New("connection reset by peer")
	reader.mu.Unlock()
	if err := ins.Sync(ctx); err == nil {
		t.Fatalf("expected Sync to return error when Iter fails")
	}
	if ins.CachedBlocks() != 3 {
		t.Errorf("expected cached blocks to remain 3 on transient Iter failure, got %d", ins.CachedBlocks())
	}
	buf.Reset()
	ins.WritePrometheusText(&buf, now)
	if !strings.Contains(buf.String(), "thanos_bucket_sync_errors_total 1") {
		t.Errorf("expected thanos_bucket_sync_errors_total 1, got:\n%s", buf.String())
	}
}

func TestAccumulateStreamIntervalsByTierCrossBoundary(t *testing.T) {
	resKey := resolutionGroupKey{tenantID: "default", producer: "receive", resolution: "raw"}
	// Block A in 7d_to_30d covers [100_000, 300_000] (200s).
	// Block B in 0_to_7d overlaps Block A and covers [200_000, 500_000] (300s, 100s overlap).
	intervals := []timeInterval{
		{minMs: 200_000, maxMs: 500_000, ageTier: ageTier0To7d},
		{minMs: 100_000, maxMs: 300_000, ageTier: ageTier7dTo30d},
	}
	out := make(map[tierGroupKey]float64)
	accumulateStreamIntervalsByTier(resKey, intervals, out)

	got7dTo30d := out[tierGroupKey{resKey.tenantID, resKey.producer, resKey.resolution, ageTier7dTo30d}]
	got0To7d := out[tierGroupKey{resKey.tenantID, resKey.producer, resKey.resolution, ageTier0To7d}]
	if got7dTo30d != 200 || got0To7d != 200 {
		t.Fatalf("expected 200s in 7d_to_30d and 200s (deduplicated) in 0_to_7d, got %v and %v", got7dTo30d, got0To7d)
	}
}

func TestFilesystemBucketIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	ulidStr := "01K6B9RVTWK2B1VSQ3R34BMHP7"
	blockDir := filepath.Join(tmpDir, "subprefix", ulidStr)
	if err := os.MkdirAll(filepath.Join(blockDir, "chunks"), 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	metaPayload := fmt.Sprintf(`{
		"ulid": %q,
		"minTime": 1000,
		"maxTime": 2000,
		"version": 1,
		"thanos": {
			"labels": {"receive": "true"},
			"downsample": {"resolution": 0}
		}
	}`, ulidStr)
	if err := os.WriteFile(filepath.Join(blockDir, "meta.json"), []byte(metaPayload), 0o600); err != nil {
		t.Fatalf("WriteFile meta.json: %v", err)
	}
	deletionMarkPayload := `{"id":"01K6B9RVTWK2B1VSQ3R34BMHP7","deletion_time":1700000000}`
	if err := os.WriteFile(filepath.Join(blockDir, "deletion-mark.json"), []byte(deletionMarkPayload), 0o600); err != nil {
		t.Fatalf("WriteFile deletion-mark.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(blockDir, "chunks", "000001"), []byte("0123456789"), 0o600); err != nil {
		t.Fatalf("WriteFile chunk: %v", err)
	}

	bkt, err := NewBucket([]byte(fmt.Sprintf(`
type: FILESYSTEM
prefix: subprefix
config:
  directory: %s
`, tmpDir)))
	if err != nil {
		t.Fatalf("NewBucket: %v", err)
	}
	defer func() { _ = bkt.Close() }()

	ins := NewInspector(bkt, InspectorConfig{})
	if err := ins.Sync(context.Background()); err != nil {
		t.Fatalf("Sync with upstream filesystem bucket failed: %v", err)
	}
	if ins.CachedBlocks() != 1 {
		t.Fatalf("CachedBlocks = %d, want 1", ins.CachedBlocks())
	}
	ins.mu.RLock()
	blk := ins.blocks[ulidStr]
	ins.mu.RUnlock()
	wantStorageBytes := int64(len(metaPayload) + len(deletionMarkPayload) + 10)
	if blk.StorageBytes != wantStorageBytes {
		t.Errorf("StorageBytes = %d, want %d (meta.json + deletion-mark.json + chunk)", blk.StorageBytes, wantStorageBytes)
	}
}

func TestInspectorStartupRetryAndLatch(t *testing.T) {
	spy := newSpyBucketReader()
	spy.mu.Lock()
	spy.iterErr = errors.New("s3 temporarily unreachable")
	spy.mu.Unlock()

	ins := NewInspector(spy, InspectorConfig{
		SyncInterval:      time.Hour,
		InitRetryInterval: 15 * time.Millisecond,
	})
	if ok, _ := ins.Ready(); ok {
		t.Fatal("expected Ready()=false before initial sync")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		ins.Run(ctx)
	}()

	// Wait for at least one failed sync attempt so Ready() records the S3 error.
	deadline := time.Now().Add(500 * time.Millisecond)
	for ins.syncErrorsTotal.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if ok, reason := ins.Ready(); ok || !strings.Contains(reason, "s3 temporarily unreachable") {
		cancel()
		<-done
		t.Fatalf("expected Ready()=false with S3 error, got ok=%v reason=%q", ok, reason)
	}

	// Clear the error and verify Run() retries with InitRetryInterval and latches Ready()=true.
	spy.mu.Lock()
	spy.iterErr = nil
	spy.mu.Unlock()

	for time.Now().Before(deadline) {
		if ok, _ := ins.Ready(); ok {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done

	if ok, reason := ins.Ready(); !ok {
		t.Fatalf("expected Ready()=true after retry succeeded, got false (%s)", reason)
	}

	// Verify Ready() stays latched true even if a subsequent Sync fails.
	spy.mu.Lock()
	spy.iterErr = errors.New("transient s3 503")
	spy.mu.Unlock()
	if err := ins.Sync(context.Background()); err == nil {
		t.Fatal("expected subsequent Sync to return error")
	}
	if ok, _ := ins.Ready(); !ok {
		t.Fatal("expected Ready() to remain latched true after transient subsequent sync failure")
	}
}

func TestBurnRateAndDownsampleStorageRatios(t *testing.T) {
	ctx := context.Background()
	reader := newSpyBucketReader()

	now := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	nowMs := now.UnixMilli()

	// Block 0: Uncompacted raw block in the first 24 hours (midpoint age ~1h, 216000 bytes due to 3x replica expansion).
	// This block MUST BE IGNORED by the active burn rate calculation.
	ulidRawUncompacted := "01K6B9RVTWK2B1VSQ3R34BMHP6"
	minRawUncompacted := nowMs - 2*3600*1000
	maxRawUncompacted := minRawUncompacted + 2*3600*1000
	metaRawUncompacted := fmt.Sprintf(`{
		"ulid": %q,
		"minTime": %d,
		"maxTime": %d,
		"stats": {"numSamples": 30000, "numSeries": 300, "numChunks": 600},
		"compaction": {"level": 1},
		"thanos": {
			"labels": {"receive": "true", "tenant_id": "tenant-a"},
			"downsample": {"resolution": 0},
			"source": "receive",
			"files": [{"rel_path": "chunks/000001", "size_bytes": 215500}, {"rel_path": "meta.json", "size_bytes": 500}]
		}
	}`, ulidRawUncompacted, minRawUncompacted, maxRawUncompacted)

	// Block 1: Active raw block in post-compaction window [24h, 7d) (midpoint age ~47h, 2h span = 7200s, 72000 bytes => raw burn rate = 10 B/s)
	ulidRawRecent := "01K6B9RVTWK2B1VSQ3R34BMHP7"
	minRawRecent := nowMs - 48*3600*1000
	maxRawRecent := minRawRecent + 2*3600*1000
	metaRawRecent := fmt.Sprintf(`{
		"ulid": %q,
		"minTime": %d,
		"maxTime": %d,
		"stats": {"numSamples": 10000, "numSeries": 100, "numChunks": 200},
		"compaction": {"level": 2},
		"thanos": {
			"labels": {"receive": "true", "tenant_id": "tenant-a"},
			"downsample": {"resolution": 0},
			"source": "receive",
			"files": [{"rel_path": "chunks/000001", "size_bytes": 71500}, {"rel_path": "meta.json", "size_bytes": 500}]
		}
	}`, ulidRawRecent, minRawRecent, maxRawRecent)

	// Block 2: Historical raw block in 7d_to_30d tier (2h span = 7200s, 10000 bytes => rate = 10000/7200 B/s)
	ulidRawHist := "01K6B9S9BM9FDJQP4H6B20V05N"
	minHist := nowMs - 15*24*3600*1000
	maxHist := minHist + 2*3600*1000
	metaRawHist := fmt.Sprintf(`{
		"ulid": %q,
		"minTime": %d,
		"maxTime": %d,
		"stats": {"numSamples": 2000, "numSeries": 50, "numChunks": 50},
		"compaction": {"level": 2},
		"thanos": {
			"labels": {"receive": "true", "tenant_id": "tenant-a"},
			"downsample": {"resolution": 0},
			"source": "receive",
			"files": [{"rel_path": "chunks/000001", "size_bytes": 9500}, {"rel_path": "meta.json", "size_bytes": 500}]
		}
	}`, ulidRawHist, minHist, maxHist)

	// Block 3: Matching 5m downsampled block in 7d_to_30d tier covering exact same [minHist, maxHist] window!
	// Size is 18000 bytes => rate = 18000/7200 B/s.
	// Empirical downsample expansion ratio = 18000 / 10000 = 1.8! (simulating 5m scrape interval expansion)
	ulid5mHist := "01K6GWHVWRJWMP7DFXPGKBWG19"
	meta5mHist := fmt.Sprintf(`{
		"ulid": %q,
		"minTime": %d,
		"maxTime": %d,
		"stats": {"numSamples": 10000, "numSeries": 50, "numChunks": 100},
		"compaction": {"level": 2},
		"thanos": {
			"labels": {"receive": "true", "tenant_id": "tenant-a"},
			"downsample": {"resolution": 300000},
			"source": "compactor",
			"files": [{"rel_path": "chunks/000001", "size_bytes": 17500}, {"rel_path": "meta.json", "size_bytes": 500}]
		}
	}`, ulid5mHist, minHist, maxHist)

	reader.put(ulidRawUncompacted+"/meta.json", metaRawUncompacted)
	reader.put(ulidRawRecent+"/meta.json", metaRawRecent)
	reader.put(ulidRawHist+"/meta.json", metaRawHist)
	reader.put(ulid5mHist+"/meta.json", meta5mHist)

	ins := NewInspector(reader, InspectorConfig{
		SyncInterval: 15 * time.Minute,
		Retention: RetentionConfig{
			RawSeconds: 14 * 86400,  // 1209600s
			FiveMinSec: 180 * 86400, // 15552000s
			OneHourSec: 365 * 86400, // 31536000s
		},
	})

	if err := ins.Sync(ctx); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	var buf bytes.Buffer
	ins.WritePrometheusText(&buf, now)
	out := buf.String()

	// 1. Raw burn rate should be 10 B/s (72000 bytes / 7200s in 0_to_7d tier).
	wantRawBurnRate := `thanos_bucket_burn_rate_bytes_per_second{producer="receive",resolution="raw",tenant_id="tenant-a"} 10`
	if !strings.Contains(out, wantRawBurnRate) {
		t.Errorf("expected raw burn rate 10 B/s, got output:\n%s", out)
	}

	// 2. thanos_bucket_downsample_storage_ratio must NOT be emitted as a metric.
	if strings.Contains(out, "thanos_bucket_downsample_storage_ratio") {
		t.Errorf("expected thanos_bucket_downsample_storage_ratio to be pruned, but found in output:\n%s", out)
	}

	// 3. Burn rates computed using internal downsample ratios (5m ratio 1.8, 1h default ratio 0.05):
	// 5m burn rate = 10 * 1.8 = 18 B/s
	want5mBurnRate := `thanos_bucket_burn_rate_bytes_per_second{producer="receive",resolution="5m",tenant_id="tenant-a"} 18`
	if !strings.Contains(out, want5mBurnRate) {
		t.Errorf("expected 5m burn rate 18 B/s, got output:\n%s", out)
	}
	// 1h burn rate = 10 * 0.05 = 0.5 B/s
	want1hBurnRate := `thanos_bucket_burn_rate_bytes_per_second{producer="receive",resolution="1h",tenant_id="tenant-a"} 0.5`
	if !strings.Contains(out, want1hBurnRate) {
		t.Errorf("expected 1h burn rate 0.5 B/s, got output:\n%s", out)
	}

	// 5. Projected steady state with compactor delay offsets:
	//    Raw: 10 * 1209600 = 12096000
	//    5m:  18 * (15552000 - 40*3600) = 18 * 15408000 = 277344000
	//    1h:  0.5 * (31536000 - 14*86400) = 0.5 * 30326400 = 15163200
	//    Total = 12096000 + 277344000 + 15163200 = 304603200 = 3.046032e+08
	if !strings.Contains(out, `thanos_bucket_projected_steady_state_bytes{producer="receive",tenant_id="tenant-a"} 3.046032e+08`) {
		t.Errorf("expected steady state capacity 3.046032e+08 bytes, got output:\n%s", out)
	}

	// 6. When 1h retention is <= 14d, 1h effective retention is 0 (compactor never produces 1h blocks).
	insShort1h := NewInspector(reader, InspectorConfig{
		SyncInterval: 15 * time.Minute,
		Retention: RetentionConfig{
			RawSeconds: 14 * 86400,
			FiveMinSec: 14 * 86400,
			OneHourSec: 14 * 86400, // <= 14d, so effective 1h ret = 0
		},
	})
	if err := insShort1h.Sync(ctx); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	var bufShort bytes.Buffer
	insShort1h.WritePrometheusText(&bufShort, now)
	// Expected: Raw = 10 * 1209600 = 12096000, 5m = 18 * (1209600 - 144000) = 18 * 1065600 = 19180800, 1h = 0
	// Total = 12096000 + 19180800 = 31276800 = 3.12768e+07
	if !strings.Contains(bufShort.String(), `thanos_bucket_projected_steady_state_bytes{producer="receive",tenant_id="tenant-a"} 3.12768e+07`) {
		t.Errorf("expected steady state with short 1h retention to be 3.12768e+07, got:\n%s", bufShort.String())
	}

	// 7. When any retention tier is 0 (infinite retention), steady state projection must be omitted.
	insInfinite := NewInspector(reader, InspectorConfig{
		SyncInterval: 15 * time.Minute,
		Retention: RetentionConfig{
			RawSeconds: 0, // infinite
			FiveMinSec: 180 * 86400,
			OneHourSec: 365 * 86400,
		},
	})
	if err := insInfinite.Sync(ctx); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	var bufInf bytes.Buffer
	insInfinite.WritePrometheusText(&bufInf, now)
	if strings.Contains(bufInf.String(), "thanos_bucket_projected_steady_state_bytes") {
		t.Errorf("expected thanos_bucket_projected_steady_state_bytes to be omitted when retention is infinite (0), got:\n%s", bufInf.String())
	}
}

func TestExtractBlockULID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantULID string
		wantOK   bool
	}{
		{
			name:     "valid ULID with trailing slash",
			input:    "01K6B9RVTWK2B1VSQ3R34BMHP7/",
			wantULID: "01K6B9RVTWK2B1VSQ3R34BMHP7",
			wantOK:   true,
		},
		{
			name:     "valid ULID without trailing slash",
			input:    "01K6B9RVTWK2B1VSQ3R34BMHP7",
			wantULID: "01K6B9RVTWK2B1VSQ3R34BMHP7",
			wantOK:   true,
		},
		{
			name:     "valid ULID with nested subpath",
			input:    "01K6B9RVTWK2B1VSQ3R34BMHP7/meta.json",
			wantULID: "01K6B9RVTWK2B1VSQ3R34BMHP7",
			wantOK:   true,
		},
		{
			name:   "non-ULID root object",
			input:  "thanos.lock",
			wantOK: false,
		},
		{
			name:   "empty string",
			input:  "",
			wantOK: false,
		},
		{
			name:   "invalid character in ULID (U)",
			input:  "01K6B9RVTWK2B1VSQ3R34BMHPU/",
			wantOK: false,
		},
		{
			name:   "too short ULID",
			input:  "01K6B9RVTWK2B1VSQ3R34BMHP/",
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotULID, gotOK := extractBlockULID(tc.input)
			if gotOK != tc.wantOK {
				t.Errorf("extractBlockULID(%q) ok = %v, want %v", tc.input, gotOK, tc.wantOK)
			}
			if gotULID != tc.wantULID {
				t.Errorf("extractBlockULID(%q) ulid = %q, want %q", tc.input, gotULID, tc.wantULID)
			}
		})
	}
}

func TestNonRecursiveBucketSync(t *testing.T) {
	reader := newSpyBucketReader()
	ctx := context.Background()

	ulid1 := "01K6B9RVTWK2B1VSQ3R34BMHP7"
	reader.put(ulid1+"/meta.json", `{
		"ulid": "01K6B9RVTWK2B1VSQ3R34BMHP7",
		"minTime": 1000,
		"maxTime": 2000,
		"thanos": {"labels": {"receive": "true"}, "files": [{"rel_path": "meta.json", "size_bytes": 100}]}
	}`)
	// Add 50 chunk files under this block. None of them should ever be listed by root Iter!
	for i := range 50 {
		reader.put(fmt.Sprintf("%s/chunks/%06d", ulid1, i), "chunk-data")
	}

	ins := NewInspector(reader, InspectorConfig{})
	if err := ins.Sync(ctx); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	reader.mu.Lock()
	calls := slices.Clone(reader.iterCalls)
	reader.mu.Unlock()

	if len(calls) == 0 {
		t.Fatal("expected at least 1 call to Iter")
	}

	// The root Iter call must NOT have WithRecursiveIter.
	rootCall := calls[0]
	if rootCall.dir != "" {
		t.Errorf("expected root Iter call with dir='', got %q", rootCall.dir)
	}
	for _, opt := range rootCall.options {
		if opt.Type == objstore.Recursive {
			t.Errorf("root Iter call must NOT specify WithRecursiveIter option")
		}
	}

	if ins.CachedBlocks() != 1 {
		t.Fatalf("CachedBlocks = %d, want 1", ins.CachedBlocks())
	}
}

func TestSyncContextCancellation(t *testing.T) {
	reader := newSpyBucketReader()
	// Add many blocks so that processing takes time.
	for i := range 20 {
		ulidStr := fmt.Sprintf("01K6B9RVTWK2B1VSQ3R34BMH%02d", i)
		reader.put(ulidStr+"/meta.json", fmt.Sprintf(`{
			"ulid": "%s",
			"minTime": 1000,
			"maxTime": 2000,
			"thanos": {"labels": {"receive": "true"}, "files": [{"rel_path": "meta.json", "size_bytes": 100}]}
		}`, ulidStr))
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately so Sync encounters ctx.Done()
	cancel()

	ins := NewInspector(reader, InspectorConfig{})
	err := ins.Sync(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled Sync, got nil")
	}
	ready, reason := ins.Ready()
	if ready {
		t.Fatalf("inspector should NOT be ready after cancelled Sync (reason=%q)", reason)
	}
}

func TestIsBlockDeletionCandidate(t *testing.T) {
	nowMs := time.Now().UnixMilli()
	ret := RetentionConfig{
		RawSeconds: 14 * 86400, // 14 days
		FiveMinSec: 30 * 86400, // 30 days
		OneHourSec: 90 * 86400, // 90 days
	}

	tests := []struct {
		name      string
		block     BlockSummary
		ret       RetentionConfig
		fullSweep bool
		want      bool
	}{
		{
			name: "full sweep forces probe on any block",
			ret:  ret,
			block: BlockSummary{
				CompactionLevel: 3,
				Resolution:      "1h",
				MaxTime:         nowMs - 40*86400*1000, // 40 days old, level 3
			},
			fullSweep: true,
			want:      true,
		},
		{
			name: "recent block (<72h) probed even if compacted",
			ret:  ret,
			block: BlockSummary{
				CompactionLevel: 3,
				Resolution:      "raw",
				MaxTime:         nowMs - 24*3600*1000, // 24h old
			},
			fullSweep: false,
			want:      true,
		},
		{
			name: "low compaction level (<3) probed even if older than 72h",
			ret:  ret,
			block: BlockSummary{
				CompactionLevel: 2,
				Resolution:      "raw",
				MaxTime:         nowMs - 5*86400*1000, // 5 days old
			},
			fullSweep: false,
			want:      true,
		},
		{
			name: "block approaching retention expiry within 24h buffer probed",
			ret:  ret,
			block: BlockSummary{
				CompactionLevel: 4,
				Resolution:      "raw",
				// retention is 14d; block is 13.5d old (within 24h of expiring)
				MaxTime: nowMs - int64(13.5*86400*1000),
			},
			fullSweep: false,
			want:      true,
		},
		{
			name: "block past retention expiry probed",
			ret:  ret,
			block: BlockSummary{
				CompactionLevel: 4,
				Resolution:      "raw",
				MaxTime:         nowMs - 15*86400*1000,
			},
			fullSweep: false,
			want:      true,
		},
		{
			name: "old compacted block well within retention skipped on fast path",
			ret:  ret,
			block: BlockSummary{
				CompactionLevel: 4,
				Resolution:      "1h",
				// retention is 90d; block is 30d old (well clear of 72h recent and 90d retention)
				MaxTime: nowMs - 30*86400*1000,
			},
			fullSweep: false,
			want:      false,
		},
		{
			name: "infinite retention (0s) old compacted block skipped on fast path",
			ret: RetentionConfig{
				RawSeconds: 0,
				FiveMinSec: 0,
				OneHourSec: 0,
			},
			block: BlockSummary{
				CompactionLevel: 3,
				Resolution:      "1h",
				MaxTime:         nowMs - 100*86400*1000,
			},
			fullSweep: false,
			want:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isBlockDeletionCandidate(tc.block, nowMs, tc.ret, tc.fullSweep)
			if got != tc.want {
				t.Errorf("isBlockDeletionCandidate() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSyncDeletionCandidateProbeSkipsNonCandidates(t *testing.T) {
	spy := newSpyBucketReader()
	now := time.Now()

	// 1. Candidate block: recent (<72h)
	candULID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	candMeta := fmt.Sprintf(`{
		"ulid": "%s",
		"minTime": %d,
		"maxTime": %d,
		"thanos": {
			"resolution": 0,
			"downsample": {"resolution": 0},
			"labels": {"tenant_id": "t1"},
			"segment_title": "s1"
		},
		"compaction": {"level": 1, "sources": ["%s"]}
	}`, candULID, now.Add(-2*time.Hour).UnixMilli(), now.Add(-1*time.Hour).UnixMilli(), candULID)
	spy.put(candULID+"/meta.json", candMeta)

	// 2. Non-candidate block: old (>72h), compaction level 3, well within retention
	nonCandULID := "01ARZ3NDEKTSV4RRFFQ69G5FBW"
	nonCandMeta := fmt.Sprintf(`{
		"ulid": "%s",
		"minTime": %d,
		"maxTime": %d,
		"thanos": {
			"resolution": 0,
			"downsample": {"resolution": 0},
			"labels": {"tenant_id": "t1"},
			"segment_title": "s1"
		},
		"compaction": {"level": 3, "sources": ["%s"]}
	}`, nonCandULID, now.Add(-200*time.Hour).UnixMilli(), now.Add(-150*time.Hour).UnixMilli(), nonCandULID)
	spy.put(nonCandULID+"/meta.json", nonCandMeta)

	ins := NewInspector(spy, InspectorConfig{
		Retention: RetentionConfig{
			RawSeconds: 30 * 86400, // 30 days
		},
	})

	// First sync fetches both blocks
	if err := ins.Sync(context.Background()); err != nil {
		t.Fatalf("first Sync failed: %v", err)
	}
	if ins.CachedBlocks() != 2 {
		t.Fatalf("CachedBlocks = %d, want 2", ins.CachedBlocks())
	}

	spy.mu.Lock()
	firstCandExists := spy.existsCalls[candULID+"/deletion-mark.json"]
	firstNonCandExists := spy.existsCalls[nonCandULID+"/deletion-mark.json"]
	spy.mu.Unlock()

	// On initial fetch, Exists is called when reading metadata
	if firstCandExists == 0 || firstNonCandExists == 0 {
		t.Fatalf("expected Exists called during initial block ingest")
	}

	// Reset exists call counter
	spy.mu.Lock()
	spy.existsCalls = make(map[string]int)
	spy.mu.Unlock()

	// Second sync immediately after (incremental sync, fullSweep is false)
	if err := ins.Sync(context.Background()); err != nil {
		t.Fatalf("second Sync failed: %v", err)
	}

	spy.mu.Lock()
	secondCandExists := spy.existsCalls[candULID+"/deletion-mark.json"]
	secondNonCandExists := spy.existsCalls[nonCandULID+"/deletion-mark.json"]
	spy.mu.Unlock()

	if secondCandExists == 0 {
		t.Errorf("expected candidate block to be checked for deletion-mark.json on incremental sync")
	}
	if secondNonCandExists != 0 {
		t.Errorf("expected non-candidate block to be SKIPPED on incremental sync, but got %d Exists calls", secondNonCandExists)
	}

	// Now simulate full sweep (backdate lastFullSweepSecs by 7 hours)
	ins.lastFullSweepSecs.Store(time.Now().Add(-7 * time.Hour).Unix())

	spy.mu.Lock()
	spy.existsCalls = make(map[string]int)
	spy.mu.Unlock()

	if err := ins.Sync(context.Background()); err != nil {
		t.Fatalf("full sweep Sync failed: %v", err)
	}

	spy.mu.Lock()
	fullCandExists := spy.existsCalls[candULID+"/deletion-mark.json"]
	fullNonCandExists := spy.existsCalls[nonCandULID+"/deletion-mark.json"]
	spy.mu.Unlock()

	if fullCandExists == 0 || fullNonCandExists == 0 {
		t.Errorf("expected ALL blocks to be checked during full sweep, got cand=%d, nonCand=%d", fullCandExists, fullNonCandExists)
	}
}

func TestCorruptMetaJSONSkippedCleanly(t *testing.T) {
	spy := newSpyBucketReader()
	validULID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	corruptULID := "01ARZ3NDEKTSV4RRFFQ69G5FAW"

	validMeta := fmt.Sprintf(`{
		"ulid": "%s",
		"minTime": 1000,
		"maxTime": 2000,
		"thanos": {
			"resolution": 0,
			"downsample": {"resolution": 0},
			"labels": {"tenant_id": "t1"}
		}
	}`, validULID)
	spy.put(validULID+"/meta.json", validMeta)
	spy.put(corruptULID+"/meta.json", "{ malformed json ...")

	ins := NewInspector(spy, InspectorConfig{})
	if err := ins.Sync(context.Background()); err != nil {
		t.Fatalf("expected Sync() to succeed and skip corrupt block, got error: %v", err)
	}

	if ins.CachedBlocks() != 1 {
		t.Fatalf("expected 1 valid block cached, got %d", ins.CachedBlocks())
	}
	if ok, reason := ins.Ready(); !ok {
		t.Fatalf("expected Ready() = true, got false (%s)", reason)
	}
}

func TestDisableDownsampling(t *testing.T) {
	spy := newSpyBucketReader()
	now := time.Now()
	rawULID := "01ARZ3NDEKTSV4RRFFQ69G5FA1"
	downsampleULID := "01ARZ3NDEKTSV4RRFFQ69G5FA2"

	// Raw block: 48h to 72h old (24h span)
	rawMin := now.Add(-72 * time.Hour).UnixMilli()
	rawMax := now.Add(-48 * time.Hour).UnixMilli()
	rawMeta := fmt.Sprintf(`{
		"ulid": "%s",
		"minTime": %d,
		"maxTime": %d,
		"thanos": {
			"downsample": {"resolution": 0},
			"labels": {"tenant_id": "default", "receive": "true"},
			"files": [{"rel_path": "chunks/000001", "size_bytes": 1000000}]
		},
		"compaction": {"level": 2}
	}`, rawULID, rawMin, rawMax)
	spy.put(rawULID+"/meta.json", rawMeta)

	// 5m downsampled block
	downMeta := fmt.Sprintf(`{
		"ulid": "%s",
		"minTime": %d,
		"maxTime": %d,
		"thanos": {
			"downsample": {"resolution": 300000},
			"labels": {"tenant_id": "default", "receive": "true"},
			"files": [{"rel_path": "chunks/000001", "size_bytes": 500000}]
		},
		"compaction": {"level": 2}
	}`, downsampleULID, rawMin, rawMax)
	spy.put(downsampleULID+"/meta.json", downMeta)

	enableDown := false
	ins := NewInspector(spy, InspectorConfig{
		Retention: RetentionConfig{
			RawSeconds: 30 * 86400,
			// Infinite downsample retention (0s)
			FiveMinSec: 0,
			OneHourSec: 0,
		},
		EnableDownsampling: &enableDown,
	})

	if err := ins.Sync(context.Background()); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	var buf bytes.Buffer
	ins.WritePrometheusText(&buf, now)
	metricsText := buf.String()
	// Assert 5m and 1h burn rates are 0
	if !strings.Contains(metricsText, `thanos_bucket_burn_rate_bytes_per_second{producer="receive",resolution="5m",tenant_id="default"} 0`) {
		t.Errorf("expected 5m burn rate = 0, got:\n%s", metricsText)
	}
	if !strings.Contains(metricsText, `thanos_bucket_burn_rate_bytes_per_second{producer="receive",resolution="1h",tenant_id="default"} 0`) {
		t.Errorf("expected 1h burn rate = 0, got:\n%s", metricsText)
	}
	// Assert steady state projection IS emitted because raw retention is finite (30d), despite downsample retention being 0s
	if !strings.Contains(metricsText, `thanos_bucket_projected_steady_state_bytes{producer="receive",tenant_id="default"}`) {
		t.Errorf("expected projected steady state bytes emitted when downsampling is disabled and raw retention is finite, got:\n%s", metricsText)
	}
}
