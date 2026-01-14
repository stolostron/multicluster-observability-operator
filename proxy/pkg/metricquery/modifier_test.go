// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package metricquery

import (
	"context"
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
	clusters       map[string]struct{}
	regexLabelList []string
}

func (m *MockManagedClusterInformer) Run() {}
func (m *MockManagedClusterInformer) HasSynced() bool {
	return true
}

func (m *MockManagedClusterInformer) GetAllManagedClusterNames() map[string]struct{} {
	return m.clusters
}

func (m *MockManagedClusterInformer) GetManagedClusterLabelList() []string {
	return m.regexLabelList
}

func newHTTPRequest() *http.Request {
	req, _ := http.NewRequest("GET", "http://127.0.0.1:3002/metrics/query?query=foo", nil)
	req.Header.Set("X-Forwarded-User", "test")
	req.Header.Set("X-Forwarded-Access-Token", "test")
	return req
}

func TestRewriteQuery(t *testing.T) {
	testCases := []struct {
		name              string
		query             string
		userMetricsAccess map[string][]string
		expected          string
		expectedError     bool
	}{
		{
			name:              "simple query with two clusters",
			query:             "up",
			userMetricsAccess: map[string][]string{"c1": {"*"}, "c2": {"*"}},
			expected:          `up{cluster=~"c1|c2"}`,
			expectedError:     false,
		},
		{
			name:              "query with namespace filtering",
			query:             "up",
			userMetricsAccess: map[string][]string{"c1": {"ns1"}},
			expected:          `up{cluster="c1",namespace="ns1"}`,
			expectedError:     false,
		},
		{
			name:              "empty user access",
			query:             "up",
			userMetricsAccess: map[string][]string{},
			expected:          `up{cluster=""}`,
			expectedError:     false,
		},
		{
			name:              "empty query",
			query:             "",
			userMetricsAccess: map[string][]string{"c1": {"*"}},
			expected:          "",
			expectedError:     false,
		},
		{
			name:              "query for acm_managed_cluster_labels should use 'name' label",
			query:             proxyconfig.ACMManagedClusterLabelNamesMetricName,
			userMetricsAccess: map[string][]string{"c1": {"*"}, "c2": {"*"}},
			expected:          `acm_managed_cluster_labels{name=~"c1|c2"}`,
			expectedError:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			modifiedQuery, err := rewriteQuery(tc.query, tc.userMetricsAccess)
			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, modifiedQuery)
			}
		})
	}
}

func TestRewriteQueryValues(t *testing.T) {
	testCases := []struct {
		name              string
		values            url.Values
		userMetricsAccess map[string][]string
		expectedValues    url.Values
		expectedError     bool
	}{
		{
			name:              "rewrite query and match[]",
			values:            url.Values{"query": {"up"}, "match[]": {"up", "down"}},
			userMetricsAccess: map[string][]string{"c1": {"*"}},
			expectedValues:    url.Values{"query": {`up{cluster="c1"}`}, "match[]": {`up{cluster="c1"}`, `down{cluster="c1"}`}},
			expectedError:     false,
		},
		{
			name:              "rewrite only query",
			values:            url.Values{"query": {"up"}},
			userMetricsAccess: map[string][]string{"c1": {"ns1"}},
			expectedValues:    url.Values{"query": {`up{cluster="c1",namespace="ns1"}`}},
			expectedError:     false,
		},
		{
			name:              "no relevant params",
			values:            url.Values{"other": {"value"}},
			userMetricsAccess: map[string][]string{"c1": {"*"}},
			expectedValues:    url.Values{"other": {"value"}},
			expectedError:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			modifiedValues, err := rewriteQueryValues(tc.values, tc.userMetricsAccess)
			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedValues, modifiedValues)
			}
		})
	}
}

func TestCanAccessAll(t *testing.T) {
	testCases := []struct {
		name               string
		allManagedClusters map[string]struct{}
		clusterNamespaces  map[string][]string
		expected           bool
	}{
		{
			name:               "Both allManagedClusterNames and clusterNamespaces are empty",
			allManagedClusters: map[string]struct{}{},
			clusterNamespaces:  map[string][]string{},
			expected:           false,
		},
		{
			name:               "Cluster not in clusterNamespaces",
			allManagedClusters: map[string]struct{}{"cluster1": {}},
			clusterNamespaces:  map[string][]string{},
			expected:           false,
		},
		{
			name:               "Cluster does not have access to all namespaces",
			allManagedClusters: map[string]struct{}{"cluster1": {}},
			clusterNamespaces:  map[string][]string{"cluster1": {"namespace1"}},
			expected:           false,
		},
		{
			name:               "Cluster has access to all namespaces",
			allManagedClusters: map[string]struct{}{"cluster1": {}},
			clusterNamespaces:  map[string][]string{"cluster1": {"*"}},
			expected:           true,
		},
		{
			name:               "Multiple clusters with full access",
			allManagedClusters: map[string]struct{}{"cluster1": {}, "cluster2": {}},
			clusterNamespaces:  map[string][]string{"cluster1": {"*"}, "cluster2": {"*"}},
			expected:           true,
		},
		{
			name:               "Multiple clusters, one missing full access",
			allManagedClusters: map[string]struct{}{"cluster1": {}, "cluster2": {}},
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

func TestFilterProjectsToManagedClusters(t *testing.T) {
	testCases := []struct {
		name        string
		projectList []string
		clusterList map[string]struct{}
		expected    int
	}{
		{"no project", []string{}, map[string]struct{}{}, 0},
		{"should get 1 cluster", []string{"c1", "c2"}, map[string]struct{}{"c1": {}}, 1},
		{"should get 2 clusters", []string{"c1", "c2"}, map[string]struct{}{"c1": {}, "c2": {}}, 2},
		{"no cluster if project not in cluster list", []string{"c1"}, map[string]struct{}{}, 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output := filterProjectsToManagedClusters(tc.projectList, tc.clusterList)
			assert.Len(t, output, tc.expected)
		})
	}
}

func TestGetUserMetricsACLs(t *testing.T) {
	testCases := []struct {
		name              string
		managedClusters   map[string]struct{}
		metricsAccess     map[string][]string
		cachedProjectList []string
		expectedACLs      map[string][]string
	}{
		{
			name:              "project access only (backward compatibility)",
			managedClusters:   map[string]struct{}{"c1": {}, "c2": {}},
			metricsAccess:     map[string][]string{},
			cachedProjectList: []string{"c1"},
			expectedACLs:      map[string][]string{"c1": {"*"}},
		},
		{
			name:              "metrics ACLs only",
			managedClusters:   map[string]struct{}{"c1": {}, "c2": {}},
			metricsAccess:     map[string][]string{"c2": {"ns2"}},
			cachedProjectList: []string{},
			expectedACLs:      map[string][]string{"c2": {"ns2"}},
		},
		{
			name:              "specific metrics ACLs override project access",
			managedClusters:   map[string]struct{}{"c1": {}, "c2": {}},
			metricsAccess:     map[string][]string{"c1": {"ns1"}},
			cachedProjectList: []string{"c1"},
			expectedACLs:      map[string][]string{"c1": {"ns1"}},
		},
		{
			name:              "wildcard cluster metrics ACL expansion",
			managedClusters:   map[string]struct{}{"c1": {}, "c2": {}},
			metricsAccess:     map[string][]string{"*": {"ns-all"}},
			cachedProjectList: []string{},
			expectedACLs:      map[string][]string{"c1": {"ns-all"}, "c2": {"ns-all"}},
		},
		{
			name:              "wildcard and specific ACLs are merged",
			managedClusters:   map[string]struct{}{"c1": {}, "c2": {}},
			metricsAccess:     map[string][]string{"*": {"ns-all"}, "c1": {"ns1"}},
			cachedProjectList: []string{},
			expectedACLs:      map[string][]string{"c1": {"ns1", "ns-all"}, "c2": {"ns-all"}},
		},
		{
			name:              "no access",
			managedClusters:   map[string]struct{}{"c1": {}, "c2": {}},
			metricsAccess:     map[string][]string{},
			cachedProjectList: []string{},
			expectedACLs:      map[string][]string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			userName := "test-user"
			token := "test-token"

			mockAccessReviewer := &MockAccessReviewer{metricsAccess: tc.metricsAccess}
			mockMCI := &MockManagedClusterInformer{clusters: tc.managedClusters}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			upi := cache.NewUserProjectInfo(ctx, time.Minute, time.Minute)
			upi.UpdateUserProject(userName, token, tc.cachedProjectList)

			modifier := &Modifier{
				AccessReviewer: mockAccessReviewer,
				UPI:            upi,
				MCI:            mockMCI,
			}

			acls, err := modifier.getUserMetricsACLs(userName, token)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedACLs, acls)
		})
	}
}

func TestModifyMetricsQueryParams(t *testing.T) {
	testCases := []struct {
		name               string
		clusters           map[string]struct{}
		expected           string
		mockAccessReviewer *MockAccessReviewer
	}{
		{
			name:     "do not need modify params when user has access to all namespaces",
			clusters: map[string]struct{}{"c0": {}},
			expected: "query=foo",
			mockAccessReviewer: &MockAccessReviewer{
				metricsAccess: map[string][]string{"c0": {"*"}},
			},
		},
		{
			name:     "modify params with 1 cluster",
			clusters: map[string]struct{}{"c0": {}, "c2": {}},
			expected: `query=foo{cluster="c0"}`,
			mockAccessReviewer: &MockAccessReviewer{
				metricsAccess: map[string][]string{"c0": {"*"}},
			},
		},
		{
			name:     "modify params when user has access to all clusters",
			clusters: map[string]struct{}{"c0": {}, "c1": {}},
			expected: `query=foo`,
			mockAccessReviewer: &MockAccessReviewer{
				metricsAccess: map[string][]string{"c0": {"*"}, "c1": {"*"}},
			},
		},
		{
			name:     "no cluster",
			clusters: map[string]struct{}{},
			expected: `query=foo{cluster=""}`,
			mockAccessReviewer: &MockAccessReviewer{
				metricsAccess: map[string][]string{},
			},
		},
		{
			name:     "modify params with namespace filtering",
			clusters: map[string]struct{}{"c0": {}},
			expected: `query=foo{cluster="c0",namespace="ns1"}`,
			mockAccessReviewer: &MockAccessReviewer{
				metricsAccess: map[string][]string{"c0": {"ns1"}},
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	upi := cache.NewUserProjectInfo(ctx, 60*time.Second, 0)

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
