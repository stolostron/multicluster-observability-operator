package metricfamily

import (
	clientmodel "github.com/prometheus/client_model/go"
)

type Transformer interface {
	Transform(*clientmodel.MetricFamily) (ok bool, err error)
}

type TransformerFunc func(*clientmodel.MetricFamily) (ok bool, err error)

func (f TransformerFunc) Transform(family *clientmodel.MetricFamily) (ok bool, err error) {
	return f(family)
}

// MetricsCount returns the number of unique metrics in the given families. It skips
// nil families but does not skip nil metrics.
func MetricsCount(families []*clientmodel.MetricFamily) int {
	count := 0
	for _, family := range families {
		if family == nil {
			continue
		}
		count += len(family.Metric)
	}
	return count
}

func Filter(families []*clientmodel.MetricFamily, filter Transformer) error {
	for i, family := range families {
		ok, err := filter.Transform(family)
		if err != nil {
			return err
		}
		if !ok {
			families[i] = nil
		}
	}
	return nil
}
