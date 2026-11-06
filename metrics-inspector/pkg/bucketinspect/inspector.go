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
	indexFilename        = "index"
	chunksDirname        = "chunks"

	msPerDay = 86400 * 1000

	ageTier0To7d     = "0_to_7d"
	ageTier7dTo30d   = "7d_to_30d"
	ageTier30dTo90d  = "30d_to_90d"
	ageTier90dTo180d = "90d_to_180d"
	ageTier180dPlus  = "180d_plus"

	fileTypeChunks = "chunks"
	fileTypeIndex  = "index"
	fileTypeMeta   = "meta"
	fileTypeOther  = "other"
)

var allAgeTiers = []string{
	ageTier0To7d,
	ageTier7dTo30d,
	ageTier30dTo90d,
	ageTier90dTo180d,
	ageTier180dPlus,
}

var allFileTypes = []string{
	fileTypeChunks,
	fileTypeIndex,
	fileTypeMeta,
	fileTypeOther,
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
	ChunkBytes        int64
	IndexBytes        int64
	MetaBytes         int64
	OtherBytes        int64
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
	SyncInterval time.Duration
	Retention    RetentionConfig
	Concurrency  int
	Logger       *slog.Logger
}

// Inspector periodically scans a Thanos object storage bucket using an incremental
// in-memory ULID cache and exposes raw storage primitives in Prometheus text format.
type Inspector struct {
	bkt         objstore.BucketReader
	interval    time.Duration
	retention   RetentionConfig
	concurrency int
	logger      *slog.Logger

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
	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = defaultMetaFetchConcurrency
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Inspector{
		bkt:         bkt,
		interval:    interval,
		retention:   cfg.Retention,
		concurrency: concurrency,
		logger:      logger,
		blocks:      make(map[string]BlockSummary),
	}
}

// Run executes an initial bucket sync immediately and then syncs on every interval tick
// until ctx is canceled. Object storage errors are logged and counted without crashing.
func (ins *Inspector) Run(ctx context.Context) {
	if ins.bkt == nil {
		return
	}
	if err := ins.Sync(ctx); err != nil && ctx.Err() == nil {
		ins.logger.Warn("initial thanos bucket inspection failed", "err", err)
	}

	ticker := time.NewTicker(ins.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := ins.Sync(ctx); err != nil && ctx.Err() == nil {
				ins.logger.Warn("periodic thanos bucket inspection failed", "err", err)
			}
		}
	}
}

type discoveredBlockFiles struct {
	hasMeta     bool
	hasDeletion bool
	relPaths    []string
}

// Sync performs a single incremental inspection pass against the Thanos object storage bucket.
func (ins *Inspector) Sync(ctx context.Context) error {
	if ins.bkt == nil {
		return nil
	}
	start := time.Now()
	ins.syncsTotal.Add(1)

	// Group recursively listed object keys by top-level <ULID>.
	discovered := make(map[string]*discoveredBlockFiles)
	err := ins.bkt.Iter(ctx, "", func(name string) error {
		ulidStr, relPath, ok := splitBlockKey(name)
		if !ok {
			return nil
		}
		entry := discovered[ulidStr]
		if entry == nil {
			entry = &discoveredBlockFiles{}
			discovered[ulidStr] = entry
		}
		entry.relPaths = append(entry.relPaths, relPath)
		switch relPath {
		case metaFilename:
			entry.hasMeta = true
		case deletionMarkFilename:
			entry.hasDeletion = true
		}
		return nil
	}, objstore.WithRecursiveIter())
	if err != nil {
		ins.syncErrorsTotal.Add(1)
		ins.lastSyncDurationNano.Store(time.Since(start).Nanoseconds())
		return fmt.Errorf("failed to list bucket objects: %w", err)
	}

	// Identify which ULIDs have a valid meta.json and are not yet cached.
	ins.mu.RLock()
	var missingULIDs []string
	for ulidStr, files := range discovered {
		if !files.hasMeta {
			continue
		}
		if _, cached := ins.blocks[ulidStr]; !cached {
			missingULIDs = append(missingULIDs, ulidStr)
		}
	}
	ins.mu.RUnlock()
	slices.Sort(missingULIDs)

	// Fetch meta.json only for newly discovered ULIDs using a bounded worker pool.
	fetched := make(map[string]BlockSummary, len(missingULIDs))
	var fetchMu sync.Mutex
	var firstFetchErr error
	var fetchErrors int

	if len(missingULIDs) > 0 {
		sem := make(chan struct{}, ins.concurrency)
		var wg sync.WaitGroup
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
					fetchMu.Lock()
					fetchErrors++
					if firstFetchErr == nil {
						firstFetchErr = err
					}
					fetchMu.Unlock()
					return
				}
				files := discovered[ulidStr]
				summary, err := parseBlockSummary(ctx, ins.bkt, ulidStr, rawMeta, files)
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
		wg.Wait()
	}

	// Reconcile the in-memory ULID cache:
	// 1. Evict ULIDs no longer present in the bucket.
	// 2. Insert newly fetched ULIDs.
	// 3. Refresh MarkedForDeletion on all present ULIDs.
	ins.mu.Lock()
	for existingULID := range ins.blocks {
		files, exists := discovered[existingULID]
		if !exists || !files.hasMeta {
			delete(ins.blocks, existingULID)
		}
	}
	maps.Copy(ins.blocks, fetched)
	for ulidStr, summary := range ins.blocks {
		if files, ok := discovered[ulidStr]; ok {
			summary.MarkedForDeletion = files.hasDeletion
			ins.blocks[ulidStr] = summary
		}
	}
	cachedCount := len(ins.blocks)
	ins.mu.Unlock()

	duration := time.Since(start)
	ins.lastSyncDurationNano.Store(duration.Nanoseconds())

	if fetchErrors > 0 {
		ins.syncErrorsTotal.Add(1)
		return fmt.Errorf("failed to fetch %d/%d new block meta.json files (first error: %w)", fetchErrors, len(missingULIDs), firstFetchErr)
	}

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

// splitBlockKey checks whether key has the form "<ULID>/<relPath>" where <ULID> is a valid
// 26-character Crockford Base32 ULID.
func splitBlockKey(key string) (ulidStr, relPath string, ok bool) {
	trimmed := strings.TrimPrefix(key, "/")
	slashIdx := strings.IndexByte(trimmed, '/')
	if slashIdx != 26 {
		return "", "", false
	}
	candidate := trimmed[:slashIdx]
	rest := trimmed[slashIdx+1:]
	if rest == "" {
		return "", "", false
	}
	if _, err := ulid.ParseStrict(candidate); err != nil {
		return "", "", false
	}
	return candidate, rest, true
}

func parseBlockSummary(
	ctx context.Context,
	bkt objstore.BucketReader,
	ulidStr string,
	rawMeta []byte,
	listed *discoveredBlockFiles,
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

	var chunkBytes, indexBytes, metaBytes, otherBytes int64
	var hasMetaSize bool
	for _, f := range meta.Thanos.Files {
		if f.SizeBytes <= 0 {
			continue
		}
		if f.RelPath == metaFilename {
			hasMetaSize = true
		}
		classifyFileSize(f.RelPath, f.SizeBytes, &chunkBytes, &indexBytes, &metaBytes, &otherBytes)
	}

	// Fall back to querying object attributes when thanos.files is unpopulated (e.g. older TSDB blocks).
	var markedForDeletion bool
	if listed != nil {
		markedForDeletion = listed.hasDeletion
		if chunkBytes == 0 && indexBytes == 0 && bkt != nil {
			for _, relPath := range listed.relPaths {
				if relPath == metaFilename {
					continue
				}
				attrs, err := bkt.Attributes(ctx, ulidStr+"/"+relPath)
				if err != nil || attrs.Size <= 0 {
					continue
				}
				classifyFileSize(relPath, attrs.Size, &chunkBytes, &indexBytes, &metaBytes, &otherBytes)
			}
		}
	}
	if !hasMetaSize {
		metaBytes += int64(len(rawMeta))
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
		ChunkBytes:        chunkBytes,
		IndexBytes:        indexBytes,
		MetaBytes:         metaBytes,
		OtherBytes:        otherBytes,
		MarkedForDeletion: markedForDeletion,
	}, nil
}

func classifyFileSize(relPath string, size int64, chunkBytes, indexBytes, metaBytes, otherBytes *int64) {
	switch {
	case strings.HasPrefix(relPath, chunksDirname+"/"):
		*chunkBytes += size
	case relPath == indexFilename:
		*indexBytes += size
	case relPath == metaFilename || relPath == deletionMarkFilename:
		*metaBytes += size
	default:
		*otherBytes += size
	}
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
// sum by (tenant_id, producer, resolution) (thanos_bucket_spanned_seconds) equals the
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

type storageKey struct {
	tenantID          string
	producer          string
	resolution        string
	fileType          string
	ageTier           string
	markedForDeletion string
}

type blockCountKey struct {
	tenantID          string
	producer          string
	resolution        string
	compactionLevel   int
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
		"Total bytes stored in the Thanos bucket by tenant, producer, resolution, file type, age tier, and deletion mark status.",
		[]string{"tenant_id", "producer", "resolution", "file_type", "age_tier", "marked_for_deletion"}, nil,
	)
	descBlocks = prometheus.NewDesc(
		"thanos_bucket_blocks",
		"Number of TSDB blocks in the Thanos bucket by tenant, producer, resolution, compaction level, and deletion mark status.",
		[]string{"tenant_id", "producer", "resolution", "compaction_level", "marked_for_deletion"}, nil,
	)
	descSpannedSeconds = prometheus.NewDesc(
		"thanos_bucket_spanned_seconds",
		"Non-overlapping time span in seconds covered by active TSDB blocks in each age tier.",
		[]string{"tenant_id", "producer", "resolution", "age_tier"}, nil,
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
	descSamples = prometheus.NewDesc(
		"thanos_bucket_samples",
		"Total samples stored across active TSDB blocks.",
		[]string{"tenant_id", "producer", "resolution"}, nil,
	)
	descChunks = prometheus.NewDesc(
		"thanos_bucket_chunks",
		"Total chunks stored across active TSDB blocks.",
		[]string{"tenant_id", "producer", "resolution"}, nil,
	)
	descRecentBlockSeries = prometheus.NewDesc(
		"thanos_bucket_recent_block_series",
		"Series count reported in meta.json of the most recent active raw TSDB block.",
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
	ch <- descSpannedSeconds
	ch <- descOldestBlockTimestampSecs
	ch <- descNewestBlockTimestampSecs
	ch <- descSamples
	ch <- descChunks
	ch <- descRecentBlockSeries
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
	samplesByRes := make(map[resolutionGroupKey]uint64)
	chunksByRes := make(map[resolutionGroupKey]uint64)

	type recentCandidate struct {
		level1    bool
		maxTime   int64
		numSeries uint64
	}
	recentByProducer := make(map[producerGroupKey]recentCandidate)
	activeResGroups := make(map[resolutionGroupKey]struct{})

	for _, b := range blocks {
		ageTier := classifyAgeTier(b.MinTime, b.MaxTime, nowMs)
		delStr := strconv.FormatBool(b.MarkedForDeletion)

		storageByKey[storageKey{b.TenantID, b.Producer, b.Resolution, fileTypeChunks, ageTier, delStr}] += b.ChunkBytes
		storageByKey[storageKey{b.TenantID, b.Producer, b.Resolution, fileTypeIndex, ageTier, delStr}] += b.IndexBytes
		storageByKey[storageKey{b.TenantID, b.Producer, b.Resolution, fileTypeMeta, ageTier, delStr}] += b.MetaBytes
		if b.OtherBytes > 0 {
			storageByKey[storageKey{b.TenantID, b.Producer, b.Resolution, fileTypeOther, ageTier, delStr}] += b.OtherBytes
		}

		blocksByKey[blockCountKey{b.TenantID, b.Producer, b.Resolution, b.CompactionLevel, delStr}]++

		// Only active (non-deletion-marked) blocks participate in retention span, watermarks, and volume stats.
		if b.MarkedForDeletion {
			continue
		}

		resKey := resolutionGroupKey{b.TenantID, b.Producer, b.Resolution}
		activeResGroups[resKey] = struct{}{}

		if b.MaxTime > b.MinTime {
			intervalsByRes[resKey] = append(intervalsByRes[resKey], timeInterval{minMs: b.MinTime, maxMs: b.MaxTime, ageTier: ageTier})
		}

		if curOldest, exists := oldestByRes[resKey]; !exists || b.MinTime < curOldest {
			oldestByRes[resKey] = b.MinTime
		}
		if curNewest, exists := newestByRes[resKey]; !exists || b.MaxTime > curNewest {
			newestByRes[resKey] = b.MaxTime
		}

		samplesByRes[resKey] += b.NumSamples
		chunksByRes[resKey] += b.NumChunks

		// Prefer CompactionLevel == 1 (uncompacted 2h raw blocks) so thanos_bucket_recent_block_series
		// reflects a single 2h block's series count rather than a multi-day compacted block's union.
		// Fall back to CompactionLevel > 1 only if no level-1 raw block exists for that producer.
		if b.Resolution == "raw" {
			pKey := producerGroupKey{b.TenantID, b.Producer}
			isLevel1 := b.CompactionLevel == 1
			cur, exists := recentByProducer[pKey]
			if !exists ||
				(isLevel1 && !cur.level1) ||
				(isLevel1 == cur.level1 && (b.MaxTime > cur.maxTime || (b.MaxTime == cur.maxTime && b.NumSeries > cur.numSeries))) {
				recentByProducer[pKey] = recentCandidate{level1: isLevel1, maxTime: b.MaxTime, numSeries: b.NumSeries}
			}
		}
	}

	spannedSecByTier := make(map[tierGroupKey]float64)
	// Ensure all 5 age tiers exist for each active (tenant, producer, resolution) so Grafana stacked
	// charts and burn-rate denominators have complete label sets.
	for resKey := range activeResGroups {
		for _, tier := range allAgeTiers {
			tKey := tierGroupKey{resKey.tenantID, resKey.producer, resKey.resolution, tier}
			spannedSecByTier[tKey] = 0
			for _, ft := range allFileTypes[:3] { // chunks, index, meta
				sKey := storageKey{resKey.tenantID, resKey.producer, resKey.resolution, ft, tier, "false"}
				if _, ok := storageByKey[sKey]; !ok {
					storageByKey[sKey] = 0
				}
			}
		}
		accumulateStreamIntervalsByTier(resKey, intervalsByRes[resKey], spannedSecByTier)
	}

	for k, val := range storageByKey {
		ch <- prometheus.MustNewConstMetric(
			descStorageBytes,
			prometheus.GaugeValue,
			float64(val),
			k.tenantID, k.producer, k.resolution, k.fileType, k.ageTier, k.markedForDeletion,
		)
	}

	for k, val := range blocksByKey {
		ch <- prometheus.MustNewConstMetric(
			descBlocks,
			prometheus.GaugeValue,
			float64(val),
			k.tenantID, k.producer, k.resolution, strconv.Itoa(k.compactionLevel), k.markedForDeletion,
		)
	}

	for k, spannedSec := range spannedSecByTier {
		ch <- prometheus.MustNewConstMetric(
			descSpannedSeconds,
			prometheus.GaugeValue,
			math.Round(spannedSec),
			k.tenantID, k.producer, k.resolution, k.ageTier,
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
		ch <- prometheus.MustNewConstMetric(
			descSamples,
			prometheus.GaugeValue,
			float64(samplesByRes[k]),
			k.tenantID, k.producer, k.resolution,
		)
		ch <- prometheus.MustNewConstMetric(
			descChunks,
			prometheus.GaugeValue,
			float64(chunksByRes[k]),
			k.tenantID, k.producer, k.resolution,
		)
	}

	for k, cand := range recentByProducer {
		ch <- prometheus.MustNewConstMetric(
			descRecentBlockSeries,
			prometheus.GaugeValue,
			float64(cand.numSeries),
			k.tenantID, k.producer,
		)
	}
}
