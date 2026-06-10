// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"hash/fnv"
	"io"
	"net/http"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

const (
	defaultAdmin = "WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000"
	// maxDashboardSize limits the size of the dashboard JSON we are willing to buffer in memory (10MB).
	maxDashboardSize = 10 * 1024 * 1024
	// grafanaUIDMaxLength is the maximum length allowed for a Grafana dashboard UID.
	grafanaUIDMaxLength = 40
)

var (
	httpClient *http.Client
	once       sync.Once
)

// GenerateUID generates UID for customized dashboard.
func GenerateUID(namespace string, name string) (string, error) {
	uid := namespace + "-" + name
	if len(uid) > grafanaUIDMaxLength {
		hasher := fnv.New128a()
		_, err := hasher.Write([]byte(uid))
		if err != nil {
			return "", err
		}
		uid = hex.EncodeToString(hasher.Sum(nil))
	}
	return uid, nil
}

// getHTTPClient returns a singleton http client.
func getHTTPClient() *http.Client {
	once.Do(func() {
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		}
	})
	return httpClient
}

// SendRequest sends an HTTP request with context support.
func SendRequest(ctx context.Context, client *http.Client, method string, url string, body io.Reader) ([]byte, int) {
	if client == nil {
		client = getHTTPClient()
	}

	var payload []byte
	if body != nil {
		var err error
		// Read up to maxDashboardSize + 1 to detect if the payload is too large.
		payload, err = io.ReadAll(io.LimitReader(body, maxDashboardSize+1))
		if err != nil {
			klog.ErrorS(err, "failed to read request body")
			return nil, http.StatusInternalServerError
		}
		if len(payload) > maxDashboardSize {
			klog.ErrorS(errors.New("payload too large"), "dashboard payload exceeds size limit", "limit", maxDashboardSize)
			return nil, http.StatusRequestEntityTooLarge
		}
	}

	var reqBody io.Reader
	if payload != nil {
		reqBody = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		klog.ErrorS(err, "failed to create HTTP request", "method", method, "url", url)
		return nil, http.StatusInternalServerError
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-User", defaultAdmin)

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, http.StatusRequestTimeout
		}
		klog.ErrorS(err, "failed to send HTTP request", "method", method, "url", url)
		return nil, http.StatusServiceUnavailable
	}

	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			klog.V(2).InfoS("failed to close response body", "error", closeErr)
		}
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		klog.ErrorS(err, "failed to read response body")
		return nil, resp.StatusCode
	}

	return respBody, resp.StatusCode
}
