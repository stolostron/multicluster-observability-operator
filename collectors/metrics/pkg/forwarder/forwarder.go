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
	"sigs.k8s.io/controller-runtime/pkg/client"

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

	matchParam = "match[]"
	queryParam = "query"
)

// Config defines the parameters that can be used to configure a worker.
type Config struct {
	// StatusClient is a kube client used to report status to the hub.
	StatusClient client.Client
	Logger       log.Logger
	Metrics      *workerMetrics

	// FromClientConfig is the config for the client used in sending /federate requests to Prometheus.
	FromClientConfig FromClientConfig
	// ToClientConfig is the config for the client used in sending remote write requests to Thanos Receive.
	ToClientConfig ToClientConfig
	// Enable debug roundtrippers for from and to clients.
	Debug bool
	// LimitBytes limits the size of the response read from requests made to from and to clients.
	LimitBytes int64

	// Interval is the interval at which workers will federate Prometheus and send remote write requests.
	// 4m30s by default
	Interval time.Duration
	// EvaluateInterval is actually used to configure collectrule evaluator in collectrule/evaluator.go.
	EvaluateInterval time.Duration

	// Fields for anonymizing metrics.
	AnonymizeLabels   []string
	AnonymizeSalt     string
	AnonymizeSaltFile string

	// Matchers is the list of matchers to use for filtering metrics, they are appended to URL during /federate calls.
	Matchers []string
	// RecordingRules is the list of recording rules to evaluate and send as a new series in remote write.
	// TODO(saswatamcode): Kill this feature.
	RecordingRules []string
	// CollectRules are unique rules, that basically add matchers, based on some PromQL rule.
	// They are used to collect additional metrics when things are going wrong.
	// TODO(saswatamcode): Do this some place else or re-evaluate if we even need this.
	CollectRules []string
	// SimulateTimeseriesFile provides configuration for sending simulated data. Used in perfscale tests?
	// TODO(saswatamcode): Kill this feature, simulation testing logic should not be included in business logic.
	SimulatedTimeseriesFile string

	// Transformer is used to transform metrics before sending them to Thanos Receive.
	// We pass in transformers for eliding labels, hypershift etc.
	Transformer metricfamily.Transformer
}

type FromClientConfig struct {
	URL       *url.URL
	QueryURL  *url.URL
	CAFile    string
	Token     string
	TokenFile string
}

type ToClientConfig struct {
	URL      *url.URL
	CAFile   string
	CertFile string
	KeyFile  string
}

// CreateFromClient creates a new metrics client for the from URL.
// Needs to be exported here so that it can be used in collectrule evaluator.
func (cfg Config) CreateFromClient(
	metrics *workerMetrics,
	interval time.Duration,
	name string,
	logger log.Logger,
) (*metricsclient.Client, error) {
	fromTransport := metricsclient.DefaultTransport(logger)

	if len(cfg.FromClientConfig.CAFile) > 0 {
		if fromTransport.TLSClientConfig == nil {
			fromTransport.TLSClientConfig = &tls.Config{
				MinVersion: tls.VersionTLS12,
			}
		}

		pool, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("failed to read system certificates: %w", err)
		}

		data, err := os.ReadFile(cfg.FromClientConfig.CAFile)
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

	if len(cfg.FromClientConfig.Token) == 0 && len(cfg.FromClientConfig.TokenFile) > 0 {
		data, err := os.ReadFile(cfg.FromClientConfig.TokenFile)
		if err != nil {
			return nil, fmt.Errorf("unable to read from-token-file: %w", err)
		}
		cfg.FromClientConfig.Token = strings.TrimSpace(string(data))
	}

	if len(cfg.FromClientConfig.Token) > 0 {
		fromClient.Transport = metricshttp.NewBearerRoundTripper(cfg.FromClientConfig.Token, fromClient.Transport)
	}

	return metricsclient.New(logger, metrics.clientMetrics, fromClient, cfg.LimitBytes, interval, "federate_from"), nil
}

// CreateToClient creates a new metrics client for the to URL.
// Uses config for CA, Cert, Key for configuring mTLS transport.
// Skips if nothing is provided.
func (cfg Config) CreateToClient(
	metrics *workerMetrics,
	interval time.Duration,
	name string,
	logger log.Logger,
) (*metricsclient.Client, error) {
	var err error
	toTransport := metricsclient.DefaultTransport(logger)

	if len(cfg.ToClientConfig.CAFile) > 0 {
		toTransport, err = metricsclient.MTLSTransport(logger, cfg.ToClientConfig.CAFile, cfg.ToClientConfig.CertFile, cfg.ToClientConfig.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to create TLS transport: %w", err)
		}
	} else {
		if toTransport.TLSClientConfig == nil {
			// #nosec G402 -- Only used if no TLS config is provided.
			toTransport.TLSClientConfig = &tls.Config{
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: true,
			}
		}
	}

	toTransport.Proxy = http.ProxyFromEnvironment
	toClient := &http.Client{Transport: toTransport}
	if cfg.Debug {
		toClient.Transport = metricshttp.NewDebugRoundTripper(logger, toClient.Transport)
	}

	return metricsclient.New(logger, metrics.clientMetrics, toClient, cfg.LimitBytes, interval, name), nil
}

// GetTransformer creates a new transformer based on the provided Config.
func (cfg Config) GetTransformer(logger log.Logger) (metricfamily.MultiTransformer, error) {
	var transformer metricfamily.MultiTransformer

	// Configure the anonymization.
	anonymizeSalt := cfg.AnonymizeSalt
	if len(cfg.AnonymizeSalt) == 0 && len(cfg.AnonymizeSaltFile) > 0 {
		data, err := os.ReadFile(cfg.AnonymizeSaltFile)
		if err != nil {
			return transformer, fmt.Errorf("failed to read anonymize-salt-file: %w", err)
		}
		anonymizeSalt = strings.TrimSpace(string(data))
	}

	if len(cfg.AnonymizeLabels) != 0 && len(anonymizeSalt) == 0 {
		return transformer, errors.New("anonymize-salt must be specified if anonymize-labels is set")
	}

	if len(cfg.AnonymizeLabels) == 0 {
		rlogger.Log(logger, rlogger.Warn, "msg", "not anonymizing any labels")
	}

	// Combine with config transformer
	if cfg.Transformer != nil {
		transformer.With(cfg.Transformer)
	}

	if len(cfg.AnonymizeLabels) > 0 {
		transformer.With(metricfamily.NewMetricsAnonymizer(anonymizeSalt, cfg.AnonymizeLabels, nil))
	}

	return transformer, nil
}

// Worker represents a metrics forwarding agent. It collects metrics from a source URL and forwards them to a sink.
// A Worker should be configured with a `Config` and instantiated with the `New` func.
// Workers are thread safe; all access to shared fields are synchronized.
type Worker struct {
	logger          log.Logger
	status          status.Reporter
	reconfigure     chan struct{}
	lock            sync.Mutex
	metrics         *workerMetrics
	forwardFailures int

	fromClient *metricsclient.Client
	toClient   *metricsclient.Client
	from       *url.URL
	fromQuery  *url.URL
	to         *url.URL

	interval time.Duration

	transformer             metricfamily.Transformer
	matchers                []string
	recordingRules          []string
	simulatedTimeseriesFile string

	lastMetrics []*clientmodel.MetricFamily
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

// New creates a new Worker based on the provided Config.
func New(cfg Config) (*Worker, error) {
	if cfg.FromClientConfig.URL == nil {
		return nil, errors.New("a URL from which to scrape is required")
	}

	logger := log.With(cfg.Logger, "component", "forwarder")
	rlogger.Log(logger, rlogger.Warn, "msg", cfg.ToClientConfig.URL)

	w := Worker{
		from:                    cfg.FromClientConfig.URL,
		fromQuery:               cfg.FromClientConfig.QueryURL,
		interval:                cfg.Interval,
		reconfigure:             make(chan struct{}),
		to:                      cfg.ToClientConfig.URL,
		logger:                  log.With(cfg.Logger, "component", "forwarder/worker"),
		simulatedTimeseriesFile: cfg.SimulatedTimeseriesFile,
		metrics:                 cfg.Metrics,
	}

	if w.interval == 0 {
		w.interval = 4*time.Minute + 30*time.Second
	}

	fromClient, err := cfg.CreateFromClient(w.metrics, w.interval, "federate_from", logger)
	if err != nil {
		return nil, err
	}

	toClient, err := cfg.CreateToClient(w.metrics, w.interval, "federate_to", logger)
	if err != nil {
		return nil, err
	}

	transformer, err := cfg.GetTransformer(logger)
	if err != nil {
		return nil, err
	}

	w.fromClient = fromClient
	w.toClient = toClient
	w.transformer = transformer

	w.matchers = cfg.Matchers

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

	w.status = &status.NoopReporter{}
	if cfg.StatusClient != nil {
		standalone := os.Getenv("STANDALONE") == "true"
		isUwl := strings.Contains(os.Getenv("FROM"), uwlPromURL)
		s, err := status.New(cfg.StatusClient, logger, standalone, isUwl)
		if err != nil {
			return nil, fmt.Errorf("unable to create StatusReport: %w", err)
		}
		w.status = s
	}

	return &w, nil
}

// TODO(saswatamcode): This is a relic of telemeter code, but with how our workers are configured, there will often
// be no meaningful reload semantics, as most values are often kept as a flags which need restarts.
// There is an option to explore this later, by instead "watching" matcherfile.
// Keeping this method for now, but it is effectively unused.
// Reconfigure temporarily stops a worker and reconfigures is with the provided Condfig.
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
	w.matchers = worker.matchers
	w.recordingRules = worker.recordingRules

	// Signal a restart to Run func.
	// Do this in a goroutine since we do not care if restarting the Run loop is asynchronous.
	go func() { w.reconfigure <- struct{}{} }()
	return nil
}

// TODO(saswatamcode): This is a relic of telemeter code, remove this.
// There is no such utility to exposing this information as to what the last value of metrics sent was
// Rarely would this be used as a tool for debugging, when you already have remote write metrics.
func (w *Worker) LastMetrics() []*clientmodel.MetricFamily {
	w.lock.Lock()
	defer w.lock.Unlock()
	return w.lastMetrics
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		// If the context is canceled, then we're done.
		case <-ctx.Done():
			ticker.Stop()
			return
		case <-ticker.C:
			if err := w.forward(ctx); err != nil {
				rlogger.Log(w.logger, rlogger.Error, "msg", "unable to forward results", "err", err)
			}
		// We want to be able to interrupt a sleep to immediately apply a new configuration.
		case <-w.reconfigure:
			w.lock.Lock()
			ticker.Reset(w.interval)
			w.lock.Unlock()
		}
	}
}

func (w *Worker) forward(ctx context.Context) error {
	w.lock.Lock()
	defer w.lock.Unlock()

	updateStatus := func(reason statuslib.Reason, message string) {
		if reason == statuslib.ForwardFailed {
			w.forwardFailures += 1
			if w.forwardFailures < 3 {
				return
			}
		}

		w.forwardFailures = 0

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
	if len(w.matchers) == 0 {
		return families, nil
	}

	for _, matcher := range w.matchers {
		v.Add(matchParam, matcher)
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

	if len(w.recordingRules) == 0 {
		return families, nil
	}

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
		v.Add(queryParam, rquery)
		from.RawQuery = v.Encode()

		req := &http.Request{Method: "GET", URL: from}
		rfamilies, err := w.fromClient.RetrieveRecordingMetrics(ctx, req, rname)
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
