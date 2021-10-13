package metricfamily

import (
	clientmodel "github.com/prometheus/client_model/go"
)

type MultiTransformer struct {
	transformers []Transformer
	builderFuncs []func() Transformer
}

func (a *MultiTransformer) With(t Transformer) {
	if t != nil {
		a.transformers = append(a.transformers, t)
	}
}

func (a *MultiTransformer) WithFunc(f func() Transformer) {
	a.builderFuncs = append(a.builderFuncs, f)
}

func (a MultiTransformer) Transform(family *clientmodel.MetricFamily) (bool, error) {
	var ts []Transformer

	for _, f := range a.builderFuncs {
		ts = append(ts, f())
	}

	ts = append(ts, a.transformers...)

	for _, t := range ts {
		ok, err := t.Transform(family)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}

	return true, nil
}
