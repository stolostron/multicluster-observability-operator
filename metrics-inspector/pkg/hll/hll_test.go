// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package hll

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"math"
	"testing"

	"github.com/cespare/xxhash/v2"
)

func hashInt(i int) uint64 {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(i))
	return xxhash.Sum64(b[:])
}

func TestSketchEstimateAndPromotion(t *testing.T) {
	for _, tc := range []struct {
		name      string
		precision uint8
		count     int
		wantSpars bool
		maxErrPct float64
	}{
		{name: "empty", precision: 14, count: 0, wantSpars: true, maxErrPct: 0},
		{name: "small-sparse-p14", precision: 14, count: 100, wantSpars: true, maxErrPct: 3.0},
		{name: "medium-dense-p14", precision: 14, count: 10000, wantSpars: false, maxErrPct: 2.5},
		{name: "large-dense-p14", precision: 14, count: 100000, wantSpars: false, maxErrPct: 2.0},
		{name: "small-sparse-p12", precision: 12, count: 50, wantSpars: true, maxErrPct: 4.0},
		{name: "medium-dense-p12", precision: 12, count: 25000, wantSpars: false, maxErrPct: 4.0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := NewByPrecision(tc.precision)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			defer s.Release()

			for i := range tc.count {
				// Insert each element twice to verify idempotency.
				h := hashInt(i)
				s.InsertHash(h)
				s.InsertHash(h)
			}

			if s.IsSparse() != tc.wantSpars {
				t.Fatalf("IsSparse() = %v, want %v", s.IsSparse(), tc.wantSpars)
			}

			got := s.Estimate()
			if tc.count == 0 {
				if got != 0 {
					t.Fatalf("Estimate() = %d, want 0", got)
				}
				return
			}

			errPct := math.Abs(float64(got)-float64(tc.count)) / float64(tc.count) * 100
			if errPct > tc.maxErrPct {
				t.Fatalf("Estimate() = %d for count %d (err %.2f%% > max %.2f%%)", got, tc.count, errPct, tc.maxErrPct)
			}
		})
	}
}

func TestSketchMergeAndGob(t *testing.T) {
	s1, err := NewByPrecision(14)
	if err != nil {
		t.Fatal(err)
	}
	defer s1.Release()

	s2, err := NewByPrecision(14)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Release()

	// Insert [0, 5000) into s1 and [2500, 7500) into s2 -> union is [0, 7500) = 7500 unique items.
	for i := range 5000 {
		s1.InsertHash(hashInt(i))
	}
	for i := 2500; i < 7500; i++ {
		s2.InsertHash(hashInt(i))
	}

	clone := s1.Clone()
	defer clone.Release()
	if err := clone.Merge(s2); err != nil {
		t.Fatalf("Merge failed: %v", err)
	}

	est := clone.Estimate()
	errPct := math.Abs(float64(est)-7500) / 7500 * 100
	if errPct > 2.5 {
		t.Fatalf("merged Estimate() = %d, want ~7500 (err %.2f%%)", est, errPct)
	}

	// Verify Gob round-trip.
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(clone); err != nil {
		t.Fatalf("gob encode failed: %v", err)
	}
	var decoded Sketch
	if err := gob.NewDecoder(&buf).Decode(&decoded); err != nil {
		t.Fatalf("gob decode failed: %v", err)
	}
	defer decoded.Release()

	if decoded.Estimate() != est {
		t.Fatalf("decoded Estimate() = %d, want %d", decoded.Estimate(), est)
	}
}
