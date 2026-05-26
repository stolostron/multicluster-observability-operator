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
	"regexp"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/run"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/collectrule"
	"github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/forwarder"
	collectorhttp "github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/http"
	"github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/logger"
	"github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/metricfamily"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
)

func main() {
	opt := &Options{
		From:                    "http://localhost:9090",
		Listen:                  "localhost:9002",
		LimitBytes:              200 * 1024,
		Matchers:                []string{`{__name__="up"}`},
		Interval:                4*time.Minute + 30*time.Second,
		EvaluateInterval:        30 * time.Second,
		WorkerNum:               1,
		DisableHyperShift:       false,
		DisableStatusReporting:  false,
		SimulatedTimeseriesFile: "",
	}
	cmd := &cobra.Command{
		Short:         "Remote write federated metrics from prometheus",
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
		"The number of workers that will work in parallel to send metrics.")
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

	cmd.Flags().StringArrayVar(
		&opt.Matchers,
		"match",
		opt.Matchers,
		"Match rules to federate.")
	cmd.Flags().StringVar(
		&opt.MatcherFile,
		"match-file",
		opt.MatcherFile,
		"A file containing match rules to federate, one rule per line.")
	cmd.Flags().StringArrayVar(
		&opt.RecordingRules,
		"recordingrule",
		opt.RecordingRules,
		"Define recording rule is to generate new metrics based on specified query expression.")
	cmd.Flags().StringArrayVar(
		&opt.CollectRules,
		"collectrule",
		opt.CollectRules,
		"Define metrics collect rule is to collect additional metrics based on specified event.")

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

	cmd.Flags().BoolVar(
		&opt.DisableStatusReporting,
		"disable-status-reporting",
		opt.DisableStatusReporting,
		"Disable status reporting to hub cluster.")

	cmd.Flags().BoolVar(
		&opt.DisableHyperShift,
		"disable-hypershift",
		opt.DisableHyperShift,
		"Disable hypershift related metrics collection.")

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

	Matchers       []string
	MatcherFile    string
	RecordingRules []string
	CollectRules   []string

	LabelFlag []string
	Labels    map[string]string

	Interval         time.Duration
	EvaluateInterval time.Duration

	LogLevel string
	Logger   log.Logger

	// simulation file
	SimulatedTimeseriesFile string

	// how many threads are running
	// for production, it is always 1
	WorkerNum int64

	DisableHyperShift      bool
	DisableStatusReporting bool
}

// Run is the entry point of the metrics collector
// It is in charge of running forwarders, collectrule agent and recording rule agents.
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

	cfgRR, err := initShardedConfigs(o, AgentRecordingRule)
	if err != nil {
		return err
	}

	shardCfgs, err := initShardedConfigs(o, AgentShardedForwarder)
	if err != nil {
		return err
	}

	evalCfg, err := initShardedConfigs(o, AgentCollectRule)
	if err != nil {
		return err
	}

	metrics := forwarder.NewWorkerMetrics(metricsReg)
	evalCfg[0].Metrics = metrics
	cfgRR[0].Metrics = metrics
	recordingRuleWorker, err := forwarder.New(*cfgRR[0])
	if err != nil {
		return fmt.Errorf("failed to configure recording rule worker: %w", err)
	}

	shardWorkers := make([]*forwarder.Worker, len(shardCfgs))
	for i, shardCfg := range shardCfgs {
		shardCfg.Metrics = metrics
		shardWorkers[i], err = forwarder.New(*shardCfg)
		if err != nil {
			return fmt.Errorf("failed to configure shard worker %d: %w", i, err)
		}
	}

	logger.Log(
		o.Logger, logger.Info,
		"msg", "starting metrics collector",
		"from", o.From,
		"to", o.ToUpload,
		"listen", o.Listen)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	{
		// Execute the recording rule worker's `Run` func.
		g.Add(func() error {
			recordingRuleWorker.Run(ctx)
			return nil
		}, func(error) {
			cancel()
		})
	}
	{
		// Execute the shard workers' `Run` func.
		g.Add(func() error {
			for i, shardWorker := range shardWorkers {
				go func(i int, shardWorker *forwarder.Worker) {
					logger.Log(o.Logger, logger.Info, "msg", "Starting shard worker", "worker", i)
					shardWorker.Run(ctx)
				}(i, shardWorker)
			}
			<-ctx.Done()
			return nil
		}, func(error) {
			cancel()
		})
	}

	if len(o.Listen) > 0 {
		handlers := http.NewServeMux()
		collectorhttp.DebugRoutes(handlers)
		collectorhttp.HealthRoutes(handlers)
		collectorhttp.MetricRoutes(handlers, metricsReg)
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
				err := s.Shutdown(ctx)
				if err != nil {
					logger.Log(o.Logger, logger.Error, "msg", "failed to close listener", "err", err)
				}
			})
		}
	}

	// Run the simulation agent.
	err = runMultiWorkers(o, evalCfg[0])
	if err != nil {
		return err
	}

	// Run the Collectrules agent.
	if len(o.CollectRules) != 0 {
		evaluator, err := collectrule.New(*evalCfg[0])
		if err != nil {
			return fmt.Errorf("failed to configure collect rule evaluator: %w", err)
		}
		g.Add(func() error {
			evaluator.Run(ctx)
			return nil
		}, func(error) {
			cancel()
		})
	}

	return g.Run()
}

// splitMatchersIntoShards divides the matchers into approximately equal shards
func splitMatchersIntoShards(matchers []string, shardCount int) [][]string {
	if shardCount <= 1 {
		return [][]string{matchers}
	}

	shards := make([][]string, shardCount)
	for i, matcher := range matchers {
		shardIndex := i % shardCount
		shards[shardIndex] = append(shards[shardIndex], matcher)
	}

	return shards
}

// Agent is the type of the worker agent that will be running.
// They are classified according to what they collect.
type Agent string

const (
	AgentCollectRule      Agent = "collectrule"
	AgentRecordingRule    Agent = "recordingrule"
	AgentShardedForwarder Agent = "forwarder"
)

func initShardedConfigs(o *Options, agent Agent) ([]*forwarder.Config, error) {
	if len(o.From) == 0 {
		return nil, errors.New("you must specify a Prometheus server to federate from (e.g. http://localhost:9090)")
	}

	for _, flag := range o.LabelFlag {
		values := strings.SplitN(flag, "=", 2)
		if len(values) != 2 {
			return nil, fmt.Errorf("--label must be of the form key=value: %s", flag)
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
			return nil, fmt.Errorf("--rename must be of the form OLD_NAME=NEW_NAME: %s", flag)
		}
		if o.Renames == nil {
			o.Renames = make(map[string]string)
		}
		o.Renames[values[0]] = values[1]
	}

	from, err := url.Parse(o.From)
	if err != nil {
		return nil, fmt.Errorf("--from is not a valid URL: %w", err)
	}
	from.Path = strings.TrimRight(from.Path, "/")
	if len(from.Path) == 0 {
		from.Path = "/federate"
	}

	fromQuery, err := url.Parse(o.FromQuery)
	if err != nil {
		return nil, fmt.Errorf("--from-query is not a valid URL: %w", err)
	}

	fromQuery.Path = strings.TrimRight(fromQuery.Path, "/")
	if len(fromQuery.Path) == 0 {
		fromQuery.Path = "/api/v1/query"
	}

	var toUpload *url.URL
	if len(o.ToUpload) > 0 {
		toUpload, err = url.Parse(o.ToUpload)
		if err != nil {
			return nil, fmt.Errorf("--to-upload is not a valid URL: %w", err)
		}
	}

	if toUpload == nil {
		return nil, errors.New("--to-upload must be specified")
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

	// TODO(saswatamcode): Kill this feature.
	// This is too messy of an approach, to get hypershift specific labels into metrics we send.
	// There is much better way to do this, with relabel configs.
	// A collection agent shouldn't be calling out to Kube API server just to add labels.
	if !o.DisableHyperShift {
		isHypershift, err := metricfamily.CheckCRDExist(o.Logger)
		if err != nil {
			return nil, err
		}
		if isHypershift {
			config, err := clientcmd.BuildConfigFromFlags("", "")
			if err != nil {
				return nil, errors.New("failed to create the kube config for hypershiftv1")
			}
			s := scheme.Scheme
			if err := hyperv1.AddToScheme(s); err != nil {
				return nil, errors.New("failed to add observabilityaddon into scheme")
			}
			hClient, err := client.New(config, client.Options{Scheme: s})
			if err != nil {
				return nil, errors.New("failed to create the kube client")
			}

			hyperTransformer, err := metricfamily.NewHypershiftTransformer(hClient, o.Logger, o.Labels)
			if err != nil {
				return nil, err
			}
			transformer.WithFunc(func() metricfamily.Transformer {
				return hyperTransformer
			})
		}
	}

	// Configure matchers.
	matchers := o.Matchers
	if len(o.MatcherFile) > 0 {
		data, err := os.ReadFile(o.MatcherFile)
		if err != nil {
			return nil, fmt.Errorf("unable to read match-file: %w", err)
		}
		matchers = append(matchers, strings.Split(string(data), "\n")...)
	}
	for i := 0; i < len(matchers); {
		s := strings.TrimSpace(matchers[i])
		if len(s) == 0 {
			matchers = append(matchers[:i], matchers[i+1:]...)
			continue
		}
		matchers[i] = s
		i++
	}

	var statusClient client.Client

	// TODO(saswatamcode): Evaluate better way for status reporting.
	// Currently, it reports status for every 3 errors in forward request. This is too much of an overhead
	// Every remote write request error does not need to be reported.
	// Instead we can try to do this with metrics-collector meta-monitoring
	if !o.DisableStatusReporting {
		config, err := clientcmd.BuildConfigFromFlags("", "")
		if err != nil {
			return nil, errors.New("failed to create the kube config for status")
		}
		s := scheme.Scheme
		if err := oav1beta1.AddToScheme(s); err != nil {
			return nil, errors.New("failed to add observabilityaddon into scheme")
		}

		statusClient, err = client.New(config, client.Options{Scheme: s})
		if err != nil {
			return nil, errors.New("failed to create the kube client")
		}
	}

	switch agent {
	case AgentCollectRule:
		f := forwarder.Config{
			FromClientConfig: forwarder.FromClientConfig{
				URL:       from,
				QueryURL:  fromQuery,
				Token:     o.FromToken,
				TokenFile: o.FromTokenFile,
				CAFile:    o.FromCAFile,
			},
			ToClientConfig: forwarder.ToClientConfig{
				URL:      toUpload,
				CAFile:   o.ToUploadCA,
				CertFile: o.ToUploadCert,
				KeyFile:  o.ToUploadKey,
			},

			StatusClient:      statusClient,
			AnonymizeLabels:   o.AnonymizeLabels,
			AnonymizeSalt:     o.AnonymizeSalt,
			AnonymizeSaltFile: o.AnonymizeSaltFile,
			Debug:             o.Verbose,
			Interval:          o.Interval,
			EvaluateInterval:  o.EvaluateInterval,
			LimitBytes:        o.LimitBytes,
			Matchers:          matchers,
			RecordingRules:    o.RecordingRules,
			CollectRules:      o.CollectRules,
			Transformer:       transformer,

			Logger:                  o.Logger,
			SimulatedTimeseriesFile: o.SimulatedTimeseriesFile,
		}
		return []*forwarder.Config{&f}, nil

	case AgentRecordingRule:
		f := forwarder.Config{
			FromClientConfig: forwarder.FromClientConfig{
				URL:       from,
				QueryURL:  fromQuery,
				Token:     o.FromToken,
				TokenFile: o.FromTokenFile,
				CAFile:    o.FromCAFile,
			},
			ToClientConfig: forwarder.ToClientConfig{
				URL:      toUpload,
				CAFile:   o.ToUploadCA,
				CertFile: o.ToUploadCert,
				KeyFile:  o.ToUploadKey,
			},

			StatusClient:      statusClient,
			AnonymizeLabels:   o.AnonymizeLabels,
			AnonymizeSalt:     o.AnonymizeSalt,
			AnonymizeSaltFile: o.AnonymizeSaltFile,
			Debug:             o.Verbose,
			Interval:          o.Interval,
			EvaluateInterval:  o.EvaluateInterval,
			LimitBytes:        o.LimitBytes,
			RecordingRules:    o.RecordingRules,
			Transformer:       transformer,

			Logger:                  o.Logger,
			SimulatedTimeseriesFile: o.SimulatedTimeseriesFile,
		}
		return []*forwarder.Config{&f}, nil

	case AgentShardedForwarder:
		if len(matchers) < int(o.WorkerNum) {
			return nil, errors.New("number of shards is greater than the number of matchers")
		}

		shards := splitMatchersIntoShards(matchers, int(o.WorkerNum))

		shardCfgs := make([]*forwarder.Config, len(shards))
		for i, shard := range shards {
			// make sure we copy the URL object so it's not shared between workers
			fromCopy := *from
			fromQueryCopy := *fromQuery
			shardCfgs[i] = &forwarder.Config{
				FromClientConfig: forwarder.FromClientConfig{
					URL:       &fromCopy,
					QueryURL:  &fromQueryCopy,
					Token:     o.FromToken,
					TokenFile: o.FromTokenFile,
					CAFile:    o.FromCAFile,
				},
				StatusClient: statusClient,
				ToClientConfig: forwarder.ToClientConfig{
					URL:      toUpload,
					CAFile:   o.ToUploadCA,
					CertFile: o.ToUploadCert,
					KeyFile:  o.ToUploadKey,
				},
				AnonymizeLabels:         o.AnonymizeLabels,
				AnonymizeSalt:           o.AnonymizeSalt,
				AnonymizeSaltFile:       o.AnonymizeSaltFile,
				Debug:                   o.Verbose,
				Interval:                o.Interval,
				EvaluateInterval:        o.EvaluateInterval,
				LimitBytes:              o.LimitBytes,
				Matchers:                shard,
				Transformer:             transformer,
				Logger:                  log.With(o.Logger, "shard", i),
				SimulatedTimeseriesFile: o.SimulatedTimeseriesFile,
			}
		}
		return shardCfgs, nil
	default:
		return nil, errors.New("invalid agent type")
	}
}

func runMultiWorkers(o *Options, cfg *forwarder.Config) error {
	if o.WorkerNum > 1 && o.SimulatedTimeseriesFile == "" {
		return nil
	}

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
			Matchers:                o.Matchers,
			RecordingRules:          o.RecordingRules,
			Interval:                o.Interval,
			Labels:                  map[string]string{},
			SimulatedTimeseriesFile: o.SimulatedTimeseriesFile,
			Logger:                  o.Logger,
			DisableHyperShift:       o.DisableHyperShift,
			DisableStatusReporting:  o.DisableStatusReporting,
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

		forwardCfg, err := initShardedConfigs(opt, AgentCollectRule)
		if err != nil {
			return err
		}

		forwardCfg[0].Metrics = cfg.Metrics
		forwardWorker, err := forwarder.New(*forwardCfg[0])
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
