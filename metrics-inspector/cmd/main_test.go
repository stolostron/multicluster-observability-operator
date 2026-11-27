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
	"sync"
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

	writeMux := newWriteServerMux(eng, pusher, metrics, remotewrite.DefaultTenantHeader, protoparser.DefaultMaxWriteRequestSize, true, slog.Default())
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

	pusher := remotewrite.NewPusher(remotewrite.PusherConfig{}, eng)
	inspector := bucketinspect.NewInspector(nil, bucketinspect.InspectorConfig{})
	metrics := &serverMetrics{}

	mux := newServerMux(eng, pusher, inspector, metrics, remotewrite.DefaultTenantHeader, protoparser.DefaultMaxWriteRequestSize, false, slog.Default())
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// 1. /healthz and /readyz are 200 OK immediately when server starts (decoupled from backend probes).
	hResp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz failed: %v", err)
	}
	_ = hResp.Body.Close()
	if hResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from /healthz, got %d", hResp.StatusCode)
	}

	rResp, err := http.Get(srv.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz failed: %v", err)
	}
	_ = rResp.Body.Close()
	if rResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from /readyz, got %d", rResp.StatusCode)
	}

	// 2. During graceful shutdown, /readyz returns 503 while /healthz remains 200 OK.
	metrics.shuttingDown.Store(true)
	rRespShutdown, err := http.Get(srv.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz failed: %v", err)
	}
	_ = rRespShutdown.Body.Close()
	if rRespShutdown.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 from /readyz during shutdown, got %d", rRespShutdown.StatusCode)
	}

	hRespShutdown, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz failed: %v", err)
	}
	_ = hRespShutdown.Body.Close()
	if hRespShutdown.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from /healthz during shutdown, got %d", hRespShutdown.StatusCode)
	}
}

func TestTenantNotObservedOnInvalidWriteRequest(t *testing.T) {
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

	pusher := remotewrite.NewPusher(remotewrite.PusherConfig{}, eng)
	inspector := bucketinspect.NewInspector(nil, bucketinspect.InspectorConfig{})
	metrics := &serverMetrics{}

	mux := newServerMux(eng, pusher, inspector, metrics, remotewrite.DefaultTenantHeader, protoparser.DefaultMaxWriteRequestSize, false, slog.Default())
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// 1. Post corrupt body with tenant header. Should return 400 Bad Request and not observe tenant.
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/write", bytes.NewReader([]byte("not-valid-snappy-payload")))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set(remotewrite.DefaultTenantHeader, "malformed-tenant")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", resp.StatusCode)
	}
	if got := pusher.TenantID(); got != remotewrite.DefaultTenantID {
		t.Fatalf("expected default tenant ID %q after invalid write request, got %q", remotewrite.DefaultTenantID, got)
	}

	// 2. Post valid write request. Tenant should be observed.
	wr := prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels:  []prompb.Label{{Name: "__name__", Value: "up"}},
				Samples: []prompb.Sample{{Value: 1, Timestamp: 1000}},
			},
		},
	}
	pb, _ := wr.Marshal()
	validReq, err := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/write", bytes.NewReader(snappy.Encode(nil, pb)))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	validReq.Header.Set(remotewrite.DefaultTenantHeader, "valid-tenant")
	validResp, err := http.DefaultClient.Do(validReq)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_ = validResp.Body.Close()
	if validResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 No Content, got %d", validResp.StatusCode)
	}
	if got := pusher.TenantID(); got != "valid-tenant" {
		t.Fatalf("expected 'valid-tenant', got %q", got)
	}
}

func TestInitBucketInspectorFailFast(t *testing.T) {
	var bgWg sync.WaitGroup
	if _, err := initBucketInspector(t.Context(), &bgWg, "/nonexistent/objstore.yaml", time.Minute, "15d", "90d", "180d", true, slog.Default()); err == nil {
		t.Fatal("expected error for missing objstore config file")
	}

	cfgPath := filepath.Join(t.TempDir(), "thanos.yaml")
	if err := os.WriteFile(cfgPath, []byte(fmt.Sprintf("type: FILESYSTEM\nconfig:\n  directory: %s\n", t.TempDir())), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := initBucketInspector(t.Context(), &bgWg, cfgPath, time.Minute, "invalid-duration", "90d", "180d", true, slog.Default()); err == nil {
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

func TestRotatingTLSConfigLiveReload(t *testing.T) {
	tmpDir := t.TempDir()
	_, certFile1, keyFile1 := generateTestCerts(t, tmpDir, "cert1", true)

	// Copy cert1 to target cert/key paths
	certPath := filepath.Join(tmpDir, "tls.crt")
	keyPath := filepath.Join(tmpDir, "tls.key")
	certBytes1, err := os.ReadFile(certFile1)
	if err != nil {
		t.Fatalf("ReadFile cert1: %v", err)
	}
	keyBytes1, err := os.ReadFile(keyFile1)
	if err != nil {
		t.Fatalf("ReadFile key1: %v", err)
	}
	if err := os.WriteFile(certPath, certBytes1, 0o600); err != nil {
		t.Fatalf("WriteFile cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyBytes1, 0o600); err != nil {
		t.Fatalf("WriteFile key: %v", err)
	}

	// Instantiate rotating config initially with cert1
	rot, err := newRotatingTLSConfig(certPath, keyPath, "", tls.VersionTLS12, nil)
	if err != nil {
		t.Fatalf("newRotatingTLSConfig failed: %v", err)
	}
	rotCfg1, err := rot.getConfig(nil)
	if err != nil {
		t.Fatalf("getConfig failed: %v", err)
	}
	leafCert1 := rotCfg1.Certificates[0].Certificate[0]

	// Fast path check: subsequent call returns identical config pointer without disk I/O
	rotCfgFast, err := rot.getConfig(nil)
	if err != nil {
		t.Fatalf("getConfig fast path failed: %v", err)
	}
	if rotCfgFast != rotCfg1 {
		t.Fatal("expected cached tls.Config instance returned on fast path")
	}

	// Generate cert2 and overwrite files with a distinct future timestamp via os.Chtimes
	_, certFile2, keyFile2 := generateTestCerts(t, tmpDir, "cert2", true)
	certBytes2, err := os.ReadFile(certFile2)
	if err != nil {
		t.Fatalf("ReadFile cert2: %v", err)
	}
	keyBytes2, err := os.ReadFile(keyFile2)
	if err != nil {
		t.Fatalf("ReadFile key2: %v", err)
	}
	if err := os.WriteFile(certPath, certBytes2, 0o600); err != nil {
		t.Fatalf("WriteFile cert2: %v", err)
	}
	if err := os.WriteFile(keyPath, keyBytes2, 0o600); err != nil {
		t.Fatalf("WriteFile key2: %v", err)
	}
	futureTime := time.Now().Add(5 * time.Second)
	if err := os.Chtimes(certPath, futureTime, futureTime); err != nil {
		t.Fatalf("Chtimes certPath: %v", err)
	}
	if err := os.Chtimes(keyPath, futureTime, futureTime); err != nil {
		t.Fatalf("Chtimes keyPath: %v", err)
	}

	// Within 10s cooldown window, fast path STILL returns cached cert1 despite disk change
	rotCfgStillCached, err := rot.getConfig(nil)
	if err != nil {
		t.Fatalf("getConfig during cooldown failed: %v", err)
	}
	if rotCfgStillCached != rotCfg1 {
		t.Fatal("expected cached config returned within cooldown window")
	}

	// Simulate cooldown expiration: backdate lastCheckNano
	rot.lastCheckNano.Store(time.Since(rot.startTime).Nanoseconds() - int64(20*time.Second))

	// Next call must trigger reload and return cert2
	reloadedCfg, err := rot.getConfig(nil)
	if err != nil {
		t.Fatalf("getConfig after cooldown failed: %v", err)
	}
	leafCert2 := reloadedCfg.Certificates[0].Certificate[0]
	if bytes.Equal(leafCert1, leafCert2) {
		t.Fatal("expected leaf certificate to change after live reload")
	}
}

func TestRotatingTLSConfigK8sSymlinkSwapSameModTime(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Create two backing directories representing Kubernetes Secret revisions (..v1 and ..v2)
	v1Dir := filepath.Join(tmpDir, "..v1")
	v2Dir := filepath.Join(tmpDir, "..v2")
	if err := os.MkdirAll(v1Dir, 0o700); err != nil {
		t.Fatalf("MkdirAll v1Dir: %v", err)
	}
	if err := os.MkdirAll(v2Dir, 0o700); err != nil {
		t.Fatalf("MkdirAll v2Dir: %v", err)
	}

	// Generate cert1 and cert2
	_, certFile1, keyFile1 := generateTestCerts(t, tmpDir, "cert1", true)
	_, certFile2, keyFile2 := generateTestCerts(t, tmpDir, "cert2", true)

	certBytes1, err := os.ReadFile(certFile1)
	if err != nil {
		t.Fatalf("ReadFile cert1: %v", err)
	}
	keyBytes1, err := os.ReadFile(keyFile1)
	if err != nil {
		t.Fatalf("ReadFile key1: %v", err)
	}
	certBytes2, err := os.ReadFile(certFile2)
	if err != nil {
		t.Fatalf("ReadFile cert2: %v", err)
	}
	keyBytes2, err := os.ReadFile(keyFile2)
	if err != nil {
		t.Fatalf("ReadFile key2: %v", err)
	}

	v1Cert := filepath.Join(v1Dir, "tls.crt")
	v1Key := filepath.Join(v1Dir, "tls.key")
	v2Cert := filepath.Join(v2Dir, "tls.crt")
	v2Key := filepath.Join(v2Dir, "tls.key")

	if err := os.WriteFile(v1Cert, certBytes1, 0o600); err != nil {
		t.Fatalf("WriteFile v1Cert: %v", err)
	}
	if err := os.WriteFile(v1Key, keyBytes1, 0o600); err != nil {
		t.Fatalf("WriteFile v1Key: %v", err)
	}
	if err := os.WriteFile(v2Cert, certBytes2, 0o600); err != nil {
		t.Fatalf("WriteFile v2Cert: %v", err)
	}
	if err := os.WriteFile(v2Key, keyBytes2, 0o600); err != nil {
		t.Fatalf("WriteFile v2Key: %v", err)
	}

	// Enforce identical ModTime on both versions to ensure detection relies on os.SameFile (inode change)
	sameTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, f := range []string{v1Cert, v1Key, v2Cert, v2Key} {
		if err := os.Chtimes(f, sameTime, sameTime); err != nil {
			t.Fatalf("Chtimes on %s: %v", f, err)
		}
	}

	// 2. Setup kubelet AtomicWriter symlink structure:
	// ..data -> ..v1
	// tls.crt -> ..data/tls.crt
	// tls.key -> ..data/tls.key
	dataSymlink := filepath.Join(tmpDir, "..data")
	if err := os.Symlink(v1Dir, dataSymlink); err != nil {
		t.Fatalf("Symlink ..data -> ..v1: %v", err)
	}

	certPath := filepath.Join(tmpDir, "tls.crt")
	keyPath := filepath.Join(tmpDir, "tls.key")
	if err := os.Symlink(filepath.Join(dataSymlink, "tls.crt"), certPath); err != nil {
		t.Fatalf("Symlink tls.crt -> ..data/tls.crt: %v", err)
	}
	if err := os.Symlink(filepath.Join(dataSymlink, "tls.key"), keyPath); err != nil {
		t.Fatalf("Symlink tls.key -> ..data/tls.key: %v", err)
	}

	// 3. Initialize rotating config pointing to symlinks and verify cert1 is loaded
	rot, err := newRotatingTLSConfig(certPath, keyPath, "", tls.VersionTLS12, nil)
	if err != nil {
		t.Fatalf("newRotatingTLSConfig failed: %v", err)
	}
	rotCfg1, err := rot.getConfig(nil)
	if err != nil {
		t.Fatalf("getConfig failed: %v", err)
	}
	if len(rotCfg1.Certificates) == 0 {
		t.Fatal("expected certificate loaded")
	}
	leafCert1 := rotCfg1.Certificates[0].Certificate[0]

	// 4. Atomically swap ..data to ..v2 as AtomicWriter does:
	// symlink ..data_tmp -> ..v2 followed by rename(..data_tmp, ..data)
	dataTmpSymlink := filepath.Join(tmpDir, "..data_tmp")
	if err := os.Symlink(v2Dir, dataTmpSymlink); err != nil {
		t.Fatalf("Symlink ..data_tmp -> ..v2: %v", err)
	}
	if err := os.Rename(dataTmpSymlink, dataSymlink); err != nil {
		t.Fatalf("Rename ..data_tmp -> ..data: %v", err)
	}

	// Within 10s cooldown window, fast path STILL returns cached cert1
	rotCfgCached, err := rot.getConfig(nil)
	if err != nil {
		t.Fatalf("getConfig during cooldown failed: %v", err)
	}
	if rotCfgCached != rotCfg1 {
		t.Fatal("expected cached config returned within cooldown window")
	}

	// 5. Expire cooldown: backdate lastCheckNano
	rot.lastCheckNano.Store(time.Since(rot.startTime).Nanoseconds() - int64(20*time.Second))

	// 6. Assert reload occurs and returns cert2 even though ModTime is identical
	rotCfg2, err := rot.getConfig(nil)
	if err != nil {
		t.Fatalf("getConfig after cooldown failed: %v", err)
	}
	leafCert2 := rotCfg2.Certificates[0].Certificate[0]
	if bytes.Equal(leafCert1, leafCert2) {
		t.Fatal("expected leaf certificate to change after atomic symlink swap despite identical ModTime")
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
