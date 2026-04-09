// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
)

const (
	trueStr = "true"
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
	if !isKind {
		req.Host = opt.HubCluster.GrafanaHost
	}

	return client.Do(req)
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
		return false, err
	}
	return meta != nil, nil
}

func GetDashboardMetadata(ctx context.Context, opt TestOptions, title string) (*DashboardMeta, error) {
	params := url.Values{}
	params.Add("query", title)
	path := "/api/search?" + params.Encode()
	resp, err := doGrafanaGet(ctx, opt, path)
	if err != nil {
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

func GetGrafanaHomeDashboard(ctx context.Context, opt TestOptions) (int64, error) {
	resp, err := doGrafanaGet(ctx, opt, "/api/org/preferences")
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	res, err := grafanaDecodeJSON[struct {
		HomeDashboardID int64 `json:"homeDashboardId"`
	}](resp, "failed to access grafana preferences")
	if err != nil {
		return 0, err
	}

	return res.HomeDashboardID, nil
}

func GetDashboardUIDByID(ctx context.Context, opt TestOptions, id int64) (string, error) {
	if id == 0 {
		return "", nil
	}
	// Grafana search API doesn't have a direct "by id" filter, so we list all and find it.
	// This is only used for verification in tests.
	resp, err := doGrafanaGet(ctx, opt, "/api/search?type=dash-db")
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	results, err := grafanaDecodeJSON[[]struct {
		ID  int64  `json:"id"`
		UID string `json:"uid"`
	}](resp, "failed to search dashboards for UID resolution")
	if err != nil {
		return "", err
	}

	for _, res := range results {
		if res.ID == id {
			return res.UID, nil
		}
	}

	return "", nil
}

func FolderExists(ctx context.Context, opt TestOptions, title string) (bool, error) {
	resp, err := doGrafanaGet(ctx, opt, "/api/search?type=dash-folder")
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()

	results, err := grafanaDecodeJSON[[]struct {
		Title string `json:"title"`
	}](resp, "failed to search folders")
	if err != nil {
		return false, err
	}

	for _, res := range results {
		if res.Title == title {
			return true, nil
		}
	}

	return false, nil
}
