// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package proxy

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/util"
)

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
	util.InitUserProjectInfo()
	up := util.NewUserProject("test", "test", []string{"p"})
	util.UpdateUserProject(up)
	util.InitAllManagedClusterNames()
	clusters := util.GetAllManagedClusterNames()
	clusters["p"] = "p"
	err := preCheckRequest(req)
	if err != nil {
		t.Errorf("failed to test preCheckRequest: %v", err)
	}

	resp.Request.Header.Del("X-Forwarded-Access-Token")
	resp.Request.Header.Add("Authorization", "test")
	err = preCheckRequest(req)
	if err != nil {
		t.Errorf("failed to test preCheckRequest with bear token: %v", err)
	}

	resp.Request.Header.Del("X-Forwarded-User")
	err = preCheckRequest(req)
	if !strings.Contains(err.Error(), "failed to found user name") {
		t.Errorf("failed to test preCheckRequest: %v", err)
	}

	resp.Request.Header.Del("X-Forwarded-Access-Token")
	resp.Request.Header.Del("Authorization")
	err = preCheckRequest(req)
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
		t.Errorf("(%v) is not the expected: (\"\")", req.Header.Get("Content-Type"))
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

		if req.URL.Scheme != "" {
			t.Errorf("(%v) is not the expected \"\"", req.URL.Scheme)
		}

		if req.URL.Host != "" {
			t.Errorf("(%v) is not the expected \"\"", req.URL.Host)
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
