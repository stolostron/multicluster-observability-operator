// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package remotewrite

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stolostron/multicluster-observability-operator/metrics-inspector/pkg/estimator"
)

type statusError struct {
	statusCode int
	message    string
}

func (e *statusError) Error() string {
	return e.message
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	var sErr *statusError
	if errors.As(err, &sErr) {
		return sErr.statusCode >= 500 || sErr.statusCode == http.StatusTooManyRequests
	}
	return true
}

const (
	DefaultFlushInterval = 60 * time.Second
	// DefaultInitialDelay is set to 10m (covering multiple 5m PrometheusAgent/metrics-collector
	// scrape intervals and queue delivery) so all staggered scrape jobs complete multiple
	// full pushes after a Pod rollout before the first cardinality report is flushed.
	DefaultInitialDelay       = 10 * time.Minute
	DefaultProbeRetryInterval = 5 * time.Second
	DefaultBatchSize          = 10000
	DefaultTimeout            = 30 * time.Second
	DefaultTenantHeader       = "THANOS-TENANT"
	DefaultTenantID           = "default"
)

// PusherConfig configures the periodic Remote Write v1 cardinality report pusher.
type PusherConfig struct {
	Endpoint      string
	FlushInterval time.Duration
	// InitialDelay specifies an optional warm-up duration before the first
	// Remote Write report is pushed after startup. This allows all managed
	// clusters (including Legacy metrics-collector with a 5m interval) to report
	// at least once after a Pod restart before overwriting fleet-wide aggregates
	// in Thanos Receive.
	InitialDelay       time.Duration
	ProbeRetryInterval time.Duration
	BatchSize          int
	Timeout            time.Duration
	RetryBackoff       []time.Duration
	TenantHeader       string
	TenantID           string
	Logger             *slog.Logger
}

// Pusher periodically evaluates stream reports from an estimator.Engine and pushes
// them to Thanos Receive via Prometheus Remote Write v1.
type Pusher struct {
	cfg    PusherConfig
	engine *estimator.Engine
	client *http.Client

	tenantMu sync.RWMutex
	tenantID string

	ready      atomic.Bool
	readyErrMu sync.RWMutex
	readyErr   string

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
	if cfg.ProbeRetryInterval <= 0 {
		cfg.ProbeRetryInterval = DefaultProbeRetryInterval
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = DefaultBatchSize
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.RetryBackoff == nil {
		cfg.RetryBackoff = []time.Duration{500 * time.Millisecond, 1 * time.Second}
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

	p := &Pusher{
		cfg:    cfg,
		engine: engine,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		tenantID: cfg.TenantID,
		readyErr: "waiting for initial thanos receive connectivity check",
	}
	if cfg.Endpoint == "" {
		p.ready.Store(true)
	}
	return p
}

// Ready reports whether initial connectivity to the configured Remote Write endpoint
// has been verified. Once latched true, Ready remains true for the life of the Pusher.
func (p *Pusher) Ready() (bool, string) {
	if p == nil || p.cfg.Endpoint == "" || p.ready.Load() {
		return true, ""
	}
	p.readyErrMu.RLock()
	defer p.readyErrMu.RUnlock()
	return false, p.readyErr
}

func (p *Pusher) setReadyErr(err error) {
	p.readyErrMu.Lock()
	if err != nil {
		p.readyErr = err.Error()
	}
	p.readyErrMu.Unlock()
}

// Probe sends an empty Snappy-compressed WriteRequest to verify connectivity and
// configuration of the Remote Write endpoint without ingesting any series.
func (p *Pusher) Probe(ctx context.Context) error {
	if p.cfg.Endpoint == "" {
		p.ready.Store(true)
		return nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, min(p.cfg.Timeout, DefaultProbeRetryInterval))
	defer cancel()
	if err := p.sendBatch(probeCtx, nil); err != nil {
		p.setReadyErr(err)
		return err
	}
	p.ready.Store(true)
	return nil
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

// TenantID returns the currently configured or observed Thanos tenant ID.
func (p *Pusher) TenantID() string {
	return p.getTenantID()
}

// Run starts the periodic flush loop until ctx is canceled.
func (p *Pusher) Run(ctx context.Context) {
	if p.cfg.Endpoint == "" {
		p.cfg.Logger.Info("remote-write endpoint is empty; periodic report push is disabled")
		return
	}

	p.cfg.Logger.Info("started remote-write cardinality report pusher",
		"endpoint", p.cfg.Endpoint,
		"interval", p.cfg.FlushInterval.String(),
		"initial_delay", p.cfg.InitialDelay.String(),
	)

	if !p.probeUntilReady(ctx) {
		return
	}

	if p.cfg.InitialDelay > 0 {
		timer := time.NewTimer(p.cfg.InitialDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case now := <-timer.C:
			if err := p.FlushAt(ctx, now); err != nil {
				p.cfg.Logger.Error("failed to push initial cardinality reports", "err", err)
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
				p.cfg.Logger.Error("failed to push cardinality reports", "err", err)
			}
		}
	}
}

func (p *Pusher) probeUntilReady(ctx context.Context) bool {
	if err := p.Probe(ctx); err == nil {
		return true
	} else if ctx.Err() == nil {
		p.cfg.Logger.Warn("initial thanos receive connectivity probe failed; retrying", "endpoint", p.cfg.Endpoint, "err", err)
	}

	ticker := time.NewTicker(p.cfg.ProbeRetryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if err := p.Probe(ctx); err == nil {
				p.cfg.Logger.Info("thanos receive connectivity verified", "endpoint", p.cfg.Endpoint)
				return true
			} else if ctx.Err() == nil {
				p.cfg.Logger.Warn("thanos receive connectivity probe failed; retrying", "endpoint", p.cfg.Endpoint, "err", err)
			}
		}
	}
}

// FlushAt evaluates all stream reports at now and pushes them to the configured Remote Write endpoint.
func (p *Pusher) FlushAt(ctx context.Context, now time.Time) error {
	flushStart := time.Now()
	reports := p.engine.Evaluate(now)
	tsMs := now.UnixMilli()

	estimatedCap := 0
	for i := range reports {
		estimatedCap += len(reports[i].Groups)*2 + 4
	}
	allSeries := make([]prompb.TimeSeries, 0, estimatedCap)
	for i := range reports {
		allSeries = append(allSeries, reports[i].ToTimeSeries(tsMs)...)
	}
	if len(allSeries) == 0 {
		return nil
	}

	for start := 0; start < len(allSeries); start += p.cfg.BatchSize {
		end := min(start+p.cfg.BatchSize, len(allSeries))
		batch := allSeries[start:end]
		var sendErr error
		for attempt := 0; ; attempt++ {
			sendErr = p.sendBatch(ctx, batch)
			if sendErr == nil {
				break
			}
			if !isRetryable(sendErr) || attempt >= len(p.cfg.RetryBackoff) {
				p.pushErrorsTotal.Add(1)
				return sendErr
			}
			p.cfg.Logger.Warn("transient error pushing remote-write batch; retrying",
				"attempt", attempt+1,
				"max_retries", len(p.cfg.RetryBackoff),
				"backoff", p.cfg.RetryBackoff[attempt],
				"err", sendErr,
			)
			select {
			case <-ctx.Done():
				p.pushErrorsTotal.Add(1)
				return ctx.Err()
			case <-time.After(p.cfg.RetryBackoff[attempt]):
			}
		}
		p.pushedSeriesTotal.Add(uint64(end - start)) //nolint:gosec // end > start guaranteed by loop bounds
		p.pushSuccessTotal.Add(1)
	}

	p.ready.Store(true)
	p.lastPushTimestampS.Store(now.Unix())
	p.cfg.Logger.Debug("pushed cardinality reports",
		"series_count", len(allSeries),
		"duration_ms", time.Since(flushStart).Milliseconds(),
	)
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
	req.Header.Set("User-Agent", "acm-metrics-inspector/1.0")
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
		return &statusError{
			statusCode: resp.StatusCode,
			message:    fmt.Sprintf("unexpected status %d from %s: %s", resp.StatusCode, p.cfg.Endpoint, string(body)),
		}
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
