// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"context"
	"errors"
	"fmt"
	stdlog "log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/common/expfmt"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/uuid"

	"github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/collectrule"
	"github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/forwarder"
	collectorhttp "github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/http"
	"github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/logger"
	"github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/metricfamily"
)

func main() {
	opt := &Options{
		From:             "http://localhost:9090",
		Listen:           "localhost:9002",
		LimitBytes:       200 * 1024,
		Rules:            []string{`{__name__="up"}`},
		Interval:         4*time.Minute + 30*time.Second,
		EvaluateInterval: 30 * time.Second,
		WorkerNum:        1,
	}
	cmd := &cobra.Command{
		Short:         "Federate Prometheus via push",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return opt.Run()
		},
	}

	cmd.Flags().Int64Var(
		&opt.WorkerNum,
		"worker-number",
		opt.WorkerNum,
		"The number of client runs in the simulate environment.")
	cmd.Flags().StringVar(
		&opt.Listen,
		"listen",
		opt.Listen,
		"A host:port to listen on for health and metrics.")
	cmd.Flags().StringVar(
		&opt.From,
		"from",
		opt.From,
		"The Prometheus server to federate from.")
	cmd.Flags().StringVar(
		&opt.FromQuery,
		"from-query",
		opt.From,
		"The Prometheus server to query from.")
	cmd.Flags().StringVar(
		&opt.FromToken,
		"from-token",
		opt.FromToken,
		"A bearer token to use when authenticating to the source Prometheus server.")
	cmd.Flags().StringVar(
		&opt.FromCAFile,
		"from-ca-file",
		opt.FromCAFile,
		`A file containing the CA certificate to use to verify the --from URL in
		 addition to the system roots certificates.`)
	cmd.Flags().StringVar(
		&opt.FromTokenFile,
		"from-token-file",
		opt.FromTokenFile,
		"A file containing a bearer token to use when authenticating to the source Prometheus server.")
	cmd.Flags().StringVar(
		&opt.ToUpload,
		"to-upload",
		opt.ToUpload,
		"A server endpoint to push metrics to.")
	cmd.Flags().StringVar(
		&opt.ToUploadCA,
		"to-upload-ca",
		opt.ToUploadCA,
		"A file containing the CA certificate to verify the --to-upload URL in addition to the system certificates.")
	cmd.Flags().StringVar(
		&opt.ToUploadCert,
		"to-upload-cert",
		opt.ToUploadCert,
		"A file containing the certificate to use to secure the request to the --to-upload URL.")
	cmd.Flags().StringVar(
		&opt.ToUploadKey,
		"to-upload-key",
		opt.ToUploadKey,
		"A file containing the certificate key to use to secure the request to the --to-upload URL.")
	cmd.Flags().DurationVar(
		&opt.Interval,
		"interval",
		opt.Interval,
		`The interval between scrapes. Prometheus returns the last 5 minutes of 
		 metrics when invoking the federation endpoint.`)
	cmd.Flags().DurationVar(
		&opt.EvaluateInterval,
		"evaluate-interval",
		opt.EvaluateInterval,
		"The interval between collect rule evaluation.")
	cmd.Flags().Int64Var(
		&opt.LimitBytes,
		"limit-bytes",
		opt.LimitBytes,
		"The maxiumum acceptable size of a response returned when scraping Prometheus.")

	// TODO: more complex input definition, such as a JSON struct
	cmd.Flags().StringArrayVar(
		&opt.Rules,
		"match",
		opt.Rules,
		"Match rules to federate.")
	cmd.Flags().StringVar(
		&opt.RulesFile,
		"match-file",
		opt.RulesFile,
		"A file containing match rules to federate, one rule per line.")
	cmd.Flags().StringArrayVar(
		&opt.RecordingRules,
		"recordingrule",
		opt.RecordingRules,
		"Define recording rule is to generate new metrics based on specified query expression.")
	cmd.Flags().StringVar(
		&opt.RecordingRulesFile,
		"recording-file",
		opt.RulesFile,
		"A file containing recording rules.")
	cmd.Flags().StringArrayVar(
		&opt.CollectRules,
		"collectrule",
		opt.CollectRules,
		"Define metrics collect rule is to collect additional metrics based on specified event.")
	cmd.Flags().StringVar(
		&opt.RecordingRulesFile,
		"collect-file",
		opt.RecordingRulesFile,
		"A file containing collect rules.")

	cmd.Flags().StringSliceVar(
		&opt.LabelFlag,
		"label",
		opt.LabelFlag,
		"Labels to add to each outgoing metric, in key=value form.")
	cmd.Flags().StringSliceVar(
		&opt.RenameFlag,
		"rename",
		opt.RenameFlag,
		"Rename metrics before sending by specifying OLD=NEW name pairs.")
	cmd.Flags().StringArrayVar(
		&opt.ElideLabels,
		"elide-label",
		opt.ElideLabels,
		`A list of labels to be elided from outgoing metrics. Default to elide 
		 label prometheus and prometheus_replica`)

	cmd.Flags().StringSliceVar(
		&opt.AnonymizeLabels,
		"anonymize-labels",
		opt.AnonymizeLabels,
		"Anonymize the values of the provided values before sending them on.")
	cmd.Flags().StringVar(
		&opt.AnonymizeSalt,
		"anonymize-salt",
		opt.AnonymizeSalt,
		"A secret and unguessable value used to anonymize the input data.")
	cmd.Flags().StringVar(
		&opt.AnonymizeSaltFile,
		"anonymize-salt-file",
		opt.AnonymizeSaltFile,
		"A file containing a secret and unguessable value used to anonymize the input data.")

	cmd.Flags().BoolVarP(
		&opt.Verbose,
		"verbose", "v",
		opt.Verbose,
		"Show verbose output.")

	cmd.Flags().StringVar(
		&opt.LogLevel,
		"log-level",
		opt.LogLevel,
		"Log filtering level. e.g info, debug, warn, error")

	// deprecated opt
	cmd.Flags().StringVar(
		&opt.Identifier,
		"id",
		opt.Identifier,
		"The unique identifier for metrics sent with this client.")

	// simulation test
	cmd.Flags().StringVar(
		&opt.SimulatedTimeseriesFile,
		"simulated-timeseries-file",
		opt.SimulatedTimeseriesFile,
		"A file containing the sample of timeseries.")

	l := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	lvl, err := cmd.Flags().GetString("log-level")
	if err != nil {
		logger.Log(l, logger.Error, "msg", "could not parse log-level.")
	}
	l = level.NewFilter(l, logger.LogLevelFromString(lvl))
	l = log.WithPrefix(l, "ts", log.DefaultTimestampUTC)
	l = log.WithPrefix(l, "caller", log.DefaultCaller)
	stdlog.SetOutput(log.NewStdlibAdapter(l))
	opt.Logger = l
	logger.Log(l, logger.Info, "msg", "metrics collector initialized")

	if err := cmd.Execute(); err != nil {
		logger.Log(l, logger.Error, "err", err)
		os.Exit(1)
	}
}

type Options struct {
	Listen     string
	LimitBytes int64
	Verbose    bool

	From          string
	FromQuery     string
	ToUpload      string
	FromCAFile    string
	FromToken     string
	FromTokenFile string
	ToUploadCA    string
	ToUploadCert  string
	ToUploadKey   string

	RenameFlag []string
	Renames    map[string]string

	ElideLabels []string

	AnonymizeLabels   []string
	AnonymizeSalt     string
	AnonymizeSaltFile string

	Rules              []string
	RulesFile          string
	RecordingRules     []string
	RecordingRulesFile string
	CollectRules       []string
	CollectRulesFile   string

	LabelFlag []string
	Labels    map[string]string

	Interval         time.Duration
	EvaluateInterval time.Duration

	LogLevel string
	Logger   log.Logger

	// deprecated
	Identifier string

	// simulation file
	SimulatedTimeseriesFile string

	// how many threads are running
	// for production, it is always 1
	WorkerNum int64
}

func (o *Options) Run() error {
	var g run.Group

	metricsReg := prometheus.NewRegistry()
	metricsReg.MustRegister(
		collectors.NewBuildInfoCollector(),
		collectors.NewGoCollector(
			collectors.WithGoCollectorRuntimeMetrics(collectors.GoRuntimeMetricsRule{Matcher: regexp.MustCompile("/.*")}),
		),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	// Some packages still use default Register. Replace to have those metrics.
	prometheus.DefaultRegisterer = metricsReg

	err, cfg := initConfig(o)
	if err != nil {
		return err
	}

	metrics := forwarder.NewWorkerMetrics(metricsReg)
	cfg.Metrics = metrics
	worker, err := forwarder.New(*cfg)
	if err != nil {
		return fmt.Errorf("failed to configure metrics collector: %w", err)
	}

	logger.Log(
		o.Logger, logger.Info,
		"msg", "starting metrics collector",
		"from", o.From,
		"to", o.ToUpload,
		"listen", o.Listen)

	{
		// Execute the worker's `Run` func.
		ctx, cancel := context.WithCancel(context.Background())
		g.Add(func() error {
			worker.Run(ctx)
			return nil
		}, func(error) {
			cancel()
		})
	}

	{
		// Notify and reload on SIGHUP.
		hup := make(chan os.Signal, 1)
		signal.Notify(hup, syscall.SIGHUP)
		cancel := make(chan struct{})
		g.Add(func() error {
			for {
				select {
				case <-hup:
					if err := worker.Reconfigure(*cfg); err != nil {
						logger.Log(o.Logger, logger.Error, "msg", "failed to reload config", "err", err)
						return err
					}
				case <-cancel:
					return nil
				}
			}
		}, func(error) {
			close(cancel)
		})
	}

	if len(o.Listen) > 0 {
		handlers := http.NewServeMux()
		collectorhttp.DebugRoutes(handlers)
		collectorhttp.HealthRoutes(handlers)
		collectorhttp.MetricRoutes(handlers, metricsReg)
		collectorhttp.ReloadRoutes(handlers, func() error {
			return worker.Reconfigure(*cfg)
		})
		handlers.Handle("/federate", serveLastMetrics(o.Logger, worker))
		s := http.Server{
			Addr:              o.Listen,
			Handler:           handlers,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      12 * time.Minute,
		}

		{
			// Run the HTTP server.
			g.Add(func() error {
				if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					logger.Log(o.Logger, logger.Error, "msg", "server exited unexpectedly", "err", err)
					return err
				}
				return nil
			}, func(error) {
				err := s.Shutdown(context.Background())
				if err != nil {
					logger.Log(o.Logger, logger.Error, "msg", "failed to close listener", "err", err)
				}
			})
		}
	}

	err = runMultiWorkers(o, cfg)
	if err != nil {
		return err
	}

	if len(o.CollectRules) != 0 {
		evaluator, err := collectrule.New(*cfg)
		if err != nil {
			return fmt.Errorf("failed to configure collect rule evaluator: %w", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		g.Add(func() error {
			evaluator.Run(ctx)
			return nil
		}, func(error) {
			cancel()
		})
	}

	return g.Run()
}

func runMultiWorkers(o *Options, cfg *forwarder.Config) error {
	for i := 1; i < int(o.WorkerNum); i++ {
		opt := &Options{
			From:                    o.From,
			FromQuery:               o.FromQuery,
			ToUpload:                o.ToUpload,
			FromCAFile:              o.FromCAFile,
			FromTokenFile:           o.FromTokenFile,
			ToUploadCA:              o.ToUploadCA,
			ToUploadCert:            o.ToUploadCert,
			ToUploadKey:             o.ToUploadKey,
			Rules:                   o.Rules,
			RenameFlag:              o.RenameFlag,
			RecordingRules:          o.RecordingRules,
			Interval:                o.Interval,
			Labels:                  map[string]string{},
			SimulatedTimeseriesFile: o.SimulatedTimeseriesFile,
			Logger:                  o.Logger,
		}
		for _, flag := range o.LabelFlag {
			values := strings.SplitN(flag, "=", 2)
			if len(values) != 2 {
				return fmt.Errorf("--label must be of the form key=value: %s", flag)
			}
			if values[0] == "cluster" {
				values[1] += "-" + fmt.Sprint(i)
			}
			if values[0] == "clusterID" {
				values[1] = string(uuid.NewUUID())
			}
			opt.Labels[values[0]] = values[1]
		}
		err, forwardCfg := initConfig(opt)
		if err != nil {
			return err
		}

		forwardCfg.Metrics = cfg.Metrics
		forwardWorker, err := forwarder.New(*forwardCfg)
		if err != nil {
			return fmt.Errorf("failed to configure metrics collector: %w", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			forwardWorker.Run(ctx)
			cancel()
		}()

	}
	return nil
}

func initConfig(o *Options) (error, *forwarder.Config) {
	if len(o.From) == 0 {
		return errors.New("you must specify a Prometheus server to federate from (e.g. http://localhost:9090)"), nil
	}

	for _, flag := range o.LabelFlag {
		values := strings.SplitN(flag, "=", 2)
		if len(values) != 2 {
			return fmt.Errorf("--label must be of the form key=value: %s", flag), nil
		}
		if o.Labels == nil {
			o.Labels = make(map[string]string)
		}
		o.Labels[values[0]] = values[1]
	}

	for _, flag := range o.RenameFlag {
		if len(flag) == 0 {
			continue
		}
		values := strings.SplitN(flag, "=", 2)
		if len(values) != 2 {
			return fmt.Errorf("--rename must be of the form OLD_NAME=NEW_NAME: %s", flag), nil
		}
		if o.Renames == nil {
			o.Renames = make(map[string]string)
		}
		o.Renames[values[0]] = values[1]
	}

	from, err := url.Parse(o.From)
	if err != nil {
		return fmt.Errorf("--from is not a valid URL: %w", err), nil
	}
	from.Path = strings.TrimRight(from.Path, "/")
	if len(from.Path) == 0 {
		from.Path = "/federate"
	}

	fromQuery, err := url.Parse(o.FromQuery)
	if err != nil {
		return fmt.Errorf("--from-query is not a valid URL: %w", err), nil
	}
	fromQuery.Path = strings.TrimRight(fromQuery.Path, "/")
	if len(fromQuery.Path) == 0 {
		fromQuery.Path = "/api/v1/query"
	}

	var toUpload *url.URL
	if len(o.ToUpload) > 0 {
		toUpload, err = url.Parse(o.ToUpload)
		if err != nil {
			return fmt.Errorf("--to-upload is not a valid URL: %w", err), nil
		}
	}

	if toUpload == nil {
		return errors.New("--to-upload must be specified"), nil
	}

	var transformer metricfamily.MultiTransformer

	if len(o.Labels) > 0 {
		transformer.WithFunc(func() metricfamily.Transformer {
			return metricfamily.NewLabel(o.Labels, nil)
		})
	}

	if len(o.Renames) > 0 {
		transformer.WithFunc(func() metricfamily.Transformer {
			return metricfamily.RenameMetrics{Names: o.Renames}
		})
	}

	if len(o.ElideLabels) == 0 {
		// While forwarding alerts from managed clusters to ACM alert manager on the hub,
		// prometheus on managed clusters is configured to add a "managed_cluster" label
		// to alerts and metrics. Strip this label from metrics to conserve resources.
		// This must match the value of github.com/stolostron/multicluster-observability-operator/operators/pkg/config
		o.ElideLabels = []string{"prometheus", "prometheus_replica", "managed_cluster"}
	}
	transformer.WithFunc(func() metricfamily.Transformer {
		return metricfamily.NewElide(o.ElideLabels...)
	})

	transformer.WithFunc(func() metricfamily.Transformer {
		return metricfamily.NewDropInvalidFederateSamples(time.Now().Add(-24 * time.Hour))
	})

	transformer.With(metricfamily.TransformerFunc(metricfamily.PackMetrics))
	transformer.With(metricfamily.TransformerFunc(metricfamily.SortMetrics))

	isHypershift, err := metricfamily.CheckCRDExist(o.Logger)
	if err != nil {
		return err, nil
	}
	if isHypershift {
		hyperTransformer, err := metricfamily.NewHypershiftTransformer(o.Logger, nil, o.Labels)
		if err != nil {
			return err, nil
		}
		transformer.WithFunc(func() metricfamily.Transformer {
			return hyperTransformer
		})
	}

	return nil, &forwarder.Config{
		From:          from,
		FromQuery:     fromQuery,
		ToUpload:      toUpload,
		FromToken:     o.FromToken,
		FromTokenFile: o.FromTokenFile,
		FromCAFile:    o.FromCAFile,
		ToUploadCA:    o.ToUploadCA,
		ToUploadCert:  o.ToUploadCert,
		ToUploadKey:   o.ToUploadKey,

		AnonymizeLabels:   o.AnonymizeLabels,
		AnonymizeSalt:     o.AnonymizeSalt,
		AnonymizeSaltFile: o.AnonymizeSaltFile,
		Debug:             o.Verbose,
		Interval:          o.Interval,
		EvaluateInterval:  o.EvaluateInterval,
		LimitBytes:        o.LimitBytes,
		Rules:             o.Rules,
		RulesFile:         o.RulesFile,
		RecordingRules:    o.RecordingRules,
		CollectRules:      o.CollectRules,
		Transformer:       transformer,

		Logger:                  o.Logger,
		SimulatedTimeseriesFile: o.SimulatedTimeseriesFile,
	}
}

// serveLastMetrics retrieves the last set of metrics served.
func serveLastMetrics(l log.Logger, worker *forwarder.Worker) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		families := worker.LastMetrics()
		protoTextFormat := expfmt.NewFormat(expfmt.TypeProtoText)
		w.Header().Set("Content-Type", string(protoTextFormat))
		encoder := expfmt.NewEncoder(w, protoTextFormat)
		for _, family := range families {
			if family == nil {
				continue
			}
			if err := encoder.Encode(family); err != nil {
				logger.Log(l, logger.Error, "msg", "unable to write metrics for family", "err", err)
				break
			}
		}
	})
}
