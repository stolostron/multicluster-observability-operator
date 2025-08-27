// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package metricquery

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/cache"
	proxyconfig "github.com/stolostron/multicluster-observability-operator/proxy/pkg/config"
	"github.com/stretchr/testify/assert"
)

// MockAccessReviewer is a mock implementation of the AccessReviewer interface.
type MockAccessReviewer struct {
	metricsAccess map[string][]string
	err           error
}

func (m *MockAccessReviewer) GetMetricsAccess(token string, extraArgs ...string) (map[string][]string, error) {
	return m.metricsAccess, m.err
}

// MockManagedClusterInformer is a mock implementation of the ManagedClusterInformable interface.
type MockManagedClusterInformer struct {
	clusters     map[string]string
	labels       map[string]bool
	labelsConfig *proxyconfig.ManagedClusterLabelList
}

func (m *MockManagedClusterInformer) Run() {}
func (m *MockManagedClusterInformer) GetAllManagedClusterNames() map[string]string {
	return m.clusters
}
func (m *MockManagedClusterInformer) GetAllManagedClusterLabelNames() map[string]bool {
	return m.labels
}
func (m *MockManagedClusterInformer) GetManagedClusterLabelList() *proxyconfig.ManagedClusterLabelList {
	return m.labelsConfig
}

func newHTTPRequest() *http.Request {
	req, _ := http.NewRequest("GET", "http://127.0.0.1:3002/metrics/query?query=foo", nil)
	req.Header.Set("X-Forwarded-User", "test")
	req.Header.Set("X-Forwarded-Access-Token", "test")
	return req
}

func TestRewriteQuery(t *testing.T) {
	testCases := []struct {
		name        string
		urlValue    url.Values
		clusterList []string
		key         string
		expected    string
	}{
		{
			name:        "should not rewrite empty values",
			urlValue:    map[string][]string{},
			clusterList: []string{"c1", "c2"},
			key:         "key",
			expected:    "",
		},
		{
			name:        "should rewrite simple query",
			urlValue:    map[string][]string{"key": {"value"}},
			clusterList: []string{"c1", "c2"},
			key:         "key",
			expected:    `value{cluster=~"c1|c2",namespace=""}`,
		},
		{
			name:        "should handle empty cluster list",
			urlValue:    map[string][]string{"key": {"value"}},
			clusterList: []string{},
			key:         "key",
			expected:    `value{cluster=""}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clusterMap := make(map[string][]string, len(tc.clusterList))
			for _, cluster := range tc.clusterList {
				clusterMap[cluster] = []string{cluster}
			}
			output := rewriteQuery(tc.urlValue, clusterMap, tc.key)
			assert.Equal(t, tc.expected, output.Get(tc.key))
		})
	}
}

func TestCanAccessAll(t *testing.T) {
	testCases := []struct {
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
			clusterNamespaces:  map[string][]string{"cluster1": {"namespace1"}},
			expected:           false,
		},
		{
			name:               "Cluster has access to all namespaces",
			allManagedClusters: map[string]string{"cluster1": "cluster1"},
			clusterNamespaces:  map[string][]string{"cluster1": {"*"}},
			expected:           true,
		},
		{
			name:               "Multiple clusters with full access",
			allManagedClusters: map[string]string{"cluster1": "cluster1", "cluster2": "cluster2"},
			clusterNamespaces:  map[string][]string{"cluster1": {"*"}, "cluster2": {"*"}},
			expected:           true,
		},
		{
			name:               "Multiple clusters, one missing full access",
			allManagedClusters: map[string]string{"cluster1": "cluster1", "cluster2": "cluster2"},
			clusterNamespaces:  map[string][]string{"cluster1": {"*"}, "cluster2": {"namespace1"}},
			expected:           false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := canAccessAll(tc.clusterNamespaces, tc.allManagedClusters)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestGetUserClusterList(t *testing.T) {
	testCases := []struct {
		name        string
		projectList []string
		clusterList map[string]string
		expected    int
	}{
		{"no project", []string{}, map[string]string{}, 0},
		{"should get 1 cluster", []string{"c1", "c2"}, map[string]string{"c1": "c1"}, 1},
		{"should get 2 clusters", []string{"c1", "c2"}, map[string]string{"c1": "c1", "c2": "c2"}, 2},
		{"no cluster if project not in cluster list", []string{"c1"}, map[string]string{}, 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output := getUserClusterList(tc.projectList, tc.clusterList)
			assert.Len(t, output, tc.expected)
		})
	}
}

func TestModifyMetricsQueryParams(t *testing.T) {
	testCases := []struct {
		name               string
		clusters           map[string]string
		expected           string
		mockAccessReviewer *MockAccessReviewer
	}{
		{
			name:     "do not need modify params when user has access to all namespaces",
			clusters: map[string]string{"c0": "c0"},
			expected: "query=foo",
			mockAccessReviewer: &MockAccessReviewer{
				metricsAccess: map[string][]string{"c0": {"*"}},
			},
		},
		{
			name:     "modify params with 1 cluster",
			clusters: map[string]string{"c0": "c0", "c2": "c2"},
			expected: `query=foo{cluster="c0"}`,
			mockAccessReviewer: &MockAccessReviewer{
				metricsAccess: map[string][]string{"c0": {"*"}},
			},
		},
		{
			name:     "modify params when user has access to all clusters",
			clusters: map[string]string{"c0": "c0", "c1": "c1"},
			expected: `query=foo`,
			mockAccessReviewer: &MockAccessReviewer{
				metricsAccess: map[string][]string{"c0": {"*"}, "c1": {"*"}},
			},
		},
		{
			name:     "no cluster",
			clusters: map[string]string{},
			expected: `query=foo{cluster=""}`,
			mockAccessReviewer: &MockAccessReviewer{
				metricsAccess: map[string][]string{},
			},
		},
	}

	upi := cache.NewUserProjectInfo(60*time.Second, 0)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := newHTTPRequest()
			mci := &MockManagedClusterInformer{
				clusters: tc.clusters,
			}
			modifier := &Modifier{
				Req:            req,
				ReqURL:         "http://127.0.0.1:3002/",
				AccessReviewer: tc.mockAccessReviewer,
				UPI:            upi,
				MCI:            mci,
			}
			modifier.Modify()
			decodedQuery, _ := url.QueryUnescape(req.URL.RawQuery)
			assert.Equal(t, tc.expected, decodedQuery)
		})
	}
}
