// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package remotewrite

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stolostron/multicluster-observability-operator/estimators/cardinality/pkg/estimator"
	"github.com/stolostron/multicluster-observability-operator/estimators/cardinality/pkg/protoparser"
)

func TestPusherFlushAt(t *testing.T) {
	var (
		mu             sync.Mutex
		receivedSeries []prompb.TimeSeries
		receivedTenant string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Encoding") != "snappy" {
			t.Errorf("expected Content-Encoding=snappy, got %q", r.Header.Get("Content-Encoding"))
		}
		compressed, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		raw, err := snappy.Decode(nil, compressed)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var wr prompb.WriteRequest
		if err := wr.Unmarshal(raw); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		mu.Lock()
		receivedTenant = r.Header.Get(DefaultTenantHeader)
		receivedSeries = append(receivedSeries, wr.Timeseries...)
		mu.Unlock()

		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

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

	var batch []protoparser.TimeSeries
	for i := range 20 {
		name := fmt.Sprintf("up_%d", i)
		batch = append(batch, protoparser.TimeSeries{
			Hash: xxhash.Sum64([]byte("cls1|" + name)),
			Labels: []prompb.Label{
				{Name: "__name__", Value: name},
				{Name: "cluster", Value: "cls1"},
			},
		})
	}
	eng.Ingest(batch)

	pusher := NewPusher(PusherConfig{
		Endpoint:  srv.URL,
		BatchSize: 2,
		TenantID:  "initial-tenant",
	}, eng)
	pusher.ObserveTenantID("observed-tenant-uuid")

	now := time.Now()
	if err := pusher.FlushAt(context.Background(), now); err != nil {
		t.Fatalf("FlushAt failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if receivedTenant != "observed-tenant-uuid" {
		t.Fatalf("expected tenant header %q, got %q", "observed-tenant-uuid", receivedTenant)
	}
	// Expect 4 series: cardinality_estimate (cls1), cardinality_churn_ratio (cls1), group size, and group limit
	if len(receivedSeries) != 4 {
		t.Fatalf("expected 4 pushed series, got %d", len(receivedSeries))
	}
	if pusher.PushedSeriesTotal() != 4 {
		t.Fatalf("expected PushedSeriesTotal()=4, got %d", pusher.PushedSeriesTotal())
	}
	if pusher.PushSuccessTotal() != 2 {
		t.Fatalf("expected PushSuccessTotal()=2 batches, got %d", pusher.PushSuccessTotal())
	}
}

func TestPusherRunInitialDelay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	cfg, err := estimator.ParseConfig([]byte(`
streams:
  - interval: 15m
    group_by: [cluster]
    hll_precision: 14
`))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}
	eng, err := estimator.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer eng.Stop()

	eng.Ingest([]protoparser.TimeSeries{
		{
			Hash: xxhash.Sum64([]byte("cls1|up")),
			Labels: []prompb.Label{
				{Name: "__name__", Value: "up"},
				{Name: "cluster", Value: "cls1"},
			},
		},
	})

	pusher := NewPusher(PusherConfig{
		Endpoint:      srv.URL,
		InitialDelay:  80 * time.Millisecond,
		FlushInterval: 1 * time.Hour,
	}, eng)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		pusher.Run(ctx)
	}()

	time.Sleep(25 * time.Millisecond)
	if got := pusher.PushSuccessTotal(); got != 0 {
		cancel()
		<-done
		t.Fatalf("expected 0 pushes before InitialDelay elapsed, got %d", got)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for pusher.PushSuccessTotal() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done

	if got := pusher.PushSuccessTotal(); got != 1 {
		t.Fatalf("expected 1 push immediately after InitialDelay elapsed, got %d", got)
	}
}
