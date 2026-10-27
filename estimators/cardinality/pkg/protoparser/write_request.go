// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package protoparser

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"github.com/cespare/xxhash/v2"
	"github.com/prometheus/prometheus/prompb"
)

// DefaultMaxWriteRequestSize is the default maximum uncompressed Remote Write payload size (64 MiB).
const DefaultMaxWriteRequestSize = 64 * 1024 * 1024

// WriteRequest holds the parsed TimeSeries from a Prometheus Remote Write v1 request.
type WriteRequest struct {
	Timeseries  []TimeSeries
	tmsPool     []TimeSeries
	labelsPool  []prompb.Label
	hashersPool []xxhash.Digest
	hashesPool  []uint64
}

// Reset clears the WriteRequest while retaining underlying slices for reuse.
func (wr *WriteRequest) Reset() {
	for i := range wr.Timeseries {
		wr.Timeseries[i].Reset()
	}
	wr.Timeseries = wr.Timeseries[:0]

	clear(wr.tmsPool)
	wr.tmsPool = wr.tmsPool[:0]

	clear(wr.labelsPool)
	wr.labelsPool = wr.labelsPool[:0]

	for i := range wr.hashersPool {
		wr.hashersPool[i].Reset()
	}
	wr.hashersPool = wr.hashersPool[:0]
	wr.hashesPool = wr.hashesPool[:0]
}

// TimeSeries represents the labels and pre-computed xxhash fingerprints of a single series.
type TimeSeries struct {
	Labels       []prompb.Label
	LabelsHashes []uint64
	Hash         uint64
}

// Reset clears the TimeSeries fields.
func (ts *TimeSeries) Reset() {
	ts.Labels = nil
	ts.LabelsHashes = nil
	ts.Hash = 0
}

// UnmarshalWriteRequest decodes only the TimeSeries.Labels from a protobuf-encoded
// prompb.WriteRequest buffer (skipping Samples and Exemplars/Metadata for speed).
// Notice that string slices inside Labels point directly into src until wr.Reset() is called.
func (wr *WriteRequest) UnmarshalWriteRequest(src []byte, hashEachLabel bool) error {
	tms := wr.tmsPool
	lbls := wr.labelsPool
	hashers := wr.hashersPool
	hashes := wr.hashesPool

	defer func() {
		wr.tmsPool = tms
		wr.labelsPool = lbls
		wr.hashersPool = hashers
		wr.hashesPool = hashes
	}()

	// message WriteRequest {
	//   repeated TimeSeries timeseries = 1;
	// }
	offset := 0
	for offset < len(src) {
		fieldNum, wireType, n := consumeTag(src[offset:])
		if n <= 0 {
			return fmt.Errorf("cannot read WriteRequest field tag at offset %d", offset)
		}
		offset += n

		payload, m := consumeField(src[offset:], wireType)
		if m <= 0 {
			return fmt.Errorf("cannot read WriteRequest field %d payload at offset %d", fieldNum, offset)
		}
		offset += m

		if fieldNum != 1 {
			continue
		}
		if wireType != 2 {
			return fmt.Errorf("unexpected wireType %d for TimeSeries field", wireType)
		}

		if cap(tms) > len(tms) {
			tms = tms[:len(tms)+1]
		} else {
			tms = append(tms, TimeSeries{})
		}
		ts := &tms[len(tms)-1]

		if cap(hashers) > len(hashers) {
			hashers = hashers[:len(hashers)+1]
		} else {
			hashers = append(hashers, *xxhash.New())
		}
		tsD := &hashers[len(hashers)-1]
		tsD.Reset()

		var err error
		lbls, hashes, err = unmarshalTimeSeriesLabels(ts, lbls, hashes, tsD, payload, hashEachLabel)
		if err != nil {
			return fmt.Errorf("cannot unmarshal TimeSeries: %w", err)
		}
	}

	wr.Timeseries = append(wr.Timeseries, tms...)
	return nil
}

func unmarshalTimeSeriesLabels(
	ts *TimeSeries,
	dst []prompb.Label,
	hashesDst []uint64,
	tsD *xxhash.Digest,
	src []byte,
	hashEachLabel bool,
) ([]prompb.Label, []uint64, error) {
	// message TimeSeries {
	//   repeated Label labels = 1;
	//   repeated Sample samples = 2;
	//   repeated Exemplar exemplars = 3;
	// }
	dstLen := len(dst)
	hashesDstLen := len(hashesDst)
	offset := 0
	for offset < len(src) {
		fieldNum, wireType, n := consumeTag(src[offset:])
		if n <= 0 {
			return dst, hashesDst, fmt.Errorf("cannot read TimeSeries field tag")
		}
		offset += n

		payload, m := consumeField(src[offset:], wireType)
		if m <= 0 {
			return dst, hashesDst, fmt.Errorf("cannot read TimeSeries field %d payload", fieldNum)
		}
		offset += m

		if fieldNum != 1 {
			// Skip Samples, Exemplars, Histograms.
			continue
		}
		if wireType != 2 {
			return dst, hashesDst, fmt.Errorf("unexpected wireType %d for Label field", wireType)
		}

		if cap(dst) > len(dst) {
			dst = dst[:len(dst)+1]
		} else {
			dst = append(dst, prompb.Label{})
		}
		lbl := &dst[len(dst)-1]
		if err := unmarshalLabel(lbl, payload); err != nil {
			return dst, hashesDst, fmt.Errorf("cannot unmarshal Label: %w", err)
		}

		_, _ = tsD.Write(payload)
		if hashEachLabel {
			hashesDst = append(hashesDst, xxhash.Sum64(payload))
		}
	}

	ts.Labels = dst[dstLen:]
	ts.LabelsHashes = hashesDst[hashesDstLen:]
	ts.Hash = tsD.Sum64()
	return dst, hashesDst, nil
}

func unmarshalLabel(dst *prompb.Label, src []byte) error {
	// message Label {
	//   string name = 1;
	//   string value = 2;
	// }
	dst.Name = ""
	dst.Value = ""
	offset := 0
	for offset < len(src) {
		fieldNum, wireType, n := consumeTag(src[offset:])
		if n <= 0 {
			return fmt.Errorf("cannot read Label tag")
		}
		offset += n

		payload, m := consumeField(src[offset:], wireType)
		if m <= 0 {
			return fmt.Errorf("cannot read Label field %d", fieldNum)
		}
		offset += m

		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("unexpected wireType %d for Label.name", wireType)
			}
			dst.Name = unsafeBytesToString(payload)
		case 2:
			if wireType != 2 {
				return fmt.Errorf("unexpected wireType %d for Label.value", wireType)
			}
			dst.Value = unsafeBytesToString(payload)
		}
	}
	return nil
}

func consumeTag(src []byte) (fieldNum uint32, wireType uint8, n int) {
	v, n := binary.Uvarint(src)
	if n <= 0 || (v>>3) > 0x1fffffff {
		return 0, 0, 0
	}
	return uint32(v >> 3), uint8(v & 7), n //nolint:gosec // bounded by protobuf max field number check above
}

func consumeField(src []byte, wireType uint8) (payload []byte, n int) {
	switch wireType {
	case 0: // Varint
		_, n = binary.Uvarint(src)
		if n <= 0 {
			return nil, 0
		}
		return src[:n], n
	case 1: // 64-bit
		if len(src) < 8 {
			return nil, 0
		}
		return src[:8], 8
	case 2: // Length-delimited
		length, m := binary.Uvarint(src)
		if m <= 0 {
			return nil, 0
		}
		total := uint64(m) + length
		if total > uint64(len(src)) || total < uint64(m) {
			return nil, 0
		}
		return src[m:total], int(total) //nolint:gosec // total <= len(src) verified above
	case 5: // 32-bit
		if len(src) < 4 {
			return nil, 0
		}
		return src[:4], 4
	default:
		return nil, 0
	}
}

func unsafeBytesToString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(b), len(b)) //nolint:gosec // zero-copy string view scoped to pooled WriteRequest lifetime
}
