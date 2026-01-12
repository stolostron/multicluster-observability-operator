// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package metricfamily

import (
	"sync"

	clientmodel "github.com/prometheus/client_model/go"
	"github.com/prometheus/prometheus/prompb"
	"golang.org/x/exp/slices"
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

func appendLabels(
	existing []*clientmodel.LabelPair,
	overrides map[string]*clientmodel.LabelPair) []*clientmodel.LabelPair {
	var found []string

	// remove blank names and values
	var withoutEmpties []*clientmodel.LabelPair = make([]*clientmodel.LabelPair, 0)

	for i, pair := range existing {
		name := pair.GetName()
		value := pair.GetValue()
		// remove any name == "" or value == nil
		if name != "" && value != "" {
			withoutEmpties = insertLexicographicallyByName(withoutEmpties, existing[i])
		}
	}
	existing = withoutEmpties

	// override matching existing labels
	for i, pair := range existing {
		name := pair.GetName()
		if value, ok := overrides[name]; ok {
			existing[i] = value
			found = append(found, name)
		}
	}

	// append any overrides that didn't already exist
	for k, v := range overrides {
		// only append real names and values
		// don't append anything overwritten above
		if k != "" && v.GetValue() != "" && !slices.Contains(found, k) {
			existing = insertLexicographicallyByName(existing, v)
		}
	}

	return existing
}

func insertLexicographicallyByName(
	existing []*clientmodel.LabelPair,
	value *clientmodel.LabelPair) []*clientmodel.LabelPair {
	existing = append(existing, value)
	i := len(existing) - 1
	for i > 0 && existing[i].GetName() < existing[i-1].GetName() {
		existing[i], existing[i-1] = existing[i-1], existing[i]
		i -= 1
	}
	return existing
}

func InsertLabelLexicographicallyByName(
	existing []prompb.Label,
	value prompb.Label) []prompb.Label {
	existing = append(existing, value)
	i := len(existing) - 1
	for i > 0 && existing[i].GetName() < existing[i-1].GetName() {
		existing[i], existing[i-1] = existing[i-1], existing[i]
		i -= 1
	}
	return existing
}
