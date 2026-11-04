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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thanos-io/objstore"
)

type spyBucketReader struct {
	*objstore.InMemBucket
	mu       sync.Mutex
	iterErr  error
	getCalls map[string]int
}

func newSpyBucketReader() *spyBucketReader {
	return &spyBucketReader{
		InMemBucket: objstore.NewInMemBucket(),
		getCalls:    make(map[string]int),
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
				{"rel_path": "meta.json"}
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
				{"rel_path": "meta.json"}
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

	// Verify overlapping HA blocks (ulid1 & ulid2) both covering 2h (7200s) produce union = 7200s (not 14400s!).
	wantSpan := `thanos_bucket_spanned_seconds{age_tier="0_to_7d",producer="receive",resolution="raw",tenant_id="tenant-a"} 7200`
	if !strings.Contains(out, wantSpan) {
		t.Errorf("expected non-overlapping span 7200s, got output:\n%s", out)
	}

	// Verify chunk storage sums both blocks (50000 + 50000 = 100000).
	wantChunks := `thanos_bucket_storage_bytes{age_tier="0_to_7d",file_type="chunks",marked_for_deletion="false",producer="receive",resolution="raw",tenant_id="tenant-a"} 100000`
	if !strings.Contains(out, wantChunks) {
		t.Errorf("expected chunk storage 100000, got output:\n%s", out)
	}

	// Verify recent block series picks the tie-broken max (1250).
	wantRecent := `thanos_bucket_recent_block_series{producer="receive",tenant_id="tenant-a"} 1250`
	if !strings.Contains(out, wantRecent) {
		t.Errorf("expected recent block series 1250, got output:\n%s", out)
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
				{"rel_path": "index", "size_bytes": 8000}
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

	// ulid2 is now marked_for_deletion="true", while ulid1 + ulid4 are active and contiguous (2h + 2h = 14400s).
	wantSpan2 := `thanos_bucket_spanned_seconds{age_tier="0_to_7d",producer="receive",resolution="raw",tenant_id="tenant-a"} 14400`
	if !strings.Contains(out2, wantSpan2) {
		t.Errorf("expected contiguous span 14400s after ulid4 added, got:\n%s", out2)
	}
	wantDelChunks := `thanos_bucket_storage_bytes{age_tier="0_to_7d",file_type="chunks",marked_for_deletion="true",producer="receive",resolution="raw",tenant_id="tenant-a"} 50000`
	if !strings.Contains(out2, wantDelChunks) {
		t.Errorf("expected marked_for_deletion=true chunk bytes 50000, got:\n%s", out2)
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
	wantMetaBytes := int64(len(metaPayload) + len(deletionMarkPayload))
	if blk.MetaBytes != wantMetaBytes {
		t.Errorf("MetaBytes = %d, want %d (meta.json + deletion-mark.json)", blk.MetaBytes, wantMetaBytes)
	}
}
