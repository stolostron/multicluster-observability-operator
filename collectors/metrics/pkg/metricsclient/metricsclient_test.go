// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package metricsclient

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/gogo/protobuf/proto"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	clientmodel "github.com/prometheus/client_model/go"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/assert"
)

func TestDefaultTransport(t *testing.T) {
	logger := log.NewNopLogger()
	want := &http.Transport{
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   true,
	}
	http := DefaultTransport(logger)
	if http.DialContext == nil || reflect.TypeOf(http) != reflect.TypeOf(want) {
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

	emptyLabelName := ""

	barMetricName := "bar_metric"
	barHelp := "bar help text"
	barLabelName := "bar"
	barLabelValue1 := "baz"

	value42 := 42.0
	value50 := 50.0
	timestamp := int64(15615582020000)
	now := time.Now()
	nowTimestamp := now.UnixNano() / int64(time.Millisecond)

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
			Samples: []prompb.Sample{{Value: value42, Timestamp: nowTimestamp}},
		}, {
			Labels:  []prompb.Label{{Name: nameLabelName, Value: fooMetricName}, {Name: fooLabelName, Value: fooLabelValue2}},
			Samples: []prompb.Sample{{Value: value50, Timestamp: nowTimestamp}},
		}, {
			Labels:  []prompb.Label{{Name: nameLabelName, Value: barMetricName}, {Name: barLabelName, Value: barLabelValue1}},
			Samples: []prompb.Sample{{Value: value42, Timestamp: nowTimestamp}},
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
			Samples: []prompb.Sample{{Value: value42, Timestamp: nowTimestamp}},
		}, {
			Labels:  []prompb.Label{{Name: nameLabelName, Value: fooMetricName}, {Name: fooLabelName, Value: fooLabelValue2}},
			Samples: []prompb.Sample{{Value: value50, Timestamp: nowTimestamp}},
		}, {
			Labels:  []prompb.Label{{Name: nameLabelName, Value: barMetricName}, {Name: barLabelName, Value: barLabelValue1}},
			Samples: []prompb.Sample{{Value: value42, Timestamp: nowTimestamp}},
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
			Samples: []prompb.Sample{{Value: value42, Timestamp: nowTimestamp}},
		}, {
			Labels:  []prompb.Label{{Name: nameLabelName, Value: fooMetricName}, {Name: fooLabelName, Value: fooLabelValue2}},
			Samples: []prompb.Sample{{Value: value50, Timestamp: nowTimestamp}},
		}, {
			Labels:  []prompb.Label{{Name: nameLabelName, Value: barMetricName}, {Name: barLabelName, Value: barLabelValue1}},
			Samples: []prompb.Sample{{Value: value42, Timestamp: nowTimestamp}},
		}},
	}, {
		name: "unsanitized",
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
				}, {
					// With empty label.
					Label:       []*clientmodel.LabelPair{{Name: &emptyLabelName, Value: &fooLabelValue2}},
					Counter:     &clientmodel.Counter{Value: &value50},
					TimestampMs: &timestamp,
				},
				},
			}, {
				Name: &barMetricName,
				Help: &barHelp,
				Type: &counter,
				Metric: []*clientmodel.Metric{{
					Label:       []*clientmodel.LabelPair{{Name: &barLabelName, Value: &barLabelValue1}},
					Counter:     &clientmodel.Counter{Value: &value42},
					TimestampMs: &timestamp,
				}, {
					// With duplicate labels.
					Label:       []*clientmodel.LabelPair{{Name: &fooLabelName, Value: &fooLabelValue2}, {Name: &fooLabelName, Value: &fooLabelValue2}},
					Counter:     &clientmodel.Counter{Value: &value42},
					TimestampMs: &timestamp,
				}, {
					// With out-of-order labels.
					Label:       []*clientmodel.LabelPair{{Name: &fooLabelName, Value: &fooLabelValue2}, {Name: &barLabelName, Value: &barLabelValue1}},
					Counter:     &clientmodel.Counter{Value: &value50},
					TimestampMs: &timestamp,
				}},
			}},
		},
		want: []prompb.TimeSeries{{
			Labels:  []prompb.Label{{Name: nameLabelName, Value: fooMetricName}, {Name: fooLabelName, Value: fooLabelValue1}},
			Samples: []prompb.Sample{{Value: value42, Timestamp: nowTimestamp}},
		}, {
			Labels:  []prompb.Label{{Name: nameLabelName, Value: fooMetricName}, {Name: fooLabelName, Value: fooLabelValue2}},
			Samples: []prompb.Sample{{Value: value50, Timestamp: nowTimestamp}},
		}, {
			Labels:  []prompb.Label{{Name: nameLabelName, Value: fooMetricName}},
			Samples: []prompb.Sample{{Value: value50, Timestamp: nowTimestamp}},
		}, {
			Labels:  []prompb.Label{{Name: nameLabelName, Value: barMetricName}, {Name: barLabelName, Value: barLabelValue1}},
			Samples: []prompb.Sample{{Value: value42, Timestamp: nowTimestamp}},
		}, {
			Labels:  []prompb.Label{{Name: nameLabelName, Value: barMetricName}, {Name: fooLabelName, Value: fooLabelValue2}},
			Samples: []prompb.Sample{{Value: value42, Timestamp: nowTimestamp}},
		}, {
			Labels:  []prompb.Label{{Name: nameLabelName, Value: barMetricName}, {Name: barLabelName, Value: barLabelValue1}, {Name: fooLabelName, Value: fooLabelValue2}},
			Samples: []prompb.Sample{{Value: value50, Timestamp: nowTimestamp}},
		}},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := convertToTimeseries(tt.in, now)
			if err != nil {
				t.Errorf("converting timeseries errored: %v", err)
			}
			if ok, err := timeseriesEqual(tt.want, out); !ok {
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

func TestClient_RemoteWrite(t *testing.T) {
	tests := []struct {
		name          string
		families      []*clientmodel.MetricFamily
		serverHandler http.HandlerFunc
		expect        func(t *testing.T, err error, retryCount int)
	}{
		{
			name:     "successful write with metrics",
			families: []*clientmodel.MetricFamily{mockMetricFamily()},
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			expect: func(t *testing.T, err error, retryCount int) {
				assert.NoError(t, err)
				assert.Equal(t, 1, retryCount)
			},
		},
		{
			name:     "no metrics to write",
			families: []*clientmodel.MetricFamily{},
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			expect: func(t *testing.T, err error, retryCount int) {
				assert.NoError(t, err)
				assert.Equal(t, 0, retryCount)
			},
		},
		{
			name:     "retryable error",
			families: []*clientmodel.MetricFamily{mockMetricFamily()},
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			expect: func(t *testing.T, err error, retryCount int) {
				assert.Error(t, err)
				assert.Greater(t, retryCount, 1)
			},
		},
		{
			name:     "non-retryable error",
			families: []*clientmodel.MetricFamily{mockMetricFamily()},
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusConflict)
			},
			expect: func(t *testing.T, err error, retryCount int) {
				assert.Error(t, err)
				assert.Equal(t, 1, retryCount)
				// Ensure the http error is wrapped
				var httpError *HTTPError
				assert.ErrorAs(t, err, &httpError)
				assert.Equal(t, http.StatusConflict, httpError.StatusCode)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestCount := 0

			handler := func(w http.ResponseWriter, r *http.Request) {
				requestCount++
				tt.serverHandler(w, r)
			}
			ts := httptest.NewServer(http.HandlerFunc(handler))
			defer ts.Close()

			reg := prometheus.NewRegistry()
			clientMetrics := &ClientMetrics{
				ForwardRemoteWriteRequests: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
					Name: "forward_write_requests_total",
					Help: "Counter of forward remote write requests.",
				}, []string{"status_code"}),
			}
			client := &Client{logger: log.NewNopLogger(), client: ts.Client(), metrics: clientMetrics}

			req, err := http.NewRequest("POST", ts.URL, bytes.NewBuffer([]byte{}))
			assert.NoError(t, err)

			err = client.RemoteWrite(context.Background(), req, tt.families, 30*time.Second)

			tt.expect(t, err, requestCount)
		})
	}
}

func mockMetricFamily() *clientmodel.MetricFamily {
	return &clientmodel.MetricFamily{
		Name: proto.String("test_metric"),
		Type: clientmodel.MetricType_COUNTER.Enum(),
		Metric: []*clientmodel.Metric{
			{
				Counter:     &clientmodel.Counter{Value: proto.Float64(1)},
				TimestampMs: proto.Int64(time.Now().UnixNano() / int64(time.Millisecond)),
			},
		},
	}
}
