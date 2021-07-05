// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package proxy

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/open-cluster-management/multicluster-observability-operator/proxy/pkg/util"
)

func TestNewEmptyMatrixHTTPBody(t *testing.T) {
	body := newEmptyMatrixHTTPBody()
	gr, err := gzip.NewReader(bytes.NewBuffer([]byte(body)))
	defer gr.Close()
	data, err := ioutil.ReadAll(gr)
	if err != nil {
		log.Fatal(err)
	}

	var decompressedBuff bytes.Buffer
	gr, err = gzip.NewReader(bytes.NewBuffer([]byte(data)))
	defer gr.Close()
	data, err = ioutil.ReadAll(gr)
	if err != nil {
		t.Errorf("failed to ReadAll: %v", err)
	}

	decompressedBuff.Write(data)
	emptyMatrix := `{"status":"success","data":{"resultType":"matrix","result":[]}}`
	if decompressedBuff.String() != emptyMatrix {
		t.Errorf("(%v) is not the expected: (%v)", decompressedBuff.String(), emptyMatrix)
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

func TestErrorHandle(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://127.0.0.1:3002/metrics/query?query=foo", nil)
	req.Header.Set("X-Forwarded-User", "test")
	var err error
	fakeResp := NewFakeResponse(t)
	errorHandle(fakeResp, req, err)
	if fakeResp.status != http.StatusUnauthorized {
		t.Errorf("failed to get expected status: %v", fakeResp.status)
	}
}

func TestPreCheckRequest(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://127.0.0.1:3002/metrics/query?query=foo", nil)
	resp := http.Response{
		Body:    ioutil.NopCloser(bytes.NewBufferString("test")),
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

func TestGzipWrite(t *testing.T) {
	originalStr := "test"
	var compressedBuff bytes.Buffer
	err := gzipWrite(&compressedBuff, []byte(originalStr))
	if err != nil {
		t.Errorf("failed to compressed: %v", err)
	}
	var decompressedBuff bytes.Buffer
	gr, err := gzip.NewReader(bytes.NewBuffer(compressedBuff.Bytes()))
	defer gr.Close()
	data, err := ioutil.ReadAll(gr)
	if err != nil {
		t.Errorf("failed to decompressed: %v", err)
	}
	decompressedBuff.Write(data)
	if decompressedBuff.String() != originalStr {
		t.Errorf("(%v) is not the expected: (%v)", originalStr, decompressedBuff.String())
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
