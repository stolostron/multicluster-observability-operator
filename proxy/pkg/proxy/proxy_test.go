// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package proxy

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/informer"
	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestNewProxy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse server url: %v", err)
	}

	upi := util.NewUserProjectInfo(24*60*60*time.Second, 5*60*time.Second)
	defer upi.Stop()

	p, err := NewProxy(serverURL, http.DefaultTransport, "", upi)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	if p.metricsServerURL != serverURL {
		t.Errorf("expected serverURL to be %v, got %v", serverURL, p.metricsServerURL)
	}

	if p.proxy == nil {
		t.Errorf("expected proxy to be initialized")
	}
}

func TestProxy_ServeHTTP(t *testing.T) {
	var directorCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		directorCalled = true
		if r.URL.Path != "/api/metrics/v1/default/test" {
			t.Errorf("expected path to be /api/metrics/v1/default/test, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse server url: %v", err)
	}

	tmpDir, err := os.MkdirTemp("", "certs")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	caFile, keyFile, certFile, err := generateCerts(tmpDir)
	if err != nil {
		t.Fatalf("failed to generate certs: %v", err)
	}

	transport, err := getTLSTransportWithOptions(&TLSOptions{
		CaFile:   caFile,
		KeyFile:  keyFile,
		CertFile: certFile,
	})
	if err != nil {
		t.Fatalf("failed to create transport: %v", err)
	}

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, projectsAPIPath) {
			w.Write([]byte(`{"items":[{"metadata":{"name":"dummy"},"spec":{}}]}
`))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer apiServer.Close()

	upi := util.NewUserProjectInfo(24*60*60*time.Second, 0)
	defer upi.Stop()

	p, err := NewProxy(serverURL, transport, apiServer.URL, upi)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	req := httptest.NewRequest("GET", "http://localhost/test", nil)
	req.Header.Set("X-Forwarded-User", "test")
	req.Header.Set("X-Forwarded-Access-Token", "test")
	w := httptest.NewRecorder()

	p.ServeHTTP(w, req)

	if !directorCalled {
		t.Errorf("director was not called")
	}
}

func generateCerts(tmpDir string) (string, string, string, error) {
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(2019),
		Subject: pkix.Name{
			Organization: []string{"Company, INC."},
			Country:      []string{"US"},
			Province:     []string{""},
			Locality:     []string{"San Francisco"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return "", "", "", err
	}

	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return "", "", "", err
	}

	caPEM := new(bytes.Buffer)
	pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})

	caFile := filepath.Join(tmpDir, "ca.crt")
	if err := os.WriteFile(caFile, caPEM.Bytes(), 0644); err != nil {
		return "", "", "", err
	}

	cert := &x509.Certificate{
		SerialNumber: big.NewInt(2019),
		Subject: pkix.Name{
			Organization: []string{"Company, INC."},
			Country:      []string{"US"},
			Province:     []string{""},
			Locality:     []string{"San Francisco"},
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	certPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return "", "", "", err
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, ca, &certPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return "", "", "", err
	}

	certPEM := new(bytes.Buffer)
	pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	certFile := filepath.Join(tmpDir, "tls.crt")
	if err := os.WriteFile(certFile, certPEM.Bytes(), 0644); err != nil {
		return "", "", "", err
	}

	certPrivKeyPEM := new(bytes.Buffer)
	pem.Encode(certPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey),
	})

	keyFile := filepath.Join(tmpDir, "tls.key")
	if err := os.WriteFile(keyFile, certPrivKeyPEM.Bytes(), 0600); err != nil {
		return "", "", "", err
	}

	return caFile, keyFile, certFile, nil
}

func TestNewEmptyMatrixHTTPBody(t *testing.T) {
	body := newEmptyMatrixHTTPBody()

	emptyMatrix := `{"status":"success","data":{"resultType":"matrix","result":[]}}`
	if string(body) != emptyMatrix {
		t.Errorf("(%v) is not the expected: (%v)", body, emptyMatrix)
	}
}

type FakeResponse struct {
	t       *testing.T
	headers http.Header
	body    []byte
	status  int
}

func NewFakeResponse(t *testing.T) *FakeResponse {
	return &FakeResponse{
		t:       t,
		headers: make(http.Header),
	}
}

func (r *FakeResponse) Header() http.Header {
	return r.headers
}

func (r *FakeResponse) Write(body []byte) (int, error) {
	r.body = body
	return len(body), nil
}

func (r *FakeResponse) WriteHeader(status int) {
	r.status = status
}

func TestPreCheckRequest(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://127.0.0.1:3002/metrics/query?query=foo", nil)
	resp := http.Response{
		Body:    io.NopCloser(bytes.NewBufferString("test")),
		Header:  make(http.Header),
		Request: req,
	}
	resp.Request.Header.Set("X-Forwarded-Access-Token", "test")
	resp.Request.Header.Set("X-Forwarded-User", "test")

	upi := util.NewUserProjectInfo(24*60*60*time.Second, 5*60*time.Second)
	defer upi.Stop()
	upi.UpdateUserProject("test", "test", []string{"p"})

	informer.InitAllManagedClusterNames()
	clusters := informer.GetAllManagedClusterNames()
	clusters["p"] = "p"
	serverUrl, err := url.Parse("http://localhost/test")
	assert.NoError(t, err)
	p, err := NewProxy(serverUrl, nil, "", upi)
	assert.NoError(t, err)
	err = p.preCheckRequest(req)
	if err != nil {
		t.Errorf("failed to test preCheckRequest: %v", err)
	}

	resp.Request.Header.Del("X-Forwarded-Access-Token")
	resp.Request.Header.Add("Authorization", "Bearer test")
	err = p.preCheckRequest(req)
	if err != nil {
		t.Errorf("failed to test preCheckRequest with bearer token: %v", err)
	}
	// Check if the token is set correctly
	if resp.Request.Header.Get("X-Forwarded-Access-Token") != "test" {
		t.Errorf("expected X-Forwarded-Access-Token to be set to 'test', got: %s", resp.Request.Header.Get("X-Forwarded-Access-Token"))
	}

	resp.Request.Header.Del("X-Forwarded-User")
	err = p.preCheckRequest(req)
	if !strings.Contains(err.Error(), "failed to find user name") {
		t.Errorf("failed to test preCheckRequest: %v", err)
	}

	resp.Request.Header.Del("X-Forwarded-Access-Token")
	resp.Request.Header.Del("Authorization")
	err = p.preCheckRequest(req)
	if !strings.Contains(err.Error(), "found unauthorized user") {
		t.Errorf("failed to test preCheckRequest: %v", err)
	}

}

func TestProxyRequest(t *testing.T) {
	req := http.Request{}
	req.URL = &url.URL{}
	req.Header = http.Header(map[string][]string{})
	proxyRequest(&req)
	if req.Body != nil {
		t.Errorf("(%v) is not the expected nil", req.Body)
	}
	if req.Header.Get("Content-Type") != "" {
		t.Errorf(`(%v) is not the expected: ("")`, req.Header.Get("Content-Type"))
	}

	req.Method = http.MethodGet
	pathList := []string{
		"/api/v1/query",
		"/api/v1/query_range",
		"/api/v1/series",
	}

	for _, path := range pathList {
		req.URL.Path = path
		proxyRequest(&req)
		if req.Method != http.MethodPost {
			t.Errorf("(%v) is not the expected: (%v)", http.MethodPost, req.Method)
		}

		if req.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("(%v) is not the expected: (%v)", req.Header.Get("Content-Type"), "application/x-www-form-urlencoded")
		}

		if req.Body == nil {
			t.Errorf("(%v) is not the expected non-nil", req.Body)
		}
	}
}

func TestModifyAPISeriesResponseSeriesPOST(t *testing.T) {
	testCase := struct {
		name     string
		expected bool
	}{
		"should modify the api series response",
		true,
	}
	req := http.Request{}
	req.URL = &url.URL{}
	req.URL.Path = "/api/v1/series"
	req.Method = "POST"
	req.Header = http.Header(map[string][]string{})

	stringReader := strings.NewReader(config.GetRBACProxyLabelMetricName())
	stringReadClose := io.NopCloser(stringReader)
	req.Body = stringReadClose

	resp := NewFakeResponse(t)
	config.GetManagedClusterLabelList().RegexLabelList = []string{"cloud", "vendor"}
	if ok := shouldModifyAPISeriesResponse(resp, &req); !ok {
		t.Errorf("case (%v) output: (%v) is not the expected: (%v)", testCase.name, ok, testCase.expected)
	}

	stringReader = strings.NewReader("kube_pod_info")
	stringReadClose = io.NopCloser(stringReader)
	req.Body = stringReadClose

	if ok := shouldModifyAPISeriesResponse(resp, &req); ok {
		t.Errorf("case (%v) output: (%v) is not the expected: (%v)", testCase.name, ok, !testCase.expected)
	}
}

func TestModifyAPISeriesResponseSeriesGET(t *testing.T) {
	testCase := struct {
		name     string
		expected bool
	}{
		"should modify the api series response",
		true,
	}
	req := http.Request{}
	req.URL, _ = url.Parse("https://dummy.com/api/v1/series?match[]=" + config.GetRBACProxyLabelMetricName())
	req.Method = "GET"
	req.Header = http.Header(map[string][]string{})

	resp := NewFakeResponse(t)
	config.GetManagedClusterLabelList().RegexLabelList = []string{"cloud", "vendor"}
	if ok := shouldModifyAPISeriesResponse(resp, &req); !ok {
		t.Errorf("case (%v) output: (%v) is not the expected: (%v)", testCase.name, ok, testCase.expected)
	}

	req.URL, _ = url.Parse("https://dummy.com/api/v1/series?match[]=kube_pod_info")

	if ok := shouldModifyAPISeriesResponse(resp, &req); ok {
		t.Errorf("case (%v) output: (%v) is not the expected: (%v)", testCase.name, ok, !testCase.expected)
	}
}

func TestModifyAPISeriesResponseLabelsGET(t *testing.T) {
	testCase := struct {
		name     string
		expected bool
	}{
		"should modify the api series response",
		true,
	}
	req := http.Request{}
	req.URL, _ = url.Parse("https://dummy.com/api/v1/label/label_name/values?match[]=" + config.GetRBACProxyLabelMetricName())
	req.Method = "GET"

	req.Header = http.Header(map[string][]string{})

	resp := NewFakeResponse(t)
	config.GetManagedClusterLabelList().RegexLabelList = []string{"cloud", "vendor"}
	if ok := shouldModifyAPISeriesResponse(resp, &req); !ok {
		t.Errorf("case (%v) output: (%v) is not the expected: (%v)", testCase.name, ok, testCase.expected)
	}

	req.URL, _ = url.Parse("https://dummy.com/api/v1/label/label_name/values?matchf[]=kube_pod_info")

	if ok := shouldModifyAPISeriesResponse(resp, &req); ok {
		t.Errorf("case (%v) output: (%v) is not the expected: (%v)", testCase.name, ok, !testCase.expected)
	}
}
