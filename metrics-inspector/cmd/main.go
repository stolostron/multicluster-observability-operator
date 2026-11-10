// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stolostron/multicluster-observability-operator/metrics-inspector/pkg/bucketinspect"
	"github.com/stolostron/multicluster-observability-operator/metrics-inspector/pkg/estimator"
	"github.com/stolostron/multicluster-observability-operator/metrics-inspector/pkg/protoparser"
	"github.com/stolostron/multicluster-observability-operator/metrics-inspector/pkg/remotewrite"
	k8sapiflag "k8s.io/component-base/cli/flag"
)

func main() {
	os.Exit(run())
}

func run() int {
	var (
		configFile        = flag.String("config.file", "/etc/metrics-inspector/config.yaml", "Path to the metrics inspector YAML configuration file.")
		windowInterval    = flag.Duration("estimator.interval", estimator.DefaultInterval, "Default sliding window interval for cardinality streams when not specified in config.yaml.")
		maxItemsPerWindow = flag.Int(
			"estimator.max-items-per-interval",
			estimator.DefaultMaxItemsPerInterval,
			"Default Bloom filter deduplication capacity per interval when not specified in config.yaml.",
		)
		listenAddr        = flag.String("web.listen-address", ":8490", "Address to listen on for Remote Write ingestion and health probes.")
		metricsListenAddr = flag.String("metrics.listen-address", "", "Optional address to listen on for operational /metrics over plain HTTP (e.g. 127.0.0.1:8080 for kube-rbac-proxy).")
		tlsServerCertFile = flag.String("tls.server.cert-file", "", "Optional path to the server TLS certificate file for the Remote Write listener.")
		tlsServerKeyFile  = flag.String("tls.server.key-file", "", "Optional path to the server TLS private key file for the Remote Write listener.")
		tlsClientCAFile   = flag.String("tls.client-ca-file", "", "Optional path to the client CA certificate bundle for mTLS verification on the Remote Write listener.")
		tlsMinVersion     = flag.String("tls-min-version", "VersionTLS12", "Minimum TLS version supported by the Remote Write listener (e.g. VersionTLS12, VersionTLS13).")
		tlsCipherSuites   = flag.String("tls-cipher-suites", "", "Comma-separated list of TLS cipher suite names for the Remote Write listener. If omitted, Go defaults are used.")
		remoteWriteURL    = flag.String("remote-write.url", "", "Thanos Receive Remote Write v1 endpoint URL for periodic cardinality report flushing.")
		flushInterval     = flag.Duration("remote-write.interval", remotewrite.DefaultFlushInterval, "Interval between Remote Write cardinality report flushes.")
		initialDelay      = flag.Duration(
			"remote-write.initial-delay",
			remotewrite.DefaultInitialDelay,
			"Initial warm-up delay after startup before pushing the first Remote Write cardinality report.",
		)
		tenantHeader       = flag.String("remote-write.tenant-header", remotewrite.DefaultTenantHeader, "HTTP header name used to propagate the Thanos tenant ID.")
		tenantID           = flag.String("remote-write.tenant", remotewrite.DefaultTenantID, "Fallback Thanos tenant ID before any incoming write header is observed.")
		maxRequestSize     = flag.Int64("max-request-size", protoparser.DefaultMaxWriteRequestSize, "Maximum uncompressed Remote Write payload size in bytes.")
		objstoreConfigFile = flag.String("objstore.config-file", "", "Optional path to Thanos object storage configuration YAML file for bucket inspection.")
		bucketSyncInterval = flag.Duration("bucket-inspect.interval", bucketinspect.DefaultSyncInterval, "Interval between background Thanos object storage bucket inspection sweeps.")
		retentionRaw       = flag.String("retention.resolution-raw", bucketinspect.DefaultRetentionDuration, "Configured Thanos retention duration for raw (0s) resolution blocks.")
		retention5m        = flag.String("retention.resolution-5m", bucketinspect.DefaultRetentionDuration, "Configured Thanos retention duration for 5m downsampled blocks.")
		retention1h        = flag.String("retention.resolution-1h", bucketinspect.DefaultRetentionDuration, "Configured Thanos retention duration for 1h downsampled blocks.")
		logLevel           = flag.String("log.level", "info", "Log level for structured JSON output (debug, info, warn, error).")
	)
	flag.Parse()

	parsedLevel, err := parseLogLevel(*logLevel)
	if err != nil {
		slog.Error("invalid --log.level flag value", "level", *logLevel, "err", err)
		return 1
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parsedLevel}))
	slog.SetDefault(logger)

	cfg, err := estimator.LoadConfigWithDefaults(*configFile, *windowInterval, *maxItemsPerWindow)
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inspector, err := initBucketInspector(
		ctx,
		*objstoreConfigFile,
		*bucketSyncInterval,
		*retentionRaw,
		*retention5m,
		*retention1h,
		logger,
	)
	if err != nil {
		logger.Error("failed to initialize bucket inspector", "file", *objstoreConfigFile, "err", err)
		return 1
	}

	metrics := &serverMetrics{}
	requireClientCert := *tlsClientCAFile != ""

	var mainMux *http.ServeMux
	var metricsSrv *http.Server
	if *metricsListenAddr != "" {
		mainMux = newWriteServerMux(eng, pusher, inspector, metrics, *tenantHeader, *maxRequestSize, requireClientCert, logger)
		metricsMux := newMetricsServerMux(eng, pusher, inspector, metrics, logger)
		metricsSrv = &http.Server{
			Addr:              *metricsListenAddr,
			Handler:           metricsMux,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       120 * time.Second,
		}
	} else {
		mainMux = newServerMux(eng, pusher, inspector, metrics, *tenantHeader, *maxRequestSize, requireClientCert, logger)
	}

	srv := &http.Server{
		Addr:              *listenAddr,
		Handler:           mainMux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	useTLS := *tlsServerCertFile != "" || *tlsServerKeyFile != ""
	if useTLS {
		tlsCfg, err := buildTLSConfig(*tlsServerCertFile, *tlsServerKeyFile, *tlsClientCAFile, *tlsMinVersion, *tlsCipherSuites)
		if err != nil {
			logger.Error("failed to configure TLS for Remote Write server", "err", err)
			return 1
		}
		srv.TLSConfig = tlsCfg
	}

	go pusher.Run(ctx)

	if metricsSrv != nil {
		go func() {
			logger.Info("starting metrics HTTP server", "listen", *metricsListenAddr)
			if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("metrics HTTP server terminated with error", "err", err)
				cancel()
			}
		}()
	}

	stopSigCh := make(chan os.Signal, 1)
	signal.Notify(stopSigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-stopSigCh
		logger.Info("received shutdown signal", "signal", sig.String())
		metrics.shuttingDown.Store(true)
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if metricsSrv != nil {
			if err := metricsSrv.Shutdown(shutdownCtx); err != nil {
				logger.Warn("failed to gracefully shut down metrics HTTP server", "err", err)
			}
		}
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Warn("failed to gracefully shut down Remote Write server", "err", err)
		}
		cancel()
	}()

	logger.Info("starting ACM metrics inspector", "listen", *listenAddr, "tls", useTLS, "mtls", requireClientCert)
	if useTLS {
		ln, err := net.Listen("tcp", *listenAddr)
		if err != nil {
			logger.Error("failed to listen on address", "listen", *listenAddr, "err", err)
			return 1
		}
		tlsListener := tls.NewListener(ln, srv.TLSConfig)
		if err := srv.Serve(tlsListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTPS server terminated with error", "err", err)
			return 1
		}
		return 0
	}
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("HTTP server terminated with error", "err", err)
		return 1
	}
	return 0
}

func initBucketInspector(
	ctx context.Context,
	objstoreConfigFile string,
	syncInterval time.Duration,
	retentionRaw, retention5m, retention1h string,
	logger *slog.Logger,
) (*bucketinspect.Inspector, error) {
	if objstoreConfigFile == "" {
		return nil, nil
	}

	bkt, err := bucketinspect.NewBucketFromFile(objstoreConfigFile)
	if err != nil {
		return nil, fmt.Errorf("initialize object storage bucket client: %w", err)
	}

	retCfg, err := bucketinspect.ParseRetentionConfig(retentionRaw, retention5m, retention1h)
	if err != nil {
		if closeErr := bkt.Close(); closeErr != nil {
			logger.Warn("failed to close object storage bucket client", "err", closeErr)
		}
		return nil, fmt.Errorf("parse retention configuration: %w", err)
	}

	inspector := bucketinspect.NewInspector(bkt, bucketinspect.InspectorConfig{
		SyncInterval: syncInterval,
		Retention:    retCfg,
		Logger:       logger,
	})
	go func() {
		defer func() {
			if err := bkt.Close(); err != nil {
				logger.Warn("failed to close object storage bucket client", "err", err)
			}
		}()
		inspector.Run(ctx)
	}()
	return inspector, nil
}

func parseLogLevel(level string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unsupported log level %q (expected debug, info, warn, or error)", level)
	}
}

func buildTLSConfig(certFile, keyFile, clientCAFile, minVersionStr, cipherSuitesStr string) (*tls.Config, error) {
	if minVersionStr == "" {
		minVersionStr = "VersionTLS12"
	}
	minVersion, err := k8sapiflag.TLSVersion(minVersionStr)
	if err != nil {
		return nil, fmt.Errorf("invalid TLS min version %q: %w", minVersionStr, err)
	}

	var cipherSuites []uint16
	if trimmed := strings.TrimSpace(cipherSuitesStr); trimmed != "" {
		parts := strings.Split(trimmed, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		cipherSuites, err = k8sapiflag.TLSCipherSuites(parts)
		if err != nil {
			return nil, fmt.Errorf("invalid TLS cipher suites %q: %w", cipherSuitesStr, err)
		}
	}

	if _, err := loadHandshakeTLSConfig(certFile, keyFile, clientCAFile, minVersion, cipherSuites); err != nil {
		return nil, err
	}
	return &tls.Config{ //nolint:gosec // TLS min version and cipher suites are configured dynamically from the cluster TLS security profile
		MinVersion:   minVersion,
		CipherSuites: cipherSuites,
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			return loadHandshakeTLSConfig(certFile, keyFile, clientCAFile, minVersion, cipherSuites)
		},
	}, nil
}

func loadHandshakeTLSConfig(certFile, keyFile, clientCAFile string, minVersion uint16, cipherSuites []uint16) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load server TLS key pair: %w", err)
	}
	cfg := &tls.Config{ //nolint:gosec // TLS min version and cipher suites are configured dynamically from the cluster TLS security profile
		MinVersion:   minVersion,
		CipherSuites: cipherSuites,
		Certificates: []tls.Certificate{cert},
	}
	if clientCAFile != "" {
		caPEM, err := os.ReadFile(clientCAFile) // #nosec G304 -- Path is provided via operator CLI flag.
		if err != nil {
			return nil, fmt.Errorf("failed to read client CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, errors.New("failed to parse client CA certificate PEM")
		}
		cfg.ClientCAs = pool
		cfg.ClientAuth = tls.VerifyClientCertIfGiven
	}
	return cfg, nil
}

func newServerMux(
	eng *estimator.Engine,
	pusher *remotewrite.Pusher,
	inspector *bucketinspect.Inspector,
	m *serverMetrics,
	tenantHeader string,
	maxRequestSize int64,
	requireClientCert bool,
	logger *slog.Logger,
) *http.ServeMux {
	mux := http.NewServeMux()
	registerWriteHandlers(mux, eng, pusher, m, tenantHeader, maxRequestSize, requireClientCert, logger)
	registerMetricsHandlers(mux, eng, pusher, inspector, m, logger)
	registerHealthHandlers(mux, pusher, inspector, m)
	return mux
}

func newWriteServerMux(
	eng *estimator.Engine,
	pusher *remotewrite.Pusher,
	inspector *bucketinspect.Inspector,
	m *serverMetrics,
	tenantHeader string,
	maxRequestSize int64,
	requireClientCert bool,
	logger *slog.Logger,
) *http.ServeMux {
	mux := http.NewServeMux()
	registerWriteHandlers(mux, eng, pusher, m, tenantHeader, maxRequestSize, requireClientCert, logger)
	registerHealthHandlers(mux, pusher, inspector, m)
	return mux
}

func newMetricsServerMux(
	eng *estimator.Engine,
	pusher *remotewrite.Pusher,
	inspector *bucketinspect.Inspector,
	m *serverMetrics,
	logger *slog.Logger,
) *http.ServeMux {
	mux := http.NewServeMux()
	registerMetricsHandlers(mux, eng, pusher, inspector, m, logger)
	registerHealthHandlers(mux, pusher, inspector, m)
	return mux
}

func registerWriteHandlers(
	mux *http.ServeMux,
	eng *estimator.Engine,
	pusher *remotewrite.Pusher,
	m *serverMetrics,
	tenantHeader string,
	maxRequestSize int64,
	requireClientCert bool,
	logger *slog.Logger,
) {
	writeHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if requireClientCert && (r.TLS == nil || len(r.TLS.VerifiedChains) == 0) {
			m.writeErrorsTotal.Add(1)
			http.Error(w, "client certificate required", http.StatusUnauthorized)
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
}

func registerMetricsHandlers(
	mux *http.ServeMux,
	eng *estimator.Engine,
	pusher *remotewrite.Pusher,
	inspector *bucketinspect.Inspector,
	m *serverMetrics,
	logger *slog.Logger,
) {
	mux.HandleFunc("/cardinality/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		if err := eng.WritePrometheusText(w, time.Now()); err != nil {
			logger.Warn("failed to write cardinality report metrics", "err", err)
		}
	})

	reg := prometheus.NewRegistry()
	reg.MustRegister(operationalCollector{eng: eng, pusher: pusher, m: m})
	if inspector != nil {
		reg.MustRegister(inspector)
	}
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
}

func registerHealthHandlers(
	mux *http.ServeMux,
	pusher *remotewrite.Pusher,
	inspector *bucketinspect.Inspector,
	m *serverMetrics,
) {
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if m != nil && m.shuttingDown.Load() {
			http.Error(w, "shutting down", http.StatusServiceUnavailable)
			return
		}
		if ok, reason := pusher.Ready(); !ok {
			http.Error(w, "thanos receive not ready: "+reason, http.StatusServiceUnavailable)
			return
		}
		if ok, reason := inspector.Ready(); !ok {
			http.Error(w, "object storage not ready: "+reason, http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
}
