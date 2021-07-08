// Copyright Contributors to the Open Cluster Management project

package main

import (
	"context"
	"fmt"
	stdlog "log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/oklog/run"
	"github.com/prometheus/common/expfmt"
	"github.com/spf13/cobra"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"

	"github.com/open-cluster-management/multicluster-observability-operator/collectors/metrics/pkg/forwarder"
	collectorhttp "github.com/open-cluster-management/multicluster-observability-operator/collectors/metrics/pkg/http"
	"github.com/open-cluster-management/multicluster-observability-operator/collectors/metrics/pkg/logger"
	"github.com/open-cluster-management/multicluster-observability-operator/collectors/metrics/pkg/metricfamily"
)

func main() {
	opt := &Options{
		Listen:     "localhost:9002",
		LimitBytes: 200 * 1024,
		Rules:      []string{`{__name__="up"}`},
		Interval:   4*time.Minute + 30*time.Second,
	}
	cmd := &cobra.Command{
		Short:         "Federate Prometheus via push",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return opt.Run()
		},
	}

	cmd.Flags().StringVar(&opt.Listen, "listen", opt.Listen, "A host:port to listen on for health and metrics.")
	cmd.Flags().StringVar(&opt.From, "from", opt.From, "The Prometheus server to federate from.")
	cmd.Flags().StringVar(&opt.FromToken, "from-token", opt.FromToken, "A bearer token to use when authenticating to the source Prometheus server.")
	cmd.Flags().StringVar(&opt.FromCAFile, "from-ca-file", opt.FromCAFile, "A file containing the CA certificate to use to verify the --from URL in addition to the system roots certificates.")
	cmd.Flags().StringVar(&opt.FromTokenFile, "from-token-file", opt.FromTokenFile, "A file containing a bearer token to use when authenticating to the source Prometheus server.")
	cmd.Flags().StringVar(&opt.ToUpload, "to-upload", opt.ToUpload, "A server endpoint to push metrics to.")
	cmd.Flags().DurationVar(&opt.Interval, "interval", opt.Interval, "The interval between scrapes. Prometheus returns the last 5 minutes of metrics when invoking the federation endpoint.")
	cmd.Flags().Int64Var(&opt.LimitBytes, "limit-bytes", opt.LimitBytes, "The maxiumum acceptable size of a response returned when scraping Prometheus.")

	// TODO: more complex input definition, such as a JSON struct
	cmd.Flags().StringArrayVar(&opt.Rules, "match", opt.Rules, "Match rules to federate.")
	cmd.Flags().StringArrayVar(&opt.RecordingRules, "recordingrule", opt.RecordingRules, "Define recording rule is to generate new metrics based on specified query expression.")
	cmd.Flags().StringVar(&opt.RulesFile, "match-file", opt.RulesFile, "A file containing match rules to federate, one rule per line.")

	cmd.Flags().StringSliceVar(&opt.LabelFlag, "label", opt.LabelFlag, "Labels to add to each outgoing metric, in key=value form.")
	cmd.Flags().StringSliceVar(&opt.RenameFlag, "rename", opt.RenameFlag, "Rename metrics before sending by specifying OLD=NEW name pairs.")
	cmd.Flags().StringArrayVar(&opt.ElideLabels, "elide-label", opt.ElideLabels, "A list of labels to be elided from outgoing metrics. Default to elide label prometheus and prometheus_replica")

	cmd.Flags().StringSliceVar(&opt.AnonymizeLabels, "anonymize-labels", opt.AnonymizeLabels, "Anonymize the values of the provided values before sending them on.")
	cmd.Flags().StringVar(&opt.AnonymizeSalt, "anonymize-salt", opt.AnonymizeSalt, "A secret and unguessable value used to anonymize the input data.")
	cmd.Flags().StringVar(&opt.AnonymizeSaltFile, "anonymize-salt-file", opt.AnonymizeSaltFile, "A file containing a secret and unguessable value used to anonymize the input data.")

	cmd.Flags().BoolVarP(&opt.Verbose, "verbose", "v", opt.Verbose, "Show verbose output.")

	cmd.Flags().StringVar(&opt.LogLevel, "log-level", opt.LogLevel, "Log filtering level. e.g info, debug, warn, error")

	// deprecated opt
	cmd.Flags().StringVar(&opt.Identifier, "id", opt.Identifier, "The unique identifier for metrics sent with this client.")

	//simulation test
	cmd.Flags().StringVar(&opt.SimulatedTimeseriesFile, "simulated-timeseries-file", opt.SimulatedTimeseriesFile, "A file containing the sample of timeseries.")

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
	ToUpload      string
	FromCAFile    string
	FromToken     string
	FromTokenFile string

	RenameFlag []string
	Renames    map[string]string

	ElideLabels []string

	AnonymizeLabels   []string
	AnonymizeSalt     string
	AnonymizeSaltFile string

	Rules          []string
	RecordingRules []string
	RulesFile      string

	LabelFlag []string
	Labels    map[string]string

	Interval time.Duration

	LogLevel string
	Logger   log.Logger

	// deprecated
	Identifier string

	// simulation file
	SimulatedTimeseriesFile string
}

func (o *Options) Run() error {
	if len(o.From) == 0 {
		return fmt.Errorf("you must specify a Prometheus server to federate from (e.g. http://localhost:9090)")
	}

	for _, flag := range o.LabelFlag {
		values := strings.SplitN(flag, "=", 2)
		if len(values) != 2 {
			return fmt.Errorf("--label must be of the form key=value: %s", flag)
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
			return fmt.Errorf("--rename must be of the form OLD_NAME=NEW_NAME: %s", flag)
		}
		if o.Renames == nil {
			o.Renames = make(map[string]string)
		}
		o.Renames[values[0]] = values[1]
	}

	from, err := url.Parse(o.From)
	if err != nil {
		return fmt.Errorf("--from is not a valid URL: %v", err)
	}
	from.Path = strings.TrimRight(from.Path, "/")
	if len(from.Path) == 0 {
		from.Path = "/federate"
	}

	var toUpload *url.URL
	if len(o.ToUpload) > 0 {
		toUpload, err = url.Parse(o.ToUpload)
		if err != nil {
			return fmt.Errorf("--to-upload is not a valid URL: %v", err)
		}
	}

	if toUpload == nil {
		return fmt.Errorf("--to-upload must be specified")
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
		o.ElideLabels = []string{"prometheus", "prometheus_replica"}
	}
	transformer.WithFunc(func() metricfamily.Transformer {
		return metricfamily.NewElide(o.ElideLabels...)
	})

	transformer.WithFunc(func() metricfamily.Transformer {
		return metricfamily.NewDropInvalidFederateSamples(time.Now().Add(-24 * time.Hour))
	})

	transformer.With(metricfamily.TransformerFunc(metricfamily.PackMetrics))
	transformer.With(metricfamily.TransformerFunc(metricfamily.SortMetrics))

	cfg := forwarder.Config{
		From:          from,
		ToUpload:      toUpload,
		FromToken:     o.FromToken,
		FromTokenFile: o.FromTokenFile,
		FromCAFile:    o.FromCAFile,

		AnonymizeLabels:   o.AnonymizeLabels,
		AnonymizeSalt:     o.AnonymizeSalt,
		AnonymizeSaltFile: o.AnonymizeSaltFile,
		Debug:             o.Verbose,
		Interval:          o.Interval,
		LimitBytes:        o.LimitBytes,
		Rules:             o.Rules,
		RecordingRules:    o.RecordingRules,
		RulesFile:         o.RulesFile,
		Transformer:       transformer,

		Logger:                  o.Logger,
		SimulatedTimeseriesFile: o.SimulatedTimeseriesFile,
	}

	worker, err := forwarder.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to configure metrics collector: %v", err)
	}

	logger.Log(o.Logger, logger.Info, "msg", "starting metrics collector", "from", o.From, "to", o.ToUpload, "listen", o.Listen)

	var g run.Group
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
					if err := worker.Reconfigure(cfg); err != nil {
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
		collectorhttp.MetricRoutes(handlers)
		collectorhttp.ReloadRoutes(handlers, func() error {
			return worker.Reconfigure(cfg)
		})
		handlers.Handle("/federate", serveLastMetrics(o.Logger, worker))
		l, err := net.Listen("tcp", o.Listen)
		if err != nil {
			return fmt.Errorf("failed to listen: %v", err)
		}

		{
			// Run the HTTP server.
			g.Add(func() error {
				if err := http.Serve(l, handlers); err != nil && err != http.ErrServerClosed {
					logger.Log(o.Logger, logger.Error, "msg", "server exited unexpectedly", "err", err)
					return err
				}
				return nil
			}, func(error) {
				err := l.Close()
				if err != nil {
					logger.Log(o.Logger, logger.Error, "msg", "failed to close listener", "err", err)
				}
			})
		}
	}

	return g.Run()
}

// serveLastMetrics retrieves the last set of metrics served
func serveLastMetrics(l log.Logger, worker *forwarder.Worker) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		families := worker.LastMetrics()
		w.Header().Set("Content-Type", string(expfmt.FmtText))
		encoder := expfmt.NewEncoder(w, expfmt.FmtText)
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
