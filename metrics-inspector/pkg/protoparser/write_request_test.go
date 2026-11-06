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
