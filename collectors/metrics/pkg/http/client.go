package http

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	inFlightGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "client_in_flight_requests",
			Help: "A gauge of in-flight requests for the wrapped client.",
		},
		[]string{"client"},
	)

	counter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "client_api_requests_total",
			Help: "A counter for requests from the wrapped client.",
		},
		[]string{"code", "method", "client"},
	)

	dnsLatencyVec = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "dns_duration_seconds",
			Help:    "Trace dns latency histogram.",
			Buckets: []float64{.005, .01, .025, .05},
		},
		[]string{"event", "client"},
	)

	tlsLatencyVec = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tls_duration_seconds",
			Help:    "Trace tls latency histogram.",
			Buckets: []float64{.05, .1, .25, .5},
		},
		[]string{"event", "client"},
	)

	histVec = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "request_duration_seconds",
			Help:    "A histogram of request latencies.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "client"},
	)
)

func init() {
	prometheus.MustRegister(counter, tlsLatencyVec, dnsLatencyVec, histVec, inFlightGauge)
}

func NewInstrumentedRoundTripper(clientName string, next http.RoundTripper) http.RoundTripper {
	trace := &promhttp.InstrumentTrace{
		DNSStart: func(t float64) {
			dnsLatencyVec.
				WithLabelValues("dns_start", clientName).
				Observe(t)
		},
		DNSDone: func(t float64) {
			dnsLatencyVec.
				WithLabelValues("dns_done", clientName).
				Observe(t)
		},
		TLSHandshakeStart: func(t float64) {
			tlsLatencyVec.
				WithLabelValues("tls_handshake_start", clientName).
				Observe(t)
		},
		TLSHandshakeDone: func(t float64) {
			tlsLatencyVec.
				WithLabelValues("tls_handshake_done", clientName).
				Observe(t)
		},
	}

	inFlightGauge := inFlightGauge.WithLabelValues(clientName)

	counter := counter.MustCurryWith(prometheus.Labels{
		"client": clientName,
	})

	histVec := histVec.MustCurryWith(prometheus.Labels{
		"client": clientName,
	})

	return promhttp.InstrumentRoundTripperInFlight(inFlightGauge,
		promhttp.InstrumentRoundTripperCounter(counter,
			promhttp.InstrumentRoundTripperTrace(trace,
				promhttp.InstrumentRoundTripperDuration(histVec, next),
			),
		),
	)
}
