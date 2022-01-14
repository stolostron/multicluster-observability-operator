// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package rewrite

import "testing"

func TestInjectLabels(t *testing.T) {
	caseList := []struct {
		name     string
		query    string
		label    string
		values   []string
		expected string
	}{
		{
			name:     "No metrics specified",
			query:    `{key="value"}`,
			label:    "cluster",
			values:   []string{"A"},
			expected: `{cluster="A",key="value"}`,
		},
		{
			name:     "Only metrics name",
			query:    "test_metrics",
			label:    "cluster",
			values:   []string{"A"},
			expected: `test_metrics{cluster="A"}`,
		},
		{
			name:     "Metrics with label",
			query:    `test_metrics{key="value"}`,
			label:    "cluster",
			values:   []string{"A"},
			expected: `test_metrics{cluster="A",key="value"}`,
		},
		{
			name:     "Multiple values",
			query:    `test_metrics{key="value"}`,
			label:    "cluster",
			values:   []string{"A", "B"},
			expected: `test_metrics{cluster=~"A|B",key="value"}`,
		},
		{
			name:     "Existing label for cluster",
			query:    `test_metrics{cluster="A"}`,
			label:    "cluster",
			values:   []string{"A", "B"},
			expected: `test_metrics{cluster="A",cluster=~"A|B"}`,
		},
		{
			name:     "Existing label for cluster using different ops",
			query:    `test_metrics{cluster!="A",cluster=~"C|D",cluster!~"E|F"}`,
			label:    "cluster",
			values:   []string{"A", "B"},
			expected: `test_metrics{cluster!="A",cluster!~"E|F",cluster=~"C|D",cluster=~"A|B"}`,
		},
		{
			name:     "Existing label for cluster and others",
			query:    `test_metrics{akey="value",cluster="A"}`,
			label:    "cluster",
			values:   []string{"A", "B"},
			expected: `test_metrics{cluster="A",akey="value",cluster=~"A|B"}`,
		},
		{
			name:     "Blank in existing query",
			query:    `test_metrics{akey = "value",  cluster = "A"}`,
			label:    "cluster",
			values:   []string{"A", "B"},
			expected: `test_metrics{cluster="A",akey="value",cluster=~"A|B"}`,
		},
	}

	for _, c := range caseList {
		t.Run(c.name, func(t *testing.T) {
			output, err := InjectLabels(c.query, c.label, c.values)
			if err != nil {
				t.Errorf("Encountered error during label injection: (%v)", err)
			} else if output != c.expected {
				t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, output, c.expected)
			}
		})
	}
}
