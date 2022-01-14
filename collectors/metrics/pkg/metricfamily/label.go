package metricfamily

import (
	"sync"

	clientmodel "github.com/prometheus/client_model/go"
)

type LabelRetriever interface {
	Labels() (map[string]string, error)
}

type label struct {
	labels    map[string]*clientmodel.LabelPair
	retriever LabelRetriever
	mu        sync.Mutex
}

func NewLabel(labels map[string]string, retriever LabelRetriever) Transformer {
	pairs := make(map[string]*clientmodel.LabelPair)
	for k, v := range labels {
		name, value := k, v
		pairs[k] = &clientmodel.LabelPair{Name: &name, Value: &value}
	}
	return &label{
		labels:    pairs,
		retriever: retriever,
	}
}

func (t *label) Transform(family *clientmodel.MetricFamily) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	// lazily resolve the label retriever as needed
	if t.retriever != nil && len(family.Metric) > 0 {
		added, err := t.retriever.Labels()
		if err != nil {
			return false, err
		}
		t.retriever = nil
		for k, v := range added {
			name, value := k, v
			t.labels[k] = &clientmodel.LabelPair{Name: &name, Value: &value}
		}
	}
	for _, m := range family.Metric {
		m.Label = appendLabels(m.Label, t.labels)
	}
	return true, nil
}

func appendLabels(existing []*clientmodel.LabelPair, overrides map[string]*clientmodel.LabelPair) []*clientmodel.LabelPair {
	var found []string
	for i, pair := range existing {
		name := pair.GetName()
		if value, ok := overrides[name]; ok {
			existing[i] = value
			found = append(found, name)
		}
	}
	for k, v := range overrides {
		if !contains(found, k) {
			existing = append(existing, v)
		}
	}
	return existing
}

func contains(values []string, s string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}
