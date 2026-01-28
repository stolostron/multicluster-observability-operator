// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package metricfamily

import (
	prom "github.com/prometheus/client_model/go"
)

type Elide struct {
	labelSet map[string]struct{}
}

// NewElide creates a new elide transformer for the given metrics.
func NewElide(labels ...string) *Elide {
	labelSet := make(map[string]struct{})
	for i := range labels {
		labelSet[labels[i]] = struct{}{}
	}

	return &Elide{labelSet}
}

// Transform filters label pairs in the given metrics family,
// eliding labels.
func (t *Elide) Transform(family *prom.MetricFamily) (bool, error) {
	if family == nil || len(family.Metric) == 0 {
		return true, nil
	}

	for i := range family.Metric {
		var filtered []*prom.LabelPair
		for j := range family.Metric[i].Label {
			if _, elide := t.labelSet[family.Metric[i].Label[j].GetName()]; elide {
				continue
			}
			filtered = append(filtered, family.Metric[i].Label[j])
		}
		family.Metric[i].Label = filtered
	}

	return true, nil
}
