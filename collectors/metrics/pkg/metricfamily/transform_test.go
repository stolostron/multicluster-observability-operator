package metricfamily

import (
	"reflect"
	"testing"

	clientmodel "github.com/prometheus/client_model/go"
)

func family(name string, timestamps ...int64) *clientmodel.MetricFamily {
	families := &clientmodel.MetricFamily{Name: &name}
	for i := range timestamps {
		families.Metric = append(families.Metric, &clientmodel.Metric{TimestampMs: &timestamps[i]})
	}
	return families
}

func metric(timestamp int64) *clientmodel.Metric {
	return &clientmodel.Metric{
		TimestampMs: &timestamp,
	}
}

func TestPack(t *testing.T) {
	a := family("A", 0)
	b := family("B", 1)
	c := family("C", 2)
	d := family("D")

	tests := []struct {
		name string
		args []*clientmodel.MetricFamily
		want []*clientmodel.MetricFamily
	}{
		{name: "empty", args: []*clientmodel.MetricFamily{nil, nil, nil}, want: []*clientmodel.MetricFamily{}},
		{name: "begin", args: []*clientmodel.MetricFamily{nil, a, b}, want: []*clientmodel.MetricFamily{a, b}},
		{name: "middle", args: []*clientmodel.MetricFamily{a, nil, b}, want: []*clientmodel.MetricFamily{a, b}},
		{name: "end", args: []*clientmodel.MetricFamily{a, b, nil}, want: []*clientmodel.MetricFamily{a, b}},
		{name: "skip", args: []*clientmodel.MetricFamily{a, nil, b, nil, c}, want: []*clientmodel.MetricFamily{a, b, c}},
		{name: "removes empty", args: []*clientmodel.MetricFamily{d, d}, want: []*clientmodel.MetricFamily{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Pack(tt.args); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Pack() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPackMetrics(t *testing.T) {
	tests := []struct {
		name    string
		args    *clientmodel.MetricFamily
		want    *clientmodel.MetricFamily
		wantOk  bool
		wantErr bool
	}{
		{name: "empty", args: &clientmodel.MetricFamily{}, want: &clientmodel.MetricFamily{}},
		{
			name: "all nil",
			args: &clientmodel.MetricFamily{Metric: []*clientmodel.Metric{nil, nil}},
			want: &clientmodel.MetricFamily{Metric: []*clientmodel.Metric{}},
		},
		{
			name:   "leading nil",
			args:   &clientmodel.MetricFamily{Metric: []*clientmodel.Metric{nil, metric(1)}},
			want:   &clientmodel.MetricFamily{Metric: []*clientmodel.Metric{metric(1)}},
			wantOk: true,
		},
		{
			name:   "trailing nil",
			args:   &clientmodel.MetricFamily{Metric: []*clientmodel.Metric{metric(1), nil}},
			want:   &clientmodel.MetricFamily{Metric: []*clientmodel.Metric{metric(1)}},
			wantOk: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := PackMetrics(tt.args)
			if got != tt.wantOk {
				t.Errorf("PackMetrics() = %t, want %t", got, tt.wantOk)
			}
			if (gotErr != nil) != tt.wantErr {
				t.Errorf("PackMetrics() = %v, want %t", gotErr, tt.wantErr)
			}
			if !reflect.DeepEqual(tt.args, tt.want) {
				t.Errorf("PackMetrics() = %v, want %v", tt.args, tt.want)
			}
		})
	}
}

func TestMergeSort(t *testing.T) {
	tests := []struct {
		name string
		args []*clientmodel.MetricFamily
		want []*clientmodel.MetricFamily
	}{
		{name: "empty", args: []*clientmodel.MetricFamily{}, want: []*clientmodel.MetricFamily{}},
		{name: "single", args: []*clientmodel.MetricFamily{family("A", 1)}, want: []*clientmodel.MetricFamily{family("A", 1)}},
		{name: "merge", args: []*clientmodel.MetricFamily{family("A", 1), family("A", 2)}, want: []*clientmodel.MetricFamily{family("A", 1, 2)}},
		{name: "reverse merge", args: []*clientmodel.MetricFamily{family("A", 2), family("A", 1)}, want: []*clientmodel.MetricFamily{family("A", 1, 2)}},
		{name: "differ", args: []*clientmodel.MetricFamily{family("A", 2), family("B", 1)}, want: []*clientmodel.MetricFamily{family("A", 2), family("B", 1)}},
		{name: "zip merge", args: []*clientmodel.MetricFamily{family("A", 2, 4, 6), family("A", 1, 3, 5)}, want: []*clientmodel.MetricFamily{family("A", 1, 2, 3, 4, 5, 6)}},
		{name: "zip merge - dst longer", args: []*clientmodel.MetricFamily{family("A", 2, 4, 6), family("A", 3)}, want: []*clientmodel.MetricFamily{family("A", 2, 3, 4, 6)}},
		{name: "zip merge - src longer", args: []*clientmodel.MetricFamily{family("A", 4), family("A", 1, 3, 5)}, want: []*clientmodel.MetricFamily{family("A", 1, 3, 4, 5)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MergeSortedWithTimestamps(tt.args); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MergeSortedWithTimestamps() = %v, want %v", got, tt.want)
			}
		})
	}

}
