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
// Returns 0 when either sketch is nil, when either sketch has an estimate of 0,
// or when their union estimate is 0. A non-zero baseline in both windows is required
// because churn measures turnover within an active population; metrics newly introduced,
// fully retired, or pulsed at intervals equal to or exceeding the sliding window
// (e.g. periodic 15m/1d recording rules) do not represent series turnover.
//
// If a non-nil scratch sketch is provided, it is reused for the union calculation
// to avoid heap allocations. If scratch is nil or omitted, a temporary clone of curr is allocated
// and released.
func CalculateChurnRatio(curr, prev *hll.Sketch, scratch ...*hll.Sketch) float64 {
	if curr == nil || prev == nil {
		return 0
	}
	currEst := curr.Estimate()
	prevEst := prev.Estimate()
	// Both windows must have an active population to measure turnover.
	// If either window is empty, we have no valid baseline (first-seen appearance,
	// clean metric decommission, or low-frequency periodic batch pulse).
	if currEst == 0 || prevEst == 0 {
		return 0
	}

	var union *hll.Sketch
	var autoRelease bool
	if len(scratch) > 0 && scratch[0] != nil {
		union = scratch[0]
		union.Reset()
		if err := union.Merge(curr); err != nil {
			return 0
		}
	} else {
		union = curr.Clone()
		autoRelease = true
	}
	if autoRelease {
		defer union.Release()
	}
	if err := union.Merge(prev); err != nil {
		return 0
	}
	unionEst := union.Estimate()
	if unionEst == 0 {
		return 0
	}
	churn := float64(2*unionEst) - float64(prevEst) - float64(currEst)
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
	olderAt     time.Time
	olderSketch *hll.Sketch
	newerAt     time.Time
	newerSketch *hll.Sketch
	lastSeenAt  time.Time
}

// churnTracker maintains baseline sketches across sliding intervals per group key
// so churn ratios can be computed without storing raw series IDs.
type churnTracker struct {
	mu         sync.Mutex
	interval   time.Duration
	maxEntries int
	entries    map[uint64]*churnEntry
}

func newChurnTracker(interval time.Duration, maxEntries int) *churnTracker {
	return &churnTracker{
		interval:   interval,
		maxEntries: maxEntries,
		entries:    make(map[uint64]*churnEntry),
	}
}

// updateAndRate records the current merged window sketch for groupKey at timestamp now
// and returns the resulting churn ratio in [0, 1].
func (ct *churnTracker) updateAndRate(groupKey uint64, sk, scratch *hll.Sketch, now time.Time) float64 {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	e, ok := ct.entries[groupKey]
	if !ok {
		if ct.maxEntries > 0 && len(ct.entries) >= ct.maxEntries {
			// At capacity: do not allocate additional churn sketches for burst keys.
			return 0
		}
		ct.entries[groupKey] = &churnEntry{
			olderAt:     now,
			olderSketch: sk.Clone(),
			lastSeenAt:  now,
		}
		return 0
	}

	e.lastSeenAt = now

	halfInterval := ct.interval / 2
	if e.newerSketch == nil && !e.olderAt.After(now.Add(-halfInterval)) {
		e.newerSketch = sk.Clone()
		e.newerAt = now
	} else if e.newerSketch != nil && !e.newerAt.After(now.Add(-ct.interval)) {
		recycled := e.olderSketch
		if recycled != nil {
			recycled.Reset()
			_ = recycled.Merge(sk)
		} else {
			recycled = sk.Clone()
		}
		e.olderSketch = e.newerSketch
		e.olderAt = e.newerAt
		e.newerSketch = recycled
		e.newerAt = now
	}

	if now.Sub(e.olderAt) < ct.interval {
		return 0
	}

	return CalculateChurnRatio(sk, e.olderSketch, scratch)
}

// cleanup evicts entries that have not been updated within interval.
func (ct *churnTracker) cleanup(now time.Time) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	cutoff := now.Add(-ct.interval)
	for k, e := range ct.entries {
		if e.lastSeenAt.Before(cutoff) {
			if e.olderSketch != nil {
				e.olderSketch.Release()
			}
			if e.newerSketch != nil {
				e.newerSketch.Release()
			}
			delete(ct.entries, k)
		}
	}
}

// releaseAll releases all retained HyperLogLog sketch register buffers back to the pool.
func (ct *churnTracker) releaseAll() {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	for k, e := range ct.entries {
		if e.olderSketch != nil {
			e.olderSketch.Release()
		}
		if e.newerSketch != nil {
			e.newerSketch.Release()
		}
		delete(ct.entries, k)
	}
}
