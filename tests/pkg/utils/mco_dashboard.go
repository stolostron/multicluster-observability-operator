// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"

	"k8s.io/klog/v2"
)

const (
	trueStr = "true"
	// grafanaAdminBypassHeader bypasses the OAuth proxy via the internal sidecar port
	grafanaAdminBypassHeader = "WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000"
	// kindGrafanaAdminBypassHeader is the admin user configured in tests/run-in-kind/grafana/grafana-config-test.yaml
	kindGrafanaAdminBypassHeader = "admin"
)

var (
	cachedGrafanaPodName string
	grafanaPodNameMutex  sync.Mutex
)

type DashboardMeta struct {
	ID          int64
	UID         string
	Tags        []string
	FolderTitle string
}

func getGrafanaHttpClient(opt TestOptions) (*http.Client, string, bool, error) {
	client := &http.Client{}
	token := ""
	isKind := os.Getenv("IS_KIND_ENV") == trueStr

	if !isKind {
		tr := &http.Transport{
			// #nosec G402 -- Used in test.
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}

		client = &http.Client{Transport: tr}
		var err error
		token, err = FetchBearerToken(opt)
		if err != nil {
			return nil, "", false, err
		}
	}
	return client, token, isKind, nil
}

func getAdminBypassHeader(isKind bool) string {
	if isKind {
		return kindGrafanaAdminBypassHeader
	}
	return grafanaAdminBypassHeader
}

// doGrafanaGet performs a standard GET request to Grafana as the configured test user.
func doGrafanaGet(ctx context.Context, opt TestOptions, path string) (*http.Response, error) {
	client, token, isKind, err := getGrafanaHttpClient(opt)
	if err != nil {
		return nil, err
	}

	grafanaConsoleURL := GetGrafanaURL(opt)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, grafanaConsoleURL+path, nil)
	if err != nil {
		return nil, err
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	if isKind {
		req.Header.Set("X-Forwarded-User", getAdminBypassHeader(isKind))
	} else {
		req.Host = opt.HubCluster.GrafanaHost
	}

	return client.Do(req)
}

func getGrafanaPodName(ctx context.Context) (string, error) {
	grafanaPodNameMutex.Lock()
	defer grafanaPodNameMutex.Unlock()
	if cachedGrafanaPodName != "" {
		return cachedGrafanaPodName, nil
	}
	podNameCmd := exec.CommandContext(ctx, "oc", "get", "pods", "-n", MCO_NAMESPACE, "-l", "app=multicluster-observability-grafana", "-o", "jsonpath={.items[0].metadata.name}")
	podNameBytes, err := podNameCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get grafana pod name: %w", err)
	}
	cachedGrafanaPodName = strings.TrimSpace(string(podNameBytes))
	return cachedGrafanaPodName, nil
}

// execGrafanaAdminGet performs a GET request to Grafana with admin privileges, bypassing the OAuth proxy on OpenShift.
func execGrafanaAdminGet(ctx context.Context, opt TestOptions, path string) ([]byte, error) {
	isKind := os.Getenv("IS_KIND_ENV") == trueStr
	if isKind {
		// In KinD, we just hit the service directly since there's no complex OAuth proxy blocking preferences
		client, _, _, err := getGrafanaHttpClient(opt)
		if err != nil {
			return nil, err
		}

		grafanaConsoleURL := GetGrafanaURL(opt)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, grafanaConsoleURL+path, nil)
		if err != nil {
			return nil, err
		}

		req.Header.Set("X-Forwarded-User", getAdminBypassHeader(isKind))

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to access grafana api, status: %d", resp.StatusCode)
		}
		return io.ReadAll(resp.Body)
	}

	// For OpenShift, bypass the OAuth proxy using oc exec
	podName, err := getGrafanaPodName(ctx)
	if err != nil {
		return nil, err
	}

	// Use the admin bypass header directly on localhost:3001
	// #nosec G204 -- This is a test utility
	execCmd := exec.CommandContext(
		ctx,
		"oc",
		"exec",
		"-n",
		MCO_NAMESPACE,
		podName,
		"-c",
		"grafana",
		"--",
		"curl",
		"-f",
		"-s",
		"-H",
		"X-Forwarded-User: "+getAdminBypassHeader(isKind),
		"http://localhost:3001"+path,
	)
	out, err := execCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to execute curl (is the path valid?): %w, output: %s", err, string(out))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("grafana api returned empty response for path: %s", path)
	}
	return out, nil
}

func grafanaDecodeJSON[T any](resp *http.Response, errMsg string) (T, error) {
	var result T
	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("%s, status: %d", errMsg, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return result, fmt.Errorf("failed to decode grafana response: %w", err)
	}

	return result, nil
}

func ContainDashboard(opt TestOptions, title string) (bool, error) {
	meta, err := GetDashboardMetadata(context.Background(), opt, title)
	if err != nil {
		klog.Errorf("ContainDashboard error for title '%s': %v", title, err)
		return false, err
	}
	return meta != nil, nil
}

func ContainDashboardByUID(ctx context.Context, opt TestOptions, uid string) (bool, error) {
	resp, err := doGrafanaGet(ctx, opt, "/api/search?type=dash-db")
	if err != nil {
		klog.Errorf("ContainDashboardByUID GET error for uid '%s': %v", uid, err)
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()

	results, err := grafanaDecodeJSON[[]struct {
		UID string `json:"uid"`
	}](resp, "failed to search dashboards for UID resolution")
	if err != nil {
		klog.Errorf("ContainDashboardByUID decode error for uid '%s': %v", uid, err)
		return false, err
	}

	for _, res := range results {
		if res.UID == uid {
			return true, nil
		}
	}

	return false, nil
}

func GetDashboardMetadata(ctx context.Context, opt TestOptions, title string) (*DashboardMeta, error) {
	params := url.Values{}
	params.Add("query", title)
	path := "/api/search?" + params.Encode()

	resp, err := doGrafanaGet(ctx, opt, path)
	if err != nil {
		klog.Errorf("GetDashboardMetadata GET error for title '%s': %v", title, err)
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	type searchResult struct {
		ID          int64    `json:"id"`
		UID         string   `json:"uid"`
		Title       string   `json:"title"`
		Tags        []string `json:"tags"`
		FolderTitle string   `json:"folderTitle"`
	}

	results, err := grafanaDecodeJSON[[]searchResult](resp, "failed to access grafana search api")
	if err != nil {
		klog.Errorf("GetDashboardMetadata decode error for title '%s': %v", title, err)
		return nil, err
	}

	for _, res := range results {
		if res.Title == title {
			return &DashboardMeta{
				ID:          res.ID,
				UID:         res.UID,
				Tags:        res.Tags,
				FolderTitle: res.FolderTitle,
			}, nil
		}
	}

	return nil, nil
}

func GetGrafanaHomeDashboard(ctx context.Context, opt TestOptions) (string, error) {
	// Only this endpoint requires the admin bypass
	data, err := execGrafanaAdminGet(ctx, opt, "/api/org/preferences")
	if err != nil {
		klog.Errorf("GetGrafanaHomeDashboard exec error: %v", err)
		return "", err
	}

	var res struct {
		HomeDashboardUID string `json:"homeDashboardUID"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		klog.Errorf("GetGrafanaHomeDashboard unmarshal error: %v", err)
		return "", fmt.Errorf("failed to access grafana preferences: %w", err)
	}

	return res.HomeDashboardUID, nil
}

func FolderExists(ctx context.Context, opt TestOptions, title string) (bool, error) {
	resp, err := doGrafanaGet(ctx, opt, "/api/search?type=dash-folder")
	if err != nil {
		klog.Errorf("FolderExists GET error for title '%s': %v", title, err)
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()

	results, err := grafanaDecodeJSON[[]struct {
		Title string `json:"title"`
	}](resp, "failed to search folders")
	if err != nil {
		klog.Errorf("FolderExists decode error for title '%s': %v", title, err)
		return false, err
	}

	for _, res := range results {
		if res.Title == title {
			return true, nil
		}
	}

	return false, nil
}
