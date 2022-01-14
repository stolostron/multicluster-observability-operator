package metricfamily

import clientmodel "github.com/prometheus/client_model/go"

func PackMetrics(family *clientmodel.MetricFamily) (bool, error) {
	metrics := family.Metric
	j := len(metrics)
	next := 0
Found:
	for i := 0; i < j; i++ {
		if metrics[i] != nil {
			continue
		}
		// scan for the next non-nil metric
		if next <= i {
			next = i + 1
		}
		for k := next; k < j; k++ {
			if metrics[k] == nil {
				continue
			}
			// fill the current i with a non-nil metric
			metrics[i], metrics[k] = metrics[k], nil
			next = k + 1
			continue Found
		}
		// no more valid metrics
		family.Metric = metrics[:i]
		break
	}
	return len(family.Metric) > 0, nil
}

// Pack returns only families with metrics in the returned array, preserving the
// order of the original slice. Nil entries are removed from the slice. The returned
// slice may be empty.
func Pack(families []*clientmodel.MetricFamily) []*clientmodel.MetricFamily {
	j := len(families)
	next := 0
Found:
	for i := 0; i < j; i++ {
		if families[i] != nil && len(families[i].Metric) > 0 {
			continue
		}
		// scan for the next non-nil family
		if next <= i {
			next = i + 1
		}
		for k := next; k < j; k++ {
			if families[k] == nil || len(families[k].Metric) == 0 {
				continue
			}
			// fill the current i with a non-nil family
			families[i], families[k] = families[k], nil
			next = k + 1
			continue Found
		}
		// no more valid families
		return families[:i]
	}
	return families
}
