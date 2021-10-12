package main

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"net/http"
	"net/http/httptrace"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/pkg/errors"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
)

var alerts = `[
  {
    "annotations":{
      "description":"just for testing\n",
      "summary":"An alert that is for testing."
    },
    "receivers":[
      {
        "name":"test"
      }
    ],
    "labels":{
      "alertname":"test",
      "cluster":"testCluster",
      "severity":"none"
    }
  }
]`

func main() {
	amHost := os.Getenv("ALERTMANAGER_HOST")
	if amHost == "" {
		log.Println("ALERTMANAGER_HOST must be specified!")
		os.Exit(1)
	}
	amUrl := (&url.URL{
		Scheme: "https",
		Host:   amHost,
		Path:   "/api/v2/alerts",
	}).String()

	amAccessToken := os.Getenv("ALERRTMANAGER_ACCESS_TOKEN")
	if amAccessToken == "" {
		log.Println("ALERRTMANAGER_ACCESS_TOKEN must be specified!")
		os.Exit(1)
	}
	maxAlertSendRoutine := os.Getenv("MAX_ALERT_SEND_ROUTINE")
	maxAlertSendRoutineNumber := 20
	if maxAlertSendRoutine == "" {
		log.Println("MAX_ALERT_SEND_ROUTINE is not specified, fallback to default value: 20")
	} else {
		i, err := strconv.Atoi(maxAlertSendRoutine)
		if err != nil {
			log.Println("invalid MAX_ALERT_SEND_ROUTINE, must be number!")
			os.Exit(1)
		}
		maxAlertSendRoutineNumber = i
	}

	alertSendInterval := os.Getenv("ALERT_SEND_INTERVAL")
	asInterval, err := time.ParseDuration(alertSendInterval)
	if err != nil {
		log.Println("invalid ALERT_SEND_INTERVAL, fallback to default value: 5s")
		asInterval = 5*time.Second
	}

	amCfg := createAlertmanagerConfig(amHost, amAccessToken)

	// client trace to log whether the request's underlying tcp connection was re-used
	clientTrace := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) { log.Printf("conn was reused: %t\n", info.Reused) },
	}
	traceCtx := httptrace.WithClientTrace(context.Background(), clientTrace)

	// create the http client to send alerts to alertmanager
	client, err := config_util.NewClientFromConfig(amCfg.HTTPClientConfig, "alertmanager", config_util.WithHTTP2Disabled())
	if err != nil {
		log.Printf("failed to create the http client: %v\n", err)
		return
	}

	// alerts send loop
	var wg sync.WaitGroup
	for i := 0; i < maxAlertSendRoutineNumber; i++ {
		log.Printf("sending alerts with go routine %d\n", i)
		wg.Add(1)
		go func(index int, client *http.Client, traceCtx context.Context, url string, payload []byte) {
			if err := sendOne(client, traceCtx, url, payload); err != nil {
				log.Printf("failed to send alerts: %v\n", err)
			}
			wg.Done()
			log.Printf("send routine %d done\n", index)
		}(i, client, traceCtx, amUrl, []byte(alerts))

		//sleep 30 for the HAProxy close the client connection
		time.Sleep(asInterval)
	}
	wg.Wait()
}

// createAlertmanagerConfig creates and returns the configuration for the target Alertmanager
func createAlertmanagerConfig(amHost, amAccessToken string) *config.AlertmanagerConfig {
	return &config.AlertmanagerConfig{
		APIVersion: config.AlertmanagerAPIVersionV2,
		PathPrefix: "/",
		Scheme:     "https",
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

// send alerts to alertmanager with one http request
func sendOne(c *http.Client, traceCtx context.Context, url string, b []byte) error {
	req, err := http.NewRequestWithContext(traceCtx, "POST", url, bytes.NewReader(b))
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
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}()

	// Any HTTP status 2xx is OK.
	if resp.StatusCode/100 != 2 {
		return errors.Errorf("bad response status %s", resp.Status)
	}

	return nil
}
