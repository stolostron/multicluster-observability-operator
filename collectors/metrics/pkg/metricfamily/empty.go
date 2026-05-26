// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package metricfamily

import clientmodel "github.com/prometheus/client_model/go"

func DropEmptyFamilies(family *clientmodel.MetricFamily) (bool, error) {
	for _, m := range family.Metric {
		if m != nil {
			return true, nil
		}
	}
	return false, nil
}
