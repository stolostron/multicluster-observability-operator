// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGrafanaClient(t *testing.T) {
	ctx := context.Background()

	t.Run("ListAllDashboards success", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != apiSearch || r.URL.Query().Get("type") != "dash-db" {
				t.Errorf("unexpected path or query: %v", r.URL)
			}
			w.Write([]byte("[{\"uid\": \"d1\", \"tags\": [\"t1\"]}]"))
		}))
		defer ts.Close()

		client := &grafanaClient{uri: ts.URL}
		dashboards, err := client.ListAllDashboards(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(dashboards) != 1 || dashboards[0].UID != "d1" {
			t.Errorf("unexpected dashboards: %v", dashboards)
		}
	})

	t.Run("DeleteDashboard cases", func(t *testing.T) {
		t.Run("success 200", func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer ts.Close()
			client := &grafanaClient{uri: ts.URL}
			if err := client.DeleteDashboard(ctx, "d1"); err != nil {
				t.Errorf("expected nil error on 200, got %v", err)
			}
		})
		t.Run("success 404", func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}))
			defer ts.Close()
			client := &grafanaClient{uri: ts.URL}
			if err := client.DeleteDashboard(ctx, "d1"); err != nil {
				t.Errorf("expected nil error on 404, got %v", err)
			}
		})
		t.Run("failure 500", func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("internal error"))
			}))
			defer ts.Close()
			client := &grafanaClient{uri: ts.URL}
			err := client.DeleteDashboard(ctx, "d1")
			if err == nil {
				t.Fatal("expected error on 500")
			}
			gErr, _ := err.(*GrafanaError)
			if gErr.Status != 500 {
				t.Errorf("expected 500 status, got %d", gErr.Status)
			}
		})
	})

	t.Run("CreateOrUpdateDashboard error handling", func(t *testing.T) {
		t.Run("raw string error", func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusPreconditionFailed)
				w.Write([]byte("version-mismatch"))
			}))
			defer ts.Close()

			client := &grafanaClient{uri: ts.URL}
			_, err := client.CreateOrUpdateDashboard(ctx, map[string]any{}, 0)
			if err == nil {
				t.Fatal("expected error")
			}
			gErr, ok := err.(*GrafanaError)
			if !ok || !gErr.IsVersionMismatch() {
				t.Errorf("expected version mismatch error, got %v", err)
			}
		})

		t.Run("json response error", func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusPreconditionFailed)
				w.Write([]byte("{\"message\":\"name-exists\"}"))
			}))
			defer ts.Close()

			client := &grafanaClient{uri: ts.URL}
			_, err := client.CreateOrUpdateDashboard(ctx, map[string]any{}, 0)
			if err == nil {
				t.Fatal("expected error")
			}
			gErr, ok := err.(*GrafanaError)
			if !ok || !gErr.IsNameExists() {
				t.Errorf("expected name exists error, got %v", err)
			}
		})
	})

	t.Run("SetHomeDashboard", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPut || r.URL.Path != apiPreferences {
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()
		client := &grafanaClient{uri: ts.URL}
		if err := client.SetHomeDashboard(ctx, 42); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Folder operations", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case apiFolders:
				if r.Method == http.MethodPost {
					w.Write([]byte("{\"id\": 10, \"uid\": \"f10\", \"title\": \"New\"}"))
				} else {
					w.Write([]byte("[{\"id\": 1, \"uid\": \"f1\", \"title\": \"General\"}]"))
				}
			case apiFoldersID + "1":
				w.Write([]byte("{\"id\": 1, \"uid\": \"f1\", \"title\": \"General\"}"))
			case "/api/folders/f1" + folderPermissionsSuffix:
				w.Write([]byte("[{\"id\": 1}]"))
			case "/api/folders/f2" + folderPermissionsSuffix:
				w.Write([]byte("[]"))
			case apiSearch:
				fUID := r.URL.Query().Get("folderUIDs")
				if fUID == "" {
					fUID = r.URL.Query().Get("folderUids")
				}
				if fUID == "f_empty" {
					w.Write([]byte("[]"))
				} else {
					w.Write([]byte("[{}]"))
				}
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer ts.Close()

		client := &grafanaClient{uri: ts.URL}

		folders, _ := client.ListFolders(ctx)
		if len(folders) != 1 || folders[0].ID != 1 {
			t.Errorf("list folders failed")
		}

		f, _ := client.GetFolderByID(ctx, 1)
		if f.UID != "f1" {
			t.Errorf("get folder by ID failed")
		}

		newF, _ := client.CreateFolder(ctx, "New")
		if newF.ID != 10 {
			t.Errorf("create folder failed")
		}

		hasPerm, _ := client.HasPermissions(ctx, "f1")
		if !hasPerm {
			t.Errorf("expected permissions")
		}
		hasPerm2, _ := client.HasPermissions(ctx, "f2")
		if hasPerm2 {
			t.Errorf("expected no permissions")
		}

		empty, _ := client.IsEmpty(ctx, "f_empty")
		if !empty {
			t.Errorf("expected empty")
		}
		empty2, _ := client.IsEmpty(ctx, "f_full")
		if empty2 {
			t.Errorf("expected not empty")
		}
	})
}

func TestGrafanaError(t *testing.T) {
	err := &GrafanaError{Status: 404, Body: "not found"}
	expected := "grafana error (status 404): not found"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}
