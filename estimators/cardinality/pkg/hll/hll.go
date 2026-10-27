// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package hll

import (
	"fmt"
	"math"
	"math/bits"
	"slices"
	"sync"
)

const (
	// MinPrecision is the minimum supported HyperLogLog register precision (2^10 = 1024 registers).
	MinPrecision = 10
	// MaxPrecision is the maximum supported HyperLogLog register precision (2^18 = 262144 registers).
	MaxPrecision = 18
	// DefaultPrecision is the default HyperLogLog precision (2^14 = 16384 registers, ~0.81% error).
	DefaultPrecision = 14
)

var registerPools [MaxPrecision + 1]sync.Pool

func getRegisters(p uint8) []uint8 {
	m := 1 << p
	if v := registerPools[p].Get(); v != nil {
		regsPtr := v.(*[]uint8)
		regs := *regsPtr
		clear(regs)
		return regs
	}
	return make([]uint8, m)
}

func putRegisters(p uint8, regs []uint8) {
	if p >= MinPrecision && p <= MaxPrecision && len(regs) == (1<<p) {
		registerPools[p].Put(&regs)
	}
}

// Sketch implements a HyperLogLog cardinality estimator with sparse-to-dense
// promotion and pooled dense register buffers to minimize GC pressure at scale.
type Sketch struct {
	precision uint8
	sparse    bool
	// sparseList stores packed (registerIndex << 6) | rho values while sparse is true.
	// Using uint32 supports up to precision 18 (18 + 6 = 24 bits <= 32 bits).
	sparseList []uint32
	// regs holds 2^precision 6-bit rank values in dense mode.
	regs []uint8
}

// NewByPrecision creates a new HyperLogLog Sketch with the specified precision in [10, 18].
// New sketches start in sparse mode and automatically promote to dense mode when the
// sparse representation reaches a quarter of the dense register byte size.
func NewByPrecision(precision uint8) (*Sketch, error) {
	if precision < MinPrecision || precision > MaxPrecision {
		return nil, fmt.Errorf("precision must be between %d and %d, got %d", MinPrecision, MaxPrecision, precision)
	}
	return &Sketch{
		precision: precision,
		sparse:    true,
	}, nil
}

// MustNewByPrecision creates a new HyperLogLog Sketch with the specified precision or panics if invalid.
func MustNewByPrecision(precision uint8) *Sketch {
	sk, err := NewByPrecision(precision)
	if err != nil {
		panic(err)
	}
	return sk
}

// Precision returns the configured register precision of the sketch.
func (s *Sketch) Precision() uint8 {
	return s.precision
}

// IsSparse reports whether the sketch is currently in sparse representation.
func (s *Sketch) IsSparse() bool {
	return s.sparse
}

// Release returns the dense register buffer to the pool and resets the sketch.
func (s *Sketch) Release() {
	if s == nil {
		return
	}
	if s.regs != nil {
		putRegisters(s.precision, s.regs)
		s.regs = nil
	}
	s.sparseList = nil
}

// Clone returns a deep copy of the Sketch.
func (s *Sketch) Clone() *Sketch {
	if s == nil {
		return nil
	}
	clone := &Sketch{
		precision: s.precision,
		sparse:    s.sparse,
	}
	if s.sparse {
		if len(s.sparseList) > 0 {
			clone.sparseList = make([]uint32, len(s.sparseList))
			copy(clone.sparseList, s.sparseList)
		}
	} else if len(s.regs) > 0 {
		clone.regs = getRegisters(s.precision)
		copy(clone.regs, s.regs)
	}
	return clone
}

// InsertHash inserts a pre-computed 64-bit hash value into the sketch.
func (s *Sketch) InsertHash(hash uint64) {
	idx, rho := s.indexAndRank(hash)
	if s.sparse {
		s.insertSparse(idx, rho)
		return
	}
	if rho > s.regs[idx] {
		s.regs[idx] = rho
	}
}

func (s *Sketch) indexAndRank(hash uint64) (uint32, uint8) {
	idx := uint32(hash >> (64 - s.precision)) //nolint:gosec // precision is in [10, 18], so idx fits in 18 bits
	w := hash << s.precision
	rho := uint8(bits.LeadingZeros64(w)) + 1 //nolint:gosec // LeadingZeros64 is in [0, 64], so rho fits in uint8
	return idx, rho
}

func (s *Sketch) insertSparse(idx uint32, rho uint8) {
	entry := (idx << 6) | uint32(rho&0x3f)
	// Binary search by register index (top bits) to avoid duplicate register entries in sparseList.
	pos, found := slices.BinarySearchFunc(s.sparseList, idx, func(elem uint32, targetIdx uint32) int {
		elemIdx := elem >> 6
		if elemIdx < targetIdx {
			return -1
		}
		if elemIdx > targetIdx {
			return 1
		}
		return 0
	})
	if found {
		oldRho := uint8(s.sparseList[pos] & 0x3f) //nolint:gosec // masked to 6 bits
		if rho > oldRho {
			s.sparseList[pos] = entry
		}
		return
	}

	s.sparseList = slices.Insert(s.sparseList, pos, entry)

	// Promote to dense when sparseList memory (len * 4 bytes) reaches 1/4 of dense register size (2^p bytes),
	// i.e., when len(sparseList) >= 2^(p-4).
	maxSparseLen := 1 << (s.precision - 4)
	if len(s.sparseList) >= maxSparseLen {
		s.promoteToDense()
	}
}

func (s *Sketch) promoteToDense() {
	regs := getRegisters(s.precision)
	for _, entry := range s.sparseList {
		idx := entry >> 6
		rho := uint8(entry & 0x3f) //nolint:gosec // masked to 6 bits
		if rho > regs[idx] {
			regs[idx] = rho
		}
	}
	s.sparseList = nil
	s.regs = regs
	s.sparse = false
}

// Merge combines another Sketch of the same precision into s.
func (s *Sketch) Merge(other *Sketch) error {
	if other == nil {
		return nil
	}
	if s.precision != other.precision {
		return fmt.Errorf("cannot merge sketches with different precisions: %d vs %d", s.precision, other.precision)
	}

	if other.sparse {
		for _, entry := range other.sparseList {
			idx := entry >> 6
			rho := uint8(entry & 0x3f) //nolint:gosec // masked to 6 bits
			if s.sparse {
				s.insertSparse(idx, rho)
			} else if rho > s.regs[idx] {
				s.regs[idx] = rho
			}
		}
		return nil
	}

	if s.sparse {
		s.promoteToDense()
	}
	for i, v := range other.regs {
		if v > s.regs[i] {
			s.regs[i] = v
		}
	}
	return nil
}

// Estimate returns the estimated number of unique elements inserted into the sketch.
// Uses Ertl's (2017) improved HyperLogLog estimator over a single-pass rank histogram.
func (s *Sketch) Estimate() uint64 {
	if s == nil {
		return 0
	}
	m := float64(uint64(1) << s.precision)

	if s.sparse {
		used := len(s.sparseList)
		if used == 0 {
			return 0
		}
		v := m - float64(used)
		return uint64(math.Round(m * math.Log(m/v)))
	}

	var counts [65]uint32
	for _, val := range s.regs {
		counts[val]++
	}
	if counts[0] == (1 << s.precision) {
		return 0
	}

	q := int(64 - s.precision)
	z := m * tau(1.0-float64(counts[q+1])/m)
	for k := q; k >= 1; k-- {
		z = (z + float64(counts[k])) * 0.5
	}
	z += m * sigma(float64(counts[0])/m)

	const alphaInf = 0.7213475204444817 // 1 / (2 * ln(2))
	est := alphaInf * m * m / z
	return uint64(math.Round(est))
}

// sigma implements Ertl's (2017) small-range correction series for HyperLogLog.
func sigma(x float64) float64 {
	if x == 1.0 {
		return math.Inf(1)
	}
	z := x
	y := 1.0
	for {
		x *= x
		zPrime := z
		z += x * y
		y += y
		if z == zPrime {
			break
		}
	}
	return z
}

// tau implements Ertl's (2017) large-range correction series for HyperLogLog.
func tau(x float64) float64 {
	if x == 0.0 || x == 1.0 {
		return 0.0
	}
	var z float64
	y := 1.0
	for {
		x = math.Sqrt(x)
		zPrime := z
		y *= 0.5
		oneMinusX := 1.0 - x
		z -= oneMinusX * oneMinusX * y
		if z == zPrime {
			break
		}
	}
	return z / 3.0
}

type gobSketch struct {
	Precision  uint8
	Sparse     bool
	SparseList []uint32
	Regs       []uint8
}

// GobEncode implements gob.GobEncoder for snapshot serialization.
func (s *Sketch) GobEncode() ([]byte, error) {
	gs := gobSketch{
		Precision:  s.precision,
		Sparse:     s.sparse,
		SparseList: s.sparseList,
		Regs:       s.regs,
	}
	return encodeGobSketch(gs)
}

// GobDecode implements gob.GobDecoder for snapshot deserialization.
func (s *Sketch) GobDecode(data []byte) error {
	gs, err := decodeGobSketch(data)
	if err != nil {
		return err
	}
	s.precision = gs.Precision
	s.sparse = gs.Sparse
	s.sparseList = gs.SparseList
	s.regs = gs.Regs
	return nil
}

func encodeGobSketch(gs gobSketch) ([]byte, error) {
	// Compact binary encoding: [precision(1B), sparse(1B), payload...]
	if gs.Sparse {
		out := make([]byte, 2+len(gs.SparseList)*4)
		out[0] = gs.Precision
		out[1] = 1
		for i, v := range gs.SparseList {
			offset := 2 + i*4
			out[offset] = byte(v >> 24)
			out[offset+1] = byte(v >> 16)
			out[offset+2] = byte(v >> 8)
			out[offset+3] = byte(v)
		}
		return out, nil
	}
	out := make([]byte, 2+len(gs.Regs))
	out[0] = gs.Precision
	out[1] = 0
	copy(out[2:], gs.Regs)
	return out, nil
}

func decodeGobSketch(data []byte) (gobSketch, error) {
	if len(data) < 2 {
		return gobSketch{}, fmt.Errorf("invalid sketch binary data: length %d", len(data))
	}
	gs := gobSketch{
		Precision: data[0],
		Sparse:    data[1] == 1,
	}
	payload := data[2:]
	if gs.Sparse {
		if len(payload)%4 != 0 {
			return gobSketch{}, fmt.Errorf("invalid sparse sketch payload length: %d", len(payload))
		}
		n := len(payload) / 4
		gs.SparseList = make([]uint32, n)
		for i := range n {
			offset := i * 4
			gs.SparseList[i] = (uint32(payload[offset]) << 24) |
				(uint32(payload[offset+1]) << 16) |
				(uint32(payload[offset+2]) << 8) |
				uint32(payload[offset+3])
		}
		return gs, nil
	}
	expected := 1 << gs.Precision
	if len(payload) != expected {
		return gobSketch{}, fmt.Errorf("invalid dense sketch payload length: got %d, want %d", len(payload), expected)
	}
	gs.Regs = getRegisters(gs.Precision)
	copy(gs.Regs, payload)
	return gs, nil
}
