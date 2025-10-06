// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"context"
	"fmt"
	stdlog "log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gogo/protobuf/proto"
	"github.com/prometheus/client_golang/prometheus"
	clientmodel "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/forwarder"
	"github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/logger"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
)

func TestMultiWorkers(t *testing.T) {
	opt := &Options{
		Listen:                  "localhost:9002",
		LimitBytes:              200 * 1024,
		Matchers:                []string{`{__name__="instance:node_vmstat_pgmajfault:rate1m"}`},
		Interval:                4*time.Minute + 30*time.Second,
		WorkerNum:               2,
		SimulatedTimeseriesFile: "../../testdata/timeseries.txt",
		From:                    "https://prometheus-k8s.openshift-monitoring.svc:9091",
		ToUpload:                "https://prometheus-k8s.openshift-monitoring.svc:9091",
		LabelFlag: []string{
			"cluster=local-cluster",
			"clusterID=245c2253-7b0d-4080-8e33-f6f0d6c6ff73",
		},
		DisableHyperShift:      true,
		DisableStatusReporting: true,
	}

	l := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	l = level.NewFilter(l, logger.LogLevelFromString("debug"))
	l = log.WithPrefix(l, "ts", log.DefaultTimestampUTC)
	l = log.WithPrefix(l, "caller", log.DefaultCaller)
	stdlog.SetOutput(log.NewStdlibAdapter(l))
	opt.Logger = l

	sc := scheme.Scheme
	if err := oav1beta1.AddToScheme(sc); err != nil {
		t.Fatal("failed to add observabilityaddon into scheme")
	}
	kubeClient := fake.NewClientBuilder().
		WithScheme(sc).
		WithStatusSubresource(&oav1beta1.ObservabilityAddon{}).
		Build()

	err := runMultiWorkers(opt, &forwarder.Config{
		Metrics:      forwarder.NewWorkerMetrics(prometheus.NewRegistry()),
		StatusClient: kubeClient,
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1 * time.Second)

}

func TestMultiWorkersRaceCondition(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		assert.NoError(t, err)
		matchers := r.Form["match[]"]

		w.Header().Set("Content-Type", string(expfmt.FmtProtoDelim))
		encoder := expfmt.NewEncoder(w, expfmt.FmtProtoDelim)

		for _, m := range matchers {
			if !strings.HasPrefix(m, `{__name__="`) || !strings.HasSuffix(m, `"}`) {
				t.Logf("Skipping malformed matcher: %s", m)
				continue
			}
			metricName := strings.TrimPrefix(m, `{__name__="`)
			metricName = strings.TrimSuffix(metricName, `"}`)

			metricFamily := &clientmodel.MetricFamily{
				Name: proto.String(metricName),
				Type: clientmodel.MetricType_GAUGE.Enum(),
				Metric: []*clientmodel.Metric{
					{
						Label: []*clientmodel.LabelPair{},
						Gauge: &clientmodel.Gauge{
							Value: proto.Float64(1.0),
						},
						TimestampMs: proto.Int64(time.Now().UnixMilli()),
					},
				},
			}
			err = encoder.Encode(metricFamily)
			assert.NoError(t, err)
		}
	}))
	defer server.Close()

	opt := &Options{
		Listen:     "localhost:9003",
		LimitBytes: 200 * 1024,
		Matchers: []string{
			`{__name__="test0"}`,
			`{__name__="test1"}`,
			`{__name__="test2"}`,
			`{__name__="test3"}`,
			`{__name__="test4"}`,
			`{__name__="test5"}`,
		},
		Interval:  100 * time.Millisecond,
		WorkerNum: 5,
		From:      server.URL,
		ToUpload:  server.URL,
		LabelFlag: []string{
			"cluster=local-cluster",
			"clusterID=245c2253-7b0d-4080-8e33-f6f0d6c6ff73",
		},
		DisableHyperShift:      true,
		DisableStatusReporting: true,
	}

	l := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	l = level.NewFilter(l, logger.LogLevelFromString("debug"))
	l = log.WithPrefix(l, "ts", log.DefaultTimestampUTC)
	l = log.WithPrefix(l, "caller", log.DefaultCaller)
	stdlog.SetOutput(log.NewStdlibAdapter(l))
	opt.Logger = l

	cfgs, err := initShardedConfigs(opt, AgentShardedForwarder)
	assert.NoError(t, err)
	assert.Equal(t, 5, len(cfgs))

	metrics := forwarder.NewWorkerMetrics(prometheus.NewRegistry())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, cfg := range cfgs {
		cfg.Metrics = metrics
		worker, err := forwarder.New(*cfg)
		assert.NoError(t, err)
		go worker.Run(ctx)
	}

	time.Sleep(2 * time.Second)
}

func TestSplitMatchersIntoShards(t *testing.T) {
	tests := []struct {
		name       string
		matchers   []string
		shardCount int
		want       [][]string
	}{
		{
			name:       "single shard",
			matchers:   []string{"match1", "match2", "match3"},
			shardCount: 1,
			want:       [][]string{{"match1", "match2", "match3"}},
		},
		{
			name:       "two shards",
			matchers:   []string{"match1", "match2", "match3", "match4"},
			shardCount: 2,
			want: [][]string{
				{"match1", "match3"},
				{"match2", "match4"},
			},
		},
		// This case should not happen and is restricted by CLI option validation.
		{
			name:       "two shards",
			matchers:   []string{"match1", "match2", "match3", "match4"},
			shardCount: 6,
			want: [][]string{
				{"match1"},
				{"match2"},
				{"match3"},
				{"match4"},
				{},
				{},
			},
		},
		{
			name:       "three shards",
			matchers:   []string{"match1", "match2", "match3", "match4", "match5"},
			shardCount: 3,
			want: [][]string{
				{"match1", "match4"},
				{"match2", "match5"},
				{"match3"},
			},
		},
		{
			name:       "more shards than matchers",
			matchers:   []string{"match1", "match2"},
			shardCount: 3,
			want: [][]string{
				{"match1"},
				{"match2"},
				{},
			},
		},
		{
			name:       "zero shards",
			matchers:   []string{"match1", "match2", "match3"},
			shardCount: 0,
			want:       [][]string{{"match1", "match2", "match3"}},
		},
		{
			name:       "negative shards",
			matchers:   []string{"match1", "match2", "match3"},
			shardCount: -1,
			want:       [][]string{{"match1", "match2", "match3"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitMatchersIntoShards(tt.matchers, tt.shardCount)
			fmt.Println(got)
			assert.Equal(t, len(tt.want), len(got))

			for i := range got {
				assert.Equal(t, len(tt.want[i]), len(got[i]))
				for j := 0; j < len(got[i]); j++ {
					assert.Equal(t, tt.want[i][j], got[i][j])
				}
			}
		})
	}
}
