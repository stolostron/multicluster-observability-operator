// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/stolostron/multicluster-observability-operator/loaders/dashboards/pkg/util"
)

const (
	// Grafana API Paths
	apiSearch               = "/api/search"
	apiDashboards           = "/api/dashboards/db"
	apiDashUID              = "/api/dashboards/uid/"
	apiFolders              = "/api/folders"
	apiFoldersID            = "/api/folders/id/"
	folderPermissionsSuffix = "/permissions"
	apiPreferences          = "/api/org/preferences"

	// Grafana Error Messages
	errVersionMismatch = "version-mismatch"
	errNameExists      = "name-exists"
)

// GrafanaError captures details of a failed Grafana API call
type GrafanaError struct {
	Status int
	Body   string
}

func (e *GrafanaError) Error() string {
	return fmt.Sprintf("grafana error (status %d): %s", e.Status, e.Body)
}

type grafanaErrorResponse struct {
	Message string `json:"message"`
}

func (e *GrafanaError) bodyMatches(keyword string) bool {
	if e.Status != http.StatusPreconditionFailed {
		return false
	}
	if e.Body == keyword {
		return true
	}
	var resp grafanaErrorResponse
	if err := json.Unmarshal([]byte(e.Body), &resp); err == nil {
		return resp.Message == keyword
	}
	return false
}

// IsVersionMismatch returns true if the error is a Grafana 412 version mismatch
func (e *GrafanaError) IsVersionMismatch() bool {
	return e.bodyMatches(errVersionMismatch)
}

// IsNameExists returns true if the error indicates a dashboard name collision
func (e *GrafanaError) IsNameExists() bool {
	return e.bodyMatches(errNameExists)
}

// grafanaDashboard represents the structure returned by Grafana search
type grafanaDashboard struct {
	UID       string `json:"uid"`
	FolderUID string `json:"folderUid"`
}

// grafanaFolder represents a Grafana folder
type grafanaFolder struct {
	ID    int64  `json:"id"`
	UID   string `json:"uid"`
	Title string `json:"title"`
}

// GrafanaClient defines the interface for interacting with Grafana
type GrafanaClient interface {
	ListAllDashboards(ctx context.Context) ([]grafanaDashboard, error)
	DeleteDashboard(ctx context.Context, uid string) error
	CreateOrUpdateDashboard(ctx context.Context, dashboard map[string]any, folderID int64) (int64, error)
	SetHomeDashboard(ctx context.Context, id int64) error

	ListFolders(ctx context.Context) ([]grafanaFolder, error)
	GetFolderByID(ctx context.Context, id int64) (*grafanaFolder, error)
	CreateFolder(ctx context.Context, title string) (*grafanaFolder, error)
	DeleteFolder(ctx context.Context, uid string) error
	HasPermissions(ctx context.Context, uid string) (bool, error)
	IsEmpty(ctx context.Context, uid string) (bool, error)
}

type grafanaClient struct {
	uri string
}

func (g *grafanaClient) ListAllDashboards(ctx context.Context) ([]grafanaDashboard, error) {
	params := url.Values{}
	params.Add("type", "dash-db")
	targetURL := g.uri + apiSearch + "?" + params.Encode()
	body, status := util.SendRequest(ctx, nil, http.MethodGet, targetURL, nil)
	if status != http.StatusOK {
		return nil, &GrafanaError{Status: status, Body: string(body)}
	}
	var res []grafanaDashboard
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}
	return res, nil
}

func (g *grafanaClient) DeleteDashboard(ctx context.Context, uid string) error {
	targetURL := g.uri + apiDashUID + uid
	body, status := util.SendRequest(ctx, nil, http.MethodDelete, targetURL, nil)
	if status != http.StatusOK && status != http.StatusNotFound {
		return &GrafanaError{Status: status, Body: string(body)}
	}
	return nil
}

func (g *grafanaClient) CreateOrUpdateDashboard(ctx context.Context, dashboard map[string]any, folderID int64) (int64, error) {
	data := map[string]any{
		"folderId": folderID,
		// We always overwrite to enforce the "Stateless Exclusive Ownership" model.
		// Kubernetes is the absolute source of truth; any manual changes in the
		// Grafana UI are intentionally disregarded and overwritten.
		"overwrite": true,
		"dashboard": dashboard,
	}
	b, err := json.Marshal(data)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal dashboard payload: %w", err)
	}
	targetURL := g.uri + apiDashboards
	body, status := util.SendRequest(ctx, nil, http.MethodPost, targetURL, bytes.NewBuffer(b))
	if status != http.StatusOK {
		return 0, &GrafanaError{Status: status, Body: string(body)}
	}
	var resp struct {
		ID *int64 `json:"id"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("failed to unmarshal Grafana response: %w (body: %s)", err, truncate(string(body), 256))
	}
	if resp.ID == nil {
		return 0, fmt.Errorf("invalid Grafana dashboard response: missing id (body: %s)", truncate(string(body), 256))
	}
	return *resp.ID, nil
}

func (g *grafanaClient) SetHomeDashboard(ctx context.Context, id int64) error {
	data := map[string]int64{"homeDashboardId": id}
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal preferences payload: %w", err)
	}
	targetURL := g.uri + apiPreferences
	body, status := util.SendRequest(ctx, nil, http.MethodPut, targetURL, bytes.NewBuffer(b))
	if status != http.StatusOK {
		return &GrafanaError{Status: status, Body: string(body)}
	}
	return nil
}

func (g *grafanaClient) ListFolders(ctx context.Context) ([]grafanaFolder, error) {
	targetURL := g.uri + apiFolders
	body, status := util.SendRequest(ctx, nil, http.MethodGet, targetURL, nil)
	if status != http.StatusOK {
		return nil, &GrafanaError{Status: status, Body: string(body)}
	}
	var res []grafanaFolder
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}
	return res, nil
}

func (g *grafanaClient) GetFolderByID(ctx context.Context, id int64) (*grafanaFolder, error) {
	targetURL := fmt.Sprintf("%s%s%d", g.uri, apiFoldersID, id)
	body, status := util.SendRequest(ctx, nil, http.MethodGet, targetURL, nil)
	if status != http.StatusOK {
		return nil, &GrafanaError{Status: status, Body: string(body)}
	}
	var res grafanaFolder
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (g *grafanaClient) CreateFolder(ctx context.Context, title string) (*grafanaFolder, error) {
	payload, err := json.Marshal(map[string]string{"title": title})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal folder payload: %w", err)
	}
	targetURL := g.uri + apiFolders
	body, status := util.SendRequest(ctx, nil, http.MethodPost, targetURL, bytes.NewReader(payload))
	if status != http.StatusOK {
		return nil, &GrafanaError{Status: status, Body: string(body)}
	}
	var res grafanaFolder
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (g *grafanaClient) DeleteFolder(ctx context.Context, uid string) error {
	targetURL := g.uri + apiFolders + "/" + uid
	body, status := util.SendRequest(ctx, nil, http.MethodDelete, targetURL, nil)
	if status != http.StatusOK {
		return &GrafanaError{Status: status, Body: string(body)}
	}
	return nil
}

func (g *grafanaClient) HasPermissions(ctx context.Context, uid string) (bool, error) {
	targetURL := g.uri + apiFolders + "/" + uid + folderPermissionsSuffix
	body, status := util.SendRequest(ctx, nil, http.MethodGet, targetURL, nil)
	if status != http.StatusOK {
		return false, &GrafanaError{Status: status, Body: string(body)}
	}
	var res []any
	if err := json.Unmarshal(body, &res); err != nil {
		return false, err
	}
	return len(res) > 0, nil
}

func (g *grafanaClient) IsEmpty(ctx context.Context, uid string) (bool, error) {
	params := url.Values{}
	params.Add("folderUIDs", uid)
	params.Add("type", "dash-db")
	params.Add("limit", "1") // We only need to know if at least one dashboard exists
	targetURL := g.uri + apiSearch + "?" + params.Encode()
	body, status := util.SendRequest(ctx, nil, http.MethodGet, targetURL, nil)
	if status != http.StatusOK {
		return false, &GrafanaError{Status: status, Body: string(body)}
	}
	var res []any
	if err := json.Unmarshal(body, &res); err != nil {
		return false, err
	}
	return len(res) == 0, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
