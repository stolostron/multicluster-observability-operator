package metricfamily

import (
	"fmt"

	clientmodel "github.com/prometheus/client_model/go"
)

type requireLabel struct {
	labels map[string]string
}

func NewRequiredLabels(labels map[string]string) Transformer {
	return requireLabel{labels: labels}
}

var (
	ErrRequiredLabelMissing = fmt.Errorf("a required label is missing from the metric")
)

func (t requireLabel) Transform(family *clientmodel.MetricFamily) (bool, error) {
	for k, v := range t.labels {
	Metrics:
		for _, m := range family.Metric {
			if m == nil {
				continue
			}
			for _, label := range m.Label {
				if label == nil {
					continue
				}
				if label.GetName() == k {
					if label.GetValue() != v {
						return false, fmt.Errorf("expected label %s to have value %s instead of %s", label.GetName(), v, label.GetValue())
					}
					continue Metrics
				}
			}
			return false, ErrRequiredLabelMissing
		}
	}
	return true, nil
}
