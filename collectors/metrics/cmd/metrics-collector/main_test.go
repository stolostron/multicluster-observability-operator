// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	stdlog "log"
	"os"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/forwarder"
	"github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/logger"
)

func init() {
	os.Setenv("UNIT_TEST", "true")
}

func TestMultiWorkers(t *testing.T) {

	opt := &Options{
		Listen:                  "localhost:9002",
		LimitBytes:              200 * 1024,
		Rules:                   []string{`{__name__="instance:node_vmstat_pgmajfault:rate1m"}`},
		Interval:                4*time.Minute + 30*time.Second,
		WorkerNum:               2,
		SimulatedTimeseriesFile: "../../testdata/timeseries.txt",
		From:                    "https://prometheus-k8s.openshift-monitoring.svc:9091",
		ToUpload:                "https://prometheus-k8s.openshift-monitoring.svc:9091",
		LabelFlag: []string{
			"cluster=local-cluster",
			"clusterID=245c2253-7b0d-4080-8e33-f6f0d6c6ff73",
		},
		FromCAFile:    "../../testdata/service-ca.crt",
		FromTokenFile: "../../testdata/token",
	}

	l := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	l = level.NewFilter(l, logger.LogLevelFromString("debug"))
	l = log.WithPrefix(l, "ts", log.DefaultTimestampUTC)
	l = log.WithPrefix(l, "caller", log.DefaultCaller)
	stdlog.SetOutput(log.NewStdlibAdapter(l))
	opt.Logger = l

	err := runMultiWorkers(opt, &forwarder.Config{Metrics: forwarder.NewWorkerMetrics(prometheus.NewRegistry())})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1 * time.Second)

}
