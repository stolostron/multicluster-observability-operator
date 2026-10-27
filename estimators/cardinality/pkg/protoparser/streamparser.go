// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package protoparser

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/golang/snappy"
)

var (
	compressedBufPool = sync.Pool{
		New: func() any {
			return new(bytes.Buffer)
		},
	}
	uncompressedBufPool = sync.Pool{
		New: func() any {
			b := make([]byte, 0, 64*1024)
			return &b
		},
	}
	writeRequestPool = sync.Pool{
		New: func() any {
			return &WriteRequest{}
		},
	}
)

// ParseStream reads a Snappy-compressed Prometheus Remote Write v1 HTTP request,
// decompresses the body using pooled buffers, unmarshals only the TimeSeries labels,
// and invokes callback with the parsed WriteRequest.
func ParseStream(req *http.Request, hashEachLabel bool, maxSize int64, callback func(wr *WriteRequest) error) error {
	if maxSize <= 0 {
		maxSize = DefaultMaxWriteRequestSize
	}

	cBuf := compressedBufPool.Get().(*bytes.Buffer)
	cBuf.Reset()
	defer compressedBufPool.Put(cBuf)

	lr := io.LimitReader(req.Body, maxSize+1)
	if _, err := cBuf.ReadFrom(lr); err != nil {
		return fmt.Errorf("cannot read compressed remote write body: %w", err)
	}
	if int64(cBuf.Len()) > maxSize {
		return fmt.Errorf("remote write compressed body exceeds limit of %d bytes", maxSize)
	}

	compressed := cBuf.Bytes()
	decodedLen, err := snappy.DecodedLen(compressed)
	if err != nil {
		return fmt.Errorf("cannot decode snappy length: %w", err)
	}
	if int64(decodedLen) > maxSize {
		return fmt.Errorf("uncompressed remote write body (%d bytes) exceeds limit (%d bytes)", decodedLen, maxSize)
	}

	uBufPtr := uncompressedBufPool.Get().(*[]byte)
	uBuf := *uBufPtr
	if cap(uBuf) < decodedLen {
		uBuf = make([]byte, decodedLen)
	} else {
		uBuf = uBuf[:decodedLen]
	}
	defer func() {
		*uBufPtr = uBuf[:0]
		uncompressedBufPool.Put(uBufPtr)
	}()

	uncompressed, err := snappy.Decode(uBuf, compressed)
	if err != nil {
		return fmt.Errorf("cannot snappy-decompress remote write body: %w", err)
	}

	wr := writeRequestPool.Get().(*WriteRequest)
	wr.Reset()
	defer func() {
		wr.Reset()
		writeRequestPool.Put(wr)
	}()

	if err := wr.UnmarshalWriteRequest(uncompressed, hashEachLabel); err != nil {
		return fmt.Errorf("cannot unmarshal remote write protobuf: %w", err)
	}

	return callback(wr)
}
