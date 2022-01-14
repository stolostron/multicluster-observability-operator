package metricfamily

import clientmodel "github.com/prometheus/client_model/go"

func None(*clientmodel.MetricFamily) (bool, error) { return true, nil }
