// Copyright Contributors to the Open Cluster Management project
package metricsclient

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	clientmodel "github.com/prometheus/client_model/go"
	"github.com/prometheus/prometheus/prompb"
)

func TestDefaultTransport(t *testing.T) {
	logger := log.NewNopLogger()
	want := &http.Transport{
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   true,
	}
	http := DefaultTransport(logger, true)
	if http.Dial == nil || reflect.TypeOf(http) != reflect.TypeOf(want) {
		t.Errorf("Default transport doesn't match expected format")
	}

}

func Test_convertToTimeseries(t *testing.T) {
	counter := clientmodel.MetricType_COUNTER
	untyped := clientmodel.MetricType_UNTYPED
	gauge := clientmodel.MetricType_GAUGE

	fooMetricName := "foo_metric"
	fooHelp := "foo help text"
	fooLabelName := "foo"
	fooLabelValue1 := "bar"
	fooLabelValue2 := "baz"

	barMetricName := "bar_metric"
	barHelp := "bar help text"
	barLabelName := "bar"
	barLabelValue1 := "baz"

	value42 := 42.0
	value50 := 50.0
	timestamp := int64(1596948588956) //15615582020000)
	now := time.Now()
	nowTimestamp := now.UnixNano() / int64(time.Millisecond)
	fmt.Println("timestamp: ", timestamp)

	fmt.Println("nowTimestamp: ", nowTimestamp)
	tests := []struct {
		name string
		in   *PartitionedMetrics
		want []prompb.TimeSeries
	}{{
		name: "counter",
		in: &PartitionedMetrics{
			Families: []*clientmodel.MetricFamily{{
				Name: &fooMetricName,
				Help: &fooHelp,
				Type: &counter,
				Metric: []*clientmodel.Metric{{
					Label:       []*clientmodel.LabelPair{{Name: &fooLabelName, Value: &fooLabelValue1}},
					Counter:     &clientmodel.Counter{Value: &value42},
					TimestampMs: &timestamp,
				}, {
					Label:       []*clientmodel.LabelPair{{Name: &fooLabelName, Value: &fooLabelValue2}},
					Counter:     &clientmodel.Counter{Value: &value50},
					TimestampMs: &timestamp,
				}},
			}, {
				Name: &barMetricName,
				Help: &barHelp,
				Type: &counter,
				Metric: []*clientmodel.Metric{{
					Label:       []*clientmodel.LabelPair{{Name: &barLabelName, Value: &barLabelValue1}},
					Counter:     &clientmodel.Counter{Value: &value42},
					TimestampMs: &timestamp,
				}},
			}},
		},
		want: []prompb.TimeSeries{{
			Labels:  []prompb.Label{{Name: nameLabelName, Value: fooMetricName}, {Name: fooLabelName, Value: fooLabelValue1}},
			Samples: []prompb.Sample{{Value: value42, Timestamp: timestamp}},
		}, {
			Labels:  []prompb.Label{{Name: nameLabelName, Value: fooMetricName}, {Name: fooLabelName, Value: fooLabelValue2}},
			Samples: []prompb.Sample{{Value: value50, Timestamp: timestamp}},
		}, {
			Labels:  []prompb.Label{{Name: nameLabelName, Value: barMetricName}, {Name: barLabelName, Value: barLabelValue1}},
			Samples: []prompb.Sample{{Value: value42, Timestamp: timestamp}},
		}},
	}, {
		name: "gauge",
		in: &PartitionedMetrics{
			Families: []*clientmodel.MetricFamily{{
				Name: &fooMetricName,
				Help: &fooHelp,
				Type: &gauge,
				Metric: []*clientmodel.Metric{{
					Label:       []*clientmodel.LabelPair{{Name: &fooLabelName, Value: &fooLabelValue1}},
					Gauge:       &clientmodel.Gauge{Value: &value42},
					TimestampMs: &timestamp,
				}, {
					Label:       []*clientmodel.LabelPair{{Name: &fooLabelName, Value: &fooLabelValue2}},
					Gauge:       &clientmodel.Gauge{Value: &value50},
					TimestampMs: &timestamp,
				}},
			}, {
				Name: &barMetricName,
				Help: &barHelp,
				Type: &gauge,
				Metric: []*clientmodel.Metric{{
					Label:       []*clientmodel.LabelPair{{Name: &barLabelName, Value: &barLabelValue1}},
					Gauge:       &clientmodel.Gauge{Value: &value42},
					TimestampMs: &timestamp,
				}},
			}},
		},
		want: []prompb.TimeSeries{{
			Labels:  []prompb.Label{{Name: nameLabelName, Value: fooMetricName}, {Name: fooLabelName, Value: fooLabelValue1}},
			Samples: []prompb.Sample{{Value: value42, Timestamp: timestamp}},
		}, {
			Labels:  []prompb.Label{{Name: nameLabelName, Value: fooMetricName}, {Name: fooLabelName, Value: fooLabelValue2}},
			Samples: []prompb.Sample{{Value: value50, Timestamp: timestamp}},
		}, {
			Labels:  []prompb.Label{{Name: nameLabelName, Value: barMetricName}, {Name: barLabelName, Value: barLabelValue1}},
			Samples: []prompb.Sample{{Value: value42, Timestamp: timestamp}},
		}},
	}, {
		name: "untyped",
		in: &PartitionedMetrics{
			Families: []*clientmodel.MetricFamily{{
				Name: &fooMetricName,
				Help: &fooHelp,
				Type: &untyped,
				Metric: []*clientmodel.Metric{{
					Label:       []*clientmodel.LabelPair{{Name: &fooLabelName, Value: &fooLabelValue1}},
					Untyped:     &clientmodel.Untyped{Value: &value42},
					TimestampMs: &timestamp,
				}, {
					Label:       []*clientmodel.LabelPair{{Name: &fooLabelName, Value: &fooLabelValue2}},
					Untyped:     &clientmodel.Untyped{Value: &value50},
					TimestampMs: &timestamp,
				}},
			}, {
				Name: &barMetricName,
				Help: &barHelp,
				Type: &untyped,
				Metric: []*clientmodel.Metric{{
					Label:       []*clientmodel.LabelPair{{Name: &barLabelName, Value: &barLabelValue1}},
					Untyped:     &clientmodel.Untyped{Value: &value42},
					TimestampMs: &timestamp,
				}},
			}},
		},
		want: []prompb.TimeSeries{{
			Labels:  []prompb.Label{{Name: nameLabelName, Value: fooMetricName}, {Name: fooLabelName, Value: fooLabelValue1}},
			Samples: []prompb.Sample{{Value: value42, Timestamp: timestamp}},
		}, {
			Labels:  []prompb.Label{{Name: nameLabelName, Value: fooMetricName}, {Name: fooLabelName, Value: fooLabelValue2}},
			Samples: []prompb.Sample{{Value: value50, Timestamp: timestamp}},
		}, {
			Labels:  []prompb.Label{{Name: nameLabelName, Value: barMetricName}, {Name: barLabelName, Value: barLabelValue1}},
			Samples: []prompb.Sample{{Value: value42, Timestamp: timestamp}},
		}},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := convertToTimeseries(tt.in, now)
			if err != nil {
				t.Errorf("converting timeseries errored: %v", err)
			}
			if ok, err := timeseriesEqual(tt.want, out); !ok {
				// t.Error("want: ", tt.want)
				// t.Error("out: ", out)

				t.Errorf("timeseries don't match: %v", err)
			}
		})
	}
}

func timeseriesEqual(t1 []prompb.TimeSeries, t2 []prompb.TimeSeries) (bool, error) {
	if len(t1) != len(t2) {
		return false, fmt.Errorf("timeseries don't match amount of series: %d != %d", len(t1), len(t2))
	}

	for i, t := range t1 {
		for j, l := range t.Labels {
			if t2[i].Labels[j].Name != l.Name {
				return false, fmt.Errorf("label names don't match: %s != %s", t2[i].Labels[j].Name, l.Name)
			}
			if t2[i].Labels[j].Value != l.Value {
				return false, fmt.Errorf("label values don't match: %s != %s", t2[i].Labels[j].Value, l.Value)
			}
		}

		for j, s := range t.Samples {
			if t2[i].Samples[j].Timestamp != s.Timestamp {
				return false, fmt.Errorf("sample timestamps don't match: %d != %d", t2[i].Samples[j].Timestamp, s.Timestamp)
			}
			if t2[i].Samples[j].Value != s.Value {
				return false, fmt.Errorf("sample values don't match: %f != %f", t2[i].Samples[j].Value, s.Value)
			}
		}
	}

	return true, nil
}
