// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package proxy

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	projectv1 "github.com/openshift/api/project/v1"
	userv1 "github.com/openshift/api/user/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/cache"
	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/config"
)

// MockManagedClusterInformer is a mock implementation of the ManagedClusterInformable interface.
type MockManagedClusterInformer struct {
	clusters       map[string]string
	labels         map[string]bool
	regexLabelList []string
}

func (m *MockManagedClusterInformer) Run() {}
func (m *MockManagedClusterInformer) HasSynced() bool {
	return true
}
func (m *MockManagedClusterInformer) GetAllManagedClusterNames() map[string]string {
	if m.clusters == nil {
		return map[string]string{}
	}
	return m.clusters
}
func (m *MockManagedClusterInformer) GetAllManagedClusterLabelNames() map[string]bool {
	if m.labels == nil {
		return map[string]bool{}
	}
	return m.labels
}
func (m *MockManagedClusterInformer) GetManagedClusterLabelList() []string {
	if m.regexLabelList == nil {
		return []string{}
	}
	return m.regexLabelList
}

// MockAccessReviewer is a mock implementation of the AccessReviewer interface.
type MockAccessReviewer struct {
	metricsAccess map[string][]string
	err           error
}

func (m *MockAccessReviewer) GetMetricsAccess(token string, extraArgs ...string) (map[string][]string, error) {
	return m.metricsAccess, m.err
}

func TestNewProxy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	assert.NoError(t, err)

	upi := cache.NewUserProjectInfo(24*60*60*time.Second, 5*60*time.Second)
	defer upi.Stop()

	mockInformer := &MockManagedClusterInformer{}
	mockAccessReviewer := &MockAccessReviewer{}

	p, err := NewProxy(serverURL, http.DefaultTransport, "", upi, mockInformer, mockAccessReviewer)
	assert.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, serverURL, p.metricsServerURL)
	assert.NotNil(t, p.proxy)
}

func TestProxy_ServeHTTP(t *testing.T) {
	var directorCalled bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		directorCalled = true
		assert.Equal(t, "/api/metrics/v1/default/metrics/query", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	assert.NoError(t, err)

	// Create a custom transport that trusts the test server's certificate.
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs: server.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs,
		},
	}

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, projectsAPIPath) {
			_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"dummy"},"spec":{}}]}`))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer apiServer.Close()

	upi := cache.NewUserProjectInfo(24*60*60*time.Second, 0)
	defer upi.Stop()

	mockInformer := &MockManagedClusterInformer{
		clusters: map[string]string{"dummy": "dummy"},
	}
	mockAccessReviewer := &MockAccessReviewer{metricsAccess: map[string][]string{}}

	p, err := NewProxy(serverURL, transport, apiServer.URL, upi, mockInformer, mockAccessReviewer)
	assert.NoError(t, err)

	req := httptest.NewRequest("GET", "http://localhost/metrics/query?query=foo", nil)
	req.Header.Set("X-Forwarded-User", "test")
	req.Header.Set("X-Forwarded-Access-Token", "test")
	w := httptest.NewRecorder()

	p.ServeHTTP(w, req)

	t.Logf("Response status: %d", w.Code)
	t.Logf("Response body: %s", w.Body.String())

	assert.True(t, directorCalled, "director was not called")
}

func TestNewEmptyMatrixHTTPBody(t *testing.T) {
	body := newEmptyMatrixHTTPBody()
	expected := `{"status":"success","data":{"resultType":"matrix","result":[]}}`
	assert.Equal(t, expected, string(body))
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
	scheme := runtime.NewScheme()
	_ = userv1.AddToScheme(scheme)
	_ = projectv1.AddToScheme(scheme)

	// Mock user and projects.
	// The user object is named "~" because the GetUserName function specifically looks for
	// a user with this name as a special shortcut for the current user. The fake client
	// needs an object with this exact name to satisfy the Get call.
	mockUser := &userv1.User{ObjectMeta: metav1.ObjectMeta{Name: "~"}, FullName: "test-user"}
	mockProjects := &projectv1.ProjectList{Items: []projectv1.Project{{ObjectMeta: metav1.ObjectMeta{Name: "p"}}}}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mockUser).WithLists(mockProjects).Build()

	upi := cache.NewUserProjectInfo(time.Minute, time.Minute)
	defer upi.Stop()

	p := &Proxy{
		userProjectInfo:        upi,
		managedClusterInformer: &MockManagedClusterInformer{clusters: map[string]string{"p": "p"}},
		accessReviewer:         &MockAccessReviewer{},
	}
	p.getKubeClientWithTokenFunc = func(token string) (client.Client, error) {
		return fakeClient, nil
	}

	req, _ := http.NewRequest("GET", "http://127.0.0.1:3002/metrics/query?query=foo", nil)
	req.Header.Set("X-Forwarded-Access-Token", "test")
	req.Header.Set("X-Forwarded-User", "test")

	// Test valid request
	err := p.preCheckRequest(req)
	assert.NoError(t, err)

	// Test with bearer token
	req.Header.Del("X-Forwarded-Access-Token")
	req.Header.Add("Authorization", "Bearer test")
	err = p.preCheckRequest(req)
	assert.NoError(t, err)
	assert.Equal(t, "test", req.Header.Get("X-Forwarded-Access-Token"))

	// Test with missing user, should be fetched automatically
	req.Header.Del("X-Forwarded-User")
	err = p.preCheckRequest(req)
	assert.NoError(t, err)
	assert.Equal(t, "~", req.Header.Get("X-Forwarded-User"))

	// Test with missing token
	req.Header.Del("X-Forwarded-Access-Token")
	req.Header.Del("Authorization")
	err = p.preCheckRequest(req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "found unauthorized user")
}

func TestProxyRequest(t *testing.T) {
	t.Run("No-op for non-GET or non-relevant paths", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", http.NoBody)
		proxyRequest(req)
		assert.Equal(t, http.NoBody, req.Body)
		assert.Equal(t, "", req.Header.Get("Content-Type"))
	})

	t.Run("Converts GET to POST for relevant paths", func(t *testing.T) {
		pathList := []string{
			"/api/v1/query",
			"/api/v1/query_range",
			"/api/v1/series",
		}

		for _, path := range pathList {
			req := httptest.NewRequest("GET", path+"?query=up", nil)
			proxyRequest(req)
			assert.Equal(t, http.MethodPost, req.Method)
			assert.Equal(t, "application/x-www-form-urlencoded", req.Header.Get("Content-Type"))
			body, err := io.ReadAll(req.Body)
			assert.NoError(t, err)
			assert.Equal(t, "query=up", string(body))
		}
	})
}

func newTestProxy(t *testing.T, labels []string) *Proxy {
	mockInformer := &MockManagedClusterInformer{
		regexLabelList: labels,
	}
	// Create a dummy server for the metrics server URL.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	serverURL, err := url.Parse(server.URL)
	assert.NoError(t, err)

	p, err := NewProxy(serverURL, server.Client().Transport, "", nil, mockInformer, nil)
	assert.NoError(t, err)
	return p
}

func TestHandleManagedClusterLabelQuery(t *testing.T) {
	p := newTestProxy(t, []string{"cloud", "vendor"})

	testCases := []struct {
		name             string
		method           string
		path             string
		body             io.Reader
		expectedToHandle bool
		expectedBody     string
	}{
		{
			name:             "should handle POST to series endpoint with correct metric",
			method:           "POST",
			path:             "/api/v1/series",
			body:             strings.NewReader("match[]=" + config.RBACProxyLabelMetricName),
			expectedToHandle: true,
			expectedBody:     `{"status":"success","data":[{"__name__":"acm_label_names","label_name":"cloud"},{"__name__":"acm_label_names","label_name":"vendor"}]}`,
		},
		{
			name:             "should not handle POST to series endpoint with other metric",
			method:           "POST",
			path:             "/api/v1/series",
			body:             strings.NewReader("match[]=kube_pod_info"),
			expectedToHandle: false,
		},
		{
			name:             "should handle GET to series endpoint with correct metric",
			method:           "GET",
			path:             "/api/v1/series?match[]=" + config.RBACProxyLabelMetricName,
			body:             nil,
			expectedToHandle: true,
			expectedBody:     `{"status":"success","data":[{"__name__":"acm_label_names","label_name":"cloud"},{"__name__":"acm_label_names","label_name":"vendor"}]}`,
		},
		{
			name:             "should not handle GET to series endpoint with other metric",
			method:           "GET",
			path:             "/api/v1/series?match[]=kube_pod_info",
			body:             nil,
			expectedToHandle: false,
		},
		{
			name:             "should handle GET to label values endpoint with correct metric",
			method:           "GET",
			path:             "/api/v1/label/label_name/values?match[]=" + config.RBACProxyLabelMetricName,
			body:             nil,
			expectedToHandle: true,
			expectedBody:     `{"status":"success","data":["cloud","vendor"]}`,
		},
		{
			name:             "should not handle GET to label values endpoint with other metric",
			method:           "GET",
			path:             "/api/v1/label/label_name/values?match[]=kube_pod_info",
			body:             nil,
			expectedToHandle: false,
		},
		{
			name:             "should not handle irrelevant path",
			method:           "GET",
			path:             "/api/v1/query",
			body:             nil,
			expectedToHandle: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp := NewFakeResponse(t)
			req := httptest.NewRequest(tc.method, tc.path, tc.body)
			handled := p.handleManagedClusterLabelQuery(resp, req)
			assert.Equal(t, tc.expectedToHandle, handled)

			if tc.expectedToHandle {
				assert.JSONEq(t, tc.expectedBody, string(resp.body))
			}
		})
	}
}

func TestCreateQueryResponse(t *testing.T) {
	testCases := []struct {
		name         string
		labels       []string
		metricName   string
		urlPath      string
		expectedJSON string
		expectErr    bool
	}{
		{
			name:         "label values path with multiple labels",
			labels:       []string{"cloud", "vendor"},
			metricName:   "acm_managed_cluster_labels",
			urlPath:      apiLabelNameValuesPath,
			expectedJSON: `{"status":"success","data":["cloud","vendor"]}`,
			expectErr:    false,
		},
		{
			name:         "label values path with no labels",
			labels:       []string{},
			metricName:   "acm_managed_cluster_labels",
			urlPath:      apiLabelNameValuesPath,
			expectedJSON: `{"status":"success","data":[]}`,
			expectErr:    false,
		},
		{
			name:         "series path with multiple labels",
			labels:       []string{"cloud", "vendor"},
			metricName:   "acm_managed_cluster_labels",
			urlPath:      apiSeriesPath,
			expectedJSON: `{"status":"success","data":[{"__name__":"acm_managed_cluster_labels","label_name":"cloud"},{"__name__":"acm_managed_cluster_labels","label_name":"vendor"}]}`,
			expectErr:    false,
		},
		{
			name:         "series path with no labels",
			labels:       []string{},
			metricName:   "acm_managed_cluster_labels",
			urlPath:      apiSeriesPath,
			expectedJSON: `{"status":"success","data":[]}`,
			expectErr:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualBytes, err := createQueryResponse(tc.labels, tc.metricName, tc.urlPath)

			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.JSONEq(t, tc.expectedJSON, string(actualBytes))
			}
		})
	}
}

// TestProxyIntegrationScenarios acts as a component integration test for various user permission scenarios.
func TestProxyIntegrationScenarios(t *testing.T) {
	testCases := []struct {
		name                          string
		token                         string
		apiProjectsResponse           string
		accessReviewResponse          map[string][]string
		expectedUpstreamQueryContains string
		expectedResponseCode          int
	}{
		{
			name:                          "Admin user with access to all clusters",
			token:                         "admin-token",
			apiProjectsResponse:           `{"items":[{"metadata":{"name":"cluster1"}},{"metadata":{"name":"cluster2"}}]}`,
			accessReviewResponse:          map[string][]string{"cluster1": {"*"}, "cluster2": {"*"}},
			expectedUpstreamQueryContains: "query=up", // No cluster filter
			expectedResponseCode:          http.StatusOK,
		},
		{
			name:                          "Scoped user with access to one cluster",
			token:                         "scoped-token",
			apiProjectsResponse:           `{"items":[{"metadata":{"name":"cluster1"}}]}`,
			accessReviewResponse:          map[string][]string{"cluster1": {"*"}},
			expectedUpstreamQueryContains: `query=up{cluster="cluster1"}`,
			expectedResponseCode:          http.StatusOK,
		},
		{
			name:                          "User with no cluster access",
			token:                         "no-access-token",
			apiProjectsResponse:           `{"items":[]}`,
			accessReviewResponse:          map[string][]string{},
			expectedUpstreamQueryContains: `query=up{cluster=""}`,
			expectedResponseCode:          http.StatusOK, // Returns empty matrix
		},
	}

	// Set up shared mock servers
	var upstreamCalled bool
	var receivedUpstreamQuery string
	metricsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		body, _ := io.ReadAll(r.Body)
		receivedUpstreamQuery, _ = url.QueryUnescape(string(body))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer metricsServer.Close()
	metricsServerURL, err := url.Parse(metricsServer.URL)
	assert.NoError(t, err)

	// The mock API server will serve different project lists based on the token.
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		for _, tc := range testCases {
			if tc.token == token {
				if strings.HasSuffix(r.URL.Path, "/users/~") {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"metadata":{"name":"test-user"}}`))
					return
				}
				if strings.HasSuffix(r.URL.Path, "/projects") {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(tc.apiProjectsResponse))
					return
				}
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer apiServer.Close()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = userv1.AddToScheme(scheme)
			_ = projectv1.AddToScheme(scheme)

			// Reset mocks for each run
			upstreamCalled = false
			receivedUpstreamQuery = ""

			transport := &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs: metricsServer.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs,
				},
			}
			userProjectCache := cache.NewUserProjectInfo(time.Minute, time.Minute)
			defer userProjectCache.Stop()

			mockInformer := &MockManagedClusterInformer{
				clusters: map[string]string{"cluster1": "cluster1", "cluster2": "cluster2"},
			}
			mockAccessReviewer := &MockAccessReviewer{
				metricsAccess: tc.accessReviewResponse,
			}

			proxy, err := NewProxy(metricsServerURL, transport, "", userProjectCache, mockInformer, mockAccessReviewer)
			assert.NoError(t, err)
			proxy.getKubeClientWithTokenFunc = func(token string) (client.Client, error) {
				// Based on the token, return a client with the correct mock data.
				fakeClientBuilder := fake.NewClientBuilder().WithScheme(scheme)
				mockUser := &userv1.User{ObjectMeta: metav1.ObjectMeta{Name: "~"}, FullName: "test-user"}
				fakeClientBuilder.WithObjects(mockUser)
				// This is a simplified mock. A real scenario would parse the apiProjectsResponse.
				switch tc.token {
				case "admin-token":
					fakeClientBuilder.WithLists(&projectv1.ProjectList{Items: []projectv1.Project{{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}}, {ObjectMeta: metav1.ObjectMeta{Name: "cluster2"}}}})
				case "scoped-token":
					fakeClientBuilder.WithLists(&projectv1.ProjectList{Items: []projectv1.Project{{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}}}})
				default:
					fakeClientBuilder.WithLists(&projectv1.ProjectList{Items: []projectv1.Project{}})
				}
				return fakeClientBuilder.Build(), nil
			}

			req := httptest.NewRequest("GET", "http://localhost/api/v1/query?query=up", nil)
			req.Header.Set("Authorization", "Bearer "+tc.token)
			recorder := httptest.NewRecorder()
			proxy.ServeHTTP(recorder, req)

			assert.Equal(t, tc.expectedResponseCode, recorder.Code)
			assert.True(t, upstreamCalled)
			assert.Contains(t, receivedUpstreamQuery, tc.expectedUpstreamQueryContains)
		})
	}
}
