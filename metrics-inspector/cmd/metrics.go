// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stolostron/multicluster-observability-operator/metrics-inspector/pkg/estimator"
	"github.com/stolostron/multicluster-observability-operator/metrics-inspector/pkg/remotewrite"
)

type serverMetrics struct {
	writeRequestsTotal atomic.Uint64
	writeErrorsTotal   atomic.Uint64
	shuttingDown       atomic.Bool
}

var (
	descWriteRequestsTotal = prometheus.NewDesc(
		"cardinality_estimator_write_requests_total",
		"Total number of Remote Write HTTP requests received.",
		nil, nil,
	)
	descWriteErrorsTotal = prometheus.NewDesc(
		"cardinality_estimator_write_errors_total",
		"Total number of Remote Write HTTP requests that failed parsing.",
		nil, nil,
	)
	descIngestedSeriesTotal = prometheus.NewDesc(
		"cardinality_estimator_ingested_series_total",
		"Total number of series observations ingested across all streams.",
		nil, nil,
	)
	descDeduplicatedSeriesTotal = prometheus.NewDesc(
		"cardinality_estimator_deduplicated_series_total",
		"Total number of series observations skipped via fast-path deduplication.",
		nil, nil,
	)
	descPushedSeriesTotal = prometheus.NewDesc(
		"cardinality_estimator_pushed_series_total",
		"Total number of synthetic cardinality series pushed via Remote Write.",
		nil, nil,
	)
	descPushSuccessTotal = prometheus.NewDesc(
		"cardinality_estimator_push_success_total",
		"Total number of successful Remote Write cardinality report flushes.",
		nil, nil,
	)
	descPushErrorsTotal = prometheus.NewDesc(
		"cardinality_estimator_push_errors_total",
		"Total number of failed Remote Write cardinality report flushes.",
		nil, nil,
	)
	descLastPushTimestampSecs = prometheus.NewDesc(
		"cardinality_estimator_last_push_timestamp_seconds",
		"Unix timestamp in seconds of the last successful Remote Write cardinality report flush.",
		nil, nil,
	)
)

type operationalCollector struct {
	eng    *estimator.Engine
	pusher *remotewrite.Pusher
	m      *serverMetrics
}

func (c operationalCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- descWriteRequestsTotal
	ch <- descWriteErrorsTotal
	ch <- descIngestedSeriesTotal
	ch <- descDeduplicatedSeriesTotal
	ch <- descPushedSeriesTotal
	ch <- descPushSuccessTotal
	ch <- descPushErrorsTotal
	ch <- descLastPushTimestampSecs
}

func (c operationalCollector) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(descWriteRequestsTotal, prometheus.CounterValue, float64(c.m.writeRequestsTotal.Load()))
	ch <- prometheus.MustNewConstMetric(descWriteErrorsTotal, prometheus.CounterValue, float64(c.m.writeErrorsTotal.Load()))
	ch <- prometheus.MustNewConstMetric(descIngestedSeriesTotal, prometheus.CounterValue, float64(c.eng.IngestedSeriesTotal()))
	ch <- prometheus.MustNewConstMetric(descDeduplicatedSeriesTotal, prometheus.CounterValue, float64(c.eng.DeduplicatedSeriesTotal()))
	ch <- prometheus.MustNewConstMetric(descPushedSeriesTotal, prometheus.CounterValue, float64(c.pusher.PushedSeriesTotal()))
	ch <- prometheus.MustNewConstMetric(descPushSuccessTotal, prometheus.CounterValue, float64(c.pusher.PushSuccessTotal()))
	ch <- prometheus.MustNewConstMetric(descPushErrorsTotal, prometheus.CounterValue, float64(c.pusher.PushErrorsTotal()))
	ch <- prometheus.MustNewConstMetric(descLastPushTimestampSecs, prometheus.GaugeValue, float64(c.pusher.LastPushTimestampSeconds()))
}
