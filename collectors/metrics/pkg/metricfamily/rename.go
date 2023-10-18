// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package metricfamily

import clientmodel "github.com/prometheus/client_model/go"

type RenameMetrics struct {
	Names map[string]string
}

func (m RenameMetrics) Transform(family *clientmodel.MetricFamily) (bool, error) {
	if family == nil || family.Name == nil {
		return true, nil
	}
	if replace, ok := m.Names[*family.Name]; ok {
		family.Name = &replace
	}
	return true, nil
}
