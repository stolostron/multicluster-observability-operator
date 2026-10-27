// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package estimator

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stolostron/multicluster-observability-operator/estimators/cardinality/pkg/hll"
	"github.com/stolostron/multicluster-observability-operator/estimators/cardinality/pkg/protoparser"
)

var digestPool = sync.Pool{
	New: func() any {
		return xxhash.New()
	},
}

func getDigest() *xxhash.Digest {
	d := digestPool.Get().(*xxhash.Digest)
	d.Reset()
	return d
}

func putDigest(d *xxhash.Digest) {
	d.Reset()
	digestPool.Put(d)
}

// Estimator maintains a sharded sliding-window HyperLogLog state for a single StreamConfig.
type Estimator struct {
	cfg             StreamConfig
	hasLabelKeyword bool
	groupSize       *groupSize
	buckets         []*estimatorBucket
	churn           *churnTracker

	ingestedInserts atomic.Uint64

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewEstimator creates and starts a sharded sliding-window Estimator.
func NewEstimator(cfg StreamConfig) (*Estimator, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	hasLabel := len(cfg.GroupBy) > 0 && cfg.GroupBy[len(cfg.GroupBy)-1] == labelKeyword
	gs := &groupSize{
		limit:          int64(cfg.GroupLimit),
		bucketSizes:    make([]int64, cfg.Buckets),
		rejectSketches: make([]*hll.Sketch, cfg.Buckets),
	}

	e := &Estimator{
		cfg:             cfg,
		hasLabelKeyword: hasLabel,
		groupSize:       gs,
		buckets:         make([]*estimatorBucket, cfg.Buckets),
		stopCh:          make(chan struct{}),
	}
	if cfg.CalculateChurn {
		e.churn = newChurnTracker(cfg.Interval)
	}

	for i := range e.buckets {
		eb := &estimatorBucket{
			idx:       i,
			precision: cfg.HLLPrecision,
			groupBy:   cfg.GroupBy,
			groupSize: gs,
		}
		if len(cfg.GroupBy) == 0 {
			eb.sketch = hll.MustNewByPrecision(cfg.HLLPrecision)
		} else {
			eb.groups = make(map[uint64]groupSketch)
			eb.prevGroups = make(map[uint64]groupSketch)
		}
		e.buckets[i] = eb
	}

	e.wg.Add(1)
	go e.runRotation()
	return e, nil
}

// Stop halts background bucket rotation and releases all pooled HLL register buffers.
func (e *Estimator) Stop() {
	close(e.stopCh)
	e.wg.Wait()
	for _, eb := range e.buckets {
		eb.releaseAll()
	}
}

// InsertMany ingests a batch of parsed time series into the estimator's sharded HLL buckets.
func (e *Estimator) InsertMany(tss []protoparser.TimeSeries) {
	if len(tss) == 0 {
		return
	}
	bucketsNum := uint64(len(e.buckets))

	if len(e.cfg.GroupBy) == 0 {
		var cnt uint64
		for i := range tss {
			ts := &tss[i]
			if !e.matchesFilter(ts.Labels) {
				continue
			}
			bi := int(ts.Hash % bucketsNum) //nolint:gosec // modulo len(e.buckets) fits in int
			e.buckets[bi].insert(ts.Hash, 0, nil)
			cnt++
		}
		e.ingestedInserts.Add(cnt)
		return
	}

	groupByKeys := e.cfg.GroupBy
	if e.hasLabelKeyword {
		groupByKeys = e.cfg.GroupBy[:len(e.cfg.GroupBy)-1]
	}
	var groupValuesBuf [maxGroupBy]string
	groupValues := groupValuesBuf[:len(e.cfg.GroupBy)]

	d := getDigest()
	defer putDigest(d)

	var cnt uint64
	for i := range tss {
		ts := &tss[i]
		if !e.matchesFilter(ts.Labels) {
			continue
		}

		d.Reset()
		clear(groupValues)

		hasNames := len(groupByKeys) == 0
		for kIdx, labelName := range groupByKeys {
			for lIdx := range ts.Labels {
				l := &ts.Labels[lIdx]
				if l.Name == labelName {
					hasNames = true
					_, _ = d.WriteString("\x00")
					_, _ = d.WriteString(l.Value)
					groupValues[kIdx] = l.Value
					break
				}
			}
		}

		if !hasNames {
			continue
		}

		if !e.hasLabelKeyword {
			key := d.Sum64()
			bi := int(key % bucketsNum) //nolint:gosec // modulo len(e.buckets) fits in int
			e.buckets[bi].insert(ts.Hash, key, groupValues)
			cnt++
			continue
		}

		labelIdx := len(e.cfg.GroupBy) - 1
		for lIdx := range ts.Labels {
			if lIdx >= len(ts.LabelsHashes) || ts.LabelsHashes[lIdx] == 0 {
				continue
			}
			l := &ts.Labels[lIdx]
			ld := *d
			if len(groupByKeys) > 0 {
				_, _ = ld.WriteString("\x00")
			}
			_, _ = ld.WriteString(l.Name)
			groupValues[labelIdx] = l.Name

			key := ld.Sum64()
			bi := int(key % bucketsNum) //nolint:gosec // modulo len(e.buckets) fits in int
			e.buckets[bi].insert(ts.LabelsHashes[lIdx], key, groupValues)
			cnt++
		}
	}

	e.ingestedInserts.Add(cnt)
}

func (e *Estimator) matchesFilter(lbls []prompb.Label) bool {
	if len(e.cfg.matchers) == 0 {
		return true
	}
	for _, m := range e.cfg.matchers {
		val := ""
		for i := range lbls {
			if lbls[i].Name == m.Name {
				val = lbls[i].Value
				break
			}
		}
		if !m.Matches(val) {
			return false
		}
	}
	return true
}

// SnapshotAt computes a point-in-time Snapshot across all buckets at timestamp now.
func (e *Estimator) SnapshotAt(now time.Time) Snapshot {
	snap := Snapshot{
		Interval:       e.cfg.Interval,
		Filter:         e.cfg.Filter,
		GroupBy:        slices.Clone(e.cfg.GroupBy),
		GroupLimit:     e.groupSize.limit,
		CalculateChurn: e.cfg.CalculateChurn,
	}

	if len(e.cfg.GroupBy) == 0 {
		resSK := hll.MustNewByPrecision(e.cfg.HLLPrecision)
		defer resSK.Release()

		for _, eb := range e.buckets {
			eb.mu.Lock()
			mergeInto(resSK, eb.sketch, eb.prevSketch)
			eb.mu.Unlock()
		}
		snap.GlobalEstimate = resSK.Estimate()
		if e.churn != nil {
			snap.GlobalChurn = e.churn.updateAndRate(0, resSK, now)
		}
		return snap
	}

	const batchSize = 1000
	keys := make([]uint64, 0, batchSize)
	resSK := hll.MustNewByPrecision(e.cfg.HLLPrecision)
	defer resSK.Release()

	for _, eb := range e.buckets {
		eb.mu.Lock()
		keys = keys[:0]
		for k := range eb.groups {
			keys = append(keys, k)
		}
		for k := range eb.prevGroups {
			if _, ok := eb.groups[k]; !ok {
				keys = append(keys, k)
			}
		}
		eb.mu.Unlock()

		for i := 0; i < len(keys); i += batchSize {
			batch := keys[i:min(i+batchSize, len(keys))]

			eb.mu.Lock()
			for _, key := range batch {
				gsk, hasCur := eb.groups[key]
				prevGSK, hasPrev := eb.prevGroups[key]
				if !hasCur && !hasPrev {
					continue
				}

				values := gsk.values
				if len(values) == 0 {
					values = prevGSK.values
				}

				var activeSK *hll.Sketch
				switch {
				case gsk.Sketch != nil && prevGSK.Sketch == nil:
					activeSK = gsk.Sketch
				case gsk.Sketch == nil && prevGSK.Sketch != nil:
					activeSK = prevGSK.Sketch
				default:
					resSK.Reset()
					mergeInto(resSK, gsk.Sketch, prevGSK.Sketch)
					activeSK = resSK
				}

				est := activeSK.Estimate()

				var churnRate float64
				if e.churn != nil {
					churnRate = e.churn.updateAndRate(key, activeSK, now)
				}

				if est < e.cfg.MinCardinality {
					snap.DroppedCount++
					continue
				}

				snap.Groups = append(snap.Groups, GroupEstimate{
					Values:     values,
					Estimate:   est,
					ChurnRatio: churnRate,
				})
			}
			eb.mu.Unlock()
		}
	}

	if e.churn != nil {
		e.churn.cleanup(now)
	}
	snap.GroupSize = e.groupSize.totalSize()
	return snap
}

func (e *Estimator) runRotation() {
	defer e.wg.Done()
	bucketInterval := max(e.cfg.Interval/2/time.Duration(len(e.buckets)), 50*time.Millisecond)
	ticker := time.NewTicker(bucketInterval)
	defer ticker.Stop()

	bucketIdx := 0
	for {
		select {
		case <-e.stopCh:
			return
		case <-ticker.C:
			e.buckets[bucketIdx].rotate()
			bucketIdx = (bucketIdx + 1) % len(e.buckets)
		}
	}
}

func mergeInto(dst, cur, prev *hll.Sketch) {
	if cur != nil {
		if err := dst.Merge(cur); err != nil {
			panic(fmt.Sprintf("BUG: cannot merge current sketch: %v", err))
		}
	}
	if prev != nil {
		if err := dst.Merge(prev); err != nil {
			panic(fmt.Sprintf("BUG: cannot merge previous sketch: %v", err))
		}
	}
}

type groupSketch struct {
	*hll.Sketch
	values []string
}

type estimatorBucket struct {
	mu        sync.Mutex
	idx       int
	precision uint8
	groupBy   []string
	groupSize *groupSize

	sketch     *hll.Sketch
	prevSketch *hll.Sketch

	groups     map[uint64]groupSketch
	prevGroups map[uint64]groupSketch
}

func (eb *estimatorBucket) insert(fp uint64, groupValuesKey uint64, groupValues []string) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if len(eb.groupBy) == 0 {
		eb.sketch.InsertHash(fp)
		return
	}

	gsk, ok := eb.groups[groupValuesKey]
	if !ok {
		var values []string
		if prevGSK, exists := eb.prevGroups[groupValuesKey]; exists {
			values = prevGSK.values
		} else if !eb.groupSize.allowInsertLocked(eb.idx, groupValuesKey) {
			return
		}

		if values == nil {
			values = make([]string, len(groupValues))
			for i, v := range groupValues {
				values[i] = strings.Clone(v)
			}
		}
		gsk = groupSketch{
			Sketch: hll.MustNewByPrecision(eb.precision),
			values: values,
		}
		eb.groups[groupValuesKey] = gsk
	}
	gsk.InsertHash(fp)
}

func (eb *estimatorBucket) rotate() {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if len(eb.groupBy) == 0 {
		if eb.prevSketch != nil {
			eb.prevSketch.Release()
		}
		eb.prevSketch = eb.sketch
		eb.sketch = hll.MustNewByPrecision(eb.precision)
		return
	}

	for _, old := range eb.prevGroups {
		if old.Sketch != nil {
			old.Release()
		}
	}
	eb.prevGroups = eb.groups
	eb.groups = make(map[uint64]groupSketch, len(eb.prevGroups))
	eb.groupSize.rotateLocked(eb.idx, int64(len(eb.prevGroups)))
}

func (eb *estimatorBucket) releaseAll() {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if eb.sketch != nil {
		eb.sketch.Release()
		eb.sketch = nil
	}
	if eb.prevSketch != nil {
		eb.prevSketch.Release()
		eb.prevSketch = nil
	}
	for _, g := range eb.groups {
		if g.Sketch != nil {
			g.Release()
		}
	}
	for _, g := range eb.prevGroups {
		if g.Sketch != nil {
			g.Release()
		}
	}
	eb.groups = nil
	eb.prevGroups = nil
}

type groupSize struct {
	limit       int64
	size        atomic.Int64
	bucketSizes []int64

	rejectMu       sync.Mutex
	rejectSketches []*hll.Sketch
}

func (gs *groupSize) allowInsertLocked(bucketIdx int, groupValuesKey uint64) bool {
	if gs.size.Load() >= gs.limit {
		gs.rejectMu.Lock()
		sk := gs.rejectSketches[bucketIdx]
		if sk == nil {
			sk = hll.MustNewByPrecision(hll.MinPrecision)
			gs.rejectSketches[bucketIdx] = sk
		}
		sk.InsertHash(groupValuesKey)
		gs.rejectMu.Unlock()
		return false
	}

	gs.bucketSizes[bucketIdx]++
	gs.size.Add(1)
	return true
}

func (gs *groupSize) rotateLocked(bucketIdx int, newSize int64) {
	if diff := gs.bucketSizes[bucketIdx] - newSize; diff > 0 {
		gs.bucketSizes[bucketIdx] -= diff
		gs.size.Add(-diff)
	}

	gs.rejectMu.Lock()
	if gs.rejectSketches[bucketIdx] != nil {
		gs.rejectSketches[bucketIdx].Release()
		gs.rejectSketches[bucketIdx] = nil
	}
	gs.rejectMu.Unlock()
}

func (gs *groupSize) totalSize() int64 {
	size := gs.size.Load()
	if size >= int64(float64(gs.limit)*0.8) {
		var rejectSize uint64
		gs.rejectMu.Lock()
		for _, sk := range gs.rejectSketches {
			if sk != nil {
				rejectSize += sk.Estimate()
			}
		}
		gs.rejectMu.Unlock()
		size += int64(rejectSize) //nolint:gosec // HLL estimate is bounded well below math.MaxInt64
	}
	return size
}

// intervalPipeline groups a Deduplicator and all Estimators that share the same sliding interval.
type intervalPipeline struct {
	interval   time.Duration
	dedup      *Deduplicator
	estimators []*Estimator
}

// Engine coordinates Bloom filter deduplication and HLL estimation across all configured streams.
type Engine struct {
	mu          sync.RWMutex
	pipelines   []*intervalPipeline
	fingerprint bool

	ingestedSeriesTotal     atomic.Uint64
	deduplicatedSeriesTotal atomic.Uint64
}

// NewEngine creates and starts an Engine from the validated Config.
func NewEngine(cfg *Config) (*Engine, error) {
	eng := &Engine{}
	if err := eng.Reload(cfg); err != nil {
		return nil, err
	}
	return eng, nil
}

// Reload atomically replaces the active pipelines with those defined in cfg.
func (eng *Engine) Reload(cfg *Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	byInterval := make(map[time.Duration][]*Estimator)
	var intervals []time.Duration
	for _, sc := range cfg.Streams {
		est, err := NewEstimator(sc)
		if err != nil {
			for _, list := range byInterval {
				for _, created := range list {
					created.Stop()
				}
			}
			return err
		}
		if _, exists := byInterval[sc.Interval]; !exists {
			intervals = append(intervals, sc.Interval)
		}
		byInterval[sc.Interval] = append(byInterval[sc.Interval], est)
	}

	newPipelines := make([]*intervalPipeline, 0, len(intervals))
	for _, iv := range intervals {
		newPipelines = append(newPipelines, &intervalPipeline{
			interval:   iv,
			dedup:      NewDeduplicator(iv, cfg.MaxItemsPerInterval),
			estimators: byInterval[iv],
		})
	}

	eng.mu.Lock()
	oldPipelines := eng.pipelines
	eng.pipelines = newPipelines
	eng.fingerprint = cfg.HasLabelKeyword()
	eng.mu.Unlock()

	stopPipelines(oldPipelines)
	return nil
}

// Stop halts all active Deduplicators and Estimators.
func (eng *Engine) Stop() {
	eng.mu.Lock()
	old := eng.pipelines
	eng.pipelines = nil
	eng.mu.Unlock()

	stopPipelines(old)
}

func stopPipelines(pipelines []*intervalPipeline) {
	for _, p := range pipelines {
		p.dedup.Stop()
		for _, est := range p.estimators {
			est.Stop()
		}
	}
}

// FingerprintLabels reports whether the parser should compute individual label fingerprints.
func (eng *Engine) FingerprintLabels() bool {
	eng.mu.RLock()
	defer eng.mu.RUnlock()
	return eng.fingerprint
}

// Ingest deduplicates and routes a batch of TimeSeries across all configured streams.
func (eng *Engine) Ingest(tss []protoparser.TimeSeries) {
	if len(tss) == 0 {
		return
	}
	eng.ingestedSeriesTotal.Add(uint64(len(tss)))

	eng.mu.RLock()
	defer eng.mu.RUnlock()

	for _, p := range eng.pipelines {
		// Filter out series already seen in this interval's Bloom filter.
		// Lazily allocate filtered so steady-state duplicate batches make 0 heap allocations.
		var filtered []protoparser.TimeSeries
		var deduped uint64
		for i := range tss {
			if p.dedup.CheckAndAdd(tss[i].Hash) {
				if filtered == nil {
					filtered = make([]protoparser.TimeSeries, 0, len(tss)/4+1)
				}
				filtered = append(filtered, tss[i])
			} else {
				deduped++
			}
		}
		eng.deduplicatedSeriesTotal.Add(deduped)
		if len(filtered) == 0 {
			continue
		}
		for _, est := range p.estimators {
			est.InsertMany(filtered)
		}
	}
}

// Snapshots evaluates and returns point-in-time Snapshots for all active streams.
func (eng *Engine) Snapshots(now time.Time) []Snapshot {
	eng.mu.RLock()
	defer eng.mu.RUnlock()

	total := 0
	for _, p := range eng.pipelines {
		total += len(p.estimators)
	}
	out := make([]Snapshot, 0, total)
	for _, p := range eng.pipelines {
		for _, est := range p.estimators {
			out = append(out, est.SnapshotAt(now))
		}
	}
	return out
}

// WritePrometheusText writes all stream snapshots in Prometheus text exposition format.
func (eng *Engine) WritePrometheusText(w io.Writer, now time.Time) error {
	snaps := eng.Snapshots(now)
	for i := range snaps {
		if err := snaps[i].WritePrometheusText(w); err != nil {
			return err
		}
	}
	return nil
}

// IngestedSeriesTotal returns the total number of time series passed to Ingest.
func (eng *Engine) IngestedSeriesTotal() uint64 {
	return eng.ingestedSeriesTotal.Load()
}

// DeduplicatedSeriesTotal returns the total number of time series dropped by the Bloom filter.
func (eng *Engine) DeduplicatedSeriesTotal() uint64 {
	return eng.deduplicatedSeriesTotal.Load()
}
