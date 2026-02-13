// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package simulator

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/log"
	clientmodel "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	rlogger "github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/logger"
)

const (
	defaultMetrisNumber = 1000
	defaultLabelNumber  = 19
	metricsNamePrefix   = "simulated_metrics"
	labelPrefix         = "label"
	labelValuePrefix    = "label-value-prefix"
)

func SimulateMetrics(logger log.Logger) []*clientmodel.MetricFamily {
	metrisNumber, err := strconv.Atoi(os.Getenv("SIMULATE_METRICS_NUM"))
	if err != nil {
		metrisNumber = defaultMetrisNumber
	}
	labelNumber, err := strconv.Atoi(os.Getenv("SIMULATE_LABEL_NUM"))
	if err != nil {
		labelNumber = defaultLabelNumber
	}

	families := make([]*clientmodel.MetricFamily, 0, 100)
	timestamp := time.Now().UnixNano() / int64(time.Millisecond)
	var sb strings.Builder
	for i := range metrisNumber {
		sb.WriteString(fmt.Sprintf("%s_%d{", metricsNamePrefix, i/1000))
		for j := range labelNumber {
			if j == 0 {
				sb.WriteString(fmt.Sprintf("%s_%d=\"%s-%d--%d\"", labelPrefix, j, labelValuePrefix, i/10, i%10))
			} else {
				sb.WriteString(fmt.Sprintf("%s_%d=\"%s-%d\"", labelPrefix, j, labelValuePrefix, i%10))
			}

			if j != labelNumber-1 {
				sb.WriteString(",")
			}
		}
		sb.WriteString("} ")
		sb.WriteString(fmt.Sprintf("%f %d", randFloat64(), timestamp))
		sb.WriteString("\n")
	}
	// rlogger.Log(logger, rlogger.Error, "data", sb.String())
	r := io.NopCloser(bytes.NewReader([]byte(sb.String())))
	decoder := expfmt.NewDecoder(r, expfmt.NewFormat(expfmt.TypeProtoText))
	for {
		family := &clientmodel.MetricFamily{}
		families = append(families, family)
		if err := decoder.Decode(family); err != nil {
			if !errors.Is(err, io.EOF) {
				rlogger.Log(logger, rlogger.Error, "msg", "error reading body", "err", err)
			}
			break
		}
	}

	return families
}

func randFloat64() float64 {
	nBig, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return 0
	}

	return (float64(nBig.Int64()) / float64(1<<62))
}

func FetchSimulatedTimeseries(timeseriesFile string) ([]*clientmodel.MetricFamily, error) {
	timestamp := time.Now().UnixNano() / int64(time.Millisecond)

	reader, err := os.Open(filepath.Clean(timeseriesFile))
	if err != nil {
		return nil, err
	}

	parser := expfmt.NewTextParser(model.LegacyValidation)

	parsed, err := parser.TextToMetricFamilies(reader)
	if err != nil {
		return nil, err
	}
	families := make([]*clientmodel.MetricFamily, 0, len(parsed))
	for _, mf := range parsed {
		for _, m := range mf.Metric {
			m.TimestampMs = &timestamp
		}
		families = append(families, mf)
	}
	return families, nil
}
