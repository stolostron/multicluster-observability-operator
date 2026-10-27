// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stolostron/multicluster-observability-operator/estimators/cardinality/pkg/estimator"
	"github.com/stolostron/multicluster-observability-operator/estimators/cardinality/pkg/protoparser"
	"github.com/stolostron/multicluster-observability-operator/estimators/cardinality/pkg/remotewrite"
)

func TestServerMuxEndToEnd(t *testing.T) {
	cfg, err := estimator.ParseConfig([]byte(`
streams:
  - interval: 15m
    group_by: [cluster]
    hll_precision: 14
    calculate_churn: true
`))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}
	eng, err := estimator.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer eng.Stop()

	pusher := remotewrite.NewPusher(remotewrite.PusherConfig{}, eng)
	metrics := &serverMetrics{}
	mux := newServerMux(eng, pusher, metrics, remotewrite.DefaultTenantHeader, protoparser.DefaultMaxWriteRequestSize, slog.Default())
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// 1. Check /healthz
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from /healthz, got %d", resp.StatusCode)
	}

	// 2. POST Remote Write batch to /api/v1/write
	wr := prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "node_cpu_seconds_total"},
					{Name: "cluster", Value: "managed-1"},
				},
				Samples: []prompb.Sample{{Value: 1, Timestamp: 1000}},
			},
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "kube_pod_info"},
					{Name: "cluster", Value: "managed-1"},
				},
				Samples: []prompb.Sample{{Value: 1, Timestamp: 1000}},
			},
		},
	}
	pb, err := wr.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	compressed := snappy.Encode(nil, pb)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/write", bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}
	req.Header.Set(remotewrite.DefaultTenantHeader, "tenant-123")
	writeResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/v1/write failed: %v", err)
	}
	_ = writeResp.Body.Close()
	if writeResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 NoContent, got %d", writeResp.StatusCode)
	}

	// 3. Check /cardinality/metrics
	cResp, err := http.Get(srv.URL + "/cardinality/metrics")
	if err != nil {
		t.Fatalf("GET /cardinality/metrics failed: %v", err)
	}
	cBody, _ := io.ReadAll(cResp.Body)
	_ = cResp.Body.Close()
	if !strings.Contains(string(cBody), `by_cluster="managed-1"} 2`) {
		t.Fatalf("expected cardinality_estimate for managed-1 = 2, got:\n%s", string(cBody))
	}

	// 4. Check /metrics
	mResp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics failed: %v", err)
	}
	mBody, _ := io.ReadAll(mResp.Body)
	_ = mResp.Body.Close()
	if !strings.Contains(string(mBody), "cardinality_estimator_ingested_series_total 2") {
		t.Fatalf("expected ingested_series_total=2, got:\n%s", string(mBody))
	}
}
