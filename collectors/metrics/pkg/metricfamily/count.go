// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package metricfamily

import clientmodel "github.com/prometheus/client_model/go"

type Count struct {
	families int
	metrics  int
}

func (t *Count) Metrics() int { return t.metrics }

func (t *Count) Transform(family *clientmodel.MetricFamily) (bool, error) {
	t.families++
	t.metrics += len(family.Metric)
	return true, nil
}
