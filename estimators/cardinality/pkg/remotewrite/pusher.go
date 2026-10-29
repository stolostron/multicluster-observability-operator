// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package remotewrite

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stolostron/multicluster-observability-operator/estimators/cardinality/pkg/estimator"
)

const (
	DefaultFlushInterval = 60 * time.Second
	DefaultInitialDelay  = 5 * time.Minute
	DefaultBatchSize     = 10000
	DefaultTimeout       = 30 * time.Second
	DefaultTenantHeader  = "THANOS-TENANT"
	DefaultTenantID      = "default"
)

// PusherConfig configures the periodic Remote Write v1 snapshot pusher.
type PusherConfig struct {
	Endpoint      string
	FlushInterval time.Duration
	// InitialDelay specifies an optional warm-up duration before the first
	// Remote Write snapshot is pushed after startup. This allows all managed
	// clusters (including Legacy metrics-collector with a 5m interval) to report
	// at least once after a Pod restart before overwriting fleet-wide aggregates
	// in Thanos Receive.
	InitialDelay time.Duration
	BatchSize    int
	Timeout      time.Duration
	TenantHeader string
	TenantID     string
	Logger       *slog.Logger
}

// Pusher periodically evaluates snapshots from an estimator.Engine and pushes
// them to Thanos Receive via Prometheus Remote Write v1.
type Pusher struct {
	cfg    PusherConfig
	engine *estimator.Engine
	client *http.Client

	tenantMu sync.RWMutex
	tenantID string

	pushedSeriesTotal  atomic.Uint64
	pushSuccessTotal   atomic.Uint64
	pushErrorsTotal    atomic.Uint64
	lastPushTimestampS atomic.Int64
}

// NewPusher creates a new Remote Write v1 Pusher.
func NewPusher(cfg PusherConfig, engine *estimator.Engine) *Pusher {
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = DefaultFlushInterval
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = DefaultBatchSize
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.TenantHeader == "" {
		cfg.TenantHeader = DefaultTenantHeader
	}
	if cfg.TenantID == "" {
		cfg.TenantID = DefaultTenantID
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &Pusher{
		cfg:    cfg,
		engine: engine,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		tenantID: cfg.TenantID,
	}
}

// ObserveTenantID updates the tenant ID used in outgoing Remote Write headers
// when a non-empty tenant header is observed on incoming requests from observatorium-api.
func (p *Pusher) ObserveTenantID(id string) {
	if id == "" {
		return
	}
	p.tenantMu.RLock()
	same := p.tenantID == id
	p.tenantMu.RUnlock()
	if same {
		return
	}

	p.tenantMu.Lock()
	p.tenantID = id
	p.tenantMu.Unlock()
}

func (p *Pusher) getTenantID() string {
	p.tenantMu.RLock()
	defer p.tenantMu.RUnlock()
	return p.tenantID
}

// Run starts the periodic flush loop until ctx is canceled.
func (p *Pusher) Run(ctx context.Context) {
	if p.cfg.Endpoint == "" {
		p.cfg.Logger.Info("remote-write endpoint is empty; periodic snapshot push is disabled")
		return
	}

	p.cfg.Logger.Info("started remote-write snapshot pusher",
		"endpoint", p.cfg.Endpoint,
		"interval", p.cfg.FlushInterval.String(),
		"initial_delay", p.cfg.InitialDelay.String(),
	)

	if p.cfg.InitialDelay > 0 {
		timer := time.NewTimer(p.cfg.InitialDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case now := <-timer.C:
			if err := p.FlushAt(ctx, now); err != nil {
				p.cfg.Logger.Error("failed to push initial cardinality snapshots", "err", err)
			}
		}
	}

	ticker := time.NewTicker(p.cfg.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := p.FlushAt(ctx, now); err != nil {
				p.cfg.Logger.Error("failed to push cardinality snapshots", "err", err)
			}
		}
	}
}

// FlushAt evaluates all stream snapshots at now and pushes them to the configured Remote Write endpoint.
func (p *Pusher) FlushAt(ctx context.Context, now time.Time) error {
	snaps := p.engine.Snapshots(now)
	tsMs := now.UnixMilli()

	estimatedCap := 0
	for i := range snaps {
		estimatedCap += len(snaps[i].Groups)*2 + 4
	}
	allSeries := make([]prompb.TimeSeries, 0, estimatedCap)
	for i := range snaps {
		allSeries = append(allSeries, snaps[i].ToTimeSeries(tsMs)...)
	}
	if len(allSeries) == 0 {
		return nil
	}

	for start := 0; start < len(allSeries); start += p.cfg.BatchSize {
		end := min(start+p.cfg.BatchSize, len(allSeries))
		if err := p.sendBatch(ctx, allSeries[start:end]); err != nil {
			p.pushErrorsTotal.Add(1)
			return err
		}
		p.pushedSeriesTotal.Add(uint64(end - start)) //nolint:gosec // end > start guaranteed by loop bounds
		p.pushSuccessTotal.Add(1)
	}

	p.lastPushTimestampS.Store(now.Unix())
	return nil
}

func (p *Pusher) sendBatch(ctx context.Context, series []prompb.TimeSeries) error {
	wr := prompb.WriteRequest{
		Timeseries: series,
	}
	pbBytes, err := wr.Marshal()
	if err != nil {
		return fmt.Errorf("marshal WriteRequest: %w", err)
	}
	compressed := snappy.Encode(nil, pbBytes)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.Endpoint, bytes.NewReader(compressed))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Encoding", "snappy")
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")
	req.Header.Set("User-Agent", "acm-cardinality-estimator/1.0")
	if tenant := p.getTenantID(); tenant != "" {
		req.Header.Set(p.cfg.TenantHeader, tenant)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("post remote-write batch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("unexpected status %d from %s: %s", resp.StatusCode, p.cfg.Endpoint, string(body))
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// PushedSeriesTotal returns the total number of time series pushed via Remote Write.
func (p *Pusher) PushedSeriesTotal() uint64 {
	return p.pushedSeriesTotal.Load()
}

// PushSuccessTotal returns the total number of successful Remote Write batch POSTs.
func (p *Pusher) PushSuccessTotal() uint64 {
	return p.pushSuccessTotal.Load()
}

// PushErrorsTotal returns the total number of failed Remote Write batch POSTs.
func (p *Pusher) PushErrorsTotal() uint64 {
	return p.pushErrorsTotal.Load()
}

// LastPushTimestampSeconds returns the Unix timestamp of the last completed flush.
func (p *Pusher) LastPushTimestampSeconds() int64 {
	return p.lastPushTimestampS.Load()
}
