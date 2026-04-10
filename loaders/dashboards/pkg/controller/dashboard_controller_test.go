// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package controller

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type mockGrafanaClient struct {
	dashboards []grafanaDashboard
	folders    []grafanaFolder

	deleteDashCalled   map[string]int
	deleteFolderCalled map[string]int
	createDashCalled   int
	setHomeCalled      int

	searchErr     error
	createErr     error
	createErrFunc func() error
	setHomeErr    error

	folderErr     error // Dedicated error for folder operations
	listErr       error
	deleteDashErr error
	deleteFoldErr error
	permErr       error
	emptyErr      error

	permResp  bool
	emptyResp map[string]bool

	lastDashboard map[string]any
}

func newMockGrafanaClient() *mockGrafanaClient {
	return &mockGrafanaClient{
		deleteDashCalled:   make(map[string]int),
		deleteFolderCalled: make(map[string]int),
		emptyResp:          make(map[string]bool),
		permResp:           true,
	}
}

func (m *mockGrafanaClient) ListAllDashboards(ctx context.Context) ([]grafanaDashboard, error) {
	return m.dashboards, m.searchErr
}

func (m *mockGrafanaClient) DeleteDashboard(ctx context.Context, uid string) error {
	m.deleteDashCalled[uid]++
	return m.deleteDashErr
}

func (m *mockGrafanaClient) CreateOrUpdateDashboard(ctx context.Context, dashboard map[string]any, folderID int64) (int64, error) {
	m.createDashCalled++
	m.lastDashboard = dashboard
	if m.createErrFunc != nil {
		return 100, m.createErrFunc()
	}
	return 100, m.createErr
}

func (m *mockGrafanaClient) SetHomeDashboard(ctx context.Context, id int64) error {
	m.setHomeCalled++
	return m.setHomeErr
}

func (m *mockGrafanaClient) ListFolders(ctx context.Context) ([]grafanaFolder, error) {
	return m.folders, m.listErr
}

func (m *mockGrafanaClient) GetFolderByID(ctx context.Context, id int64) (*grafanaFolder, error) {
	for _, f := range m.folders {
		if f.ID == id {
			return &f, nil
		}
	}
	return nil, &GrafanaError{Status: http.StatusNotFound, Body: "not found"}
}

func (m *mockGrafanaClient) CreateFolder(ctx context.Context, title string) (*grafanaFolder, error) {
	if m.folderErr != nil {
		return nil, m.folderErr
	}
	f := grafanaFolder{ID: 1, UID: "test", Title: title}
	m.folders = append(m.folders, f)
	return &f, nil
}

func (m *mockGrafanaClient) DeleteFolder(ctx context.Context, uid string) error {
	m.deleteFolderCalled[uid]++
	return m.deleteFoldErr
}

func (m *mockGrafanaClient) HasPermissions(ctx context.Context, uid string) (bool, error) {
	return m.permResp, m.permErr
}

func (m *mockGrafanaClient) IsEmpty(ctx context.Context, uid string) (bool, error) {
	if val, ok := m.emptyResp[uid]; ok {
		return val, m.emptyErr
	}
	return true, m.emptyErr
}

func createDashboard() (*corev1.ConfigMap, error) {
	data, err := os.ReadFile("../../examples/k8s-dashboard.yaml")
	if err != nil {
		panic(err)
	}
	var cm corev1.ConfigMap
	err = yaml.Unmarshal(data, &cm)
	return &cm, err
}

func newTestController(t *testing.T, ns string) (*GrafanaDashboardController, *mockGrafanaClient) {
	t.Setenv("POD_NAMESPACE", ns)
	coreClient := fake.NewClientset().CoreV1()
	mock := newMockGrafanaClient()
	c := &GrafanaDashboardController{
		kubeClient:        coreClient,
		grafana:           mock,
		maxDashboardRetry: defaultMaxDashboardRetry,
		watchedNS:         ns,
		uidMap:            make(map[string]trackedState),
		reconcileMu:       make(chan struct{}, 1),
	}
	c.reconcileMu <- struct{}{}
	return c, mock
}

func newTestQueue() workqueue.TypedRateLimitingInterface[any] {
	return workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[any]())
}

func newTestIndexer() cache.Indexer {
	return cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
}

func TestGrafanaDashboardController(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	c, mock := newTestController(t, "ns2")
	mock.folders = append(mock.folders, grafanaFolder{ID: 1, UID: "test", Title: "Custom"})

	cm, err := createDashboard()
	if err != nil {
		t.Fatalf("failed to create dashboard: %v", err)
	}
	_, err = c.kubeClient.ConfigMaps("ns2").Create(ctx, cm, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("fail to create configmap with %v", err)
	}

	queue := newTestQueue()
	informer, err := c.newKubeInformer(queue)
	if err != nil {
		t.Fatalf("failed to create informer with %v", err)
	}
	go informer.Run(ctx.Done())

	if err := c.updateDashboard(ctx, cm); err != nil {
		t.Errorf("failed to update dashboard: %v", err)
	}

	cm.Data = map[string]string{}
	_, err = c.kubeClient.ConfigMaps("ns2").Update(ctx, cm, metav1.UpdateOptions{})
	if err := c.updateDashboard(ctx, cm); err != nil {
		t.Errorf("failed to update dashboard with empty data: %v", err)
	}

	cm, _ = createDashboard()
	_, err = c.kubeClient.ConfigMaps("ns2").Update(ctx, cm, metav1.UpdateOptions{})
	if err := c.updateDashboard(ctx, cm); err != nil {
		t.Errorf("failed to update dashboard: %v", err)
	}

	c.kubeClient.ConfigMaps("ns2").Delete(ctx, cm.GetName(), metav1.DeleteOptions{})
	key, _ := cache.MetaNamespaceKeyFunc(cm)
	if err := c.deleteTrackedUIDs(ctx, key); err != nil {
		t.Errorf("failed to delete tracked uids: %v", err)
	}
}

func TestDeleteTrackedUIDs(t *testing.T) {
	ctx := context.Background()

	t.Run("retry on deletion failure", func(t *testing.T) {
		c, mock := newTestController(t, "default")
		key := "default/my-dash"
		c.uidMap[key] = trackedState{uids: []string{"uid1", "uid2"}, folder: "Custom"}

		// Fail deletion for uid1
		mock.deleteDashErr = errors.New("transient error")

		err := c.deleteTrackedUIDs(ctx, key)
		if err == nil {
			t.Error("expected error on deletion failure")
		}

		state, exists := c.uidMap[key]

		if !exists {
			t.Error("expected state to remain in map on failure")
		}
		if len(state.uids) != 2 {
			t.Errorf("expected both UIDs to remain (or at least the failed one), got %v", state.uids)
		}

		// Now succeed
		mock.deleteDashErr = nil
		err = c.deleteTrackedUIDs(ctx, key)
		if err != nil {
			t.Errorf("expected nil error on second attempt, got %v", err)
		}

		_, exists = c.uidMap[key]
		if exists {
			t.Error("expected state to be removed from map after successful deletion")
		}
	})
}

func TestNewKubeInformer_Filtering(t *testing.T) {
	c := &GrafanaDashboardController{}

	dashCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{CustomDashboardLabelKey: "true"}},
	}
	regularCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "regular"},
	}

	t.Run("enqueue new dashboard", func(t *testing.T) {
		if !c.shouldEnqueue(nil, dashCM) {
			t.Error("expected to enqueue new dashboard")
		}
	})

	t.Run("ignore new regular ConfigMap", func(t *testing.T) {
		if c.shouldEnqueue(nil, regularCM) {
			t.Error("should not enqueue regular ConfigMap")
		}
	})

	t.Run("enqueue when dashboard becomes regular CM (cleanup)", func(t *testing.T) {
		if !c.shouldEnqueue(dashCM, regularCM) {
			t.Error("expected to enqueue for immediate cleanup")
		}
	})

	t.Run("ignore regular CM update", func(t *testing.T) {
		if c.shouldEnqueue(regularCM, regularCM) {
			t.Error("should not enqueue regular CM update")
		}
	})
}

func TestCleanupOrphanDashboards_MapPopulation(t *testing.T) {
	ctx := t.Context()
	c, mock := newTestController(t, "default")

	// 1. Pre-existing ConfigMap in Kubernetes
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "pre-existing", Namespace: "default", Labels: map[string]string{CustomDashboardLabelKey: "true"}},
		Data:       map[string]string{"dash.json": "{\"title\": \"Pre\", \"uid\": \"explicit-uid\"}"},
	}
	indexer := newTestIndexer()
	indexer.Add(cm)

	// 2. Grafana has this dashboard + an orphan
	mock.dashboards = []grafanaDashboard{
		{UID: "explicit-uid"},
		{UID: "zombie-uid"},
	}

	// Run cleanup
	c.cleanupOrphanDashboards(ctx, indexer)

	// Verify zombie was deleted
	if mock.deleteDashCalled["zombie-uid"] != 1 {
		t.Error("expected zombie to be deleted")
	}

	// CRITICAL: Verify uidMap was populated during cleanup!
	state, exists := c.uidMap["default/pre-existing"]

	if !exists {
		t.Fatal("expected uidMap to be populated during initial scan")
	}
	if len(state.uids) != 1 || state.uids[0] != "explicit-uid" {
		t.Errorf("expected explicit-uid to be tracked, got %v", state.uids)
	}
}

func TestIsDesiredDashboardConfigmap(t *testing.T) {
	testCaseList := []struct {
		name     string
		cm       *corev1.ConfigMap
		expected bool
	}{
		{
			"custom dashboard",
			&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{CustomDashboardLabelKey: "true"}}},
			true,
		},
		{
			"general folder dashboard",
			&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{GeneralFolderKey: "true"}}},
			true,
		},
		{
			"not custom dashboard",
			&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{CustomDashboardLabelKey: "false"}}},
			false,
		},
		{
			"mco dashboard",
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "grafana-dashboard-mco",
					OwnerReferences: []metav1.OwnerReference{{Kind: "MultiClusterObservability"}},
				},
			},
			true,
		},
		{
			"not mco dashboard (wrong owner)",
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "grafana-dashboard-test",
					OwnerReferences: []metav1.OwnerReference{{Kind: "Pod"}},
				},
			},
			false,
		},
	}

	for _, tc := range testCaseList {
		t.Run(tc.name, func(t *testing.T) {
			if isDesiredDashboardConfigmap(tc.cm) != tc.expected {
				t.Errorf("failed %s", tc.name)
			}
		})
	}
}

func TestFindCustomFolderID(t *testing.T) {
	c, mock := newTestController(t, "default")
	mock.folders = []grafanaFolder{{ID: 1, Title: "Custom"}}

	if c.findCustomFolderID(t.Context(), "Custom") != 1 {
		t.Error("expected 1")
	}
	if c.findCustomFolderID(t.Context(), "Unknown") != 0 {
		t.Error("expected 0")
	}
}

func TestSyncHandler(t *testing.T) {
	ctx := t.Context()
	c, _ := newTestController(t, "default")

	t.Run("sync existing dashboard", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "m",
				Namespace: "default",
				Labels:    map[string]string{CustomDashboardLabelKey: "true"},
			},
			Data: map[string]string{"d": "{}"},
		}
		indexer := newTestIndexer()
		indexer.Add(cm)
		if err := c.syncHandler(ctx, "default/m", indexer); err != nil {
			t.Error(err)
		}
	})

	t.Run("non-existent key returns nil", func(t *testing.T) {
		if err := c.syncHandler(ctx, "default/missing", newTestIndexer()); err != nil {
			t.Error(err)
		}
	})
}

func TestCleanupOrphanDashboards(t *testing.T) {
	ctx := t.Context()

	t.Run("happy path", func(t *testing.T) {
		c, mock := newTestController(t, "default")
		// Resolve the expected UID for the existing CM
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "exists", Namespace: "default", Labels: map[string]string{CustomDashboardLabelKey: "true"}},
			Data:       map[string]string{"key1": "{}"},
		}
		expectedUID, _ := c.resolveDashboardUID(nil, cm, "key1")

		mock.dashboards = []grafanaDashboard{
			{UID: expectedUID},
			{UID: "orphan"},
		}
		mock.folders = []grafanaFolder{{ID: 10, UID: "empty-uid", Title: "Empty"}}
		mock.emptyResp["empty-uid"] = true

		indexer := newTestIndexer()
		indexer.Add(cm)

		c.cleanupOrphanDashboards(ctx, indexer)

		if mock.deleteDashCalled["orphan"] != 1 {
			t.Error("orphan not deleted")
		}
		if mock.deleteDashCalled[expectedUID] != 0 {
			t.Error("valid dashboard should not be deleted")
		}
		if mock.deleteFolderCalled["empty-uid"] != 1 {
			t.Error("empty folder not deleted")
		}
	})

	t.Run("search error returns early", func(t *testing.T) {
		c, mock := newTestController(t, "default")
		mock.searchErr = errors.New("search fail")
		mock.dashboards = []grafanaDashboard{{UID: "any"}}
		c.cleanupOrphanDashboards(ctx, newTestIndexer())
		if len(mock.deleteDashCalled) > 0 {
			t.Error("should not have deleted anything on search error")
		}
	})

	t.Run("non-dashboard CM does not protect dashboards", func(t *testing.T) {
		c, mock := newTestController(t, "default")
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "regular", Namespace: "default"},
			Data:       map[string]string{"key1": "{}"},
		}
		uid, _ := c.resolveDashboardUID(nil, cm, "key1")
		mock.dashboards = []grafanaDashboard{{UID: uid}}
		indexer := newTestIndexer()
		indexer.Add(cm)
		c.cleanupOrphanDashboards(ctx, indexer)
		if mock.deleteDashCalled[uid] != 1 {
			t.Error("dashboard from non-dashboard CM should be deleted")
		}
	})

	t.Run("multi-key CM protects all its dashboards", func(t *testing.T) {
		c, mock := newTestController(t, "default")
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "multi", Namespace: "default", Labels: map[string]string{CustomDashboardLabelKey: "true"}},
			Data:       map[string]string{"k1": "{}", "k2": "{}"},
		}
		u1, _ := c.resolveDashboardUID(nil, cm, "k1")
		u2, _ := c.resolveDashboardUID(nil, cm, "k2")
		mock.dashboards = []grafanaDashboard{{UID: u1}, {UID: u2}}
		indexer := newTestIndexer()
		indexer.Add(cm)
		c.cleanupOrphanDashboards(ctx, indexer)
		if mock.deleteDashCalled[u1] != 0 || mock.deleteDashCalled[u2] != 0 {
			t.Error("dashboards from multi-key CM should be protected")
		}
	})
}

func TestResolveDashboardUID(t *testing.T) {
	c := &GrafanaDashboardController{}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "my-cm", Namespace: "my-ns"}}

	t.Run("uid exists", func(t *testing.T) {
		dash := map[string]any{"uid": "existing-uid"}
		uid, _ := c.resolveDashboardUID(dash, cm, "key1")
		if uid != "existing-uid" {
			t.Errorf("got %s", uid)
		}
	})
	t.Run("fallback", func(t *testing.T) {
		dash := map[string]any{"uid": ""}
		uid, _ := c.resolveDashboardUID(dash, cm, "key1")
		// Fallback should be name-namespace (legacy)
		// Since key1 is NOT in my-cm, it should be my-cm-key1-my-ns
		if uid != "my-cm-key1-my-ns" {
			t.Errorf("expected my-cm-key1-my-ns, got %s", uid)
		}
	})

	t.Run("legacy fallback (key matches name)", func(t *testing.T) {
		cm2 := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "my-dash", Namespace: "ns"}}
		uid, _ := c.resolveDashboardUID(nil, cm2, "my-dash.json")
		if uid != "my-dash-ns" {
			t.Errorf("expected my-dash-ns, got %s", uid)
		}
	})

	t.Run("nil dashboard generates fallback UID", func(t *testing.T) {
		uid, err := c.resolveDashboardUID(nil, cm, "key1")
		if err != nil {
			t.Fatal(err)
		}
		if uid == "" {
			t.Error("expected non-empty generated UID")
		}
		uid2, _ := c.resolveDashboardUID(nil, cm, "key1")
		if uid != uid2 {
			t.Error("UID must be deterministic")
		}
	})

	t.Run("multi-dashboard uniqueness", func(t *testing.T) {
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "multi", Namespace: "ns"}}
		u1, _ := c.resolveDashboardUID(nil, cm, "k1")
		u2, _ := c.resolveDashboardUID(nil, cm, "k2")
		if u1 == u2 {
			t.Errorf("expected unique UIDs for different keys, both got %s", u1)
		}
		if !strings.Contains(u1, "multi-k1") || !strings.Contains(u2, "multi-k2") {
			t.Errorf("UIDs should contain the key name: %s, %s", u1, u2)
		}
	})
}

func TestResolveDashboardUID_Extensions(t *testing.T) {
	c := &GrafanaDashboardController{}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "my-dash", Namespace: "ns"}}

	testCases := []struct {
		key      string
		expected string
	}{
		{"my-dash.json", "my-dash-ns"},
		{"my-dash.yaml", "my-dash-ns"},
		{"my-dash.yml", "my-dash-ns"},
		{"other.json", "my-dash-other-ns"},
	}

	for _, tc := range testCases {
		t.Run(tc.key, func(t *testing.T) {
			uid, _ := c.resolveDashboardUID(nil, cm, tc.key)
			if uid != tc.expected {
				t.Errorf("for key %q: expected %q, got %q", tc.key, tc.expected, uid)
			}
		})
	}
}

func TestCreateCustomFolder_PermissionsRetry(t *testing.T) {
	ctx := t.Context()
	c, mock := newTestController(t, "default")
	mock.permResp = false // Force permission failure

	id := c.createCustomFolder(ctx, "RetryFolder")
	if id != 0 {
		t.Errorf("expected 0 ID, got %v", id)
	}
	if mock.deleteFolderCalled["test"] != 1 {
		t.Error("expected folder deletion on permission failure")
	}
}

func TestUpdateDashboard_GrafanaErrors(t *testing.T) {
	ctx := t.Context()

	testCases := []struct {
		name           string
		err            error
		expectedErrSub string
	}{
		{
			"name exists",
			&GrafanaError{Status: http.StatusPreconditionFailed, Body: "name-exists"},
			"the dashboard name already existed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c, mock := newTestController(t, "default")
			mock.createErr = tc.err

			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "err-cm", Namespace: "default", Labels: map[string]string{CustomDashboardLabelKey: "true"}},
				Data:       map[string]string{"k": "{}"},
			}
			err := c.updateDashboard(ctx, cm)
			if err == nil || !strings.Contains(err.Error(), tc.expectedErrSub) {
				t.Errorf("expected %q, got %v", tc.expectedErrSub, err)
			}
		})
	}

	t.Run("SetHomeDashboard failure should not block other dashboards and trigger retry", func(t *testing.T) {
		c, mock := newTestController(t, "default")
		mock.setHomeErr = errors.New("api failure")

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: "multi-dash", Namespace: "default",
				Labels: map[string]string{
					CustomDashboardLabelKey: "true",
					HomeDashboardUIDKey:     "uid1",
				},
				Annotations: map[string]string{
					SetHomeDashboardKey: "true",
				},
			},
			Data: map[string]string{
				"dash1.json": "{\"title\": \"D1\", \"uid\": \"uid1\"}",
				"dash2.json": "{\"title\": \"D2\", \"uid\": \"uid2\"}",
			},
		}

		err := c.updateDashboard(ctx, cm)
		if err == nil || !strings.Contains(err.Error(), "api failure") {
			t.Errorf("expected error from SetHomeDashboard failure, got %v", err)
		}

		if mock.createDashCalled != 2 {
			t.Errorf("expected 2 dashboards created, got %d", mock.createDashCalled)
		}
		if mock.setHomeCalled != 1 {
			t.Errorf("expected 1 setHome call, got %d", mock.setHomeCalled)
		}

		state := c.uidMap["default/multi-dash"]
		if len(state.uids) != 2 {
			t.Errorf("expected 2 UIDs in map, got %v", state.uids)
		}
		// Verify content
		found1, found2 := false, false
		for _, u := range state.uids {
			if u == "uid1" {
				found1 = true
			}
			if u == "uid2" {
				found2 = true
			}
		}
		if !found1 || !found2 {
			t.Errorf("missing UIDs in map: %v", state.uids)
		}
	})

	t.Run("partial success: one fails creation, sibling succeeds. Both tracked in map.", func(t *testing.T) {
		c, mock := newTestController(t, "default")
		// Fail only on first call
		callCount := 0
		mock.createErrFunc = func() error {
			callCount++
			if callCount == 1 {
				return errors.New("creation fail")
			}
			return nil
		}

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "partial", Namespace: "default", Labels: map[string]string{CustomDashboardLabelKey: "true"}},
			Data: map[string]string{
				"dash1.json": "{\"title\": \"D1\", \"uid\": \"uid1\"}",
				"dash2.json": "{\"title\": \"D2\", \"uid\": \"uid2\"}",
			},
		}

		err := c.updateDashboard(ctx, cm)
		if err == nil || !strings.Contains(err.Error(), "creation fail") {
			t.Errorf("expected creation fail error, got %v", err)
		}

		if mock.createDashCalled != 2 {
			t.Errorf("expected 2 create attempts, got %d", mock.createDashCalled)
		}

		state := c.uidMap["default/partial"]
		// BOTH should be in the map to protect the failing one from accidental deletion
		if len(state.uids) != 2 {
			t.Errorf("expected 2 UIDs in map (including failing one), got %v", state.uids)
		}
	})

	t.Run("unmarshal failure on one key does not block cleanup for orphans", func(t *testing.T) {
		c, mock := newTestController(t, "default")
		// Pre-populate map with an "old" UID that SHOULD be deleted
		c.uidMap["default/unmarshal"] = trackedState{uids: []string{"old-orphan", "good-uid"}, folder: "Custom"}

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "unmarshal", Namespace: "default", Labels: map[string]string{CustomDashboardLabelKey: "true"}},
			Data: map[string]string{
				"bad.json":  "{",
				"good.json": "{\"title\": \"Good\", \"uid\": \"good-uid\"}",
			},
		}

		err := c.updateDashboard(ctx, cm)
		if err != nil {
			t.Errorf("expected nil error for unmarshal failure (best-effort), got %v", err)
		}

		// Verify cleanup RAN: old-orphan should be deleted
		if mock.deleteDashCalled["old-orphan"] != 1 {
			t.Error("expected cleanup to run despite unmarshal failure on a different key")
		}

		state := c.uidMap["default/unmarshal"]
		// Map should only contain good-uid because currentUIDs is the new truth
		if len(state.uids) != 1 || state.uids[0] != "good-uid" {
			t.Errorf("expected only good-uid in map, got %v", state.uids)
		}
	})

	t.Run("creation + deletion errors combined", func(t *testing.T) {
		c, mock := newTestController(t, "default")
		c.uidMap["default/combined"] = trackedState{uids: []string{"old-orphan"}, folder: "Custom"}
		mock.createErr = errors.New("create fail")
		mock.deleteDashErr = errors.New("delete fail")

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "combined", Namespace: "default", Labels: map[string]string{CustomDashboardLabelKey: "true"}},
			Data: map[string]string{
				"new.json": "{\"title\": \"New\", \"uid\": \"new-uid\"}",
			},
		}

		err := c.updateDashboard(ctx, cm)
		if err == nil {
			t.Fatal("expected errors")
		}
		if !strings.Contains(err.Error(), "create fail") || !strings.Contains(err.Error(), "delete fail") {
			t.Errorf("expected combined errors, got: %v", err)
		}
	})

	t.Run("name exists on one key does not block others", func(t *testing.T) {
		c, mock := newTestController(t, "default")
		// Fail only on one specific title
		mock.createErrFunc = func() error {
			if mock.lastDashboard["title"] == "Bad" {
				return &GrafanaError{Status: http.StatusPreconditionFailed, Body: "name-exists"}
			}
			return nil
		}

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "name-exists", Namespace: "default", Labels: map[string]string{CustomDashboardLabelKey: "true"}},
			Data: map[string]string{
				"bad.json":  "{\"title\": \"Bad\", \"uid\": \"bad-uid\"}",
				"good.json": "{\"title\": \"Good\", \"uid\": \"good-uid\"}",
			},
		}

		err := c.updateDashboard(ctx, cm)
		if err == nil || !strings.Contains(err.Error(), "the dashboard name already existed") {
			t.Errorf("expected name exists error, got %v", err)
		}

		if mock.createDashCalled != 2 {
			t.Errorf("expected 2 create attempts, got %d", mock.createDashCalled)
		}

		state := c.uidMap["default/name-exists"]
		if len(state.uids) != 2 {
			t.Errorf("expected both UIDs in map (protection logic), got %v", state.uids)
		}
	})
}

func TestCleanupFolder_APIErrorRetries(t *testing.T) {
	ctx := t.Context()
	c, mock := newTestController(t, "default")

	// Setup a custom folder
	mock.folders = []grafanaFolder{{ID: 1, UID: "folder-uid", Title: "Custom"}}
	// Force the IsEmpty check to return a transient API error
	mock.emptyErr = errors.New("transient grafana 500 error")

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: "default", Labels: map[string]string{CustomDashboardLabelKey: "true"}},
		Data:       map[string]string{}, // Empty data triggers immediate folder cleanup in updateDashboard
	}
	cm.Annotations = map[string]string{CustomFolderKey: "Custom"}
	c.uidMap["default/test-cm"] = trackedState{folder: "Custom"}

	err := c.updateDashboard(ctx, cm)

	if err == nil {
		t.Fatal("expected an error due to the transient API failure during cleanup")
	}

	if !strings.Contains(err.Error(), "transient grafana 500 error") {
		t.Errorf("expected the API error to bubble up, got: %v", err)
	}

	// Verify that the poller aborted immediately and didn't attempt to delete the folder
	if mock.deleteFolderCalled["folder-uid"] > 0 {
		t.Error("expected folder deletion to be skipped when the emptiness check fails")
	}
}

func TestGetDashboardCustomFolderTitle(t *testing.T) {
	testCaseList := []struct {
		name     string
		cm       *corev1.ConfigMap
		expected string
	}{
		{"nil cm", nil, "Custom"},
		{"default", &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "test"}}, "Custom"},
		{"custom", &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{CustomFolderKey: "my-folder"}}}, "my-folder"},
		{"general", &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{GeneralFolderKey: "true"}}}, ""},
	}
	for _, tc := range testCaseList {
		t.Run(tc.name, func(t *testing.T) {
			if got := getDashboardCustomFolderTitle(tc.cm); got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestProcessNextItem(t *testing.T) {
	ctx := t.Context()
	c, _ := newTestController(t, "default")

	t.Run("quit", func(t *testing.T) {
		q := newTestQueue()
		q.ShutDown()
		if c.processNextItem(ctx, q, newTestIndexer()) {
			t.Error("expected false")
		}
	})

	t.Run("retry and exhaustion", func(t *testing.T) {
		q := newTestQueue()
		indexer := newTestIndexer()
		c, mock := newTestController(t, "default")
		// Case 1: Unmarshal error - Logged and Ignored (no retry)
		cmBad := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "bad", Namespace: "default", Labels: map[string]string{CustomDashboardLabelKey: "true"}},
			Data:       map[string]string{"k": "{"},
		}
		indexer.Add(cmBad)
		q.Add("default/bad")
		if !c.processNextItem(ctx, q, indexer) {
			t.Fatal("expected true")
		}
		if q.NumRequeues("default/bad") != 0 {
			t.Errorf("expected 0 requeues for unmarshal error, got %d", q.NumRequeues("default/bad"))
		}

		// Case 2: API error - Retried
		mock.createErr = errors.New("api fail")
		cmRetry := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "retry", Namespace: "default", Labels: map[string]string{CustomDashboardLabelKey: "true"}},
			Data:       map[string]string{"k": "{}"},
		}
		indexer.Add(cmRetry)
		q.Add("default/retry")

		if !c.processNextItem(ctx, q, indexer) {
			t.Fatal("expected true")
		}
		if q.NumRequeues("default/retry") != 1 {
			t.Error("expected 1 requeue for API error")
		}

		// Exhaust retries for Case 2
		for i := 1; i < c.maxDashboardRetry; i++ {
			q.AddRateLimited("default/retry")
		}
		if !c.processNextItem(ctx, q, indexer) {
			t.Fatal("expected true")
		}
		if q.NumRequeues("default/retry") != 0 {
			t.Error("expected dropped after exhaustion")
		}
	})
}
