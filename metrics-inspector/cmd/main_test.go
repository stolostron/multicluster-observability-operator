// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stolostron/multicluster-observability-operator/metrics-inspector/pkg/bucketinspect"
	"github.com/stolostron/multicluster-observability-operator/metrics-inspector/pkg/estimator"
	"github.com/stolostron/multicluster-observability-operator/metrics-inspector/pkg/protoparser"
	"github.com/stolostron/multicluster-observability-operator/metrics-inspector/pkg/remotewrite"
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
	inspector := bucketinspect.NewInspector(nil, bucketinspect.InspectorConfig{})
	metrics := &serverMetrics{}
	mux := newServerMux(eng, pusher, inspector, metrics, remotewrite.DefaultTenantHeader, protoparser.DefaultMaxWriteRequestSize, false, slog.Default())
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
	if !strings.Contains(string(cBody), `cardinality_estimate{by_cluster="managed-1",group_by_keys="cluster",group_by_values="managed-1",interval="15m0s"} 2`) {
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
	if !strings.Contains(string(mBody), "thanos_bucket_syncs_total 0") {
		t.Fatalf("expected thanos_bucket_syncs_total in /metrics output, got:\n%s", string(mBody))
	}
	if !strings.Contains(string(mBody), `thanos_bucket_configured_retention_seconds{resolution="raw"}`) {
		t.Fatalf("expected thanos_bucket_configured_retention_seconds in /metrics output, got:\n%s", string(mBody))
	}
}

func TestWriteServerMTLSAndMetricsSeparation(t *testing.T) {
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

	tmpDir := t.TempDir()
	serverCA, serverCertFile, serverKeyFile := generateTestCerts(t, tmpDir, "server", true)
	clientCA, clientCertFile, clientKeyFile := generateTestCerts(t, tmpDir, "client", false)
	clientCAFile := filepath.Join(tmpDir, "client-ca.crt")
	if err := os.WriteFile(clientCAFile, clientCA, 0o600); err != nil {
		t.Fatalf("WriteFile client-ca.crt failed: %v", err)
	}

	tlsCfg, err := buildTLSConfig(serverCertFile, serverKeyFile, clientCAFile, "VersionTLS12", "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256")
	if err != nil {
		t.Fatalf("buildTLSConfig failed: %v", err)
	}

	pusher := remotewrite.NewPusher(remotewrite.PusherConfig{}, eng)
	inspector := bucketinspect.NewInspector(nil, bucketinspect.InspectorConfig{})
	metrics := &serverMetrics{}

	writeMux := newWriteServerMux(eng, pusher, inspector, metrics, remotewrite.DefaultTenantHeader, protoparser.DefaultMaxWriteRequestSize, true, slog.Default())
	writeSrv := httptest.NewUnstartedServer(writeMux)
	writeSrv.TLS = tlsCfg
	writeSrv.StartTLS()
	defer writeSrv.Close()

	metricsMux := newMetricsServerMux(eng, pusher, inspector, metrics, slog.Default())
	metricsSrv := httptest.NewServer(metricsMux)
	defer metricsSrv.Close()

	serverCAPool := x509.NewCertPool()
	if !serverCAPool.AppendCertsFromPEM(serverCA) {
		t.Fatal("failed to parse server CA")
	}

	// Client without a client certificate: /healthz succeeds, /api/v1/write returns 401, /metrics returns 404.
	noCertClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    serverCAPool,
				MinVersion: tls.VersionTLS12,
			},
		},
	}

	hResp, err := noCertClient.Get(writeSrv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz without client cert failed: %v", err)
	}
	_ = hResp.Body.Close()
	if hResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from /healthz without client cert, got %d", hResp.StatusCode)
	}

	m404Resp, err := noCertClient.Get(writeSrv.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics on write server failed: %v", err)
	}
	_ = m404Resp.Body.Close()
	if m404Resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 NotFound for /metrics on write server, got %d", m404Resp.StatusCode)
	}

	wr := prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "up"},
					{Name: "cluster", Value: "local-cluster"},
				},
				Samples: []prompb.Sample{{Value: 1, Timestamp: 1000}},
			},
		},
	}
	pb, _ := wr.Marshal()
	compressed := snappy.Encode(nil, pb)

	unauthResp, err := noCertClient.Post(writeSrv.URL+"/api/v1/write", "application/x-protobuf", bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("POST /api/v1/write without client cert failed: %v", err)
	}
	_ = unauthResp.Body.Close()
	if unauthResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized without client cert, got %d", unauthResp.StatusCode)
	}

	// Client with a valid client certificate: /api/v1/write succeeds with 204 NoContent.
	clientPair, err := tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
	if err != nil {
		t.Fatalf("LoadX509KeyPair failed: %v", err)
	}
	mtlsClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      serverCAPool,
				Certificates: []tls.Certificate{clientPair},
				MinVersion:   tls.VersionTLS12,
			},
		},
	}

	authResp, err := mtlsClient.Post(writeSrv.URL+"/api/v1/write", "application/x-protobuf", bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("POST /api/v1/write with mTLS client cert failed: %v", err)
	}
	_ = authResp.Body.Close()
	if authResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 NoContent with mTLS client cert, got %d", authResp.StatusCode)
	}

	// Verify /metrics on the dedicated metrics server reports the ingested series.
	mResp, err := http.Get(metricsSrv.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics on metrics server failed: %v", err)
	}
	mBody, _ := io.ReadAll(mResp.Body)
	_ = mResp.Body.Close()
	if !strings.Contains(string(mBody), "cardinality_estimator_ingested_series_total 1") {
		t.Fatalf("expected ingested_series_total=1 on metrics server, got:\n%s", string(mBody))
	}
}

func generateTestCerts(t *testing.T, dir, prefix string, isServer bool) (caPEM []byte, certFile, keyFile string) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey CA failed: %v", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: prefix + "-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("CreateCertificate CA failed: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("ParseCertificate CA failed: %v", err)
	}
	caPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey leaf failed: %v", err)
	}
	extUsage := x509.ExtKeyUsageClientAuth
	if isServer {
		extUsage = x509.ExtKeyUsageServerAuth
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: prefix + "-leaf"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{extUsage},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("CreateCertificate leaf failed: %v", err)
	}
	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		t.Fatalf("MarshalECPrivateKey failed: %v", err)
	}

	certFile = filepath.Join(dir, prefix+".crt")
	keyFile = filepath.Join(dir, prefix+".key")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}), 0o600); err != nil {
		t.Fatalf("WriteFile cert failed: %v", err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: leafKeyDER}), 0o600); err != nil {
		t.Fatalf("WriteFile key failed: %v", err)
	}
	return caPEM, certFile, keyFile
}

func TestReadinessAndLivenessProbes(t *testing.T) {
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

	receiveSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer receiveSrv.Close()

	bkt, err := bucketinspect.NewBucket([]byte(fmt.Sprintf("type: FILESYSTEM\nconfig:\n  directory: %s\n", t.TempDir())))
	if err != nil {
		t.Fatalf("NewBucket failed: %v", err)
	}
	defer func() { _ = bkt.Close() }()

	pusher := remotewrite.NewPusher(remotewrite.PusherConfig{Endpoint: receiveSrv.URL}, eng)
	inspector := bucketinspect.NewInspector(bkt, bucketinspect.InspectorConfig{})
	metrics := &serverMetrics{}

	mux := newServerMux(eng, pusher, inspector, metrics, remotewrite.DefaultTenantHeader, protoparser.DefaultMaxWriteRequestSize, false, slog.Default())
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// 1. Before initial Thanos Receive probe or bucket sync: /healthz is 200 OK, /readyz is 503 (thanos receive not ready).
	hResp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz failed: %v", err)
	}
	_ = hResp.Body.Close()
	if hResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from /healthz before ready, got %d", hResp.StatusCode)
	}

	rResp1, err := http.Get(srv.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz failed: %v", err)
	}
	rBody1, _ := io.ReadAll(rResp1.Body)
	_ = rResp1.Body.Close()
	if rResp1.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(rBody1), "thanos receive not ready") {
		t.Fatalf("expected 503 'thanos receive not ready', got %d: %s", rResp1.StatusCode, string(rBody1))
	}

	// 2. After Thanos Receive probe succeeds, /readyz still returns 503 until bucket sync succeeds.
	if err := pusher.Probe(t.Context()); err != nil {
		t.Fatalf("pusher.Probe failed: %v", err)
	}
	rResp2, err := http.Get(srv.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz failed: %v", err)
	}
	rBody2, _ := io.ReadAll(rResp2.Body)
	_ = rResp2.Body.Close()
	if rResp2.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(rBody2), "object storage not ready") {
		t.Fatalf("expected 503 'object storage not ready', got %d: %s", rResp2.StatusCode, string(rBody2))
	}

	// 3. After initial bucket sync succeeds, /readyz returns 200 OK.
	if err := inspector.Sync(t.Context()); err != nil {
		t.Fatalf("inspector.Sync failed: %v", err)
	}
	rResp3, err := http.Get(srv.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz failed: %v", err)
	}
	_ = rResp3.Body.Close()
	if rResp3.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from /readyz after both checks succeed, got %d", rResp3.StatusCode)
	}

	// 4. During graceful shutdown, /readyz returns 503 while /healthz remains 200 OK.
	metrics.shuttingDown.Store(true)
	rResp4, err := http.Get(srv.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz failed: %v", err)
	}
	_ = rResp4.Body.Close()
	if rResp4.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 from /readyz during shutdown, got %d", rResp4.StatusCode)
	}

	hResp2, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz failed: %v", err)
	}
	_ = hResp2.Body.Close()
	if hResp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from /healthz during shutdown, got %d", hResp2.StatusCode)
	}
}

func TestInitBucketInspectorFailFast(t *testing.T) {
	if _, err := initBucketInspector(t.Context(), "/nonexistent/objstore.yaml", time.Minute, "15d", "90d", "180d", slog.Default()); err == nil {
		t.Fatal("expected error for missing objstore config file")
	}

	cfgPath := filepath.Join(t.TempDir(), "thanos.yaml")
	if err := os.WriteFile(cfgPath, []byte(fmt.Sprintf("type: FILESYSTEM\nconfig:\n  directory: %s\n", t.TempDir())), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := initBucketInspector(t.Context(), cfgPath, time.Minute, "invalid-duration", "90d", "180d", slog.Default()); err == nil {
		t.Fatal("expected error for invalid retention duration")
	}
}

func TestBuildTLSConfigProfileOptions(t *testing.T) {
	tmpDir := t.TempDir()
	_, certFile, keyFile := generateTestCerts(t, tmpDir, "server", true)

	// Valid TLS 1.3 profile (no explicit cipher suites)
	cfg13, err := buildTLSConfig(certFile, keyFile, "", "VersionTLS13", "")
	if err != nil {
		t.Fatalf("buildTLSConfig VersionTLS13 failed: %v", err)
	}
	if cfg13.MinVersion != tls.VersionTLS13 {
		t.Fatalf("MinVersion = %x, want %x", cfg13.MinVersion, tls.VersionTLS13)
	}
	hs13, err := cfg13.GetConfigForClient(nil)
	if err != nil {
		t.Fatalf("GetConfigForClient failed: %v", err)
	}
	if hs13.MinVersion != tls.VersionTLS13 {
		t.Fatalf("handshake MinVersion = %x, want %x", hs13.MinVersion, tls.VersionTLS13)
	}

	// Invalid TLS version
	if _, err := buildTLSConfig(certFile, keyFile, "", "VersionTLS99", ""); err == nil {
		t.Fatal("expected error for invalid TLS min version")
	}

	// Invalid cipher suite name
	if _, err := buildTLSConfig(certFile, keyFile, "", "VersionTLS12", "INVALID_CIPHER_SUITE"); err == nil {
		t.Fatal("expected error for invalid TLS cipher suite")
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input   string
		want    slog.Level
		wantErr bool
	}{
		{input: "debug", want: slog.LevelDebug},
		{input: "DEBUG", want: slog.LevelDebug},
		{input: "info", want: slog.LevelInfo},
		{input: " warn ", want: slog.LevelWarn},
		{input: "warning", want: slog.LevelWarn},
		{input: "error", want: slog.LevelError},
		{input: "trace", wantErr: true},
		{input: "", wantErr: true},
	}
	for _, tc := range tests {
		got, err := parseLogLevel(tc.input)
		if (err != nil) != tc.wantErr {
			t.Fatalf("parseLogLevel(%q) error = %v, wantErr = %v", tc.input, err, tc.wantErr)
		}
		if !tc.wantErr && got != tc.want {
			t.Fatalf("parseLogLevel(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}
