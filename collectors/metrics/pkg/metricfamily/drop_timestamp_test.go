package metricfamily

import (
	"fmt"
	"testing"

	clientmodel "github.com/prometheus/client_model/go"
)

func TestDropTimestamp(t *testing.T) {

	family := func(name string, metrics ...*clientmodel.Metric) *clientmodel.MetricFamily {
		families := &clientmodel.MetricFamily{Name: &name}
		families.Metric = append(families.Metric, metrics...)
		return families
	}

	metric := func(timestamp *int64) *clientmodel.Metric { return &clientmodel.Metric{TimestampMs: timestamp} }

	timestamp := func(timestamp int64) *int64 { return &timestamp }

	type checkFunc func(family *clientmodel.MetricFamily, ok bool, err error) error

	isOK := func(want bool) checkFunc {
		return func(_ *clientmodel.MetricFamily, got bool, _ error) error {
			if want != got {
				return fmt.Errorf("want ok %t, got %t", want, got)
			}
			return nil
		}
	}

	hasErr := func(want error) checkFunc {
		return func(_ *clientmodel.MetricFamily, _ bool, got error) error {
			if want != got {
				return fmt.Errorf("want err %v, got %v", want, got)
			}
			return nil
		}
	}

	hasMetrics := func(want int) checkFunc {
		return func(m *clientmodel.MetricFamily, _ bool, _ error) error {
			if got := len(m.Metric); want != got {
				return fmt.Errorf("want len(m.Metric)=%v, got %v", want, got)
			}
			return nil
		}
	}

	metricsHaveTimestamps := func(want bool) checkFunc {
		return func(m *clientmodel.MetricFamily, _ bool, _ error) error {
			for _, metric := range m.Metric {
				if got := metric.TimestampMs != nil; want != got {
					return fmt.Errorf("want metrics to have timestamp %t, got %t", want, got)
				}
			}
			return nil
		}
	}

	for _, tc := range []struct {
		family *clientmodel.MetricFamily
		name   string
		checks []checkFunc
	}{
		{
			name: "nil family",
			checks: []checkFunc{
				isOK(true),
				hasErr(nil),
			},
		},
		{
			name:   "family without timestamp",
			family: family("foo"),
			checks: []checkFunc{
				isOK(true),
				hasErr(nil),
			},
		},
		{
			name:   "family without timestamp",
			family: family("foo", metric(nil)),
			checks: []checkFunc{
				isOK(true),
				hasErr(nil),
				hasMetrics(1),
				metricsHaveTimestamps(false),
			},
		},
		{
			name:   "family with timestamp",
			family: family("foo", metric(nil), metric(timestamp(1))),
			checks: []checkFunc{
				isOK(true),
				hasErr(nil),
				hasMetrics(2),
				metricsHaveTimestamps(false),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := DropTimestamp(tc.family)

			for _, check := range tc.checks {
				if err := check(tc.family, ok, err); err != nil {
					t.Error(err)
				}
			}
		})
	}
}
