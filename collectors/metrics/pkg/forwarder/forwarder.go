// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package forwarder

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	clientmodel "github.com/prometheus/client_model/go"

	metricshttp "github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/http"
	rlogger "github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/logger"
	"github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/metricfamily"
	"github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/metricsclient"
	"github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/simulator"
	"github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/status"
	statuslib "github.com/stolostron/multicluster-observability-operator/operators/pkg/status"
)

const (
	failedStatusReportMsg = "Failed to report status"
	uwlPromURL            = "https://prometheus-user-workload.openshift-user-workload-monitoring.svc:9092"
)

type RuleMatcher interface {
	MatchRules() []string
}

// Config defines the parameters that can be used to configure a worker.
// The only required field is `From`.
type Config struct {
	From          *url.URL
	FromQuery     *url.URL
	ToUpload      *url.URL
	FromToken     string
	FromTokenFile string
	FromCAFile    string
	ToUploadCA    string
	ToUploadCert  string
	ToUploadKey   string

	AnonymizeLabels    []string
	AnonymizeSalt      string
	AnonymizeSaltFile  string
	Debug              bool
	Interval           time.Duration
	EvaluateInterval   time.Duration
	LimitBytes         int64
	Rules              []string
	RulesFile          string
	RecordingRules     []string
	RecordingRulesFile string
	CollectRules       []string
	CollectRulesFile   string
	Transformer        metricfamily.Transformer

	Logger                  log.Logger
	SimulatedTimeseriesFile string

	Metrics *workerMetrics
}

// Worker represents a metrics forwarding agent. It collects metrics from a source URL and forwards them to a sink.
// A Worker should be configured with a `Config` and instantiated with the `New` func.
// Workers are thread safe; all access to shared fields are synchronized.
type Worker struct {
	fromClient *metricsclient.Client
	toClient   *metricsclient.Client
	from       *url.URL
	fromQuery  *url.URL
	to         *url.URL

	interval       time.Duration
	transformer    metricfamily.Transformer
	rules          []string
	recordingRules []string

	lastMetrics []*clientmodel.MetricFamily
	lock        sync.Mutex
	reconfigure chan struct{}

	logger log.Logger

	simulatedTimeseriesFile string

	status status.StatusReport

	metrics *workerMetrics
}

func CreateFromClient(cfg Config, metrics *workerMetrics, interval time.Duration, name string,
	logger log.Logger) (*metricsclient.Client, error) {
	fromTransport := metricsclient.DefaultTransport(logger, false)
	if len(cfg.FromCAFile) > 0 {
		if fromTransport.TLSClientConfig == nil {
			fromTransport.TLSClientConfig = &tls.Config{
				MinVersion: tls.VersionTLS12,
			}
		}
		pool, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("failed to read system certificates: %w", err)
		}
		data, err := os.ReadFile(cfg.FromCAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read from-ca-file: %w", err)
		}
		if !pool.AppendCertsFromPEM(data) {
			rlogger.Log(logger, rlogger.Warn, "msg", "no certs found in from-ca-file")
		}
		fromTransport.TLSClientConfig.RootCAs = pool
	} else {
		if fromTransport.TLSClientConfig == nil {
			// #nosec G402 -- Only used if no TLS config is provided.
			fromTransport.TLSClientConfig = &tls.Config{
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: true,
			}
		}
	}

	// Create the `fromClient`.
	fromClient := &http.Client{Transport: fromTransport}
	if cfg.Debug {
		fromClient.Transport = metricshttp.NewDebugRoundTripper(logger, fromClient.Transport)
	}
	if len(cfg.FromToken) == 0 && len(cfg.FromTokenFile) > 0 {
		data, err := os.ReadFile(cfg.FromTokenFile)
		if err != nil {
			return nil, fmt.Errorf("unable to read from-token-file: %w", err)
		}
		cfg.FromToken = strings.TrimSpace(string(data))
	}
	if len(cfg.FromToken) > 0 {
		fromClient.Transport = metricshttp.NewBearerRoundTripper(cfg.FromToken, fromClient.Transport)
	}

	from := metricsclient.New(logger, metrics.clientMetrics, fromClient, cfg.LimitBytes, interval, "federate_from")

	return from, nil
}

func createClients(cfg Config, metrics *metricsclient.ClientMetrics, interval time.Duration,
	logger log.Logger) (*metricsclient.Client, *metricsclient.Client, metricfamily.MultiTransformer, error) {

	var transformer metricfamily.MultiTransformer

	// Configure the anonymization.
	anonymizeSalt := cfg.AnonymizeSalt
	if len(cfg.AnonymizeSalt) == 0 && len(cfg.AnonymizeSaltFile) > 0 {
		data, err := os.ReadFile(cfg.AnonymizeSaltFile)
		if err != nil {
			return nil, nil, transformer, fmt.Errorf("failed to read anonymize-salt-file: %w", err)
		}
		anonymizeSalt = strings.TrimSpace(string(data))
	}
	if len(cfg.AnonymizeLabels) != 0 && len(anonymizeSalt) == 0 {
		return nil, nil, transformer, errors.New("anonymize-salt must be specified if anonymize-labels is set")
	}
	if len(cfg.AnonymizeLabels) == 0 {
		rlogger.Log(logger, rlogger.Warn, "msg", "not anonymizing any labels")
	}

	// Configure a transformer.
	if cfg.Transformer != nil {
		transformer.With(cfg.Transformer)
	}
	if len(cfg.AnonymizeLabels) > 0 {
		transformer.With(metricfamily.NewMetricsAnonymizer(anonymizeSalt, cfg.AnonymizeLabels, nil))
	}
	from, err := CreateFromClient(cfg, cfg.Metrics, interval, "federate_from", logger)
	if err != nil {
		return nil, nil, transformer, err
	}

	// Create the `toClient`.

	toTransport, err := metricsclient.MTLSTransport(logger, cfg.ToUploadCA, cfg.ToUploadCert, cfg.ToUploadKey)
	if err != nil {
		return nil, nil, transformer, fmt.Errorf("failed to create TLS transport: %w", err)
	}
	toTransport.Proxy = http.ProxyFromEnvironment
	toClient := &http.Client{Transport: toTransport}
	if cfg.Debug {
		toClient.Transport = metricshttp.NewDebugRoundTripper(logger, toClient.Transport)
	}
	to := metricsclient.New(logger, metrics, toClient, cfg.LimitBytes, interval, "federate_to")
	return from, to, transformer, nil
}

type workerMetrics struct {
	gaugeFederateSamples         prometheus.Gauge
	gaugeFederateFilteredSamples prometheus.Gauge

	clientMetrics *metricsclient.ClientMetrics
}

func NewWorkerMetrics(reg *prometheus.Registry) *workerMetrics {
	return &workerMetrics{
		gaugeFederateSamples: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "federate_samples",
			Help: "Tracks the number of samples per federation",
		}),
		gaugeFederateFilteredSamples: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "federate_filtered_samples",
			Help: "Tracks the number of samples filtered per federation",
		}),

		clientMetrics: &metricsclient.ClientMetrics{
			FederateRequests: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
				Name: "federate_requests_total",
				Help: "The number of times federating metrics",
			}, []string{"type", "status_code"}),

			ForwardRemoteWriteRequests: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
				Name: "forward_write_requests_total",
				Help: "Counter of forward remote write requests.",
			}, []string{"status_code"}),
		},
	}
}

// New creates a new Worker based on the provided Config. If the Config contains invalid
// values, then an error is returned.
func New(cfg Config) (*Worker, error) {
	if cfg.From == nil {
		return nil, errors.New("a URL from which to scrape is required")
	}
	logger := log.With(cfg.Logger, "component", "forwarder")
	rlogger.Log(logger, rlogger.Warn, "msg", cfg.ToUpload)
	w := Worker{
		from:                    cfg.From,
		fromQuery:               cfg.FromQuery,
		interval:                cfg.Interval,
		reconfigure:             make(chan struct{}),
		to:                      cfg.ToUpload,
		logger:                  log.With(cfg.Logger, "component", "forwarder/worker"),
		simulatedTimeseriesFile: cfg.SimulatedTimeseriesFile,
		metrics:                 cfg.Metrics,
	}

	if w.interval == 0 {
		w.interval = 4*time.Minute + 30*time.Second
	}

	fromClient, toClient, transformer, err := createClients(cfg, w.metrics.clientMetrics, w.interval, logger)
	if err != nil {
		return nil, err
	}
	w.fromClient = fromClient
	w.toClient = toClient
	w.transformer = transformer

	// Configure the matching rules.
	rules := cfg.Rules
	if len(cfg.RulesFile) > 0 {
		data, err := os.ReadFile(cfg.RulesFile)
		if err != nil {
			return nil, fmt.Errorf("unable to read match-file: %w", err)
		}
		rules = append(rules, strings.Split(string(data), "\n")...)
	}
	for i := 0; i < len(rules); {
		s := strings.TrimSpace(rules[i])
		if len(s) == 0 {
			rules = append(rules[:i], rules[i+1:]...)
			continue
		}
		rules[i] = s
		i++
	}
	w.rules = rules

	// Configure the recording rules.
	recordingRules := cfg.RecordingRules
	for i := 0; i < len(recordingRules); {
		s := strings.TrimSpace(recordingRules[i])
		if len(s) == 0 {
			recordingRules = append(recordingRules[:i], recordingRules[i+1:]...)
			continue
		}
		recordingRules[i] = s
		i++
	}
	w.recordingRules = recordingRules

	standalone := os.Getenv("STANDALONE") == "true"
	isUwl := strings.Contains(os.Getenv("FROM"), uwlPromURL)
	s, err := status.New(logger, standalone, isUwl)
	if err != nil {
		return nil, fmt.Errorf("unable to create StatusReport: %w", err)
	}
	w.status = *s

	return &w, nil
}

// Reconfigure temporarily stops a worker and reconfigures is with the provided Config.
// Is thread safe and can run concurrently with `LastMetrics` and `Run`.
func (w *Worker) Reconfigure(cfg Config) error {
	worker, err := New(cfg)
	if err != nil {
		return fmt.Errorf("failed to reconfigure: %w", err)
	}

	w.lock.Lock()
	defer w.lock.Unlock()

	w.fromClient = worker.fromClient
	w.toClient = worker.toClient
	w.interval = worker.interval
	w.from = worker.from
	w.to = worker.to
	w.transformer = worker.transformer
	w.rules = worker.rules
	w.recordingRules = worker.recordingRules

	// Signal a restart to Run func.
	// Do this in a goroutine since we do not care if restarting the Run loop is asynchronous.
	go func() { w.reconfigure <- struct{}{} }()
	return nil
}

func (w *Worker) LastMetrics() []*clientmodel.MetricFamily {
	w.lock.Lock()
	defer w.lock.Unlock()
	return w.lastMetrics
}

func (w *Worker) Run(ctx context.Context) {
	for {
		// Ensure that the Worker does not access critical configuration during a reconfiguration.
		w.lock.Lock()
		wait := w.interval
		// The critical section ends here.
		w.lock.Unlock()

		if err := w.forward(ctx); err != nil {
			rlogger.Log(w.logger, rlogger.Error, "msg", "unable to forward results", "err", err)
			wait = time.Minute
		}

		select {
		// If the context is canceled, then we're done.
		case <-ctx.Done():
			return
		case <-time.After(wait):
		// We want to be able to interrupt a sleep to immediately apply a new configuration.
		case <-w.reconfigure:
		}
	}
}

func (w *Worker) forward(ctx context.Context) error {
	w.lock.Lock()
	defer w.lock.Unlock()

	updateStatus := func(reason statuslib.Reason, message string) {
		if err := w.status.UpdateStatus(ctx, reason, message); err != nil {
			rlogger.Log(w.logger, rlogger.Warn, "msg", failedStatusReportMsg, "err", err)
		}
	}

	var families []*clientmodel.MetricFamily
	var err error
	if w.simulatedTimeseriesFile != "" {
		families, err = simulator.FetchSimulatedTimeseries(w.simulatedTimeseriesFile)
		if err != nil {
			rlogger.Log(w.logger, rlogger.Warn, "msg", "failed fetch simulated timeseries", "err", err)
		}
	} else if os.Getenv("SIMULATE") == "true" {
		families = simulator.SimulateMetrics(w.logger)
	} else {
		families, err = w.getFederateMetrics(ctx)
		if err != nil {
			updateStatus(statuslib.ForwardFailed, "Failed to retrieve metrics")
			return err
		}

		rfamilies, err := w.getRecordingMetrics(ctx)
		if err != nil && len(rfamilies) == 0 {
			updateStatus(statuslib.ForwardFailed, "Failed to retrieve recording metrics")
			return err
		} else {
			families = append(families, rfamilies...)
		}
	}

	before := metricfamily.MetricsCount(families)
	if err := metricfamily.Filter(families, w.transformer); err != nil {
		updateStatus(statuslib.ForwardFailed, "Failed to filter metrics")
		return err
	}

	families = metricfamily.Pack(families)
	after := metricfamily.MetricsCount(families)

	w.metrics.gaugeFederateSamples.Set(float64(before))
	w.metrics.gaugeFederateFilteredSamples.Set(float64(before - after))

	w.lastMetrics = families

	if len(families) == 0 {
		rlogger.Log(w.logger, rlogger.Warn, "msg", "no metrics to send, doing nothing")
		updateStatus(statuslib.ForwardSuccessful, "No metrics to send")
		return nil
	}

	if w.to == nil {
		rlogger.Log(w.logger, rlogger.Warn, "msg", "to is nil, doing nothing")
		updateStatus(statuslib.ForwardSuccessful, "Metrics is not required to send")
		return nil
	}

	req := &http.Request{Method: "POST", URL: w.to}
	if err := w.toClient.RemoteWrite(ctx, req, families, w.interval); err != nil {
		updateStatus(statuslib.ForwardFailed, "Failed to send metrics")
		return err
	}

	if w.simulatedTimeseriesFile == "" {
		updateStatus(statuslib.ForwardSuccessful, "Cluster metrics sent successfully")
	} else {
		rlogger.Log(w.logger, rlogger.Warn, "msg", "Simulated metrics sent successfully")
	}

	return nil
}

func (w *Worker) getFederateMetrics(ctx context.Context) ([]*clientmodel.MetricFamily, error) {
	var families []*clientmodel.MetricFamily
	var err error

	// reset query from last invocation, otherwise match rules will be appended
	from := w.from
	from.RawQuery = ""
	v := from.Query()
	for _, rule := range w.rules {
		v.Add("match[]", rule)
	}
	from.RawQuery = v.Encode()

	req := &http.Request{Method: "GET", URL: from}
	families, err = w.fromClient.Retrieve(ctx, req)
	if err != nil {
		rlogger.Log(w.logger, rlogger.Warn, "msg", "Failed to retrieve metrics", "err", err)
		return families, err
	}

	return families, nil
}

func (w *Worker) getRecordingMetrics(ctx context.Context) ([]*clientmodel.MetricFamily, error) {
	var families []*clientmodel.MetricFamily
	var e error

	from := w.fromQuery

	for _, rule := range w.recordingRules {
		var r map[string]string
		err := json.Unmarshal(([]byte)(rule), &r)
		if err != nil {
			rlogger.Log(w.logger, rlogger.Warn, "msg", "Input error", "rule", rule, "err", err)
			e = err
			continue
		}
		rname := r["name"]
		rquery := r["query"]

		// reset query from last invocation, otherwise match rules will be appended
		from.RawQuery = ""
		v := w.fromQuery.Query()
		v.Add("query", rquery)
		from.RawQuery = v.Encode()

		req := &http.Request{Method: "GET", URL: from}
		rfamilies, err := w.fromClient.RetrievRecordingMetrics(ctx, req, rname)
		if err != nil {
			rlogger.Log(w.logger, rlogger.Warn, "msg", "Failed to retrieve recording metrics", "err", err, "url", from)
			e = err
			continue
		} else {
			families = append(families, rfamilies...)
		}
	}

	return families, e
}
