// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package estimator

import (
	"sync"
	"time"

	"github.com/stolostron/multicluster-observability-operator/metrics-inspector/pkg/hll"
)

// CalculateChurnRatio computes the symmetric churn rate in [0, 1] between two
// HyperLogLog sketches using the Inclusion-Exclusion Principle:
//
//	|A Δ B| / |A ∪ B| = (2*|A ∪ B| - |A| - |B|) / |A ∪ B|
//
// Returns 0 when either sketch is nil or when their union estimate is 0.
func CalculateChurnRatio(curr, prev *hll.Sketch) float64 {
	if curr == nil || prev == nil {
		return 0
	}
	union := curr.Clone()
	defer union.Release()
	if err := union.Merge(prev); err != nil {
		return 0
	}
	unionEst := union.Estimate()
	if unionEst == 0 {
		return 0
	}
	churn := float64(2*unionEst) - float64(prev.Estimate()) - float64(curr.Estimate())
	rate := churn / float64(unionEst)
	if rate < 0 {
		return 0
	}
	if rate > 1 {
		return 1
	}
	return rate
}

type churnEntry struct {
	prevAddedAt time.Time
	prevSketch  *hll.Sketch
	currAddedAt time.Time
	currSketch  *hll.Sketch
}

// churnTracker maintains baseline sketches across sliding intervals per group key
// so churn ratios can be computed without storing raw series IDs.
type churnTracker struct {
	mu       sync.Mutex
	interval time.Duration
	entries  map[uint64]*churnEntry
}

func newChurnTracker(interval time.Duration) *churnTracker {
	return &churnTracker{
		interval: interval,
		entries:  make(map[uint64]*churnEntry),
	}
}

// updateAndRate records the current merged window sketch for groupKey at timestamp now
// and returns the resulting churn ratio in [0, 1].
func (ct *churnTracker) updateAndRate(groupKey uint64, sk *hll.Sketch, now time.Time) float64 {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	e, ok := ct.entries[groupKey]
	if !ok {
		ct.entries[groupKey] = &churnEntry{
			prevAddedAt: now,
			prevSketch:  nil,
			currAddedAt: now,
			currSketch:  sk.Clone(),
		}
		return 0
	}

	if !e.prevAddedAt.After(now.Add(-ct.interval)) {
		// Reuse the old prevSketch buffer as the new currSketch if available to avoid heap allocations.
		nextCurr := e.prevSketch
		if nextCurr != nil {
			nextCurr.Reset()
			_ = nextCurr.Merge(sk)
		} else {
			nextCurr = sk.Clone()
		}
		e.prevAddedAt = now
		e.prevSketch = e.currSketch
		e.currAddedAt = now
		e.currSketch = nextCurr
	} else {
		e.currAddedAt = now
		if e.currSketch != nil {
			e.currSketch.Reset()
			_ = e.currSketch.Merge(sk)
		} else {
			e.currSketch = sk.Clone()
		}
	}

	return CalculateChurnRatio(e.currSketch, e.prevSketch)
}

// cleanup evicts entries that have not been updated within 2 * interval.
func (ct *churnTracker) cleanup(now time.Time) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	cutoff := now.Add(-2 * ct.interval)
	for k, e := range ct.entries {
		if e.currAddedAt.Before(cutoff) {
			if e.prevSketch != nil {
				e.prevSketch.Release()
			}
			if e.currSketch != nil {
				e.currSketch.Release()
			}
			delete(ct.entries, k)
		}
	}
}
