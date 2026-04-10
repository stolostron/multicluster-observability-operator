// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

//go:build integration

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestIntegrationGrafanaDashboardController(t *testing.T) {
	testEnv := &envtest.Environment{
		ControlPlaneStopTimeout: 2 * time.Minute,
	}
	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("failed to start envtest: %v", err)
	}
	defer testEnv.Stop()

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("failed to create kubeClient: %v", err)
	}

	var mu sync.Mutex
	recordedRequests := make(map[string]int)
	dashboards := make(map[string]map[string]any)
	folders := make(map[int64]string)

	resetState := func() {
		mu.Lock()
		defer mu.Unlock()
		recordedRequests = make(map[string]int)
		dashboards = make(map[string]map[string]any)
		folders = map[int64]string{1: "General"}
	}
	resetState()

	mockGrafana := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		key := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
		recordedRequests[key]++

		if r.URL.Path == "/api/folders" {
			if r.Method == "GET" {
				var res []map[string]any
				for id, title := range folders {
					res = append(res, map[string]any{"id": id, "uid": fmt.Sprintf("uid-%d", id), "title": title})
				}
				json.NewEncoder(w).Encode(res)
				return
			}
			if r.Method == "POST" {
				var body map[string]string
				json.NewDecoder(r.Body).Decode(&body)
				id := int64(time.Now().UnixNano())
				folders[id] = body["title"]
				json.NewEncoder(w).Encode(map[string]any{"id": id, "uid": fmt.Sprintf("uid-%d", id), "title": body["title"]})
				return
			}
		}

		if strings.HasPrefix(r.URL.Path, "/api/folders/id/") {
			var id int64
			fmt.Sscanf(r.URL.Path, "/api/folders/id/%d", &id)
			if title, ok := folders[id]; ok {
				json.NewEncoder(w).Encode(map[string]any{"id": id, "uid": fmt.Sprintf("uid-%d", id), "title": title})
				return
			}
		}

		if strings.HasPrefix(r.URL.Path, "/api/folders/uid-") {
			uid := r.URL.Path[len("/api/folders/"):]
			if r.Method == "DELETE" {
				var id int64
				fmt.Sscanf(uid, "uid-%d", &id)
				delete(folders, id)
				w.WriteHeader(http.StatusOK)
				return
			}
			if strings.HasSuffix(r.URL.Path, "/permissions") {
				w.Write([]byte("[{\"id\": 1}]"))
				return
			}
		}

		if r.URL.Path == "/api/org/preferences" && r.Method == "PUT" {
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.URL.Path == "/api/search" {
			var res []map[string]any
			// Handle both documented and legacy casing
			folderUIDs := r.URL.Query().Get("folderUIDs")
			if folderUIDs == "" {
				folderUIDs = r.URL.Query().Get("folderUids")
			}

			for uid, dash := range dashboards {
				match := true
				if folderUIDs != "" {
					match = false
					dashFolderID, _ := dash["folderId"].(int64)
					if fmt.Sprintf("uid-%d", dashFolderID) == folderUIDs {
						match = true
					}
				}
				if match {
					res = append(res, map[string]any{"uid": uid, "tags": dash["tags"]})
				}
			}

			// Respect limit=1 optimization
			if r.URL.Query().Get("limit") == "1" && len(res) > 1 {
				res = res[:1]
			}

			json.NewEncoder(w).Encode(res)
			return
		}

		if r.URL.Path == "/api/dashboards/db" && r.Method == "POST" {
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			dash, _ := body["dashboard"].(map[string]any)
			uid := dash["uid"].(string)
			folderID := int64(body["folderId"].(float64))
			dash["folderId"] = folderID
			dashboards[uid] = dash
			json.NewEncoder(w).Encode(map[string]any{"id": 100, "uid": uid, "status": "success"})
			return
		}

		if strings.HasPrefix(r.URL.Path, "/api/dashboards/uid/") {
			uid := r.URL.Path[len("/api/dashboards/uid/"):]
			if r.Method == "DELETE" {
				delete(dashboards, uid)
				w.WriteHeader(http.StatusOK)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockGrafana.Close()

	ns := "test-dashboards"
	t.Setenv("POD_NAMESPACE", ns)
	kubeClient.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}, metav1.CreateOptions{})

	t.Run("Scenario 1: Initial Orphan Cleanup", func(t *testing.T) {
		resetState()
		mu.Lock()
		dashboards["zombie-uid"] = map[string]any{
			"uid": "zombie-uid",
		}
		mu.Unlock()

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		c, err := NewGrafanaDashboardController(kubeClient.CoreV1(), mockGrafana.URL)
		if err != nil {
			t.Fatalf("failed to create controller: %v", err)
		}
		go c.Run(ctx)

		err = wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 15*time.Second, true, func(ctx context.Context) (bool, error) {
			mu.Lock()
			defer mu.Unlock()
			_, exists := dashboards["zombie-uid"]
			return !exists, nil
		})
		if err != nil {
			t.Errorf("zombie dashboard was not cleaned up: %v", err)
		}
	})

	t.Run("Scenario 2: Dashboard Lifecycle", func(t *testing.T) {
		resetState()
		mu.Lock()
		folders[100] = "OldFolder"
		mu.Unlock()

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		c, err := NewGrafanaDashboardController(kubeClient.CoreV1(), mockGrafana.URL)
		if err != nil {
			t.Fatalf("failed to create controller: %v", err)
		}
		go c.Run(ctx)

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: "my-dash", Namespace: ns,
				Labels:      map[string]string{CustomDashboardLabelKey: "true", HomeDashboardUIDKey: "my-uid"},
				Annotations: map[string]string{CustomFolderKey: "OldFolder", SetHomeDashboardKey: "true"},
			},
			Data: map[string]string{"dash.json": "{\"title\": \"My Dash\", \"uid\": \"my-uid\"}"},
		}
		kubeClient.CoreV1().ConfigMaps(ns).Create(ctx, cm, metav1.CreateOptions{})

		// Verify Create + Home Preference
		err = wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 10*time.Second, true, func(ctx context.Context) (bool, error) {
			mu.Lock()
			defer mu.Unlock()
			// SetHomeDashboard failures trigger a workqueue retry, but don't block
			// the processing of other dashboards in the same ConfigMap.
			return dashboards["my-uid"] != nil && recordedRequests["PUT /api/org/preferences"] > 0, nil
		})
		if err != nil {
			t.Errorf("dashboard creation or home preference failed: %v", err)
		}

		// Scenario 3: Folder Move
		cm.Annotations[CustomFolderKey] = "NewFolder"
		_, err = kubeClient.CoreV1().ConfigMaps(ns).Update(ctx, cm, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("failed to update configmap for move: %v", err)
		}
		// Increased timeout (30s) to allow for the workqueue rate-limited retry of the folder cleanup.
		err = wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, 30*time.Second, true, func(ctx context.Context) (bool, error) {
			mu.Lock()
			defer mu.Unlock()
			foundNew, foundOld := false, false
			for _, title := range folders {
				if title == "NewFolder" {
					foundNew = true
				}
				if title == "OldFolder" {
					foundOld = true
				}
			}
			return foundNew && !foundOld, nil
		})
		if err != nil {
			t.Errorf("folder move failed: %v", err)
		}

		// Scenario 4: Deletion
		err = kubeClient.CoreV1().ConfigMaps(ns).Delete(ctx, cm.Name, metav1.DeleteOptions{})
		if err != nil {
			t.Fatalf("failed to delete configmap: %v", err)
		}

		err = wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 10*time.Second, true, func(ctx context.Context) (bool, error) {
			mu.Lock()
			defer mu.Unlock()
			return dashboards["my-uid"] == nil, nil
		})
		if err != nil {
			t.Errorf("dashboard deletion failed: %v", err)
		}

		// Scenario 5: Update Transition (Remove Label)
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: "transition-dash", Namespace: ns,
				Labels: map[string]string{CustomDashboardLabelKey: "true"},
			},
			Data: map[string]string{"dash.json": "{\"title\": \"Transition Dash\", \"uid\": \"trans-uid\"}"},
		}
		kubeClient.CoreV1().ConfigMaps(ns).Create(ctx, cm, metav1.CreateOptions{})
		err = wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 10*time.Second, true, func(ctx context.Context) (bool, error) {
			mu.Lock()
			defer mu.Unlock()
			return dashboards["trans-uid"] != nil, nil
		})
		if err != nil {
			t.Errorf("failed to create transition dashboard: %v", err)
		}

		// Remove label
		cm.Labels = map[string]string{}
		_, err = kubeClient.CoreV1().ConfigMaps(ns).Update(ctx, cm, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("failed to update configmap to remove label: %v", err)
		}

		err = wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 10*time.Second, true, func(ctx context.Context) (bool, error) {
			mu.Lock()
			defer mu.Unlock()
			return dashboards["trans-uid"] == nil, nil
		})
		if err != nil {
			t.Errorf("dashboard not deleted after removing label: %v", err)
		}
	})
}
