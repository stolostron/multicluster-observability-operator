// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package metricquery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetClustersInQuery(t *testing.T) {
	userMetricsAccess := map[string][]string{
		"cluster1":      {"namespace1", "namespace2"},
		"cluster2":      {"namespace3"},
		"dev-cluster":   {"dev-ns"},
		"prod-cluster":  {"prod-ns"},
		"test-cluster1": {"test-ns1"},
		"test-cluster2": {"test-ns2"},
	}

	testCases := []struct {
		name          string
		query         string
		expected      []string
		expectedError bool
	}{
		{
			name:          "query with no cluster label",
			query:         `up`,
			expected:      []string{},
			expectedError: false,
		},
		{
			name:          "query with cluster label",
			query:         `up{cluster="cluster1"}`,
			expected:      []string{"cluster1"},
			expectedError: false,
		},
		{
			name:          "query with regex cluster label",
			query:         `up{cluster=~"test-cluster.*"}`,
			expected:      []string{"test-cluster1", "test-cluster2"},
			expectedError: false,
		},
		{
			name:          "query with multiple cluster matchers",
			query:         `up{cluster=~"dev-.*|prod-.*"}`,
			expected:      []string{"dev-cluster", "prod-cluster"},
			expectedError: false,
		},
		{
			name:          "query with negative matcher",
			query:         `up{cluster!="cluster1"}`,
			expected:      []string{"cluster2", "dev-cluster", "prod-cluster", "test-cluster1", "test-cluster2"},
			expectedError: false,
		},
		{
			name:          "query with negative regex matcher",
			query:         `up{cluster!~"test-.*"}`,
			expected:      []string{"cluster1", "cluster2", "dev-cluster", "prod-cluster"},
			expectedError: false,
		},
		{
			name:          "invalid promql query",
			query:         `up{`,
			expected:      nil,
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filter := NewNamespaceFilter(userMetricsAccess)
			clusters, err := filter.getClustersInQuery(tc.query)

			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tc.expected, clusters)
			}
		})
	}
}

func TestGetCommonNamespacesAcrossClusters(t *testing.T) {
	metricsAccess := map[string][]string{
		"c1": {"ns1", "ns2"},
		"c2": {"ns2", "ns3"},
		"c3": {"ns1", "ns2", "ns3"},
		"c4": {"*"},
		"c5": {}, // empty slice is equivalent to wildcard
	}

	testCases := []struct {
		name     string
		clusters []string
		expected []string
	}{
		{
			name:     "no clusters specified, all clusters considered",
			clusters: []string{},
			expected: []string{"ns2"}, // ns2 is the only ns common to c1, c2, c3 (and c4, c5 are wildcards)
		},
		{
			name:     "common namespace between c1 and c3",
			clusters: []string{"c1", "c3"},
			expected: []string{"ns1", "ns2"},
		},
		{
			name:     "common namespace between c1, c2, and c3",
			clusters: []string{"c1", "c2", "c3"},
			expected: []string{"ns2"},
		},
		{
			name:     "common namespace between c1 and c2",
			clusters: []string{"c1", "c2"},
			expected: []string{"ns2"},
		},
		{
			name:     "wildcard access in one cluster (c4)",
			clusters: []string{"c1", "c4"},
			expected: []string{"ns1", "ns2"},
		},
		{
			name:     "wildcard access in one cluster (c5)",
			clusters: []string{"c1", "c5"},
			expected: []string{"ns1", "ns2"},
		},
		{
			name:     "wildcard access in all clusters",
			clusters: []string{"c4", "c5"},
			expected: []string{"*"},
		},
		{
			name:     "single cluster",
			clusters: []string{"c3"},
			expected: []string{"ns1", "ns2", "ns3"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filter := NewNamespaceFilter(metricsAccess)
			common := filter.getCommonNamespacesAcrossClusters(tc.clusters)
			assert.ElementsMatch(t, tc.expected, common)
		})
	}

	t.Run("empty clusters slice with common ns", func(t *testing.T) {
		specificMetricsAccess := map[string][]string{
			"c1": {"ns1"},
			"c2": {"ns1"},
		}
		filter := NewNamespaceFilter(specificMetricsAccess)
		common := filter.getCommonNamespacesAcrossClusters([]string{})
		assert.ElementsMatch(t, []string{"ns1"}, common)
	})
}

func TestAddNamespaceFilters(t *testing.T) {
	testCases := []struct {
		name              string
		originalQuery     string
		modifiedQuery     string
		userMetricsAccess map[string][]string
		expected          string
		expectedError     bool
	}{
		{
			name:              "user has access to all namespaces",
			originalQuery:     `up{cluster="c1"}`,
			modifiedQuery:     `up{cluster="c1"}`,
			userMetricsAccess: map[string][]string{"c1": {"*"}},
			expected:          `up{cluster="c1"}`,
			expectedError:     false,
		},
		{
			name:              "user has access to a single common namespace",
			originalQuery:     `up{cluster=~"c1|c2"}`,
			modifiedQuery:     `up{cluster=~"c1|c2"}`,
			userMetricsAccess: map[string][]string{"c1": {"ns1"}, "c2": {"ns1"}},
			expected:          `up{cluster=~"c1|c2",namespace="ns1"}`,
			expectedError:     false,
		},
		{
			name:              "user has access to multiple common namespaces",
			originalQuery:     `up{cluster=~"c1|c2"}`,
			modifiedQuery:     `up{cluster=~"c1|c2"}`,
			userMetricsAccess: map[string][]string{"c1": {"ns1", "ns2"}, "c2": {"ns1", "ns2"}},
			expected:          `up{cluster=~"c1|c2",namespace=~"ns1|ns2"}`,
			expectedError:     false,
		},
		{
			name:              "invalid promql query",
			originalQuery:     `up{`,
			modifiedQuery:     `up{`,
			userMetricsAccess: map[string][]string{"c1": {"ns1"}},
			expected:          "",
			expectedError:     true,
		},
		{
			name:              "no common namespaces",
			originalQuery:     `up{cluster=~"c1|c2"}`,
			modifiedQuery:     `up{cluster=~"c1|c2"}`,
			userMetricsAccess: map[string][]string{"c1": {"ns1"}, "c2": {"ns2"}},
			expected:          `up{cluster=~"c1|c2",namespace=""}`,
			expectedError:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filter := NewNamespaceFilter(tc.userMetricsAccess)
			result, err := filter.AddNamespaceFilters(tc.originalQuery, tc.modifiedQuery)

			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}
