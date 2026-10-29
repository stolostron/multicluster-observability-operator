// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/stolostron/multicluster-observability-operator/estimators/cardinality/pkg/estimator"
	"github.com/stolostron/multicluster-observability-operator/estimators/cardinality/pkg/protoparser"
	"github.com/stolostron/multicluster-observability-operator/estimators/cardinality/pkg/remotewrite"
)

type serverMetrics struct {
	writeRequestsTotal atomic.Uint64
	writeErrorsTotal   atomic.Uint64
	configReloadsTotal atomic.Uint64
	configErrorsTotal  atomic.Uint64
}

func main() {
	os.Exit(run())
}

func run() int {
	var (
		configFile     = flag.String("config.file", "/etc/cardinality-estimator/config.yaml", "Path to the cardinality estimator YAML configuration file.")
		listenAddr     = flag.String("web.listen-address", ":8490", "Address to listen on for Remote Write ingestion and operational metrics.")
		remoteWriteURL = flag.String("remote-write.url", "", "Thanos Receive Remote Write v1 endpoint URL for periodic snapshot flushing.")
		flushInterval  = flag.Duration("remote-write.interval", remotewrite.DefaultFlushInterval, "Interval between Remote Write snapshot flushes.")
		initialDelay   = flag.Duration("remote-write.initial-delay", remotewrite.DefaultInitialDelay, "Initial warm-up delay after startup before pushing the first Remote Write snapshot.")
		tenantHeader   = flag.String("remote-write.tenant-header", remotewrite.DefaultTenantHeader, "HTTP header name used to propagate the Thanos tenant ID.")
		tenantID       = flag.String("remote-write.tenant", remotewrite.DefaultTenantID, "Fallback Thanos tenant ID before any incoming write header is observed.")
		maxRequestSize = flag.Int64("max-request-size", protoparser.DefaultMaxWriteRequestSize, "Maximum uncompressed Remote Write payload size in bytes.")
	)
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := estimator.LoadConfig(*configFile)
	if err != nil {
		logger.Error("failed to load initial estimator configuration", "file", *configFile, "err", err)
		return 1
	}

	eng, err := estimator.NewEngine(cfg)
	if err != nil {
		logger.Error("failed to initialize estimator engine", "err", err)
		return 1
	}
	defer eng.Stop()

	pusher := remotewrite.NewPusher(remotewrite.PusherConfig{
		Endpoint:      *remoteWriteURL,
		FlushInterval: *flushInterval,
		InitialDelay:  *initialDelay,
		TenantHeader:  *tenantHeader,
		TenantID:      *tenantID,
		Logger:        logger,
	}, eng)

	metrics := &serverMetrics{}
	mux := newServerMux(eng, pusher, metrics, *tenantHeader, *maxRequestSize, logger)

	srv := &http.Server{
		Addr:              *listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go pusher.Run(ctx)

	sighupCh := make(chan os.Signal, 1)
	signal.Notify(sighupCh, syscall.SIGHUP)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-sighupCh:
				logger.Info("received SIGHUP; reloading estimator configuration", "file", *configFile)
				newCfg, err := estimator.LoadConfig(*configFile)
				if err != nil {
					metrics.configErrorsTotal.Add(1)
					logger.Error("failed to parse reloaded configuration", "file", *configFile, "err", err)
					continue
				}
				if err := eng.Reload(newCfg); err != nil {
					metrics.configErrorsTotal.Add(1)
					logger.Error("failed to apply reloaded configuration", "file", *configFile, "err", err)
					continue
				}
				metrics.configReloadsTotal.Add(1)
				logger.Info("reloaded estimator configuration", "file", *configFile)
			}
		}
	}()

	stopSigCh := make(chan os.Signal, 1)
	signal.Notify(stopSigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-stopSigCh
		logger.Info("received shutdown signal", "signal", sig.String())
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	logger.Info("starting ACM cardinality estimator", "listen", *listenAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("HTTP server terminated with error", "err", err)
		return 1
	}
	return 0
}

func newServerMux(
	eng *estimator.Engine,
	pusher *remotewrite.Pusher,
	m *serverMetrics,
	tenantHeader string,
	maxRequestSize int64,
	logger *slog.Logger,
) *http.ServeMux {
	mux := http.NewServeMux()

	writeHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		m.writeRequestsTotal.Add(1)

		if tenantHeader != "" {
			if incomingTenant := r.Header.Get(tenantHeader); incomingTenant != "" {
				pusher.ObserveTenantID(incomingTenant)
			}
		}

		err := protoparser.ParseStream(r, eng.FingerprintLabels(), maxRequestSize, func(wr *protoparser.WriteRequest) error {
			eng.Ingest(wr.Timeseries)
			return nil
		})
		if err != nil {
			m.writeErrorsTotal.Add(1)
			logger.Warn("failed to parse remote write stream", "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}

	mux.HandleFunc("/api/v1/write", writeHandler)
	mux.HandleFunc("/cardinality/api/v1/write", writeHandler)

	mux.HandleFunc("/cardinality/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		if err := eng.WritePrometheusText(w, time.Now()); err != nil {
			logger.Warn("failed to write cardinality snapshot metrics", "err", err)
		}
	})

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = fmt.Fprintf(w, "cardinality_estimator_write_requests_total %d\n", m.writeRequestsTotal.Load())
		_, _ = fmt.Fprintf(w, "cardinality_estimator_write_errors_total %d\n", m.writeErrorsTotal.Load())
		_, _ = fmt.Fprintf(w, "cardinality_estimator_ingested_series_total %d\n", eng.IngestedSeriesTotal())
		_, _ = fmt.Fprintf(w, "cardinality_estimator_deduplicated_series_total %d\n", eng.DeduplicatedSeriesTotal())
		_, _ = fmt.Fprintf(w, "cardinality_estimator_pushed_series_total %d\n", pusher.PushedSeriesTotal())
		_, _ = fmt.Fprintf(w, "cardinality_estimator_push_success_total %d\n", pusher.PushSuccessTotal())
		_, _ = fmt.Fprintf(w, "cardinality_estimator_push_errors_total %d\n", pusher.PushErrorsTotal())
		_, _ = fmt.Fprintf(w, "cardinality_estimator_last_push_timestamp_seconds %d\n", pusher.LastPushTimestampSeconds())
		_, _ = fmt.Fprintf(w, "cardinality_estimator_config_reloads_total %d\n", m.configReloadsTotal.Load())
		_, _ = fmt.Fprintf(w, "cardinality_estimator_config_reload_errors_total %d\n", m.configErrorsTotal.Load())
	})

	healthHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	}
	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/readyz", healthHandler)

	return mux
}
