// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package health

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/informer"
	"k8s.io/klog/v2"
)

// Checker implements the HTTP handlers for liveness and readiness probes.
type Checker struct {
	informer               informer.ManagedClusterInformable
	metricsServerURL       *url.URL
	metricsServerTransport http.RoundTripper
}

// NewChecker creates a new health checker.
func NewChecker(informer informer.ManagedClusterInformable, metricsTransport http.RoundTripper, metricsURL *url.URL) *Checker {
	return &Checker{
		informer:               informer,
		metricsServerURL:       metricsURL,
		metricsServerTransport: metricsTransport,
	}
}

// ServeHTTP handles incoming health check requests.
func (c *Checker) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	switch req.URL.Path {
	case "/healthz":
		c.healthz(res)
	case "/readyz":
		c.readyz(res)
	default:
		http.NotFound(res, req)
	}
}

// healthz is the liveness probe handler.
func (c *Checker) healthz(res http.ResponseWriter) {
	res.WriteHeader(http.StatusOK)
	fmt.Fprint(res, "OK")
}

// readyz is the readiness probe handler.
func (c *Checker) readyz(res http.ResponseWriter) {
	// 1. Check if the informer has synced.
	if !c.informer.HasSynced() {
		klog.Warning("Readiness probe failed: informer has not synced")
		http.Error(res, "Informer has not synced", http.StatusServiceUnavailable)
		return
	}

	// 2. Check the upstream metrics server connection.
	if err := c.checkMetricsServer(); err != nil {
		klog.Warningf("Readiness probe failed: upstream metrics server check failed: %v", err)
		http.Error(res, fmt.Sprintf("Upstream metrics server check failed: %v", err), http.StatusServiceUnavailable)
		return
	}

	res.WriteHeader(http.StatusOK)
	fmt.Fprint(res, "OK")
}

func (c *Checker) checkMetricsServer() error {
	if c.metricsServerURL == nil || c.metricsServerTransport == nil {
		return fmt.Errorf("metrics server URL or transport is not configured")
	}

	client := &http.Client{Transport: c.metricsServerTransport}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// We use a HEAD request because we only need to verify that the mTLS connection
	// can be established. We don't need a body, and any successful HTTP response
	// (even a 403 or 404) proves that the connection was successful.
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, c.metricsServerURL.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Any response code below 500 indicates a successful connection, even if it's
	// an auth error (401/403) or not found (404).
	if resp.StatusCode >= http.StatusInternalServerError {
		return fmt.Errorf("received server error status code: %d", resp.StatusCode)
	}

	return nil
}
