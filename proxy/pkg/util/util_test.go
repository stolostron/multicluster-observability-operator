// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/informer"
)

// MockAccessReviewer is a mock implementation of the AccessReviewer interface.
type MockAccessReviewer struct {
	metricsAccess map[string][]string
	err           error
}

func (m *MockAccessReviewer) GetMetricsAccess(token string, extraArgs ...string) (map[string][]string, error) {
	return m.metricsAccess, m.err
}

func newHTTPRequest() *http.Request {
	req, _ := http.NewRequest("GET", "http://127.0.0.1:3002/metrics/query?query=foo", nil)
	req.Header.Set("X-Forwarded-User", "test")
	req.Header.Set("X-Forwarded-Access-Token", "test")
	return req
}

func createFakeServerWithInvalidJSON(port string) *http.Server {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("invalid json"))
	})

	server := &http.Server{Addr: ":" + port, Handler: handler}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("could not listen on %s due to %s", port, err)
		}
	}()

	return server
}

func createFakeServer(port string) *http.Server {
	projectList := `{
		"kind": "ProjectList",
		"apiVersion": "project.openshift.io/v1",
		"metadata": {
		  "selfLink": "/apis/project.openshift.io/v1/projects"
		},
		"items": [
		  {
			"metadata": {
			  "name": "c0",
			  "selfLink": "/apis/project.openshift.io/v1/projects/c0",
			  "uid": "2f68fd63-097c-4519-8e8f-823bb0106acc",
			  "resourceVersion": "7723",
			  "creationTimestamp": "2020-09-25T13:35:09Z",
			  "annotations": {
				"openshift.io/sa.scc.mcs": "s0:c11,c10",
				"openshift.io/sa.scc.supplemental-groups": "1000130000/10000",
				"openshift.io/sa.scc.uid-range": "1000130000/10000"
			  }
			},
			"spec": {
			  "finalizers": [
				"kubernetes"
			  ]
			},
			"status": {
			  "phase": "Active"
			}
		  },
		  {
			"metadata": {
			  "name": "c1",
			  "selfLink": "/apis/project.openshift.io/v1/projects/c1",
			  "uid": "bce1176f-6dda-45ee-99ef-675a64300643",
			  "resourceVersion": "59984227",
			  "creationTimestamp": "2020-11-26T08:34:15Z",
			  "annotations": {
				"openshift.io/sa.scc.mcs": "s0:c25,c0",
				"openshift.io/sa.scc.supplemental-groups": "1000600000/10000",
				"openshift.io/sa.scc.uid-range": "1000600000/10000"
			  }
			},
			"spec": {
			  "finalizers": [
				"kubernetes"
			  ]
			},
			"status": {
			  "phase": "Active"
			}
		  }
		]
	  }`

	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(projectList))
	})

	server := &http.Server{Addr: ":" + port, Handler: handler}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("could not listen on %s due to %s", port, err)
		}
	}()

	return server
}

func TestModifyMetricsQueryParams(t *testing.T) {
	testCaseList := []struct {
		name     string
		clusters map[string]string
		expected int
	}{
		{"1 cluster", map[string]string{"c0": "c0"}, 1},
		{"2 clusters", map[string]string{"c0": "c0", "c2": "c2"}, 2},
		{"no cluster", map[string]string{}, 0},
	}
	informer.AllManagedClusterNames = map[string]string{"c0": "c0", "c1": "c1"}
	for _, c := range testCaseList {
		informer.AllManagedClusterNames = c.clusters
		if len(informer.GetAllManagedClusterNames()) != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, len(informer.GetAllManagedClusterNames()), c.expected)
		}
	}
}

func TestRewriteQuery(t *testing.T) {
	testCaseList := []struct {
		name        string
		urlValue    url.Values
		clusterList []string
		key         string
		expected    string
	}{
		{
			"should not rewrite",
			map[string][]string{},
			[]string{"c1", "c2"},
			"key",
			"",
		},

		{
			"should rewrite",
			map[string][]string{"key": {"value"}},
			[]string{"c1", "c2"},
			"key",
			"value{cluster=~\"c1|c2\",namespace=~\"\"}",
		},

		{
			"empty cluster list",
			map[string][]string{"key": {"value"}},
			[]string{},
			"key",
			"value{cluster=~\"\"}",
		},
	}

	for _, c := range testCaseList {
		clusterMap := make(map[string][]string, len(c.clusterList))
		for _, cluster := range c.clusterList {
			clusterMap[cluster] = []string{cluster}
		}
		output := rewriteQuery(c.urlValue, clusterMap, c.key)
		if output.Get(c.key) != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, output.Get(c.key), c.expected)
		}
	}
}

func TestCanAccessAll(t *testing.T) {
	// Helper function to set global variable
	setAllManagedClusterNames := func(names map[string]string) {
		informer.AllManagedClusterNames = names
	}

	tests := []struct {
		name               string
		allManagedClusters map[string]string
		clusterNamespaces  map[string][]string
		expected           bool
	}{
		{
			name:               "Both allManagedClusterNames and clusterNamespaces are empty",
			allManagedClusters: map[string]string{},
			clusterNamespaces:  map[string][]string{},
			expected:           false,
		},
		{
			name:               "Cluster not in clusterNamespaces",
			allManagedClusters: map[string]string{"cluster1": "cluster1"},
			clusterNamespaces:  map[string][]string{},
			expected:           false,
		},
		{
			name:               "Cluster does not have access to all namespaces",
			allManagedClusters: map[string]string{"cluster1": "cluster1"},
			clusterNamespaces: map[string][]string{
				"cluster1": {"namespace1"},
			},
			expected: false,
		},
		{
			name:               "Cluster has access to all namespaces",
			allManagedClusters: map[string]string{"cluster1": "cluster1"},
			clusterNamespaces: map[string][]string{
				"cluster1": {"*"},
			},
			expected: true,
		},
		{
			name:               "Multiple clusters with full access",
			allManagedClusters: map[string]string{"cluster1": "cluster1", "cluster2": "cluster2"},
			clusterNamespaces: map[string][]string{
				"cluster1": {"*"},
				"cluster2": {"*"},
			},
			expected: true,
		},
		{
			name:               "Multiple clusters, one missing full access",
			allManagedClusters: map[string]string{"cluster1": "cluster1", "cluster2": "cluster2"},
			clusterNamespaces: map[string][]string{
				"cluster1": {"*"},
				"cluster2": {"namespace1"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the global variable
			setAllManagedClusterNames(tt.allManagedClusters)

			got := CanAccessAll(tt.clusterNamespaces)
			if got != tt.expected {
				t.Errorf("CanAccessAll() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFetchUserProjectList(t *testing.T) {
	testCaseList := []struct {
		name     string
		token    string
		url      string
		expected int
	}{
		{"get 2 projects", "", "http://127.0.0.1:4002/", 2},
		{"invalid url", "", "http://127.0.0.1:300/", 0},
	}

	// Create a fake server with a custom port
	port := "4002"
	server := createFakeServer(port)
	defer server.Close()

	time.Sleep(time.Second) // Wait a bit for the server to start

	for _, c := range testCaseList {
		output := FetchUserProjectList(c.token, c.url)
		if len(output) != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, len(output), c.expected)
		}
	}

	// Create a fake server with invalid JSON using a custom port
	invalidPort := "5002"
	invalidJSONServer := createFakeServerWithInvalidJSON(invalidPort)
	defer invalidJSONServer.Close()

	time.Sleep(time.Second) // Wait a bit for the server to start

	output := FetchUserProjectList("", "http://127.0.0.1:"+invalidPort+"/")
	if len(output) != 0 {
		t.Errorf("case (invalid json) output: (%v) is not the expected: (0)", len(output))
	}
}

func TestGetUserClusterList(t *testing.T) {
	testCaseList := []struct {
		name        string
		projectList []string
		clusterList map[string]string
		expected    int
	}{
		{"no project", []string{}, map[string]string{}, 0},
		{"should get 1 cluster", []string{"c1", "c2"}, map[string]string{"c1": "c1"}, 1},
		{"should get 2 cluster", []string{"c1", "c2"}, map[string]string{"c1": "c1", "c2": "c2"}, 2},
		{"no cluster", []string{"c1"}, map[string]string{}, 0},
	}

	for _, c := range testCaseList {
		informer.AllManagedClusterNames = c.clusterList
		output := getUserClusterList(c.projectList)
		if len(output) != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, output, c.expected)
		}
	}
}

func TestWriteError(t *testing.T) {
	writeError("test")
	data, _ := os.ReadFile("/tmp/health")
	if !strings.Contains(string(data), "test") {
		t.Errorf("failed to find the health file")
	}
}

func TestGetAllManagedClusterNames(t *testing.T) {
	testCaseList := []struct {
		name               string
		clusters           map[string]string
		expected           string
		mockAccessReviewer *MockAccessReviewer
	}{
		{
			"do not need modify params",
			map[string]string{"c0": "c0"},
			"query=foo",
			&MockAccessReviewer{
				metricsAccess: map[string][]string{
					"c0": {"*", "*"},
				},
			},
		},
		{
			"modify params with 1 cluster",
			map[string]string{"c0": "c0", "c2": "c2"},
			`query=foo%7Bcluster%3D%22c0%22%7D`,
			&MockAccessReviewer{
				metricsAccess: map[string][]string{
					"c0": {"*", "*"},
				},
			},
		},
		{
			"modify params with all cluster",
			map[string]string{"c0": "c0", "c1": "c1"},
			`query=foo`,
			&MockAccessReviewer{
				metricsAccess: map[string][]string{
					"c0": {"*", "*"},
					"c1": {"*", "*"},
				},
			},
		},
		{
			"no cluster",
			map[string]string{},
			"query=foo",
			&MockAccessReviewer{
				metricsAccess: map[string][]string{
					"c0": {"", ""},
				},
			},
		},
	}

	// Create a fake server with a custom port
	port := "3002"
	server := createFakeServer(port)
	defer server.Close()

	time.Sleep(time.Second) // Wait a bit for the server to start

	informer.InitAllManagedClusterNames()
	upi := NewUserProjectInfo(60*time.Second, 0)

	for _, c := range testCaseList {
		informer.AllManagedClusterNames = c.clusters
		accessReviewer := c.mockAccessReviewer
		req := newHTTPRequest()
		ModifyMetricsQueryParams(req, "http://127.0.0.1:"+port+"/", accessReviewer, upi)
		if req.URL.RawQuery != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, req.URL.RawQuery, c.expected)
		}
	}
}
