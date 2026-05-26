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
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
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

type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
}

type Client struct {
	client      *http.Client
	maxBytes    int64
	timeout     time.Duration
	metricsName string
	logger      log.Logger

	metrics *ClientMetrics
}

type ClientMetrics struct {
	FederateRequests           *prometheus.CounterVec
	ForwardRemoteWriteRequests *prometheus.CounterVec
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

func (c *Client) RetrieveRecordingMetrics(
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
			c.metrics.FederateRequests.WithLabelValues("recording", "200").Inc()
		case http.StatusUnauthorized:
			c.metrics.FederateRequests.WithLabelValues("recording", "401").Inc()
			return fmt.Errorf("prometheus server requires authentication: %s", resp.Request.URL)
		case http.StatusForbidden:
			c.metrics.FederateRequests.WithLabelValues("recording", "403").Inc()
			return fmt.Errorf("prometheus server forbidden: %s", resp.Request.URL)
		case http.StatusBadRequest:
			c.metrics.FederateRequests.WithLabelValues("recording", "400").Inc()
			return fmt.Errorf("bad request: %s", resp.Request.URL)
		default:
			c.metrics.FederateRequests.WithLabelValues("recording", strconv.Itoa(resp.StatusCode)).Inc()
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
			var ls []labels.Label
			for k, v := range r.Metric {
				l := &labels.Label{
					Name:  k,
					Value: v,
				}
				ls = append(ls, *l)
			}
			vec = append(vec, promql.Sample{
				Metric: ls,
				T:      t,
				F:      v,
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
			protMetric.Untyped.Value = proto.Float64(s.F)

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
	protoDelimFormat := expfmt.NewFormat(expfmt.TypeProtoDelim)
	protoTextFormat := expfmt.NewFormat(expfmt.TypeProtoText)
	req.Header.Set("Accept", strings.Join([]string{string(protoDelimFormat), string(protoTextFormat)}, " , "))

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	req = req.WithContext(ctx)
	defer cancel()

	families := make([]*clientmodel.MetricFamily, 0, 100)
	err := withCancel(ctx, c.client, req, func(resp *http.Response) error {
		switch resp.StatusCode {
		case http.StatusOK:
			c.metrics.FederateRequests.WithLabelValues("normal", "200").Inc()
		case http.StatusUnauthorized:
			c.metrics.FederateRequests.WithLabelValues("normal", "401").Inc()
			return fmt.Errorf("prometheus server requires authentication: %s", resp.Request.URL)
		case http.StatusForbidden:
			c.metrics.FederateRequests.WithLabelValues("normal", "403").Inc()
			return fmt.Errorf("prometheus server forbidden: %s", resp.Request.URL)
		case http.StatusBadRequest:
			c.metrics.FederateRequests.WithLabelValues("normal", "400").Inc()
			return fmt.Errorf("bad request: %s", resp.Request.URL)
		default:
			c.metrics.FederateRequests.WithLabelValues("normal", strconv.Itoa(resp.StatusCode)).Inc()
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

// // TODO(saswatamcode): This is no longer used, remove it in the future.
func Read(r io.Reader) ([]*clientmodel.MetricFamily, error) {
	decompress := snappy.NewReader(r)
	decoder := expfmt.NewDecoder(decompress, expfmt.NewFormat(expfmt.TypeProtoDelim))
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

// TODO(saswatamcode): This is no longer used, remove it in the future.
func Write(w io.Writer, families []*clientmodel.MetricFamily) error {
	// output the filtered set
	compress := snappy.NewBufferedWriter(w)
	encoder := expfmt.NewEncoder(compress, expfmt.NewFormat(expfmt.TypeProtoDelim))
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

func DefaultTransport(logger log.Logger) *http.Transport {
	return &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
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
			dedup[nameLabelName] = struct{}{}
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

	// uncomment here to generate timeseries
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
			return c.sendRequest(ctx, req.URL.String(), compressed)
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

func (c *Client) sendRequest(ctx context.Context, serverURL string, body []byte) error {
	req1, err := http.NewRequest(http.MethodPost, serverURL, bytes.NewBuffer(body))
	if err != nil {
		wrappedErr := fmt.Errorf("failed to create forwarding request: %w", err)
		c.metrics.ForwardRemoteWriteRequests.WithLabelValues("0").Inc()
		return backoff.Permanent(wrappedErr)
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req1 = req1.WithContext(ctx)

	resp, err := c.client.Do(req1)
	if err != nil {
		c.metrics.ForwardRemoteWriteRequests.WithLabelValues("0").Inc()

		wrappedErr := fmt.Errorf("failed to forward request: %w", err)
		if isTransientError(err) {
			return wrappedErr
		}

		return backoff.Permanent(wrappedErr)
	}

	c.metrics.ForwardRemoteWriteRequests.WithLabelValues(strconv.Itoa(resp.StatusCode)).Inc()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// surfacing upstreams error to our users too
		defer resp.Body.Close()
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.Log(c.logger, logger.Warn, err)
		}

		retErr := &HTTPError{
			StatusCode: resp.StatusCode,
			Message:    string(bodyBytes),
		}

		if isTransientResponseError(resp) {
			return retErr
		}

		return backoff.Permanent(retErr)
	}

	return nil
}

func isTransientError(err error) bool {
	if urlErr, ok := err.(*url.Error); ok {
		return urlErr.Timeout()
	}

	return false
}

func isTransientResponseError(resp *http.Response) bool {
	if resp.StatusCode >= 500 && resp.StatusCode != http.StatusNotImplemented {
		return true
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}

	return false
}
