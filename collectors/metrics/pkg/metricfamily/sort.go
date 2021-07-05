package metricfamily

import (
	"sort"

	clientmodel "github.com/prometheus/client_model/go"
)

func SortMetrics(family *clientmodel.MetricFamily) (bool, error) {
	sort.Sort(MetricsByTimestamp(family.Metric))
	return true, nil
}

type MetricsByTimestamp []*clientmodel.Metric

func (m MetricsByTimestamp) Len() int {
	return len(m)
}

func (m MetricsByTimestamp) Less(i int, j int) bool {
	a, b := m[i], m[j]
	if a == nil {
		return b != nil
	}
	if b == nil {
		return false
	}
	if a.TimestampMs == nil {
		return b.TimestampMs != nil
	}
	if b.TimestampMs == nil {
		return false
	}
	return *a.TimestampMs < *b.TimestampMs
}

func (m MetricsByTimestamp) Swap(i int, j int) {
	m[i], m[j] = m[j], m[i]
}

// MergeSortedWithTimestamps collapses metrics families with the same name into a single family,
// preserving the order of the metrics. Families must be dense (no nils for families or metrics),
// all metrics must be sorted, and all metrics must have timestamps.
func MergeSortedWithTimestamps(families []*clientmodel.MetricFamily) []*clientmodel.MetricFamily {
	var dst *clientmodel.MetricFamily
	for pos, src := range families {
		if dst == nil {
			dst = src
			continue
		}
		if dst.GetName() != src.GetName() {
			dst = nil
			continue
		}

		lenI, lenJ := len(dst.Metric), len(src.Metric)

		// if the ranges don't overlap, we can block merge
		dstBegin, dstEnd := *dst.Metric[0].TimestampMs, *dst.Metric[lenI-1].TimestampMs
		srcBegin, srcEnd := *src.Metric[0].TimestampMs, *src.Metric[lenJ-1].TimestampMs
		if dstEnd < srcBegin {
			dst.Metric = append(dst.Metric, src.Metric...)
			families[pos] = nil
			continue
		}
		if srcEnd < dstBegin {
			dst.Metric = append(src.Metric, dst.Metric...)
			families[pos] = nil
			continue
		}

		// zip merge
		i, j := 0, 0
		result := make([]*clientmodel.Metric, 0, lenI+lenJ)
	Merge:
		for {
			switch {
			case j >= lenJ:
				for ; i < lenI; i++ {
					result = append(result, dst.Metric[i])
				}
				break Merge
			case i >= lenI:
				for ; j < lenJ; j++ {
					result = append(result, src.Metric[j])
				}
				break Merge
			default:
				a, b := *dst.Metric[i].TimestampMs, *src.Metric[j].TimestampMs
				if a <= b {
					result = append(result, dst.Metric[i])
					i++
				} else {
					result = append(result, src.Metric[j])
					j++
				}
			}
		}
		dst.Metric = result
		families[pos] = nil
	}
	return Pack(families)
}

// PackedFamilyWithTimestampsByName sorts a packed slice of metrics
// (no nils, all families have at least one metric, and all metrics
// have timestamps) in order of metric name and then oldest sample
type PackedFamilyWithTimestampsByName []*clientmodel.MetricFamily

func (families PackedFamilyWithTimestampsByName) Len() int {
	return len(families)
}

func (families PackedFamilyWithTimestampsByName) Less(i int, j int) bool {
	a, b := families[i].GetName(), families[j].GetName()
	if a < b {
		return true
	}
	if a > b {
		return false
	}
	tA, tB := *families[i].Metric[0].TimestampMs, *families[j].Metric[0].TimestampMs
	return tA < tB
}

func (families PackedFamilyWithTimestampsByName) Swap(i int, j int) {
	families[i], families[j] = families[j], families[i]
}
