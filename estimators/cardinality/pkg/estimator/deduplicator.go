// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package estimator

import (
	"sync"
	"sync/atomic"
	"time"
)

const (
	// 16 bits per item with 4 hashes gives ~0.045% false-positive rate per bucket.
	bitsPerItem       = 16
	dedupBucketsCount = 10
	hashesCount       = 4
)

// Deduplicator implements a sharded, lock-free Bloom filter using Kirsch-Mitzenmacher
// double hashing. Fingerprints are deterministically routed to one of 10 shards via
// hash % 10, requiring only a single shard lookup per sample. One shard is cleared
// every interval/10. If a shard reaches its capacity before rotation, it switches to
// pass-through mode so Bloom filter saturation never drops genuinely new series before
// they reach the idempotent HLL sketches.
type Deduplicator struct {
	interval      time.Duration
	bucketMaxSize uint64
	bfs           [dedupBucketsCount]atomic.Pointer[bloomFilter]
	stopCh        chan struct{}
	wg            sync.WaitGroup
}

type bloomFilter struct {
	bits []uint64
	size atomic.Uint64
}

func newBloomFilter(expectedItems int) *bloomFilter {
	numBits := max(expectedItems*bitsPerItem, 64)
	numWords := (numBits + 63) / 64
	return &bloomFilter{
		bits: make([]uint64, numWords),
	}
}

// isNew checks if h is absent from the Bloom filter and sets its 4 hash bits.
// Returns true if h was newly added, or false if all 4 bits were already set.
func (bf *bloomFilter) isNew(h uint64) bool {
	numBits := uint64(len(bf.bits)) * 64
	h1 := uint32(h)       //nolint:gosec // intentional lower 32 bits for Kirsch-Mitzenmacher double hashing
	h2 := uint32(h >> 32) //nolint:gosec // intentional upper 32 bits
	allSet := true
	for i := range uint32(hashesCount) {
		bitPos := uint64(h1+i*h2) % numBits
		wordIdx := bitPos / 64
		bitMask := uint64(1) << (bitPos % 64)

		wordPtr := &bf.bits[wordIdx]
		for {
			oldWord := atomic.LoadUint64(wordPtr)
			if oldWord&bitMask != 0 {
				break
			}
			allSet = false
			if atomic.CompareAndSwapUint64(wordPtr, oldWord, oldWord|bitMask) {
				break
			}
		}
	}
	if !allSet {
		bf.size.Add(1)
	}
	return !allSet
}

// NewDeduplicator creates and starts a sharded Bloom filter deduplicator.
func NewDeduplicator(interval time.Duration, maxItemsPerInterval int) *Deduplicator {
	if maxItemsPerInterval <= 0 {
		maxItemsPerInterval = defaultMaxItems
	}
	expectedPerBucket := max(maxItemsPerInterval/dedupBucketsCount, 1)

	d := &Deduplicator{
		interval:      interval,
		bucketMaxSize: uint64(expectedPerBucket), //nolint:gosec // expectedPerBucket >= 1
		stopCh:        make(chan struct{}),
	}
	for i := range d.bfs {
		d.bfs[i].Store(newBloomFilter(expectedPerBucket))
	}

	d.wg.Add(1)
	go d.rotateLoop()
	return d
}

// Stop halts the background shard rotation goroutine.
func (d *Deduplicator) Stop() {
	close(d.stopCh)
	d.wg.Wait()
}

// CheckAndAdd returns true if hash should be forwarded to the HLL estimators (either
// because it was not seen in its shard yet, or because its shard has reached capacity
// and switched to pass-through mode). Returns false if hash is a duplicate.
func (d *Deduplicator) CheckAndAdd(hash uint64) bool {
	bf := d.bfs[hash%dedupBucketsCount].Load()
	if bf.size.Load() >= d.bucketMaxSize {
		// Pass through when saturated: HLL is idempotent, so passing duplicates
		// is harmless whereas Bloom filter false positives would under-count.
		return true
	}
	return bf.isNew(hash)
}

func (d *Deduplicator) resetShard(idx int) {
	d.bfs[idx].Store(newBloomFilter(int(d.bucketMaxSize))) //nolint:gosec // bounded by maxItemsPerInterval/10
}

func (d *Deduplicator) rotateLoop() {
	defer d.wg.Done()
	bucketDur := max(d.interval/dedupBucketsCount, 10*time.Millisecond)
	timer := time.NewTimer(bucketDur)
	defer timer.Stop()

	shardIdx := 0
	for {
		select {
		case <-d.stopCh:
			return
		case <-timer.C:
			d.resetShard(shardIdx)
			shardIdx = (shardIdx + 1) % dedupBucketsCount
			timer.Reset(bucketDur)
		}
	}
}
