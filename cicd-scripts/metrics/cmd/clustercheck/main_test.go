// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

type mockAPI struct {
	v1.API
	queryFunc func(ctx context.Context, query string, ts time.Time, opts ...v1.Option) (model.Value, v1.Warnings, error)
}

func (m *mockAPI) Query(ctx context.Context, query string, ts time.Time, opts ...v1.Option) (model.Value, v1.Warnings, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, query, ts, opts...)
	}
	return nil, nil, nil
}

func Test_getManagedClusters(t *testing.T) {
	tests := []struct {
		name      string
		regex     string
		mockValue model.Value
		mockErr   error
		want      []string
		wantErr   bool
		wantQuery string
	}{
		{
			name:  "success",
			regex: "",
			mockValue: model.Vector{
				&model.Sample{
					Metric: model.Metric{"name": "cluster1"},
				},
				&model.Sample{
					Metric: model.Metric{"name": "cluster2"},
				},
			},
			want:      []string{"cluster1", "cluster2"},
			wantErr:   false,
			wantQuery: "group by (name) (acm_managed_cluster_labels)",
		},
		{
			name:      "empty result",
			regex:     "",
			mockValue: model.Vector{},
			want:      []string(nil),
			wantErr:   false,
			wantQuery: "group by (name) (acm_managed_cluster_labels)",
		},
		{
			name:  "with regex",
			regex: "cluster1.*",
			mockValue: model.Vector{
				&model.Sample{
					Metric: model.Metric{"name": "cluster1-dev"},
				},
			},
			want:      []string{"cluster1-dev"},
			wantErr:   false,
			wantQuery: "group by (name) (acm_managed_cluster_labels{name=~\"cluster1.*\"})",
		},
		{
			name:      "query error",
			regex:     "",
			mockErr:   fmt.Errorf("connection refused"),
			mockValue: nil,
			want:      nil,
			wantErr:   true,
			wantQuery: "group by (name) (acm_managed_cluster_labels)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedQuery string
			api := &mockAPI{
				queryFunc: func(ctx context.Context, query string, ts time.Time, opts ...v1.Option) (model.Value, v1.Warnings, error) {
					capturedQuery = query
					return tt.mockValue, nil, tt.mockErr
				},
			}
			got, err := getManagedClusters(context.Background(), api, tt.regex)
			if (err != nil) != tt.wantErr {
				t.Errorf("getManagedClusters() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getManagedClusters() = %v, want %v", got, tt.want)
			}
			if capturedQuery != tt.wantQuery {
				t.Errorf("getManagedClusters() query = %v, want %v", capturedQuery, tt.wantQuery)
			}
		})
	}
}

func Test_fetchMetricAvailability(t *testing.T) {
	clusters := []string{"cluster1", "cluster2"}
	collectedMetrics := []string{"metric1", "metric2", "metric3"}

	tests := []struct {
		name         string
		chunkSize    int
		mockValue    model.Value
		want         map[string]map[string]bool
		wantErr      bool
		checkQueries func(t *testing.T, queries []string)
	}{
		{
			name:      "all metrics present",
			chunkSize: 50,
			mockValue: model.Vector{
				&model.Sample{Metric: model.Metric{"cluster": "cluster1", "__name__": "metric1"}},
				&model.Sample{Metric: model.Metric{"cluster": "cluster1", "__name__": "metric2"}},
				&model.Sample{Metric: model.Metric{"cluster": "cluster1", "__name__": "metric3"}},
				&model.Sample{Metric: model.Metric{"cluster": "cluster2", "__name__": "metric1"}},
				&model.Sample{Metric: model.Metric{"cluster": "cluster2", "__name__": "metric2"}},
				&model.Sample{Metric: model.Metric{"cluster": "cluster2", "__name__": "metric3"}},
			},
			want: map[string]map[string]bool{
				"cluster1": {"metric1": true, "metric2": true, "metric3": true},
				"cluster2": {"metric1": true, "metric2": true, "metric3": true},
			},
			wantErr: false,
			checkQueries: func(t *testing.T, queries []string) {
				if len(queries) != 1 {
					t.Errorf("Expected 1 query, got %d", len(queries))
				}
				if !strings.Contains(queries[0], "metric1|metric2|metric3") {
					t.Errorf("Query doesn't contain all metrics: %s", queries[0])
				}
			},
		},
		{
			name:      "some metrics missing",
			chunkSize: 50,
			mockValue: model.Vector{
				&model.Sample{Metric: model.Metric{"cluster": "cluster1", "__name__": "metric1"}},
				&model.Sample{Metric: model.Metric{"cluster": "cluster2", "__name__": "metric3"}},
			},
			want: map[string]map[string]bool{
				"cluster1": {"metric1": true},
				"cluster2": {"metric3": true},
			},
			wantErr: false,
		},
		{
			name:      "chunking works",
			chunkSize: 2,
			mockValue: model.Vector{
				&model.Sample{Metric: model.Metric{"cluster": "cluster1", "__name__": "metric1"}},
			},
			want: map[string]map[string]bool{
				"cluster1": {"metric1": true},
				"cluster2": {},
			},
			wantErr: false,
			checkQueries: func(t *testing.T, queries []string) {
				if len(queries) != 2 {
					t.Errorf("Expected 2 queries, got %d", len(queries))
				}
				if !strings.Contains(queries[0], "metric1|metric2") {
					t.Errorf("First query doesn't contain correct metrics: %s", queries[0])
				}
				if !strings.Contains(queries[1], "metric3") {
					t.Errorf("Second query doesn't contain correct metrics: %s", queries[1])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedQueries []string
			api := &mockAPI{
				queryFunc: func(ctx context.Context, query string, ts time.Time, opts ...v1.Option) (model.Value, v1.Warnings, error) {
					capturedQueries = append(capturedQueries, query)
					return tt.mockValue, nil, nil
				},
			}
			got, err := fetchMetricAvailability(context.Background(), api, clusters, collectedMetrics, tt.chunkSize)
			if (err != nil) != tt.wantErr {
				t.Errorf("fetchMetricAvailability() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("fetchMetricAvailability() = %v, want %v", got, tt.want)
			}
			if tt.checkQueries != nil {
				tt.checkQueries(t, capturedQueries)
			}
		})
	}
}
