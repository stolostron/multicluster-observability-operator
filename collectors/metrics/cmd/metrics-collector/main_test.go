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
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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
