// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package metricsclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/go-kit/log"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"
	clientmodel "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql"

	"github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/logger"
	"github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/reader"
)

const (
	nameLabelName   = "__name__"
	maxSeriesLength = 10000
)

type Client struct {
	client      *http.Client
	maxBytes    int64
	timeout     time.Duration
	metricsName string
	logger      log.Logger

	metrics *ClientMetrics
}

type ClientMetrics struct {
	GaugeRequestRetrieve *prometheus.GaugeVec
	GaugeRequestSend     *prometheus.GaugeVec
}

type PartitionedMetrics struct {
	Families []*clientmodel.MetricFamily
}

func New(logger log.Logger, metrics *ClientMetrics, client *http.Client, maxBytes int64, timeout time.Duration, metricsName string) *Client {
	return &Client{
		client:      client,
		maxBytes:    maxBytes,
		timeout:     timeout,
		metricsName: metricsName,
		logger:      log.With(logger, "component", "metricsclient"),
		metrics:     metrics,
	}
}

type MetricsJson struct {
	Status string      `json:"status"`
	Data   MetricsData `json:"data"`
}

type MetricsData struct {
	Type   string          `json:"resultType"`
	Result []MetricsResult `json:"result"`
}

type MetricsResult struct {
	Metric map[string]string `json:"metric"`
	Value  []interface{}     `json:"value"`
}

func (c *Client) RetrievRecordingMetrics(
	ctx context.Context,
	req *http.Request,
	name string) ([]*clientmodel.MetricFamily, error) {

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	req = req.WithContext(ctx)
	defer cancel()
	families := make([]*clientmodel.MetricFamily, 0, 100)
	err := withCancel(ctx, c.client, req, func(resp *http.Response) error {
		switch resp.StatusCode {
		case http.StatusOK:
			c.metrics.GaugeRequestRetrieve.WithLabelValues(c.metricsName, "200").Inc()
		case http.StatusUnauthorized:
			c.metrics.GaugeRequestRetrieve.WithLabelValues(c.metricsName, "401").Inc()
			return fmt.Errorf("prometheus server requires authentication: %s", resp.Request.URL)
		case http.StatusForbidden:
			c.metrics.GaugeRequestRetrieve.WithLabelValues(c.metricsName, "403").Inc()
			return fmt.Errorf("prometheus server forbidden: %s", resp.Request.URL)
		case http.StatusBadRequest:
			c.metrics.GaugeRequestRetrieve.WithLabelValues(c.metricsName, "400").Inc()
			return fmt.Errorf("bad request: %s", resp.Request.URL)
		default:
			c.metrics.GaugeRequestRetrieve.WithLabelValues(c.metricsName, strconv.Itoa(resp.StatusCode)).Inc()
			return fmt.Errorf("prometheus server reported unexpected error code: %d", resp.StatusCode)
		}

		decoder := json.NewDecoder(resp.Body)
		var data MetricsJson
		err := decoder.Decode(&data)
		if err != nil {
			logger.Log(c.logger, logger.Error, "msg", "failed to decode", "err", err)
			return nil
		}
		vec := make(promql.Vector, 0, 100)
		for _, r := range data.Data.Result {
			var t int64
			var v float64
			t = int64(r.Value[0].(float64) * 1000)
			v, _ = strconv.ParseFloat(r.Value[1].(string), 64)
			ls := []labels.Label{}
			for k, v := range r.Metric {
				l := &labels.Label{
					Name:  k,
					Value: v,
				}
				ls = append(ls, *l)
			}
			vec = append(vec, promql.Sample{
				Metric: ls,
				Point:  promql.Point{T: t, V: v},
			})
		}

		for _, s := range vec {
			protMetric := &clientmodel.Metric{
				Untyped: &clientmodel.Untyped{},
			}
			protMetricFam := &clientmodel.MetricFamily{
				Type: clientmodel.MetricType_UNTYPED.Enum(),
				Name: proto.String(name),
			}
			for _, l := range s.Metric {
				if l.Value == "" {
					// No value means unset. Never consider those labels.
					// This is also important to protect against nameless metrics.
					continue
				}
				protMetric.Label = append(protMetric.Label, &clientmodel.LabelPair{
					Name:  proto.String(l.Name),
					Value: proto.String(l.Value),
				})
			}

			protMetric.TimestampMs = proto.Int64(s.T)
			protMetric.Untyped.Value = proto.Float64(s.V)

			protMetricFam.Metric = append(protMetricFam.Metric, protMetric)
			families = append(families, protMetricFam)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return families, nil
}

func (c *Client) Retrieve(ctx context.Context, req *http.Request) ([]*clientmodel.MetricFamily, error) {
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	req.Header.Set("Accept", strings.Join([]string{string(expfmt.FmtProtoDelim), string(expfmt.FmtText)}, " , "))

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	req = req.WithContext(ctx)
	defer cancel()

	families := make([]*clientmodel.MetricFamily, 0, 100)
	err := withCancel(ctx, c.client, req, func(resp *http.Response) error {
		switch resp.StatusCode {
		case http.StatusOK:
			c.metrics.GaugeRequestRetrieve.WithLabelValues(c.metricsName, "200").Inc()
		case http.StatusUnauthorized:
			c.metrics.GaugeRequestRetrieve.WithLabelValues(c.metricsName, "401").Inc()
			return fmt.Errorf("prometheus server requires authentication: %s", resp.Request.URL)
		case http.StatusForbidden:
			c.metrics.GaugeRequestRetrieve.WithLabelValues(c.metricsName, "403").Inc()
			return fmt.Errorf("prometheus server forbidden: %s", resp.Request.URL)
		case http.StatusBadRequest:
			c.metrics.GaugeRequestRetrieve.WithLabelValues(c.metricsName, "400").Inc()
			return fmt.Errorf("bad request: %s", resp.Request.URL)
		default:
			c.metrics.GaugeRequestRetrieve.WithLabelValues(c.metricsName, strconv.Itoa(resp.StatusCode)).Inc()
			return fmt.Errorf("prometheus server reported unexpected error code: %d", resp.StatusCode)
		}

		// read the response into memory
		format := expfmt.ResponseFormat(resp.Header)
		r := &reader.LimitedReader{R: resp.Body, N: c.maxBytes}
		decoder := expfmt.NewDecoder(r, format)
		for {
			family := &clientmodel.MetricFamily{}
			families = append(families, family)
			if err := decoder.Decode(family); err != nil {
				if err != io.EOF {
					logger.Log(c.logger, logger.Error, "msg", "error reading body", "err", err)
				}
				break
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return families, nil
}

func (c *Client) Send(ctx context.Context, req *http.Request, families []*clientmodel.MetricFamily) error {
	buf := &bytes.Buffer{}
	if err := Write(buf, families); err != nil {
		return err
	}

	if req.Header == nil {
		req.Header = make(http.Header)
	}
	req.Header.Set("Content-Type", string(expfmt.FmtProtoDelim))
	req.Header.Set("Content-Encoding", "snappy")
	req.Body = io.NopCloser(buf)

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	req = req.WithContext(ctx)
	defer cancel()
	logger.Log(c.logger, logger.Debug, "msg", "start to send")
	return withCancel(ctx, c.client, req, func(resp *http.Response) error {
		defer func() {
			if _, err := io.Copy(io.Discard, resp.Body); err != nil {
				logger.Log(c.logger, logger.Error, "msg", "error copying body", "err", err)
			}
			if err := resp.Body.Close(); err != nil {
				logger.Log(c.logger, logger.Error, "msg", "error closing body", "err", err)
			}
		}()
		logger.Log(c.logger, logger.Debug, "msg", resp.StatusCode)
		switch resp.StatusCode {
		case http.StatusOK:
			c.metrics.GaugeRequestSend.WithLabelValues(c.metricsName, "200").Inc()
		case http.StatusUnauthorized:
			c.metrics.GaugeRequestSend.WithLabelValues(c.metricsName, "401").Inc()
			return fmt.Errorf("gateway server requires authentication: %s", resp.Request.URL)
		case http.StatusForbidden:
			c.metrics.GaugeRequestSend.WithLabelValues(c.metricsName, "403").Inc()
			return fmt.Errorf("gateway server forbidden: %s", resp.Request.URL)
		case http.StatusBadRequest:
			c.metrics.GaugeRequestSend.WithLabelValues(c.metricsName, "400").Inc()
			logger.Log(c.logger, logger.Debug, "msg", resp.Body)
			return fmt.Errorf("gateway server bad request: %s", resp.Request.URL)
		default:
			c.metrics.GaugeRequestSend.WithLabelValues(c.metricsName, strconv.Itoa(resp.StatusCode)).Inc()
			body, _ := io.ReadAll(resp.Body)
			if len(body) > 1024 {
				body = body[:1024]
			}
			return fmt.Errorf("gateway server reported unexpected error code: %d: %s", resp.StatusCode, string(body))
		}

		return nil
	})
}

func Read(r io.Reader) ([]*clientmodel.MetricFamily, error) {
	decompress := snappy.NewReader(r)
	decoder := expfmt.NewDecoder(decompress, expfmt.FmtProtoDelim)
	families := make([]*clientmodel.MetricFamily, 0, 100)
	for {
		family := &clientmodel.MetricFamily{}
		if err := decoder.Decode(family); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		families = append(families, family)
	}
	return families, nil
}

func Write(w io.Writer, families []*clientmodel.MetricFamily) error {
	// output the filtered set
	compress := snappy.NewBufferedWriter(w)
	encoder := expfmt.NewEncoder(compress, expfmt.FmtProtoDelim)
	for _, family := range families {
		if family == nil {
			continue
		}
		if err := encoder.Encode(family); err != nil {
			return err
		}
	}
	if err := compress.Flush(); err != nil {
		return err
	}
	return nil
}

func withCancel(ctx context.Context, client *http.Client, req *http.Request, fn func(*http.Response) error) error {
	resp, err := client.Do(req)
	// TODO(saswatamcode): Check error.
	//nolint:errcheck
	defer func() error {
		if resp != nil {
			if err = resp.Body.Close(); err != nil {
				return err
			}
		}
		return nil
	}()
	if err != nil {
		return err
	}

	done := make(chan struct{})
	go func() {
		err = fn(resp)
		close(done)
	}()

	select {
	case <-ctx.Done():
		closeErr := resp.Body.Close()

		// wait for the goroutine to finish.
		<-done

		// err is propagated from the goroutine above
		// if it is nil, we bubble up the close err, if any.
		if err == nil {
			err = closeErr
		}

		// if there is no close err,
		// we propagate the context context error.
		if err == nil {
			err = ctx.Err()
		}
	case <-done:
		// propagate the err from the spawned goroutine, if any.
	}

	return err
}

func MTLSTransport(logger log.Logger, caCertFile, tlsCrtFile, tlsKeyFile string) (*http.Transport, error) {
	testMode := os.Getenv("UNIT_TEST") != ""
	if testMode {
		caCertFile = "../../testdata/tls/ca.crt"
		tlsKeyFile = "../../testdata/tls/tls.key"
		tlsCrtFile = "../../testdata/tls/tls.crt"
	}
	// Load Server CA cert
	var caCert []byte
	var err error

	caCert, err = os.ReadFile(filepath.Clean(caCertFile))
	if err != nil {
		return nil, fmt.Errorf("failed to load server ca cert file: %w", err)
	}

	// Load client cert signed by Client CA
	cert, err := tls.LoadX509KeyPair(tlsCrtFile, tlsKeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load client ca cert: %w", err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	if os.Getenv("HTTPS_PROXY_CA_BUNDLE") != "" {
		customCaCert, err := base64.StdEncoding.DecodeString(os.Getenv("HTTPS_PROXY_CA_BUNDLE"))
		logger.Log(logger, logger.Log("msg", "caCert", "caCert", caCert))
		if err != nil {
			return nil, fmt.Errorf("failed to decode server ca cert: %w", err)
		}
		caCertPool.AppendCertsFromPEM(customCaCert)
	}

	// Setup HTTPS client
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS12,
	}
	return &http.Transport{
		Dial: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   true,
		TLSClientConfig:     tlsConfig,
	}, nil

}

func DefaultTransport(logger log.Logger, isTLS bool) *http.Transport {
	return &http.Transport{
		Dial: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   true,
	}
}

func convertToTimeseries(p *PartitionedMetrics, now time.Time) ([]prompb.TimeSeries, error) {
	var timeseries []prompb.TimeSeries

	timestamp := now.UnixNano() / int64(time.Millisecond)
	for _, f := range p.Families {
		for _, m := range f.Metric {
			var ts prompb.TimeSeries

			labelpairs := []prompb.Label{{
				Name:  nameLabelName,
				Value: *f.Name,
			}}

			dedup := make(map[string]struct{})
			for _, l := range m.Label {
				// Skip empty labels.
				if *l.Name == "" || *l.Value == "" {
					continue
				}
				// Check for duplicates
				if _, ok := dedup[*l.Name]; ok {
					continue
				}
				labelpairs = append(labelpairs, prompb.Label{
					Name:  *l.Name,
					Value: *l.Value,
				})
				dedup[*l.Name] = struct{}{}
			}

			s := prompb.Sample{
				Timestamp: *m.TimestampMs,
			}
			// If the sample is in the future, overwrite it.
			if *m.TimestampMs > timestamp {
				s.Timestamp = timestamp
			}

			switch *f.Type {
			case clientmodel.MetricType_COUNTER:
				s.Value = *m.Counter.Value
			case clientmodel.MetricType_GAUGE:
				s.Value = *m.Gauge.Value
			case clientmodel.MetricType_UNTYPED:
				s.Value = *m.Untyped.Value
			default:
				return nil, fmt.Errorf("metric type %s not supported", f.Type.String())
			}

			ts.Labels = append(ts.Labels, labelpairs...)
			sortLabels(ts.Labels)

			ts.Samples = append(ts.Samples, s)

			timeseries = append(timeseries, ts)
		}
	}

	return timeseries, nil
}

func sortLabels(labels []prompb.Label) {
	lset := sortableLabels(labels)
	sort.Sort(&lset)
}

// Extension on top of prompb.Label to allow for easier sorting.
// Based on https://github.com/prometheus/prometheus/blob/main/model/labels/labels.go#L44
type sortableLabels []prompb.Label

func (sl *sortableLabels) Len() int           { return len(*sl) }
func (sl *sortableLabels) Swap(i, j int)      { (*sl)[i], (*sl)[j] = (*sl)[j], (*sl)[i] }
func (sl *sortableLabels) Less(i, j int) bool { return (*sl)[i].Name < (*sl)[j].Name }

// RemoteWrite is used to push the metrics to remote thanos endpoint.
func (c *Client) RemoteWrite(ctx context.Context, req *http.Request,
	families []*clientmodel.MetricFamily, interval time.Duration) error {

	timeseries, err := convertToTimeseries(&PartitionedMetrics{Families: families}, time.Now())
	if err != nil {
		msg := "failed to convert timeseries"
		logger.Log(c.logger, logger.Warn, "msg", msg, "err", err)
		return errors.New(msg)
	}

	if len(timeseries) == 0 {
		logger.Log(c.logger, logger.Info, "msg", "no time series to forward to receive endpoint")
		return nil
	}
	logger.Log(c.logger, logger.Debug, "timeseries number", len(timeseries))

	//uncomment here to generate timeseries
	/*
		for i := 0; i < len(families); i++ {
			var buff bytes.Buffer
			textEncoder := expfmt.NewEncoder(&buff, expfmt.FmtText)
			err = textEncoder.Encode(families[i])
			if err != nil {
				logger.Log(c.logger, logger.Error, "unexpected error during encode", err.Error())
			}
			fmt.Println(string(buff.Bytes()))
		}
	*/

	for i := 0; i < len(timeseries); i += maxSeriesLength {
		length := len(timeseries)
		if i+maxSeriesLength < length {
			length = i + maxSeriesLength
		}
		subTimeseries := timeseries[i:length]

		wreq := &prompb.WriteRequest{Timeseries: subTimeseries}
		data, err := proto.Marshal(wreq)
		if err != nil {
			msg := "failed to marshal proto"
			logger.Log(c.logger, logger.Warn, "msg", msg, "err", err)
			return errors.New(msg)
		}
		compressed := snappy.Encode(nil, data)

		// retry RemoteWrite with exponential back-off
		b := backoff.NewExponentialBackOff()
		// Do not set max elapsed time more than half the scrape interval
		halfInterval := len(timeseries) * 2 / maxSeriesLength
		if halfInterval < 2 {
			halfInterval = 2
		}
		b.MaxElapsedTime = interval / time.Duration(halfInterval)
		retryable := func() error {
			return c.sendRequest(req.URL.String(), compressed)
		}
		notify := func(err error, t time.Duration) {
			msg := fmt.Sprintf("error: %v happened at time: %v", err, t)
			logger.Log(c.logger, logger.Warn, "msg", msg)
		}
		err = backoff.RetryNotify(retryable, b, notify)
		if err != nil {
			return err
		}
	}
	logger.Log(c.logger, logger.Info, "msg", "metrics pushed successfully")
	return nil
}

func (c *Client) sendRequest(serverURL string, body []byte) error {
	req1, err := http.NewRequest(http.MethodPost, serverURL, bytes.NewBuffer(body))
	if err != nil {
		msg := "failed to create forwarding request"
		logger.Log(c.logger, logger.Warn, "msg", msg, "err", err)
		return errors.New(msg)
	}

	//req.Header.Add("THANOS-TENANT", tenantID)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req1 = req1.WithContext(ctx)

	resp, err := c.client.Do(req1)
	if err != nil {
		msg := "failed to forward request"
		logger.Log(c.logger, logger.Warn, "msg", msg, "err", err)
		return errors.New(msg)
	}

	if resp.StatusCode/100 != 2 {
		// surfacing upstreams error to our users too
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.Log(c.logger, logger.Warn, err)
		}
		bodyString := string(bodyBytes)
		msg := fmt.Sprintf("response status code is %s, response body is %s", resp.Status, bodyString)
		logger.Log(c.logger, logger.Warn, msg)
		return errors.New(msg)

	}
	return nil
}
