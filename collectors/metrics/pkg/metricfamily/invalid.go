package metricfamily

import (
	"fmt"
	"time"

	clientmodel "github.com/prometheus/client_model/go"
)

type errorInvalidFederateSamples struct {
	min int64
}

func NewErrorInvalidFederateSamples(min time.Time) Transformer {
	return &errorInvalidFederateSamples{
		min: min.Unix() * 1000,
	}
}

func (t *errorInvalidFederateSamples) Transform(family *clientmodel.MetricFamily) (bool, error) {
	name := family.GetName()
	if len(name) == 0 {
		return false, nil
	}
	if len(name) > 255 {
		return false, fmt.Errorf("metrics_name cannot be longer than 255 characters")
	}
	if family.Type == nil {
		return false, nil
	}
	switch t := *family.Type; t {
	case clientmodel.MetricType_COUNTER:
	case clientmodel.MetricType_GAUGE:
	case clientmodel.MetricType_HISTOGRAM:
	case clientmodel.MetricType_SUMMARY:
	case clientmodel.MetricType_UNTYPED:
	default:
		return false, fmt.Errorf("unknown metric type %s", t)
	}

	for _, m := range family.Metric {
		if m == nil {
			continue
		}
		for _, label := range m.Label {
			if label.Name == nil || len(*label.Name) == 0 || len(*label.Name) > 255 {
				return false, fmt.Errorf("label_name cannot be longer than 255 characters")
			}
			if label.Value == nil || len(*label.Value) > 255 {
				return false, fmt.Errorf("label_value cannot be longer than 255 characters")
			}
		}
		if m.TimestampMs == nil {
			return false, ErrNoTimestamp
		}
		if *m.TimestampMs < t.min {
			return false, ErrTimestampTooOld
		}
		switch t := *family.Type; t {
		case clientmodel.MetricType_COUNTER:
			if m.Counter == nil || m.Gauge != nil || m.Histogram != nil || m.Summary != nil || m.Untyped != nil {
				return false, fmt.Errorf("metric type %s must have counter field set", t)
			}
		case clientmodel.MetricType_GAUGE:
			if m.Counter != nil || m.Gauge == nil || m.Histogram != nil || m.Summary != nil || m.Untyped != nil {
				return false, fmt.Errorf("metric type %s must have gauge field set", t)
			}
		case clientmodel.MetricType_HISTOGRAM:
			if m.Counter != nil || m.Gauge != nil || m.Histogram == nil || m.Summary != nil || m.Untyped != nil {
				return false, fmt.Errorf("metric type %s must have histogram field set", t)
			}
		case clientmodel.MetricType_SUMMARY:
			if m.Counter != nil || m.Gauge != nil || m.Histogram != nil || m.Summary == nil || m.Untyped != nil {
				return false, fmt.Errorf("metric type %s must have summary field set", t)
			}
		case clientmodel.MetricType_UNTYPED:
			if m.Counter != nil || m.Gauge != nil || m.Histogram != nil || m.Summary != nil || m.Untyped == nil {
				return false, fmt.Errorf("metric type %s must have untyped field set", t)
			}
		}
	}
	return true, nil
}

type dropInvalidFederateSamples struct {
	min int64
}

func NewDropInvalidFederateSamples(min time.Time) Transformer {
	return &dropInvalidFederateSamples{
		min: min.Unix() * 1000,
	}
}

func (t *dropInvalidFederateSamples) Transform(family *clientmodel.MetricFamily) (bool, error) {
	name := family.GetName()
	if len(name) == 0 {
		return false, nil
	}
	if len(name) > 255 {
		return false, nil
	}
	if family.Type == nil {
		return false, nil
	}
	switch t := *family.Type; t {
	case clientmodel.MetricType_COUNTER:
	case clientmodel.MetricType_GAUGE:
	case clientmodel.MetricType_HISTOGRAM:
	case clientmodel.MetricType_SUMMARY:
	case clientmodel.MetricType_UNTYPED:
	default:
		return false, nil
	}

	for i, m := range family.Metric {
		if m == nil {
			continue
		}
		packLabels := false
		for j, label := range m.Label {
			if label.Name == nil || len(*label.Name) == 0 || len(*label.Name) > 255 {
				m.Label[j] = nil
				packLabels = true
			}
			if label.Value == nil || len(*label.Value) > 255 {
				m.Label[j] = nil
				packLabels = true
			}
		}
		if packLabels {
			m.Label = PackLabels(m.Label)
		}
		if m.TimestampMs == nil || *m.TimestampMs < t.min {
			family.Metric[i] = nil
			continue
		}
		switch t := *family.Type; t {
		case clientmodel.MetricType_COUNTER:
			if m.Counter == nil || m.Gauge != nil || m.Histogram != nil || m.Summary != nil || m.Untyped != nil {
				family.Metric[i] = nil
			}
		case clientmodel.MetricType_GAUGE:
			if m.Counter != nil || m.Gauge == nil || m.Histogram != nil || m.Summary != nil || m.Untyped != nil {
				family.Metric[i] = nil
			}
		case clientmodel.MetricType_HISTOGRAM:
			if m.Counter != nil || m.Gauge != nil || m.Histogram == nil || m.Summary != nil || m.Untyped != nil {
				family.Metric[i] = nil
			}
		case clientmodel.MetricType_SUMMARY:
			if m.Counter != nil || m.Gauge != nil || m.Histogram != nil || m.Summary == nil || m.Untyped != nil {
				family.Metric[i] = nil
			}
		case clientmodel.MetricType_UNTYPED:
			if m.Counter != nil || m.Gauge != nil || m.Histogram != nil || m.Summary != nil || m.Untyped == nil {
				family.Metric[i] = nil
			}
		}
	}
	return true, nil
}

// PackLabels fills holes in the label slice by shifting items towards the zero index.
// It will modify the slice in place.
func PackLabels(labels []*clientmodel.LabelPair) []*clientmodel.LabelPair {
	j := len(labels)
	next := 0
Found:
	for i := 0; i < j; i++ {
		if labels[i] != nil {
			continue
		}
		// scan for the next non-nil metric
		if next <= i {
			next = i + 1
		}
		for k := next; k < j; k++ {
			if labels[k] == nil {
				continue
			}
			// fill the current i with a non-nil metric
			labels[i], labels[k] = labels[k], nil
			next = k + 1
			continue Found
		}
		// no more valid metrics
		labels = labels[:i]
		break
	}
	return labels
}
