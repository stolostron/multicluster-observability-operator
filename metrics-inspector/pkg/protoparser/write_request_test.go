// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package protoparser

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

func TestParseStreamAndUnmarshalWriteRequest(t *testing.T) {
	pbReq := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "kube_pod_info"},
					{Name: "cluster", Value: "managed-1"},
					{Name: "namespace", Value: "openshift-monitoring"},
				},
				Samples: []prompb.Sample{
					{Value: 1.0, Timestamp: 1700000000000},
				},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "up"},
					{Name: "cluster", Value: "managed-2"},
				},
				Samples: []prompb.Sample{
					{Value: 1.0, Timestamp: 1700000000000},
				},
			},
		},
	}

	raw, err := proto.Marshal(pbReq)
	if err != nil {
		t.Fatalf("proto.Marshal failed: %v", err)
	}
	compressed := snappy.Encode(nil, raw)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/write", bytes.NewReader(compressed))
	var parsedCount int
	err = ParseStream(req, true, DefaultMaxWriteRequestSize, func(wr *WriteRequest) error {
		parsedCount = len(wr.Timeseries)
		if parsedCount != 2 {
			t.Fatalf("len(wr.Timeseries) = %d, want 2", parsedCount)
		}
		ts0 := wr.Timeseries[0]
		if len(ts0.Labels) != 3 || ts0.Labels[1].Name != "cluster" || ts0.Labels[1].Value != "managed-1" {
			t.Fatalf("unexpected ts0 labels: %+v", ts0.Labels)
		}
		if len(ts0.LabelsHashes) != 3 {
			t.Fatalf("len(ts0.LabelsHashes) = %d, want 3", len(ts0.LabelsHashes))
		}
		if ts0.Hash == 0 || ts0.Hash == wr.Timeseries[1].Hash {
			t.Fatalf("unexpected series hashes: %d vs %d", ts0.Hash, wr.Timeseries[1].Hash)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ParseStream failed: %v", err)
	}
	if parsedCount != 2 {
		t.Fatalf("callback was not invoked properly, parsedCount=%d", parsedCount)
	}
}

func TestParseStreamRW1ProtocolEdgeCases(t *testing.T) {
	marshalSnappy := func(t *testing.T, wr *prompb.WriteRequest) []byte {
		t.Helper()
		raw, err := proto.Marshal(wr)
		if err != nil {
			t.Fatalf("proto.Marshal failed: %v", err)
		}
		return snappy.Encode(nil, raw)
	}

	histLabels := []prompb.Label{
		{Name: "__name__", Value: "http_request_duration_seconds"},
		{Name: "cluster", Value: "managed-1"},
		{Name: "method", Value: "GET"},
	}

	// Compute reference hash for the same labels with a plain sample only.
	var refHash uint64
	refBody := marshalSnappy(t, &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels:  histLabels,
				Samples: []prompb.Sample{{Value: 1, Timestamp: 1700000000000}},
			},
		},
	})
	if err := ParseStream(httptest.NewRequest(http.MethodPost, "/api/v1/write", bytes.NewReader(refBody)), false, DefaultMaxWriteRequestSize, func(wr *WriteRequest) error {
		refHash = wr.Timeseries[0].Hash
		return nil
	}); err != nil {
		t.Fatalf("reference ParseStream failed: %v", err)
	}

	tests := []struct {
		name      string
		body      []byte
		maxSize   int64
		wantCount int
		wantHash  uint64
		wantErr   bool
	}{
		{
			name:      "empty WriteRequest",
			body:      marshalSnappy(t, &prompb.WriteRequest{}),
			wantCount: 0,
		},
		{
			name: "metadata-only WriteRequest skipped cleanly",
			body: marshalSnappy(t, &prompb.WriteRequest{
				Metadata: []prompb.MetricMetadata{
					{
						Type:             prompb.MetricMetadata_COUNTER,
						MetricFamilyName: "http_requests_total",
						Help:             "Total HTTP requests",
						Unit:             "requests",
					},
				},
			}),
			wantCount: 0,
		},
		{
			name: "native histograms, exemplars, and metadata skipped without affecting label hash",
			body: marshalSnappy(t, &prompb.WriteRequest{
				Timeseries: []prompb.TimeSeries{
					{
						Labels: histLabels,
						Samples: []prompb.Sample{
							{Value: 0.25, Timestamp: 1700000000000},
						},
						Exemplars: []prompb.Exemplar{
							{
								Labels:    []prompb.Label{{Name: "trace_id", Value: "abc123"}},
								Value:     0.25,
								Timestamp: 1700000000000,
							},
						},
						Histograms: []prompb.Histogram{
							{
								Count:          &prompb.Histogram_CountInt{CountInt: 42},
								Sum:            12.5,
								Schema:         3,
								ZeroThreshold:  0.001,
								ZeroCount:      &prompb.Histogram_ZeroCountInt{ZeroCountInt: 5},
								PositiveSpans:  []prompb.BucketSpan{{Offset: 0, Length: 2}},
								PositiveDeltas: []int64{10, -3},
								Timestamp:      1700000000000,
							},
						},
					},
				},
				Metadata: []prompb.MetricMetadata{
					{
						Type:             prompb.MetricMetadata_HISTOGRAM,
						MetricFamilyName: "http_request_duration_seconds",
					},
				},
			}),
			wantCount: 1,
			wantHash:  refHash,
		},
		{
			name:    "empty HTTP body rejected as invalid Snappy",
			body:    nil,
			wantErr: true,
		},
		{
			name:    "corrupt Snappy payload rejected",
			body:    []byte{0xff, 0xff, 0xff, 0xff, 0x00, 0x01},
			wantErr: true,
		},
		{
			name:    "valid Snappy wrapping malformed protobuf rejected",
			body:    snappy.Encode(nil, []byte{0x0a, 0x7f, 0x01, 0x02}), // truncated length-delimited field 1
			wantErr: true,
		},
		{
			name:    "uncompressed payload exceeding maxSize rejected",
			body:    refBody,
			maxSize: 16,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			maxSize := tc.maxSize
			if maxSize == 0 {
				maxSize = DefaultMaxWriteRequestSize
			}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/write", bytes.NewReader(tc.body))
			var gotCount int
			var gotHash uint64
			err := ParseStream(req, false, maxSize, func(wr *WriteRequest) error {
				gotCount = len(wr.Timeseries)
				if gotCount > 0 {
					gotHash = wr.Timeseries[0].Hash
				}
				return nil
			})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (parsed %d series)", gotCount)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotCount != tc.wantCount {
				t.Fatalf("len(wr.Timeseries) = %d, want %d", gotCount, tc.wantCount)
			}
			if tc.wantHash != 0 && gotHash != tc.wantHash {
				t.Fatalf("series hash = %d, want %d (skipped fields must not affect fingerprint)", gotHash, tc.wantHash)
			}
		})
	}
}

func TestTimeSeriesHashWireOrderIndependence(t *testing.T) {
	// Label: name="foo", value="bar"
	// Order 1: tag 1 (name) then tag 2 (value)
	lblOrder1 := []byte{
		0x0a, 0x03, 'f', 'o', 'o',
		0x12, 0x03, 'b', 'a', 'r',
	}
	// Order 2: tag 2 (value) then tag 1 (name)
	lblOrder2 := []byte{
		0x12, 0x03, 'b', 'a', 'r',
		0x0a, 0x03, 'f', 'o', 'o',
	}

	buildMsg := func(lblBytes []byte) []byte {
		// TimeSeries message (tag 1: repeated Label = 0x0a)
		tsMsg := append([]byte{0x0a, byte(len(lblBytes))}, lblBytes...)
		// WriteRequest message (tag 1: repeated TimeSeries = 0x0a)
		wrMsg := append([]byte{0x0a, byte(len(tsMsg))}, tsMsg...)
		return snappy.Encode(nil, wrMsg)
	}

	var hash1, hash2 uint64
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/write", bytes.NewReader(buildMsg(lblOrder1)))
	err := ParseStream(req1, false, DefaultMaxWriteRequestSize, func(wr *WriteRequest) error {
		if len(wr.Timeseries) != 1 {
			t.Fatalf("expected 1 timeseries, got %d", len(wr.Timeseries))
		}
		hash1 = wr.Timeseries[0].Hash
		return nil
	})
	if err != nil {
		t.Fatalf("ParseStream failed for order 1: %v", err)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/write", bytes.NewReader(buildMsg(lblOrder2)))
	err = ParseStream(req2, false, DefaultMaxWriteRequestSize, func(wr *WriteRequest) error {
		if len(wr.Timeseries) != 1 {
			t.Fatalf("expected 1 timeseries, got %d", len(wr.Timeseries))
		}
		hash2 = wr.Timeseries[0].Hash
		return nil
	})
	if err != nil {
		t.Fatalf("ParseStream failed for order 2: %v", err)
	}

	if hash1 == 0 {
		t.Fatal("expected non-zero hash")
	}
	if hash1 != hash2 {
		t.Fatalf("expected identical hash regardless of field wire order, got %d vs %d", hash1, hash2)
	}
}
