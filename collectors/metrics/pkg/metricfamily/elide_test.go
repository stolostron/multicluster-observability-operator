package metricfamily

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	clientmodel "github.com/prometheus/client_model/go"
)

func TestElide(t *testing.T) {
	family := func(metrics ...*clientmodel.Metric) *clientmodel.MetricFamily {
		families := &clientmodel.MetricFamily{Name: proto.String("test")}
		families.Metric = append(families.Metric, metrics...)
		return families
	}

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

	metricIsNil := func(want bool) checkFunc {
		return func(m *clientmodel.MetricFamily, _ bool, _ error) error {
			if got := m == nil; want != got {
				return fmt.Errorf("want metric to be nil=%t, got %t", want, got)
			}
			return nil
		}
	}

	hasMetricCount := func(want int) checkFunc {
		return func(m *clientmodel.MetricFamily, _ bool, _ error) error {
			if got := len(m.Metric); want != got {
				return fmt.Errorf("want len(m.Metric)=%v, got %v", want, got)
			}
			return nil
		}
	}

	hasLabelCount := func(want ...int) checkFunc {
		return func(family *clientmodel.MetricFamily, _ bool, _ error) error {
			for i := range family.Metric {
				if got := len(family.Metric[i].Label); got != want[i] {
					return fmt.Errorf(
						"want len(m.Metric[%v].Label)=%v, got %v",
						i, want[i], got)
				}
			}
			return nil
		}
	}

	hasLabels := func(want bool, labels ...string) checkFunc {
		return func(family *clientmodel.MetricFamily, _ bool, _ error) error {
			labelSet := make(map[string]struct{})
			for i := range family.Metric {
				for j := range family.Metric[i].Label {
					labelSet[family.Metric[i].Label[j].GetName()] = struct{}{}
				}
			}

			for _, label := range labels {
				if _, got := labelSet[label]; want != got {
					wants := "present"
					if !want {
						wants = "not present"
					}

					gots := "is"
					if !got {
						gots = "isn't"
					}

					return fmt.Errorf(
						"want label %q be %s in metrics, but it %s",
						label, wants, gots,
					)
				}
			}

			return nil
		}
	}

	metricWithLabels := func(labels ...string) *clientmodel.Metric {
		var labelPairs []*clientmodel.LabelPair
		for _, l := range labels {
			labelPairs = append(labelPairs, &clientmodel.LabelPair{Name: proto.String(l)})
		}
		return &clientmodel.Metric{Label: labelPairs}
	}

	for _, tc := range []struct {
		family *clientmodel.MetricFamily
		elide  *elide
		name   string
		checks []checkFunc
	}{
		{
			name:   "nil family",
			family: nil,
			elide:  NewElide("elide"),
			checks: []checkFunc{
				isOK(true),
				hasErr(nil),
				metricIsNil(true),
			},
		},
		{
			name:   "empty family",
			family: family(),
			elide:  NewElide("elide"),
			checks: []checkFunc{
				isOK(true),
				hasErr(nil),
				hasMetricCount(0),
			},
		},
		{
			name:   "one elide one retain",
			family: family(metricWithLabels("retain", "elide")),
			elide:  NewElide("elide"),
			checks: []checkFunc{
				isOK(true),
				hasErr(nil),
				hasMetricCount(1),
				hasLabelCount(1),
				hasLabels(false, "elide"),
				hasLabels(true, "retain"),
			},
		},
		{
			name:   "no match",
			family: family(metricWithLabels("retain")),
			elide:  NewElide("elide"),
			checks: []checkFunc{
				isOK(true),
				hasErr(nil),
				hasMetricCount(1),
				hasLabelCount(1),
				hasLabels(false, "elide"),
				hasLabels(true, "retain"),
			},
		},
		{
			name:   "single match",
			family: family(metricWithLabels("elide")),
			elide:  NewElide("elide"),
			checks: []checkFunc{
				isOK(true),
				hasErr(nil),
				hasMetricCount(1),
				hasLabelCount(0),
				hasLabels(false, "elide"),
			},
		},
		{
			name: "multiple retains, multiple elides",
			family: family(
				metricWithLabels("elide1", "elide2", "retain1", "retain2"),
			),
			elide: NewElide("elide1", "elide2"),
			checks: []checkFunc{
				isOK(true),
				hasErr(nil),
				hasMetricCount(1),
				hasLabelCount(2),
				hasLabels(false, "elide1"),
				hasLabels(false, "elide2"),
				hasLabels(true, "retain1"),
				hasLabels(true, "retain2"),
			},
		},
		{
			name: "empty elider",
			family: family(
				metricWithLabels("retain1", "retain2"),
			),
			elide: NewElide(),
			checks: []checkFunc{
				isOK(true),
				hasErr(nil),
				hasMetricCount(1),
				hasLabelCount(2),
				hasLabels(true, "retain1"),
				hasLabels(true, "retain2"),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := tc.elide.Transform(tc.family)

			for _, check := range tc.checks {
				if err := check(tc.family, ok, err); err != nil {
					t.Error(err)
				}
			}
		})
	}
}
