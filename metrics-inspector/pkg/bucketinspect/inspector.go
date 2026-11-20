// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package bucketinspect

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/thanos-io/objstore"
)

const (
	defaultMetaFetchConcurrency = 8

	// maxMetaJSONBytes caps the size of any single meta.json read into memory (16 MiB).
	maxMetaJSONBytes = 16 * 1024 * 1024

	metaFilename         = "meta.json"
	deletionMarkFilename = "deletion-mark.json"

	msPerDay = 86400 * 1000

	// Thanos Compactor downsampling delay windows (hardcoded in upstream Thanos Compactor).
	// Compactor never downsamples blocks younger than 40h to 5m, or younger than 14d to 1h.
	thanosDownsampleDelay5mSec = 40 * 3600
	thanosDownsampleDelay1hSec = 14 * 24 * 3600

	ageTier0To7d     = "0_to_7d"
	ageTier7dTo30d   = "7d_to_30d"
	ageTier30dTo90d  = "30d_to_90d"
	ageTier90dTo180d = "90d_to_180d"
	ageTier180dPlus  = "180d_plus"
)

var allAgeTiers = []string{
	ageTier0To7d,
	ageTier7dTo30d,
	ageTier30dTo90d,
	ageTier90dTo180d,
	ageTier180dPlus,
}

// BlockSummary stores the immutable metadata of a single TSDB block in object storage
// along with its mutable deletion-mark state.
type BlockSummary struct {
	ULID              string
	MinTime           int64 // Unix timestamp in milliseconds
	MaxTime           int64 // Unix timestamp in milliseconds
	TenantID          string
	Producer          string
	Resolution        string // "raw", "5m", "1h"
	CompactionLevel   int
	NumSamples        uint64
	NumSeries         uint64
	NumChunks         uint64
	StorageBytes      int64
	MarkedForDeletion bool
}

// thanosBlockMeta embeds upstream Prometheus tsdb.BlockMeta and adds the Thanos-specific
// meta.json extension fields. Note: github.com/thanos-io/thanos/pkg/block/metadata is not
// imported directly because it transitively imports github.com/thanos-io/promql-engine,
// which is incompatible with the CVE-2026-42151 signature change in prometheus v0.305.3.
type thanosBlockMeta struct {
	tsdb.BlockMeta
	Thanos struct {
		Labels     map[string]string `json:"labels"`
		Downsample struct {
			Resolution int64 `json:"resolution"`
		} `json:"downsample"`
		Source       string   `json:"source"`
		SegmentFiles []string `json:"segment_files"`
		Files        []struct {
			RelPath   string `json:"rel_path"`
			SizeBytes int64  `json:"size_bytes"`
		} `json:"files"`
	} `json:"thanos"`
}

// InspectorConfig configures the periodic Thanos bucket inspector.
type InspectorConfig struct {
	SyncInterval      time.Duration
	InitRetryInterval time.Duration
	Retention         RetentionConfig
	Concurrency       int
	Logger            *slog.Logger
}

// Inspector periodically scans a Thanos object storage bucket using an incremental
// in-memory ULID cache and exposes raw storage primitives in Prometheus text format.
type Inspector struct {
	bkt               objstore.BucketReader
	interval          time.Duration
	initRetryInterval time.Duration
	retention         RetentionConfig
	concurrency       int
	logger            *slog.Logger

	ready      atomic.Bool
	readyErrMu sync.RWMutex
	readyErr   string

	mu     sync.RWMutex
	blocks map[string]BlockSummary

	syncsTotal            atomic.Uint64
	syncErrorsTotal       atomic.Uint64
	lastSyncTimestampSecs atomic.Int64
	lastSyncDurationNano  atomic.Int64
}

// NewInspector creates a new Thanos bucket Inspector backed by an upstream Thanos objstore.BucketReader.
func NewInspector(bkt objstore.BucketReader, cfg InspectorConfig) *Inspector {
	interval := cfg.SyncInterval
	if interval <= 0 {
		interval = DefaultSyncInterval
	}
	initRetryInterval := cfg.InitRetryInterval
	if initRetryInterval <= 0 {
		initRetryInterval = 5 * time.Second
	}
	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = defaultMetaFetchConcurrency
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	ins := &Inspector{
		bkt:               bkt,
		interval:          interval,
		initRetryInterval: initRetryInterval,
		retention:         cfg.Retention,
		concurrency:       concurrency,
		logger:            logger,
		readyErr:          "waiting for initial object storage bucket sync",
		blocks:            make(map[string]BlockSummary),
	}
	if bkt == nil {
		ins.ready.Store(true)
	}
	return ins
}

// Ready reports whether initial connectivity to the object storage bucket has been verified.
// Once latched true, Ready remains true for the life of the Inspector.
func (ins *Inspector) Ready() (bool, string) {
	if ins == nil || ins.bkt == nil || ins.ready.Load() {
		return true, ""
	}
	ins.readyErrMu.RLock()
	defer ins.readyErrMu.RUnlock()
	return false, ins.readyErr
}

func (ins *Inspector) setReadyErr(err error) {
	ins.readyErrMu.Lock()
	if err != nil {
		ins.readyErr = err.Error()
	}
	ins.readyErrMu.Unlock()
}

// Run executes an initial bucket sync immediately (retrying with a short backoff until
// initial connectivity is established) and then syncs on every interval tick until ctx
// is canceled. Object storage errors are logged and counted without crashing.
func (ins *Inspector) Run(ctx context.Context) {
	if ins.bkt == nil {
		return
	}
	if !ins.syncUntilReady(ctx) {
		return
	}

	ticker := time.NewTicker(ins.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			syncTimeout := max(ins.interval, 15*time.Minute)
			syncCtx, cancel := context.WithTimeout(ctx, syncTimeout)
			if err := ins.Sync(syncCtx); err != nil && ctx.Err() == nil {
				ins.logger.Warn("periodic thanos bucket inspection failed", "err", err)
			}
			cancel()
		}
	}
}

func (ins *Inspector) syncUntilReady(ctx context.Context) bool {
	syncTimeout := max(ins.interval, 15*time.Minute)
	syncCtx, cancel := context.WithTimeout(ctx, syncTimeout)
	err := ins.Sync(syncCtx)
	cancel()
	if err == nil {
		return true
	} else if ctx.Err() == nil {
		ins.logger.Warn("initial thanos bucket inspection failed; retrying", "err", err)
	}

	ticker := time.NewTicker(ins.initRetryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			retryCtx, retryCancel := context.WithTimeout(ctx, syncTimeout)
			retryErr := ins.Sync(retryCtx)
			retryCancel()
			if retryErr == nil {
				return true
			} else if ctx.Err() == nil {
				ins.logger.Warn("initial thanos bucket inspection failed; retrying", "err", retryErr)
			}
		}
	}
}

// Sync performs a single incremental inspection pass against the Thanos object storage bucket.
func (ins *Inspector) Sync(ctx context.Context) error {
	if ins.bkt == nil {
		return nil
	}
	start := time.Now()
	ins.syncsTotal.Add(1)

	// List top-level bucket entries without recursion.
	// In S3, GCS, Azure, and InMemBucket, this returns top-level block directory prefixes
	// (e.g. "01K.../") rather than enumerating millions of chunk objects across all blocks.
	discoveredULIDs := make(map[string]struct{})
	err := ins.bkt.Iter(ctx, "", func(name string) error {
		if ulidStr, ok := extractBlockULID(name); ok {
			discoveredULIDs[ulidStr] = struct{}{}
		}
		return nil
	})
	if err != nil {
		ins.syncErrorsTotal.Add(1)
		ins.lastSyncDurationNano.Store(time.Since(start).Nanoseconds())
		wrappedErr := fmt.Errorf("failed to list bucket objects: %w", err)
		ins.setReadyErr(wrappedErr)
		return wrappedErr
	}

	// 1. Identify missing ULIDs to fetch.
	// 2. Identify cached ULIDs that are not yet marked for deletion to refresh deletion-mark status.
	ins.mu.RLock()
	var missingULIDs []string
	var checkDeletionULIDs []string
	for ulidStr := range discoveredULIDs {
		if cached, ok := ins.blocks[ulidStr]; !ok {
			missingULIDs = append(missingULIDs, ulidStr)
		} else if !cached.MarkedForDeletion {
			checkDeletionULIDs = append(checkDeletionULIDs, ulidStr)
		}
	}
	ins.mu.RUnlock()
	slices.Sort(missingULIDs)
	slices.Sort(checkDeletionULIDs)

	fetched := make(map[string]BlockSummary, len(missingULIDs))
	var fetchMu sync.Mutex
	var firstFetchErr error
	var fetchErrors int

	sem := make(chan struct{}, ins.concurrency)
	var wg sync.WaitGroup

	// Fetch meta.json and check deletion-mark.json for newly discovered ULIDs.
	if len(missingULIDs) > 0 {
	fetchLoop:
		for _, u := range missingULIDs {
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				break fetchLoop
			}
			wg.Add(1)
			go func(ulidStr string) {
				defer wg.Done()
				defer func() { <-sem }()

				rawMeta, err := ins.getObjectBytes(ctx, ulidStr+"/"+metaFilename)
				if err != nil {
					// If meta.json is not found (e.g. block upload incomplete or concurrently removed), skip.
					if ins.bkt.IsObjNotFoundErr(err) {
						ins.logger.Debug("block meta.json not found; skipping incomplete block", "ulid", ulidStr)
						return
					}
					fetchMu.Lock()
					fetchErrors++
					if firstFetchErr == nil {
						firstFetchErr = err
					}
					fetchMu.Unlock()
					return
				}

				hasDeletion, err := ins.bkt.Exists(ctx, ulidStr+"/"+deletionMarkFilename)
				if err != nil {
					ins.logger.Debug("failed to check deletion-mark.json for new block", "ulid", ulidStr, "err", err)
				}

				summary, err := parseBlockSummary(ctx, ins.bkt, ulidStr, rawMeta, hasDeletion)
				if err != nil {
					fetchMu.Lock()
					fetchErrors++
					if firstFetchErr == nil {
						firstFetchErr = err
					}
					fetchMu.Unlock()
					return
				}
				fetchMu.Lock()
				fetched[ulidStr] = summary
				fetchMu.Unlock()
				ins.logger.Debug("fetched thanos block meta.json",
					"ulid", ulidStr,
					"resolution", summary.Resolution,
					"producer", summary.Producer,
				)
			}(u)
		}
	}

	// Check deletion marks for existing cached blocks that are not yet marked for deletion.
	newlyMarked := make(map[string]bool)
	var newlyMarkedMu sync.Mutex

	if len(checkDeletionULIDs) > 0 {
	deletionLoop:
		for _, u := range checkDeletionULIDs {
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				break deletionLoop
			}
			wg.Add(1)
			go func(ulidStr string) {
				defer wg.Done()
				defer func() { <-sem }()

				hasDeletion, err := ins.bkt.Exists(ctx, ulidStr+"/"+deletionMarkFilename)
				if err == nil && hasDeletion {
					newlyMarkedMu.Lock()
					newlyMarked[ulidStr] = true
					newlyMarkedMu.Unlock()
					ins.logger.Debug("detected existing block marked for deletion", "ulid", ulidStr)
				}
			}(u)
		}
	}

	wg.Wait()

	if err := ctx.Err(); err != nil {
		ins.syncErrorsTotal.Add(1)
		wrappedErr := fmt.Errorf("thanos bucket inspection aborted: %w", err)
		ins.setReadyErr(wrappedErr)
		return wrappedErr
	}

	// Reconcile the in-memory ULID cache:
	// 1. Evict ULIDs no longer present in the bucket.
	// 2. Insert newly fetched ULIDs.
	// 3. Mark existing blocks that now have deletion-mark.json.
	ins.mu.Lock()
	for existingULID := range ins.blocks {
		if _, exists := discoveredULIDs[existingULID]; !exists {
			delete(ins.blocks, existingULID)
		}
	}
	maps.Copy(ins.blocks, fetched)
	for u := range newlyMarked {
		if summary, ok := ins.blocks[u]; ok {
			summary.MarkedForDeletion = true
			ins.blocks[u] = summary
		}
	}
	cachedCount := len(ins.blocks)
	ins.mu.Unlock()

	duration := time.Since(start)
	ins.lastSyncDurationNano.Store(duration.Nanoseconds())

	if fetchErrors > 0 {
		ins.syncErrorsTotal.Add(1)
		wrappedErr := fmt.Errorf("failed to fetch %d/%d new block meta.json files (first error: %w)", fetchErrors, len(missingULIDs), firstFetchErr)
		ins.setReadyErr(wrappedErr)
		return wrappedErr
	}

	ins.ready.Store(true)
	now := time.Now()
	ins.lastSyncTimestampSecs.Store(now.Unix())
	ins.logger.Info("completed thanos bucket inspection",
		"cached_blocks", cachedCount,
		"new_blocks_fetched", len(fetched),
		"duration_ms", duration.Milliseconds(),
	)
	return nil
}

func (ins *Inspector) getObjectBytes(ctx context.Context, name string) ([]byte, error) {
	rc, err := ins.bkt.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(rc, maxMetaJSONBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to read object %q: %w", name, err)
	}
	return data, nil
}

// CachedBlocks returns the number of TSDB blocks currently held in the in-memory cache.
func (ins *Inspector) CachedBlocks() int {
	ins.mu.RLock()
	defer ins.mu.RUnlock()
	return len(ins.blocks)
}

// extractBlockULID extracts and validates a 26-character Crockford Base32 TSDB block ULID
// from a top-level bucket entry (e.g. "01K6B9RVTWK2B1VSQ3R34BMHP7/" or "01K6B9RVTWK2B1VSQ3R34BMHP7").
func extractBlockULID(name string) (string, bool) {
	trimmed := strings.Trim(name, "/")
	if slashIdx := strings.IndexByte(trimmed, '/'); slashIdx != -1 {
		trimmed = trimmed[:slashIdx]
	}
	if len(trimmed) != 26 {
		return "", false
	}
	if _, err := ulid.ParseStrict(trimmed); err != nil {
		return "", false
	}
	return trimmed, true
}

func parseBlockSummary(
	ctx context.Context,
	bkt objstore.BucketReader,
	ulidStr string,
	rawMeta []byte,
	markedForDeletion bool,
) (BlockSummary, error) {
	var meta thanosBlockMeta
	if err := json.Unmarshal(rawMeta, &meta); err != nil {
		return BlockSummary{}, fmt.Errorf("invalid meta.json for block %s: %w", ulidStr, err)
	}

	tenantID := strings.TrimSpace(meta.Thanos.Labels["tenant_id"])
	if tenantID == "" {
		tenantID = "default"
	}

	producer := extractProducer(meta.Thanos.Labels, meta.Thanos.Source)
	resolution := formatResolution(meta.Thanos.Downsample.Resolution)

	var storageBytes int64
	var hasMetaSize bool
	for _, f := range meta.Thanos.Files {
		if f.SizeBytes <= 0 {
			continue
		}
		if f.RelPath == metaFilename {
			hasMetaSize = true
		}
		storageBytes += f.SizeBytes
	}

	// Fall back to querying object attributes when thanos.files is unpopulated (e.g. older TSDB blocks).
	// We only recursively iterate this single block directory, never the entire bucket.
	if storageBytes == 0 && bkt != nil {
		_ = bkt.Iter(ctx, ulidStr, func(rel string) error {
			if strings.HasSuffix(rel, metaFilename) {
				return nil
			}
			attrs, err := bkt.Attributes(ctx, rel)
			if err == nil && attrs.Size > 0 {
				storageBytes += attrs.Size
			}
			return nil
		}, objstore.WithRecursiveIter())
	}
	if !hasMetaSize {
		storageBytes += int64(len(rawMeta))
	}

	compactionLevel := meta.Compaction.Level
	if compactionLevel <= 0 {
		compactionLevel = 1
	}

	return BlockSummary{
		ULID:              ulidStr,
		MinTime:           meta.MinTime,
		MaxTime:           meta.MaxTime,
		TenantID:          tenantID,
		Producer:          producer,
		Resolution:        resolution,
		CompactionLevel:   compactionLevel,
		NumSamples:        meta.Stats.NumSamples,
		NumSeries:         meta.Stats.NumSeries,
		NumChunks:         meta.Stats.NumChunks,
		StorageBytes:      storageBytes,
		MarkedForDeletion: markedForDeletion,
	}, nil
}

// extractProducer derives the originating component ("receive" or "ruler") from external labels
// because Thanos Compactor overwrites thanos.source="compactor" on compacted blocks (level > 1).
func extractProducer(labels map[string]string, source string) string {
	if labels["receive"] == "true" || labels["receive_replica"] != "" {
		return "receive"
	}
	if labels["rule_replica"] != "" {
		return "ruler"
	}
	if src := strings.TrimSpace(source); src != "" {
		return src
	}
	return "unknown"
}

func formatResolution(resMillis int64) string {
	switch resMillis {
	case 0:
		return "raw"
	case 5 * 60 * 1000:
		return "5m"
	case 60 * 60 * 1000:
		return "1h"
	default:
		return fmt.Sprintf("%dms", resMillis)
	}
}

func classifyAgeTier(minTimeMs, maxTimeMs, nowMs int64) string {
	midpointMs := minTimeMs + (maxTimeMs-minTimeMs)/2
	ageDays := float64(nowMs-midpointMs) / float64(msPerDay)
	switch {
	case ageDays < 7:
		return ageTier0To7d
	case ageDays < 30:
		return ageTier7dTo30d
	case ageDays < 90:
		return ageTier30dTo90d
	case ageDays < 180:
		return ageTier90dTo180d
	default:
		return ageTier180dPlus
	}
}

type timeInterval struct {
	minMs   int64
	maxMs   int64
	ageTier string
}

// accumulateStreamIntervalsByTier merges overlapping intervals across an entire
// (tenant_id, producer, resolution) stream in chronological order and attributes each
// newly covered sub-interval to its block's ageTier. This guarantees that
// the accumulated tier spans equal the
// exact global non-overlapping union even when overlapping blocks straddle an age tier boundary.
func accumulateStreamIntervalsByTier(resKey resolutionGroupKey, intervals []timeInterval, out map[tierGroupKey]float64) {
	if len(intervals) == 0 {
		return
	}
	sorted := slices.Clone(intervals)
	slices.SortFunc(sorted, func(a, b timeInterval) int {
		if c := cmp.Compare(a.minMs, b.minMs); c != 0 {
			return c
		}
		return cmp.Compare(a.maxMs, b.maxMs)
	})

	cursorMs := sorted[0].minMs
	for _, iv := range sorted {
		effectiveMin := max(iv.minMs, cursorMs)
		if iv.maxMs > effectiveMin {
			tKey := tierGroupKey{resKey.tenantID, resKey.producer, resKey.resolution, iv.ageTier}
			out[tKey] += float64(iv.maxMs-effectiveMin) / 1000.0
			cursorMs = iv.maxMs
		}
	}
}

// nonOverlappingSpannedSeconds computes the deduplicated non-overlapping coverage
// in seconds across an arbitrary slice of timeIntervals.
func nonOverlappingSpannedSeconds(intervals []timeInterval) float64 {
	if len(intervals) == 0 {
		return 0
	}
	sorted := slices.Clone(intervals)
	slices.SortFunc(sorted, func(a, b timeInterval) int {
		if c := cmp.Compare(a.minMs, b.minMs); c != 0 {
			return c
		}
		return cmp.Compare(a.maxMs, b.maxMs)
	})

	var totalSec float64
	cursorMs := sorted[0].minMs
	for _, iv := range sorted {
		effectiveMin := max(iv.minMs, cursorMs)
		if iv.maxMs > effectiveMin {
			totalSec += float64(iv.maxMs-effectiveMin) / 1000.0
			cursorMs = iv.maxMs
		}
	}
	return totalSec
}

type storageKey struct {
	tenantID          string
	producer          string
	resolution        string
	ageTier           string
	markedForDeletion string
}

type blockCountKey struct {
	tenantID          string
	producer          string
	resolution        string
	markedForDeletion string
}

type tierGroupKey struct {
	tenantID   string
	producer   string
	resolution string
	ageTier    string
}

type resolutionGroupKey struct {
	tenantID   string
	producer   string
	resolution string
}

type producerGroupKey struct {
	tenantID string
	producer string
}

var (
	descSyncsTotal = prometheus.NewDesc(
		"thanos_bucket_syncs_total",
		"Total number of Thanos bucket inspection sync attempts.",
		nil, nil,
	)
	descSyncErrorsTotal = prometheus.NewDesc(
		"thanos_bucket_sync_errors_total",
		"Total number of failed Thanos bucket inspection syncs.",
		nil, nil,
	)
	descLastSyncTimestampSecs = prometheus.NewDesc(
		"thanos_bucket_last_sync_timestamp_seconds",
		"Unix timestamp of the last successful Thanos bucket sync.",
		nil, nil,
	)
	descLastSyncDurationSecs = prometheus.NewDesc(
		"thanos_bucket_last_sync_duration_seconds",
		"Duration of the last Thanos bucket inspection sync in seconds.",
		nil, nil,
	)
	descConfiguredRetentionSecs = prometheus.NewDesc(
		"thanos_bucket_configured_retention_seconds",
		"Configured Thanos Compactor retention period in seconds per resolution tier (0 means infinite).",
		[]string{"resolution"}, nil,
	)
	descCachedBlocks = prometheus.NewDesc(
		"thanos_bucket_cached_blocks",
		"Number of TSDB blocks currently held in the in-memory metadata cache.",
		nil, nil,
	)
	descStorageBytes = prometheus.NewDesc(
		"thanos_bucket_storage_bytes",
		"Total bytes stored in the Thanos bucket by tenant, producer, resolution, age tier, and deletion mark status.",
		[]string{"tenant_id", "producer", "resolution", "age_tier", "marked_for_deletion"}, nil,
	)
	descBlocks = prometheus.NewDesc(
		"thanos_bucket_blocks",
		"Number of TSDB blocks in the Thanos bucket by tenant, producer, resolution, and deletion mark status.",
		[]string{"tenant_id", "producer", "resolution", "marked_for_deletion"}, nil,
	)
	descOldestBlockTimestampSecs = prometheus.NewDesc(
		"thanos_bucket_oldest_block_timestamp_seconds",
		"Minimum minTime (in Unix seconds) across active TSDB blocks.",
		[]string{"tenant_id", "producer", "resolution"}, nil,
	)
	descNewestBlockTimestampSecs = prometheus.NewDesc(
		"thanos_bucket_newest_block_timestamp_seconds",
		"Maximum maxTime (in Unix seconds) across active TSDB blocks.",
		[]string{"tenant_id", "producer", "resolution"}, nil,
	)
	descBurnRateBytesPerSec = prometheus.NewDesc(
		"thanos_bucket_burn_rate_bytes_per_second",
		"Storage ingestion or downsampled generation rate in bytes per second by tenant, producer, and resolution.",
		[]string{"tenant_id", "producer", "resolution"}, nil,
	)
	descProjectedSteadyStateBytes = prometheus.NewDesc(
		"thanos_bucket_projected_steady_state_bytes",
		"Projected steady-state object storage usage in bytes across all retention tiers for the current active ingestion dynamic.",
		[]string{"tenant_id", "producer"}, nil,
	)
)

// Describe implements prometheus.Collector.
func (ins *Inspector) Describe(ch chan<- *prometheus.Desc) {
	if ins == nil {
		return
	}
	ch <- descSyncsTotal
	ch <- descSyncErrorsTotal
	ch <- descLastSyncTimestampSecs
	ch <- descLastSyncDurationSecs
	ch <- descConfiguredRetentionSecs
	ch <- descCachedBlocks
	ch <- descStorageBytes
	ch <- descBlocks
	ch <- descOldestBlockTimestampSecs
	ch <- descNewestBlockTimestampSecs
	ch <- descBurnRateBytesPerSec
	ch <- descProjectedSteadyStateBytes
}

// Collect implements prometheus.Collector.
func (ins *Inspector) Collect(ch chan<- prometheus.Metric) {
	ins.collectAt(ch, time.Now())
}

type timePinnedCollector struct {
	ins *Inspector
	now time.Time
}

func (c timePinnedCollector) Describe(ch chan<- *prometheus.Desc) {
	c.ins.Describe(ch)
}

func (c timePinnedCollector) Collect(ch chan<- prometheus.Metric) {
	c.ins.collectAt(ch, c.now)
}

// WritePrometheusText writes all Thanos bucket inspection metrics in Prometheus text exposition format.
func (ins *Inspector) WritePrometheusText(w io.Writer, now time.Time) {
	if ins == nil {
		return
	}
	reg := prometheus.NewRegistry()
	reg.MustRegister(timePinnedCollector{ins: ins, now: now})
	mfs, err := reg.Gather()
	if err != nil {
		return
	}
	enc := expfmt.NewEncoder(w, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, mf := range mfs {
		_ = enc.Encode(mf)
	}
}

func (ins *Inspector) collectAt(ch chan<- prometheus.Metric, now time.Time) {
	if ins == nil {
		return
	}

	ch <- prometheus.MustNewConstMetric(descSyncsTotal, prometheus.CounterValue, float64(ins.syncsTotal.Load()))
	ch <- prometheus.MustNewConstMetric(descSyncErrorsTotal, prometheus.CounterValue, float64(ins.syncErrorsTotal.Load()))
	ch <- prometheus.MustNewConstMetric(descLastSyncTimestampSecs, prometheus.GaugeValue, float64(ins.lastSyncTimestampSecs.Load()))
	ch <- prometheus.MustNewConstMetric(descLastSyncDurationSecs, prometheus.GaugeValue, time.Duration(ins.lastSyncDurationNano.Load()).Seconds())

	ch <- prometheus.MustNewConstMetric(descConfiguredRetentionSecs, prometheus.GaugeValue, ins.retention.RawSeconds, "raw")
	ch <- prometheus.MustNewConstMetric(descConfiguredRetentionSecs, prometheus.GaugeValue, ins.retention.FiveMinSec, "5m")
	ch <- prometheus.MustNewConstMetric(descConfiguredRetentionSecs, prometheus.GaugeValue, ins.retention.OneHourSec, "1h")

	ins.mu.RLock()
	blocks := slices.Collect(maps.Values(ins.blocks))
	ins.mu.RUnlock()

	ch <- prometheus.MustNewConstMetric(descCachedBlocks, prometheus.GaugeValue, float64(len(blocks)))
	if len(blocks) == 0 {
		return
	}

	nowMs := now.UnixMilli()
	storageByKey := make(map[storageKey]int64)
	blocksByKey := make(map[blockCountKey]int)
	intervalsByRes := make(map[resolutionGroupKey][]timeInterval)
	oldestByRes := make(map[resolutionGroupKey]int64)
	newestByRes := make(map[resolutionGroupKey]int64)
	activeResGroups := make(map[resolutionGroupKey]struct{})
	activeProducers := make(map[producerGroupKey]struct{})

	for _, b := range blocks {
		ageTier := classifyAgeTier(b.MinTime, b.MaxTime, nowMs)
		delStr := strconv.FormatBool(b.MarkedForDeletion)

		storageByKey[storageKey{b.TenantID, b.Producer, b.Resolution, ageTier, delStr}] += b.StorageBytes

		blocksByKey[blockCountKey{b.TenantID, b.Producer, b.Resolution, delStr}]++

		// Only active (non-deletion-marked) blocks participate in retention span, watermarks, and volume stats.
		if b.MarkedForDeletion {
			continue
		}

		resKey := resolutionGroupKey{b.TenantID, b.Producer, b.Resolution}
		activeResGroups[resKey] = struct{}{}
		pKey := producerGroupKey{b.TenantID, b.Producer}
		activeProducers[pKey] = struct{}{}

		if b.MaxTime > b.MinTime {
			intervalsByRes[resKey] = append(intervalsByRes[resKey], timeInterval{minMs: b.MinTime, maxMs: b.MaxTime, ageTier: ageTier})
		}

		if curOldest, exists := oldestByRes[resKey]; !exists || b.MinTime < curOldest {
			oldestByRes[resKey] = b.MinTime
		}
		if curNewest, exists := newestByRes[resKey]; !exists || b.MaxTime > curNewest {
			newestByRes[resKey] = b.MaxTime
		}
	}

	spannedSecByTier := make(map[tierGroupKey]float64)
	// Ensure all 5 age tiers exist for each active (tenant, producer, resolution) so Grafana stacked
	// charts and burn-rate denominators have complete label sets.
	for resKey := range activeResGroups {
		for _, tier := range allAgeTiers {
			tKey := tierGroupKey{resKey.tenantID, resKey.producer, resKey.resolution, tier}
			spannedSecByTier[tKey] = 0
			sKey := storageKey{resKey.tenantID, resKey.producer, resKey.resolution, tier, "false"}
			if _, ok := storageByKey[sKey]; !ok {
				storageByKey[sKey] = 0
			}
		}
		accumulateStreamIntervalsByTier(resKey, intervalsByRes[resKey], spannedSecByTier)
	}

	for k, val := range storageByKey {
		ch <- prometheus.MustNewConstMetric(
			descStorageBytes,
			prometheus.GaugeValue,
			float64(val),
			k.tenantID, k.producer, k.resolution, k.ageTier, k.markedForDeletion,
		)
	}

	for k, val := range blocksByKey {
		ch <- prometheus.MustNewConstMetric(
			descBlocks,
			prometheus.GaugeValue,
			float64(val),
			k.tenantID, k.producer, k.resolution, k.markedForDeletion,
		)
	}

	for k := range activeResGroups {
		ch <- prometheus.MustNewConstMetric(
			descOldestBlockTimestampSecs,
			prometheus.GaugeValue,
			float64(oldestByRes[k]/1000),
			k.tenantID, k.producer, k.resolution,
		)
		ch <- prometheus.MustNewConstMetric(
			descNewestBlockTimestampSecs,
			prometheus.GaugeValue,
			float64(newestByRes[k]/1000),
			k.tenantID, k.producer, k.resolution,
		)
	}

	type downsampleSpec struct {
		resolution   string
		defaultRatio float64
	}
	downsampleSpecs := []downsampleSpec{
		{resolution: "5m", defaultRatio: 0.5},
		{resolution: "1h", defaultRatio: 0.05},
	}

	rawRet := ins.retention.RawSeconds
	fiveMinRet := ins.retention.FiveMinSec
	oneHourRet := ins.retention.OneHourSec
	hasInfiniteRetention := rawRet <= 0 || fiveMinRet <= 0 || oneHourRet <= 0

	sortedProducers := slices.Collect(maps.Keys(activeProducers))
	slices.SortFunc(sortedProducers, func(a, b producerGroupKey) int {
		if c := cmp.Compare(a.tenantID, b.tenantID); c != 0 {
			return c
		}
		return cmp.Compare(a.producer, b.producer)
	})

	for _, pKey := range sortedProducers {
		// 1. Calculate active raw burn rate.
		// Blocks younger than 24h contain duplicate receiver replicas (e.g. RF=3) and uncompacted
		// 2h chunks before Thanos Compactor executes horizontal compaction and replica deduplication.
		// Including uncompacted <24h data inflates the steady-state burn rate by ~45-50%.
		// Therefore, we baseline the raw burn rate on the post-compaction window: 24h <= age < 7d.
		// If the non-overlapping span in [24h, 7d) is insufficient (< 1h), we fall back to older
		// post-24h blocks (age >= 24h), and only fall back to all raw blocks (including <24h)
		// if no blocks older than 24h exist (e.g. Day 1 of a newly provisioned cluster).
		newestRawMaxTimeMs := newestByRes[resolutionGroupKey{pKey.tenantID, pKey.producer, "raw"}]
		var recentCompactedBytes, olderCompactedBytes, allRawBytes int64
		var recentCompactedIntervals, olderCompactedIntervals, allRawIntervals []timeInterval

		for _, b := range blocks {
			if b.TenantID != pKey.tenantID || b.Producer != pKey.producer || b.Resolution != "raw" || b.MarkedForDeletion {
				continue
			}
			allRawBytes += b.StorageBytes
			if b.MaxTime > b.MinTime {
				allRawIntervals = append(allRawIntervals, timeInterval{minMs: b.MinTime, maxMs: b.MaxTime})
			}

			midpointMs := b.MinTime + (b.MaxTime-b.MinTime)/2
			ageFromNow := time.Duration(nowMs-midpointMs) * time.Millisecond
			ageFromNewest := time.Duration(newestRawMaxTimeMs-midpointMs) * time.Millisecond

			// Exclude uncompacted blocks younger than 24h relative to both wall-clock now and newest data.
			if ageFromNow < 24*time.Hour || ageFromNewest < 24*time.Hour {
				continue
			}

			if ageFromNow < 7*24*time.Hour {
				recentCompactedBytes += b.StorageBytes
				if b.MaxTime > b.MinTime {
					recentCompactedIntervals = append(recentCompactedIntervals, timeInterval{minMs: b.MinTime, maxMs: b.MaxTime})
				}
			} else {
				olderCompactedBytes += b.StorageBytes
				if b.MaxTime > b.MinTime {
					olderCompactedIntervals = append(olderCompactedIntervals, timeInterval{minMs: b.MinTime, maxMs: b.MaxTime})
				}
			}
		}

		var burnRateRaw float64
		recentSpan := nonOverlappingSpannedSeconds(recentCompactedIntervals)
		if recentSpan >= 3600 && recentCompactedBytes > 0 {
			burnRateRaw = float64(recentCompactedBytes) / recentSpan
		} else {
			allCompactedBytes := recentCompactedBytes + olderCompactedBytes
			allCompactedSpan := nonOverlappingSpannedSeconds(slices.Concat(recentCompactedIntervals, olderCompactedIntervals))
			if allCompactedSpan >= 3600 && allCompactedBytes > 0 {
				burnRateRaw = float64(allCompactedBytes) / allCompactedSpan
			} else {
				allRawSpan := nonOverlappingSpannedSeconds(allRawIntervals)
				if allRawSpan >= 60 && allRawBytes > 0 {
					burnRateRaw = float64(allRawBytes) / allRawSpan
				}
			}
		}

		burnRates := map[string]float64{"raw": burnRateRaw}
		ratios := make(map[string]float64, len(downsampleSpecs))

		for _, spec := range downsampleSpecs {
			res := spec.resolution
			ratio := spec.defaultRatio

			// Match overlapping age tiers where both downsampled and raw blocks exist with >=1h of span.
			var matchResBytes, matchRawBytes int64
			var matchResSpan, matchRawSpan float64
			for _, tier := range allAgeTiers {
				rSpan := spannedSecByTier[tierGroupKey{pKey.tenantID, pKey.producer, res, tier}]
				rwSpan := spannedSecByTier[tierGroupKey{pKey.tenantID, pKey.producer, "raw", tier}]
				rBytes := storageByKey[storageKey{pKey.tenantID, pKey.producer, res, tier, "false"}]
				rwBytes := storageByKey[storageKey{pKey.tenantID, pKey.producer, "raw", tier, "false"}]
				if rSpan >= 3600 && rwSpan >= 3600 && rwBytes > 0 && rBytes > 0 {
					matchResBytes += rBytes
					matchResSpan += rSpan
					matchRawBytes += rwBytes
					matchRawSpan += rwSpan
				}
			}

			if matchResSpan >= 3600 && matchRawSpan >= 3600 && matchRawBytes > 0 && matchResBytes > 0 {
				ratio = (float64(matchResBytes) / matchResSpan) / (float64(matchRawBytes) / matchRawSpan)
			} else {
				// Fallback: evaluate ratio across all tiers if downsampled blocks exist.
				var resTotalBytes int64
				var resTotalSpan float64
				for _, tier := range allAgeTiers {
					resTotalBytes += storageByKey[storageKey{pKey.tenantID, pKey.producer, res, tier, "false"}]
					resTotalSpan += spannedSecByTier[tierGroupKey{pKey.tenantID, pKey.producer, res, tier}]
				}
				if resTotalSpan >= 3600 && resTotalBytes > 0 && burnRateRaw > 0 {
					ratio = (float64(resTotalBytes) / resTotalSpan) / burnRateRaw
				}
			}

			if ratio <= 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
				ratio = spec.defaultRatio
			}
			ratios[res] = ratio
			burnRates[res] = burnRateRaw * ratio
		}

		// Fallback for burn rates if burnRateRaw was 0 but downsampled blocks exist.
		if burnRateRaw == 0 {
			for _, spec := range downsampleSpecs {
				var resTotalBytes int64
				var resTotalSpan float64
				for _, tier := range allAgeTiers {
					resTotalBytes += storageByKey[storageKey{pKey.tenantID, pKey.producer, spec.resolution, tier, "false"}]
					resTotalSpan += spannedSecByTier[tierGroupKey{pKey.tenantID, pKey.producer, spec.resolution, tier}]
				}
				if resTotalSpan >= 3600 && resTotalBytes > 0 {
					burnRates[spec.resolution] = float64(resTotalBytes) / resTotalSpan
				}
			}
		}

		for _, res := range []string{"raw", "5m", "1h"} {
			ch <- prometheus.MustNewConstMetric(
				descBurnRateBytesPerSec,
				prometheus.GaugeValue,
				burnRates[res],
				pKey.tenantID, pKey.producer, res,
			)
		}
		if !hasInfiniteRetention {
			effective5mRet := max(0, fiveMinRet-thanosDownsampleDelay5mSec)
			effective1hRet := max(0, oneHourRet-thanosDownsampleDelay1hSec)
			projectedSteadyState := burnRates["raw"]*rawRet +
				burnRates["5m"]*effective5mRet +
				burnRates["1h"]*effective1hRet
			ch <- prometheus.MustNewConstMetric(
				descProjectedSteadyStateBytes,
				prometheus.GaugeValue,
				projectedSteadyState,
				pKey.tenantID, pKey.producer,
			)
		}
	}
}
