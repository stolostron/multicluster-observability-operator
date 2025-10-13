// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rewrite

import "testing"

func TestInjectClusterLabels(t *testing.T) {
	caseList := []struct {
		name     string
		query    string
		values   []string
		expected string
	}{
		{
			name:     "Simple metric",
			query:    `test_metrics{key="value"}`,
			values:   []string{"A"},
			expected: `test_metrics{cluster="A",key="value"}`,
		},
		{
			name:     "acm_managed_cluster_labels metric should use 'name' label",
			query:    `acm_managed_cluster_labels{key="value"}`,
			values:   []string{"A"},
			expected: `acm_managed_cluster_labels{key="value",name="A"}`,
		},
		{
			name:     "Binary expression with different metrics",
			query:    `sort(label_replace(acm_managed_cluster_labels, "cluster", "$1", "name", "(.*)")) + on (cluster) (sum by (cluster) (node_cpu_seconds_total))`,
			values:   []string{"A", "B"},
			expected: `sort(label_replace(acm_managed_cluster_labels{name=~"A|B"}, "cluster", "$1", "name", "(.*)")) + on (cluster) (sum by (cluster) (node_cpu_seconds_total{cluster=~"A|B"}))`,
		},
		{
			name:     "Multiple values",
			query:    `test_metrics{key="value"}`,
			values:   []string{"A", "B"},
			expected: `test_metrics{cluster=~"A|B",key="value"}`,
		},
	}

	for _, c := range caseList {
		t.Run(c.name, func(t *testing.T) {
			output, err := InjectClusterLabels(c.query, c.values)
			if err != nil {
				t.Errorf("Encountered error during label injection: (%v)", err)
			} else if output != c.expected {
				t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, output, c.expected)
			}
		})
	}
}
