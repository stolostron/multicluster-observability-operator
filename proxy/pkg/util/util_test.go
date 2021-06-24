// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func newTTPRequest() *http.Request {
	req, _ := http.NewRequest("GET", "http://127.0.0.1:3002/metrics/query?query=foo", nil)
	req.Header.Set("X-Forwarded-User", "test")
	return req
}

func createFakeServerWithInvalidJSON(port string, t *testing.T) {
	server := http.NewServeMux()
	server.HandleFunc("/",
		func(w http.ResponseWriter, req *http.Request) {
			w.Write([]byte("invalid json"))
		},
	)
	err := http.ListenAndServe(":"+port, server)
	if err != nil {
		t.Fatal("fail to create internal server at " + port)
	}
}

func createFakeServer(port string, t *testing.T) {
	server := http.NewServeMux()
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
	server.HandleFunc("/",
		func(w http.ResponseWriter, req *http.Request) {
			w.Write([]byte(projectList))
		},
	)
	err := http.ListenAndServe(":"+port, server)
	if err != nil {
		t.Fatal("fail to create internal server at " + port)
	}
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
	allManagedClusterNames = map[string]string{"c0": "c0", "c1": "c1"}
	for _, c := range testCaseList {
		allManagedClusterNames = c.clusters
		if len(GetAllManagedClusterNames()) != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, len(GetAllManagedClusterNames()), c.expected)
		}
	}

}

func TestGetAllManagedClusterNames(t *testing.T) {
	testCaseList := []struct {
		name     string
		clusters map[string]string
		expected string
	}{
		{"do not need modify params", map[string]string{"c0": "c0"}, "query=foo"},
		{"modify params with 1 cluster", map[string]string{"c0": "c0", "c2": "c2"}, `query=foo%7Bcluster%3D%22c0%22%7D`},
		{"modify params with all cluster", map[string]string{"c0": "c0", "c1": "c1"}, `query=foo`},
		{"no cluster", map[string]string{}, "query=foo"},
	}
	go createFakeServer("3002", t)
	time.Sleep(time.Second)
	for _, c := range testCaseList {
		allManagedClusterNames = c.clusters
		req := newTTPRequest()
		ModifyMetricsQueryParams(req, "http://127.0.0.1:3002/")
		if req.URL.RawQuery != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, req.URL.RawQuery, c.expected)
		}
	}
}

func TestContains(t *testing.T) {
	testCaseList := []struct {
		name     string
		list     []string
		s        string
		expected bool
	}{
		{"contain sub string", []string{"a", "b"}, "a", true},
		{"shoud contain empty string", []string{""}, "", true},
		{"should not contain sub string", []string{"a", "b"}, "c", false},
		{"shoud not contain empty string", []string{"a", "b"}, "", false},
	}

	for _, c := range testCaseList {
		output := Contains(c.list, c.s)
		if output != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, output, c.expected)
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
			map[string][]string{"key": []string{"value"}},
			[]string{"c1", "c2"},
			"key",
			"value{cluster=~\"c1|c2\"}",
		},

		{
			"empty cluster list",
			map[string][]string{"key": []string{"value"}},
			[]string{},
			"key",
			"value{cluster=~\"\"}",
		},
	}

	for _, c := range testCaseList {
		output := rewriteQuery(c.urlValue, c.clusterList, c.key)
		if output.Get(c.key) != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, output, c.expected)
		}
	}
}

func TestCanAccessAllClusters(t *testing.T) {
	testCaseList := []struct {
		name        string
		projectList []string
		clusterList map[string]string
		expected    bool
	}{
		{"no cluster and project", []string{}, map[string]string{}, false},
		{"should access all cluster", []string{"c1", "c2"}, map[string]string{"c1": "c1", "c2": "c2"}, true},
		{"should not access all cluster", []string{"c1"}, map[string]string{"c1": "c1", "c2": "c2"}, false},
		{"no project", []string{}, map[string]string{"c1": "c1", "c2": "c2"}, false},
	}

	for _, c := range testCaseList {
		allManagedClusterNames = c.clusterList
		output := canAccessAllClusters(c.projectList)
		if output != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, output, c.expected)
		}
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
	go createFakeServer("4002", t)
	time.Sleep(time.Second)

	for _, c := range testCaseList {
		output := FetchUserProjectList(c.token, c.url)
		if len(output) != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, len(output), c.expected)
		}
	}

	go createFakeServerWithInvalidJSON("5002", t)
	output := FetchUserProjectList("", "http://127.0.0.1:5002/")
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
		allManagedClusterNames = c.clusterList
		output := getUserClusterList(c.projectList)
		if len(output) != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, output, c.expected)
		}
	}
}

func TestWriteError(t *testing.T) {
	writeError("test")
	data, _ := ioutil.ReadFile("/tmp/health")
	if !strings.Contains(string(data), "test") {
		t.Errorf("failed to find the health file")
	}
}
