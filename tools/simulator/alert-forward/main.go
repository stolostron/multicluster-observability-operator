// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	"github.com/spf13/cobra"
)

type alertForwarderOptions struct {
	amHost            string
	amScheme          string
	amAPIVersion      string
	amAccessToken     string
	amAccessTokenFile string
	interval          time.Duration
	workers           int
	alerts            string
	alertsFile        string
}

type alertForwarder struct {
	amURL    string
	amConfig *config.AlertmanagerConfig
	interval time.Duration
	workers  int
	alerts   string
}

func newAlertFowarder(opts *alertForwarderOptions) (*alertForwarder, error) {
	if len(opts.amHost) == 0 {
		return nil, errors.New("am-host must be specified")
	}

	u := &url.URL{
		Scheme: opts.amScheme,
		Host:   opts.amHost,
		Path:   fmt.Sprintf("/api/%s/alerts", opts.amAPIVersion),
	}

	var accessToken string
	switch {
	case len(opts.amAccessToken) > 0:
		accessToken = opts.amAccessToken
	case len(opts.amAccessTokenFile) > 0:
		data, err := os.ReadFile(opts.amAccessTokenFile)
		if err != nil {
			return nil, err
		}
		accessToken = strings.TrimSpace(string(data))
	default:
		return nil, errors.New("am-access-token or am-access-token-file must be specified")
	}

	var alerts string
	switch {
	case len(opts.alerts) > 0:
		alerts = opts.alerts
	case len(opts.alertsFile) > 0:
		data, err := os.ReadFile(opts.alertsFile)
		if err != nil {
			return nil, err
		}
		alerts = strings.TrimSpace(string(data))
	default:
		return nil, errors.New("alerts or alerts-file must be specified")
	}

	return &alertForwarder{
		amURL:    u.String(),
		amConfig: createAlertmanagerConfig(opts.amHost, opts.amScheme, opts.amAPIVersion, accessToken),
		interval: opts.interval,
		workers:  opts.workers,
		alerts:   alerts,
	}, nil
}

func (af *alertForwarder) Run() error {
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)

	// register the given channel to receive notifications of the specified unix signals
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// start loop in new go routinr for system terminating signal
	go func() {
		sig := <-sigs
		log.Printf("got unix terminating signal: %v\n", sig)
		done <- true
	}()

	// client trace to log whether the request's underlying tcp connection was re-used
	clientTrace := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) { log.Printf("connection was reused: %t\n", info.Reused) },
	}
	traceCtx := httptrace.WithClientTrace(context.Background(), clientTrace)

	// create the http client to send alerts to alertmanager
	client, err := config_util.NewClientFromConfig(af.amConfig.HTTPClientConfig, "alertmanager", config_util.WithHTTP2Disabled())
	if err != nil {
		log.Printf("failed to create the http client: %v\n", err)
		return err
	}

	// start alert forward worker each interval until done signal received
	ticker := time.NewTicker(af.interval)
	log.Println("starting alert forward loop....")
	for {
		select {
		case <-done:
			log.Printf("received terminating signal, shuting down the program...")
			return nil
		case <-ticker.C:
			var wg sync.WaitGroup
			for i := range af.workers {
				log.Printf("sending alerts with worker %d\n", i)
				wg.Add(1)
				go func(index int, client *http.Client, traceCtx context.Context, url string, payload []byte) {
					if err := sendOne(traceCtx, client, url, payload); err != nil {
						log.Printf("failed to send alerts: %v\n", err)
						log.Printf("failed to send alerts to %s: %v\n", url, err)
					}
					wg.Done()
					log.Printf("send routine %d done\n", index)
				}(i, client, traceCtx, af.amURL, []byte(af.alerts))
			}
			wg.Wait()
		}
	}
}

func main() {
	opts := &alertForwarderOptions{
		amScheme:     "https",
		amAPIVersion: "v2",
		interval:     30 * time.Second,
		workers:      1000,
		alertsFile:   "/tmp/alerts.json",
	}
	cmd := &cobra.Command{
		Short:         "Application for forwarding alerts to target Alertmanager.",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, _ []string) error {
			af, err := newAlertFowarder(opts)
			if err != nil {
				log.Printf("failed to create alert forwarder: %v", err)
				return err
			}
			log.Println("alert forwarder is initialized")
			return af.Run()
		},
	}

	cmd.Flags().StringVar(&opts.amHost, "am-host", opts.amHost, "Host for the target alertmanager.")
	cmd.Flags().StringVar(&opts.amScheme, "am-scheme", opts.amScheme, "Scheme for the target alertmanager.")
	cmd.Flags().StringVar(
		&opts.amAPIVersion, "am-apiversion",
		opts.amAPIVersion, "API Version for the target alertmanager.")
	cmd.Flags().StringVar(
		&opts.amAccessToken, "am-access-token",
		opts.amAccessToken, "The bearer token used to authenticate to the target alertmanager.")
	cmd.Flags().StringVar(
		&opts.amAccessTokenFile, "am-access-token-file",
		opts.amAccessTokenFile, "File containing the bearer token used to authenticate to the target alertmanager.")
	cmd.Flags().DurationVar(
		&opts.interval, "interval",
		opts.interval, "The interval between sending alert forward requests.")
	cmd.Flags().IntVar(
		&opts.workers, "workers",
		opts.workers, "The number of concurrent goroutines that forward the alerts.")
	cmd.Flags().StringVar(&opts.alerts, "alerts", opts.alerts, "The sample of alerts.")
	cmd.Flags().StringVar(&opts.alertsFile, "alerts-file", opts.alertsFile, "File containing the sample of alerts.")

	if err := cmd.Execute(); err != nil {
		log.Printf("failed to run command: %v", err)
		os.Exit(1)
	}
}

// createAlertmanagerConfig creates and returns the configuration for the target Alertmanager.
func createAlertmanagerConfig(amHost, amScheme, amAPIVersion, amAccessToken string) *config.AlertmanagerConfig {
	return &config.AlertmanagerConfig{
		APIVersion: config.AlertmanagerAPIVersion(amAPIVersion),
		PathPrefix: "/",
		Scheme:     amScheme,
		Timeout:    model.Duration(10 * time.Second),
		HTTPClientConfig: config_util.HTTPClientConfig{
			Authorization: &config_util.Authorization{
				Type:        "Bearer",
				Credentials: config_util.Secret(amAccessToken),
			},
			TLSConfig: config_util.TLSConfig{
				ServerName:         "",
				InsecureSkipVerify: true,
			},
		},
		ServiceDiscoveryConfigs: discovery.Configs{
			discovery.StaticConfig{
				{
					Source: amHost,
				},
			},
		},
	}
}

// send alerts to alertmanager with one http request.
func sendOne(traceCtx context.Context, c *http.Client, url string, b []byte) error {
	req, err := http.NewRequestWithContext(traceCtx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "testing")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return err
	}

	defer func() {
		/* #nosec */
		// TODO(saswatamcode): Check err here.
		_, _ = io.Copy(io.Discard, resp.Body)
		/* #nosec */
		_ = resp.Body.Close()
	}()

	// Any HTTP status 2xx is OK.
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("bad response status %s", resp.Status)
	}
	return nil
}
