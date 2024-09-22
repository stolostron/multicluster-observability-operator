// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package collectrule

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	clientmodel "github.com/prometheus/client_model/go"
	"github.com/prometheus/prometheus/model/labels"

	"github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/forwarder"
	rlogger "github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/logger"
	"github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/metricsclient"
)

const (
	expireDuration = 15 * time.Minute
)

var (
	config         forwarder.Config
	forwardWorker  *forwarder.Worker
	cancel         context.CancelFunc
	rules          = []CollectRule{}
	pendingRules   = map[string]*EvaluatedRule{}
	firingRules    = map[string]*EvaluatedRule{}
	enabledMatches = map[uint64][]string{}
)

type EvaluatedRule struct {
	triggerTime map[uint64]*time.Time
	resolveTime map[uint64]*time.Time
}
type CollectRule struct {
	Name        string   `json:"name"`
	Expr        string   `json:"expr"`
	DurationStr string   `json:"for"`
	Names       []string `json:"names"`
	Matches     []string `json:"matches"`
	Duration    time.Duration
}

type Evaluator struct {
	fromClient *metricsclient.Client
	from       *url.URL

	interval     time.Duration
	collectRules []string

	lock        sync.Mutex
	reconfigure chan struct{}

	logger log.Logger
}

func New(cfg forwarder.Config) (*Evaluator, error) {
	config = forwarder.Config{
		FromClientConfig: cfg.FromClientConfig,
		ToClientConfig:   cfg.ToClientConfig,

		AnonymizeLabels:   cfg.AnonymizeLabels,
		AnonymizeSalt:     cfg.AnonymizeSalt,
		AnonymizeSaltFile: cfg.AnonymizeSaltFile,
		Debug:             cfg.Debug,
		Interval:          cfg.EvaluateInterval,
		LimitBytes:        cfg.LimitBytes,
		Transformer:       cfg.Transformer,

		Logger:  cfg.Logger,
		Metrics: cfg.Metrics,
	}
	from := &url.URL{
		Scheme: cfg.FromClientConfig.URL.Scheme,
		Host:   cfg.FromClientConfig.URL.Host,
		Path:   "/api/v1/query",
	}
	evaluator := Evaluator{
		from:         from,
		interval:     cfg.EvaluateInterval,
		collectRules: cfg.CollectRules,
		reconfigure:  make(chan struct{}),
		logger:       log.With(cfg.Logger, "component", "collectrule/evaluator"),
	}

	if err := unmarshalCollectorRules(&evaluator); err != nil {
		return nil, err
	}

	if evaluator.interval == 0 {
		evaluator.interval = 30 * time.Second
	}

	fromClient, err := cfg.CreateFromClient(cfg.Metrics, evaluator.interval, "evaluate_query", cfg.Logger)
	if err != nil {
		return nil, err
	}
	evaluator.fromClient = fromClient

	return &evaluator, nil
}

func (e *Evaluator) Reconfigure(cfg forwarder.Config) error {
	evaluator, err := New(cfg)
	if err != nil {
		return fmt.Errorf("failed to reconfigure: %w", err)
	}

	e.lock.Lock()
	defer e.lock.Unlock()

	e.fromClient = evaluator.fromClient
	e.interval = evaluator.interval
	e.from = evaluator.from
	e.collectRules = evaluator.collectRules
	if err = unmarshalCollectorRules(e); err != nil {
		return err
	}

	// Signal a restart to Run func.
	// Do this in a goroutine since we do not care if restarting the Run loop is asynchronous.
	go func() { e.reconfigure <- struct{}{} }()
	return nil
}

func (e *Evaluator) Run(ctx context.Context) {
	for {
		// Ensure that the Worker does not access critical configuration during a reconfiguration.
		e.lock.Lock()
		wait := e.interval
		// The critical section ends here.
		e.lock.Unlock()

		e.evaluate(ctx)

		select {
		// If the context is canceled, then we're done.
		case <-ctx.Done():
			return
		case <-time.After(wait):
		// We want to be able to interrupt a sleep to immediately apply a new configuration.
		case <-e.reconfigure:
		}
	}
}

func unmarshalCollectorRules(e *Evaluator) error {
	rules = []CollectRule{}
	for _, ruleStr := range e.collectRules {
		rule := &CollectRule{}
		err := json.Unmarshal(([]byte)(ruleStr), rule)
		if err != nil {
			rlogger.Log(e.logger, rlogger.Error, "msg", "Input error", "err", err, "rule", rule)
			return err
		}
		if rule.DurationStr != "" {
			rule.Duration, err = time.ParseDuration(rule.DurationStr)
			if err != nil {
				rlogger.Log(e.logger, rlogger.Error, "msg", "wrong duration string found in collect rule", "for", rule.DurationStr)
			}
		}
		rules = append(rules, *rule)
		if pendingRules[rule.Name] == nil {
			pendingRules[rule.Name] = &EvaluatedRule{
				triggerTime: map[uint64]*time.Time{},
			}
		}
		if firingRules[rule.Name] == nil {
			firingRules[rule.Name] = &EvaluatedRule{
				triggerTime: map[uint64]*time.Time{},
				resolveTime: map[uint64]*time.Time{},
			}
		}
	}
	return nil
}

func getMatches() []string {
	matches := []string{}
	for _, v := range enabledMatches {
		matches = append(matches, v[:]...)
	}
	return matches
}

func startWorker() error {
	if forwardWorker == nil {
		var err error
		forwardWorker, err = forwarder.New(config)
		if err != nil {
			return fmt.Errorf("failed to configure forwarder for additional metrics: %w", err)
		}
		var ctx context.Context
		ctx, cancel = context.WithCancel(context.Background())
		go func() {
			forwardWorker.Run(ctx)
			cancel()
		}()
	} else {
		err := forwardWorker.Reconfigure(config)
		if err != nil {
			return fmt.Errorf("failed to reconfigure forwarder for additional metrics: %w", err)
		}
	}

	return nil
}

func renderMatches(r CollectRule, ls labels.Labels) []string {
	matches := []string{}
	for _, name := range r.Names {
		matches = append(matches, fmt.Sprintf(`{__name__="%s"}`, name))
	}
	labelsMap := map[string]string{}
	for _, match := range r.Matches {
		r := regexp.MustCompile(`\{\{ \$labels\.(.*) \}\}`)
		m := r.FindAllStringSubmatch(match, -1)
		for _, v := range m {
			if _, ok := labelsMap[v[1]]; !ok {
				for _, l := range ls {
					if l.Name == v[1] {
						labelsMap[l.Name] = l.Value
						break
					}
				}
			}
			original := fmt.Sprintf("{{ $labels.%s }}", v[1])
			replace := labelsMap[v[1]]
			matches = append(matches, fmt.Sprintf("{%s}", strings.ReplaceAll(match, original, replace)))
		}
		if len(m) == 0 {
			matches = append(matches, fmt.Sprintf("{%s}", match))
		}
	}
	return matches
}

func evaluateRule(logger log.Logger, r CollectRule, metrics []*clientmodel.MetricFamily) bool {
	isUpdate := false
	now := time.Now()
	pendings := map[uint64]string{}
	firings := map[uint64]string{}
	for k := range (*pendingRules[r.Name]).triggerTime {
		pendings[k] = ""
	}
	for k := range (*firingRules[r.Name]).triggerTime {
		firings[k] = ""
	}
	for _, metric := range metrics {
		for _, m := range metric.Metric {
			ls := labels.Labels{}
			for _, l := range m.Label {
				ls = append(ls, labels.Label{
					Name:  *l.Name,
					Value: *l.Value,
				})
			}
			ls = append(ls, labels.Label{
				Name:  "rule_name",
				Value: r.Name,
			})
			h := ls.Hash()
			if (*firingRules[r.Name]).triggerTime[h] != nil {
				delete(firings, h)
				if (*firingRules[r.Name]).resolveTime[h] != nil {
					// resolved rule triggered again
					delete((*firingRules[r.Name]).resolveTime, h)
				}
				continue
			}
			if (*pendingRules[r.Name]).triggerTime[h] == nil {
				if r.Duration == 0 {
					// no duration defined, fire immediately
					(*firingRules[r.Name]).triggerTime[h] = &now
					enabledMatches[h] = renderMatches(r, ls)
					isUpdate = true
					rlogger.Log(logger, rlogger.Info, "msg", "collect rule fired", "name", r.Name, "labels", ls)
				} else {
					(*pendingRules[r.Name]).triggerTime[h] = &now
				}
				continue
			}

			delete(pendings, h)
			if time.Since(*(*pendingRules[r.Name]).triggerTime[h]) >= r.Duration {
				// already passed duration, fire
				(*firingRules[r.Name]).triggerTime[h] = &now
				delete((*pendingRules[r.Name]).triggerTime, h)
				enabledMatches[h] = renderMatches(r, ls)
				isUpdate = true
				rlogger.Log(logger, rlogger.Info, "msg", "collect rule fired", "name", r.Name, "labels", ls)
			}
		}
	}
	for k := range pendings {
		delete((*pendingRules[r.Name]).triggerTime, k)
	}
	for k := range firings {
		if (*firingRules[r.Name]).resolveTime[k] == nil {
			(*firingRules[r.Name]).resolveTime[k] = &now
		} else if time.Since(*(*firingRules[r.Name]).resolveTime[k]) >= expireDuration {
			delete((*firingRules[r.Name]).triggerTime, k)
			delete((*firingRules[r.Name]).resolveTime, k)
			delete(enabledMatches, k)
			isUpdate = true
			rlogger.Log(logger, rlogger.Info, "msg", "fired collect rule resolved", "name", r.Name)
		}
	}
	return isUpdate
}

func (e *Evaluator) evaluate(ctx context.Context) {
	isUpdate := false
	for _, r := range rules {
		from := e.from
		from.RawQuery = ""
		v := e.from.Query()
		v.Add("query", r.Expr)
		from.RawQuery = v.Encode()

		req := &http.Request{Method: "GET", URL: from}
		result, err := e.fromClient.RetrievRecordingMetrics(ctx, req, r.Name)
		if err != nil {
			rlogger.Log(e.logger, rlogger.Error, "msg", "failed to evaluate collect rule", "err", err, "rule", r.Expr)
			continue
		} else {
			if evaluateRule(e.logger, r, result) {
				isUpdate = true
			}
		}
	}
	if isUpdate {
		config.Matchers = getMatches()

		if len(config.Matchers) == 0 {
			if forwardWorker != nil && cancel != nil {
				cancel()
				forwardWorker = nil
				rlogger.Log(e.logger, rlogger.Info, "msg", "forwarder stopped")
			}
		} else {
			err := startWorker()
			if err != nil {
				rlogger.Log(e.logger, rlogger.Error, "msg", "failed to start forwarder to collect metrics", "error", err)
			} else {
				rlogger.Log(e.logger, rlogger.Info, "msg", "forwarder started/reconfigued to collect metrics")
			}
		}
	}
}
