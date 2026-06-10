// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/stolostron/multicluster-observability-operator/loaders/dashboards/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

const (
	// Annotation and Label keys
	CustomFolderKey     = "observability.open-cluster-management.io/dashboard-folder"
	GeneralFolderKey    = "general-folder"
	DefaultCustomFolder = "Custom"
	HomeDashboardUIDKey = "home-dashboard-uid"
	SetHomeDashboardKey = "set-home-dashboard"

	// CustomDashboardLabelKey is the label used to identify Grafana custom dashboards
	CustomDashboardLabelKey = "grafana-custom-dashboard"

	// ARCHITECTURAL NOTE: This controller assumes EXCLUSIVE ownership of the Grafana instance's
	// dashboard space. Any dashboard found in Grafana that does not have a corresponding
	// ConfigMap in the watched namespace will be deleted during the periodic orphan cleanup.
	// This ensures that Kubernetes remains the absolute source of truth.

	// Timing and Retries
	folderCreationDelay      = 1 * time.Second
	permissionsRetryDelay    = 5 * time.Second
	defaultMaxDashboardRetry = 40
	resyncPeriod             = 10 * time.Minute
	reconcileTimeout         = 2 * time.Minute
)

type trackedState struct {
	uids   []string
	folder string
}

// GrafanaDashboardController manages the lifecycle of Grafana dashboards
type GrafanaDashboardController struct {
	kubeClient        corev1client.CoreV1Interface
	grafana           GrafanaClient
	maxDashboardRetry int
	watchedNS         string
	indexer           cache.Indexer

	// uidMap tracks which Grafana UIDs and folders are associated with a given ConfigMap (key).
	// This allows for immediate deletion of dashboards and empty folders when a ConfigMap is removed or moved.
	uidMap map[string]trackedState

	// reconcileMu ensures that only one reconciliation operation (ConfigMap update, deletion,
	// or total orphan sweep) interacts with the Grafana API and the internal state at a time.
	// This prevents race conditions between the incremental worker and the periodic sweep.
	// We use a channel as a semaphore to support context-aware locking and timeouts.
	reconcileMu chan struct{}
}

// NewGrafanaDashboardController creates a new GrafanaDashboardController
func NewGrafanaDashboardController(kubeClient corev1client.CoreV1Interface, uri string) (*GrafanaDashboardController, error) {
	if uri == "" {
		return nil, errors.New("grafana URI is required")
	}
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("invalid grafana URI %q: %w", uri, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid grafana URI %q: scheme and host are required", uri)
	}

	ns := os.Getenv("POD_NAMESPACE")
	if ns == "" {
		ns = "open-cluster-management-observability"
		klog.InfoS("POD_NAMESPACE environment variable is empty. Defaulting to standard observability namespace.", "namespace", ns)
	}
	c := &GrafanaDashboardController{
		kubeClient:        kubeClient,
		grafana:           &grafanaClient{uri: uri},
		maxDashboardRetry: defaultMaxDashboardRetry,
		watchedNS:         ns,
		uidMap:            make(map[string]trackedState),
		reconcileMu:       make(chan struct{}, 1),
	}
	c.reconcileMu <- struct{}{}
	return c, nil
}

// Run runs the controller until the context is canceled.
func (c *GrafanaDashboardController) Run(ctx context.Context) error {
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[any]())
	var wg sync.WaitGroup
	defer wg.Wait()
	defer queue.ShutDown()

	informer, err := c.newKubeInformer(queue)
	if err != nil {
		return fmt.Errorf("failed to get informer: %w", err)
	}

	go informer.Run(ctx.Done())

	klog.Info("waiting for informer caches to sync")
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		if ctx.Err() != nil {
			return fmt.Errorf("context canceled while waiting for cache sync: %w", ctx.Err())
		}
		return fmt.Errorf("failed to sync cache")
	}

	c.indexer = informer.GetIndexer()

	klog.InfoS("performing initial orphan cleanup")
	// Retry the initial sweep for up to 1 minute to allow Grafana to start.
	// This ensures we discover existing UIDs and don't start with a blank state
	// which would cause the first delete event to be ignored.
	err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 1*time.Minute, true, func(ctx context.Context) (bool, error) {
		err := c.cleanupOrphanDashboards(ctx, c.indexer)
		if err != nil {
			klog.InfoS("waiting for Grafana to be ready", "error", err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("initial orphan cleanup failed after 1 minute timeout: %w", err)
	}

	klog.Info("starting worker")
	wg.Go(func() {
		wait.UntilWithContext(ctx, func(ctx context.Context) { c.runWorker(ctx, queue, c.indexer) }, time.Second)
	})

	// Periodically run orphan cleanup to catch any missed deletions and ghost folders.
	wg.Go(func() {
		wait.UntilWithContext(ctx, func(ctx context.Context) {
			tCtx, cancel := context.WithTimeout(ctx, reconcileTimeout)
			defer cancel()
			if err := c.cleanupOrphanDashboards(tCtx, c.indexer); err != nil {
				klog.ErrorS(err, "periodic orphan cleanup failed")
			}
		}, resyncPeriod)
	})

	<-ctx.Done()
	klog.Info("stopping controller")
	return nil
}

func (c *GrafanaDashboardController) runWorker(ctx context.Context, queue workqueue.TypedRateLimitingInterface[any], indexer cache.Indexer) {
	for c.processNextItem(ctx, queue, indexer) {
	}
}

func (c *GrafanaDashboardController) processNextItem(ctx context.Context, queue workqueue.TypedRateLimitingInterface[any], indexer cache.Indexer) bool {
	obj, quit := queue.Get()
	if quit {
		return false
	}
	defer queue.Done(obj)

	tCtx, cancel := context.WithTimeout(ctx, reconcileTimeout)
	defer cancel()

	err := c.syncHandler(tCtx, obj, indexer)
	if err == nil {
		queue.Forget(obj)
		return true
	}

	if queue.NumRequeues(obj) < c.maxDashboardRetry {
		klog.ErrorS(err, "syncing dashboard failed, retrying", "object", obj)
		queue.AddRateLimited(obj)
		return true
	}

	queue.Forget(obj)
	klog.ErrorS(err, "dashboard could not be processed after maximum retries", "object", obj, "maxRetries", c.maxDashboardRetry)
	return true
}

func (c *GrafanaDashboardController) syncHandler(ctx context.Context, item any, indexer cache.Indexer) error {
	key, ok := item.(string)
	if !ok {
		klog.ErrorS(fmt.Errorf("unexpected item type: %T", item), "dropping invalid queue item")
		return nil
	}

	obj, exists, err := indexer.GetByKey(key)
	if err != nil {
		return err
	}

	if !exists {
		klog.InfoS("ConfigMap deleted, performing immediate cleanup from Grafana", "key", key)
		return c.deleteTrackedUIDs(ctx, key)
	}

	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return fmt.Errorf("unexpected type in cache: %T for key %s", obj, key)
	}

	if !isDesiredDashboardConfigmap(cm) {
		klog.InfoS("ConfigMap no longer a desired dashboard, performing immediate cleanup from Grafana", "namespace", cm.Namespace, "name", cm.Name)
		return c.deleteTrackedUIDs(ctx, key)
	}

	klog.InfoS("syncing dashboard", "namespace", cm.Namespace, "name", cm.Name)
	return c.updateDashboard(ctx, cm)
}

func (c *GrafanaDashboardController) lockReconcile(ctx context.Context) error {
	select {
	case <-c.reconcileMu:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *GrafanaDashboardController) unlockReconcile() {
	select {
	case c.reconcileMu <- struct{}{}:
	default:
		// Should not happen if used correctly
		klog.Error("reconcileMu semaphore already full during unlock")
	}
}

func (c *GrafanaDashboardController) deleteTrackedUIDs(ctx context.Context, key string) error {
	if err := c.lockReconcile(ctx); err != nil {
		return err
	}
	defer c.unlockReconcile()

	state, exists := c.uidMap[key]
	if !exists {
		return nil
	}

	var errs []error
	var failedUIDs []string
	for _, uid := range state.uids {
		klog.InfoS("deleting tracked dashboard from Grafana", "key", key, "uid", uid)
		if err := c.grafana.DeleteDashboard(ctx, uid); err != nil {
			klog.ErrorS(err, "failed to delete tracked dashboard", "uid", uid)
			errs = append(errs, err)
			failedUIDs = append(failedUIDs, uid)
		}
	}

	if len(failedUIDs) > 0 {
		// Update the map to only track what's left
		c.uidMap[key] = trackedState{
			uids:   failedUIDs,
			folder: state.folder,
		}
		return errors.Join(errs...)
	}

	// All deleted successfully
	delete(c.uidMap, key)

	if state.folder != "" {
		return c.cleanupFolderIfEmpty(ctx, state.folder)
	}

	return nil
}

func (c *GrafanaDashboardController) newKubeInformer(queue workqueue.TypedRateLimitingInterface[any]) (cache.SharedIndexInformer, error) {
	watchlist := &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, opts metav1.ListOptions) (k8sruntime.Object, error) {
			return c.kubeClient.ConfigMaps(c.watchedNS).List(ctx, opts)
		},
		WatchFuncWithContext: func(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
			return c.kubeClient.ConfigMaps(c.watchedNS).Watch(ctx, opts)
		},
	}
	kubeInformer := cache.NewSharedIndexInformer(
		watchlist,
		&corev1.ConfigMap{},
		resyncPeriod,
		cache.Indexers{},
	)

	_, err := kubeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			if cm, ok := obj.(*corev1.ConfigMap); ok && c.shouldEnqueue(nil, cm) {
				key, err := cache.MetaNamespaceKeyFunc(obj)
				if err == nil {
					queue.Add(key)
				}
			}
		},
		UpdateFunc: func(oldObj, newObj any) {
			oldCm, _ := oldObj.(*corev1.ConfigMap)
			newCm, okNew := newObj.(*corev1.ConfigMap)
			if okNew && c.shouldEnqueue(oldCm, newCm) {
				key, err := cache.MetaNamespaceKeyFunc(newObj)
				if err == nil {
					queue.Add(key)
				}
			}
		},
		DeleteFunc: func(obj any) {
			// Always enqueue deletions to ensure tracked UIDs are cleaned up.
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add(key)
			}
		},
	})
	return kubeInformer, err
}

// shouldEnqueue determines if a ConfigMap change should be added to the workqueue.
func (c *GrafanaDashboardController) shouldEnqueue(oldCm, newCm *corev1.ConfigMap) bool {
	if newCm == nil {
		return false
	}
	// Enqueue if the new state is a desired dashboard
	if isDesiredDashboardConfigmap(newCm) {
		return true
	}
	// Enqueue if it WAS a desired dashboard (to trigger immediate cleanup)
	if oldCm != nil && isDesiredDashboardConfigmap(oldCm) {
		return true
	}
	return false
}

func (c *GrafanaDashboardController) cleanupOrphanDashboards(ctx context.Context, indexer cache.Indexer) error {
	if err := c.lockReconcile(ctx); err != nil {
		return err
	}
	defer c.unlockReconcile()

	klog.InfoS("scanning for orphan dashboards in Grafana")
	dashboards, err := c.grafana.ListAllDashboards(ctx)
	if err != nil {
		return fmt.Errorf("failed to list dashboards for cleanup: %w", err)
	}

	klog.InfoS("found dashboards in Grafana", "count", len(dashboards))

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Build a set of all expected UIDs from ConfigMaps in the indexer.
	// Also populate the in-memory uidMap for immediate deletion support.
	list := indexer.List()
	expectedUIDs := make(map[string]struct{}, len(list))
	newUIDMap := make(map[string]trackedState, len(list))

	for _, obj := range list {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		cm, ok := obj.(*corev1.ConfigMap)
		if !ok {
			klog.V(4).InfoS("unexpected object type during cleanup", "type", fmt.Sprintf("%T", obj))
			continue
		}
		if isDesiredDashboardConfigmap(cm) {
			cmKey, err := cache.MetaNamespaceKeyFunc(cm)
			if err != nil {
				klog.ErrorS(err, "failed to get key for ConfigMap during cleanup", "namespace", cm.Namespace, "name", cm.Name)
				continue
			}
			folder := getDashboardCustomFolderTitle(cm)
			state := trackedState{folder: folder}
			for key, val := range cm.Data {
				dashboard := map[string]any{}
				if err := json.Unmarshal([]byte(val), &dashboard); err != nil {
					klog.V(2).InfoS("failed to unmarshal dashboard during cleanup, falling back to name-based UID", "namespace", cm.Namespace, "name", cm.Name, "key", key)
					dashboard = nil
				}
				uid, err := c.resolveDashboardUID(dashboard, cm, key)
				if err == nil {
					expectedUIDs[uid] = struct{}{}
					state.uids = append(state.uids, uid)
				} else {
					klog.ErrorS(err, "failed to resolve UID for dashboard key during cleanup", "namespace", cm.Namespace, "name", cm.Name, "key", key)
				}
			}
			newUIDMap[cmKey] = state
		}
	}

	// Update the controller's map with the fresh state discovered from Kubernetes.
	// We perform a surgical update instead of a full overwrite to minimize the risk
	// of overwriting a concurrent syncHandler update for a specific ConfigMap.
	// 1. Remove keys from the map that are no longer present in Kubernetes as desired dashboards.
	for key := range c.uidMap {
		if _, exists := newUIDMap[key]; !exists {
			delete(c.uidMap, key)
		}
	}
	// 2. Update/Add keys from the fresh scan.
	// We only overwrite the state for a ConfigMap if the desired UIDs or folder changed.
	// This preserves any "failed deletions" (extra UIDs) that updateDashboard might
	// be tracking, ensuring they continue to be retried until they successfully
	// vanish from Grafana.
	for key, newState := range newUIDMap {
		oldState, exists := c.uidMap[key]
		if !exists || oldState.folder != newState.folder || !uidsMatch(oldState.uids, newState.uids) {
			c.uidMap[key] = newState
		}
	}

	folderUIDsWithDashboards := make(map[string]struct{})
	for _, d := range dashboards {
		if _, expected := expectedUIDs[d.UID]; !expected {
			klog.InfoS("deleting orphan dashboard", "uid", d.UID)
			if err := c.grafana.DeleteDashboard(ctx, d.UID); err != nil {
				klog.ErrorS(err, "failed to delete orphan dashboard", "uid", d.UID)
			}
		} else if d.FolderUID != "" {
			folderUIDsWithDashboards[d.FolderUID] = struct{}{}
		}
	}

	// orphan deletion may leave folders empty; this is a catch-all sweep.
	c.cleanupAllEmptyFolders(ctx, folderUIDsWithDashboards)
	return nil
}

func (c *GrafanaDashboardController) findCustomFolderID(ctx context.Context, folderTitle string) int64 {
	folders, err := c.grafana.ListFolders(ctx)
	if err != nil {
		klog.ErrorS(err, "failed to list folders")
		return 0
	}

	for _, folder := range folders {
		if folder.Title == folderTitle {
			return folder.ID
		}
	}
	return 0
}

func (c *GrafanaDashboardController) createCustomFolder(ctx context.Context, folderTitle string) int64 {
	folderID := c.findCustomFolderID(ctx, folderTitle)
	if folderID != 0 {
		return folderID
	}

	folder, err := c.grafana.CreateFolder(ctx, folderTitle)
	if err != nil {
		// Handle race condition
		if retryID := c.findCustomFolderID(ctx, folderTitle); retryID != 0 {
			return retryID
		}
		klog.ErrorS(err, "failed to create custom folder", "folderTitle", folderTitle)
		return 0
	}

	select {
	case <-ctx.Done():
		return 0
	case <-time.After(folderCreationDelay):
	}

	// check permissions
	hasPerm, err := c.grafana.HasPermissions(ctx, folder.UID)
	if err != nil {
		klog.ErrorS(err, "failed to check folder permissions", "folderTitle", folderTitle)
		return 0
	}
	if !hasPerm {
		klog.InfoS("failed to set permissions for folder. Deleting folder and retrying later.", "folderTitle", folderTitle)
		select {
		case <-ctx.Done():
			return 0
		case <-time.After(permissionsRetryDelay):
		}

		if err := c.grafana.DeleteFolder(ctx, folder.UID); err != nil {
			klog.ErrorS(err, "failed to delete folder during retry", "folderUID", folder.UID)
		}
		return 0
	}

	return folder.ID
}

func (c *GrafanaDashboardController) cleanupFolderIfEmpty(ctx context.Context, folderTitle string) error {
	if folderTitle == "" {
		return nil
	}
	folderID := c.findCustomFolderID(ctx, folderTitle)
	if folderID == 0 {
		return nil
	}
	folder, err := c.grafana.GetFolderByID(ctx, folderID)
	if err != nil {
		var gErr *GrafanaError
		if errors.As(err, &gErr) && gErr.Status == http.StatusNotFound {
			return nil // Folder already gone, nothing to clean
		}
		return fmt.Errorf("failed to check folder status in Grafana: %w", err)
	}

	// Check if the folder is empty. Grafana's search index is eventually consistent,
	// so if this check fails because dashboards are still being indexed as "present",
	// we return an error to trigger a rate-limited retry in the workqueue.
	empty, err := c.grafana.IsEmpty(ctx, folder.UID)
	if err != nil {
		return fmt.Errorf("failed to check folder emptiness: %w", err)
	}
	if !empty {
		return fmt.Errorf("folder %q is not empty yet in Grafana index; will retry cleanup", folderTitle)
	}

	klog.InfoS("deleting empty folder", "title", folderTitle, "uid", folder.UID)
	if err := c.grafana.DeleteFolder(ctx, folder.UID); err != nil {
		return fmt.Errorf("failed to delete empty folder: %w", err)
	}
	return nil
}

func (c *GrafanaDashboardController) cleanupAllEmptyFolders(ctx context.Context, folderUIDsWithDashboards map[string]struct{}) {
	klog.InfoS("scanning for empty folders in Grafana")
	folders, err := c.grafana.ListFolders(ctx)
	if err != nil {
		klog.ErrorS(err, "failed to list folders for cleanup")
		return
	}

	for _, folder := range folders {
		if folder.ID == 0 || folder.UID == "" {
			continue
		}

		if _, hasDashboards := folderUIDsWithDashboards[folder.UID]; !hasDashboards {
			klog.InfoS("deleting empty folder during total sweep", "folderTitle", folder.Title, "uid", folder.UID)
			if err := c.grafana.DeleteFolder(ctx, folder.UID); err != nil {
				klog.ErrorS(err, "failed to delete empty folder", "folderTitle", folder.Title, "folderUID", folder.UID)
			}
		}
	}
}

// resolveDashboardUID extracts the UID from the dashboard JSON or generates a unique fallback.
// If dashboard is nil, it skips JSON extraction and directly generates the fallback UID.
func (c *GrafanaDashboardController) resolveDashboardUID(dashboard map[string]any, cm *corev1.ConfigMap, key string) (string, error) {
	if dashboard != nil {
		if uidVal, ok := dashboard["uid"]; ok && uidVal != nil {
			uidStr, ok := uidVal.(string)
			if ok && uidStr != "" {
				return uidStr, nil
			}
		}
	}

	// Fallback UID generation logic (Legacy Compatibility)
	// The old loader used: util.GenerateUID(cm.Name, cm.Namespace) -> "name-namespace"
	// To support multi-dashboard CMs without breaking existing single-dashboard CMs:
	// 1. If key matches CM name (common case), use CM name to keep legacy UID.
	// 2. Otherwise, use name-key to ensure uniqueness.
	nameArg := cm.GetName()
	keyBase := key
	for _, suffix := range []string{".json", ".yaml", ".yml"} {
		keyBase = strings.TrimSuffix(keyBase, suffix)
	}
	if keyBase != cm.GetName() && !strings.Contains(cm.GetName(), keyBase) {
		nameArg = cm.GetName() + "-" + keyBase
	}

	uid, err := util.GenerateUID(nameArg, cm.GetNamespace())
	if err != nil {
		return "", fmt.Errorf("failed to generate fallback dashboard UID for key %s: %w", key, err)
	}
	klog.V(2).InfoS("dashboard UID not set in JSON, using generated fallback", "uid", uid, "key", key)
	return uid, nil
}

// updateDashboard handles the synchronization of dashboards from a ConfigMap to Grafana.
// Note: In most use cases, a ConfigMap contains a single dashboard key. We optimize for this
// pattern while still supporting multi-dashboard ConfigMaps by accumulating errors and
// ensuring sibling dashboards are not blocked by a failure in one.
func (c *GrafanaDashboardController) updateDashboard(ctx context.Context, newObj *corev1.ConfigMap) error {
	if err := c.lockReconcile(ctx); err != nil {
		return err
	}
	defer c.unlockReconcile()

	var folderID int64
	folderTitle := getDashboardCustomFolderTitle(newObj)
	if folderTitle != "" {
		folderID = c.createCustomFolder(ctx, folderTitle)
		if folderID == 0 {
			return errors.New("failed to get folder id")
		}
	}

	homeDashboardUID := ""
	labels := newObj.Labels
	annotations := newObj.Annotations
	if strings.ToLower(annotations[SetHomeDashboardKey]) == "true" && labels[HomeDashboardUIDKey] != "" {
		homeDashboardUID = labels[HomeDashboardUIDKey]
	}

	cmKey, err := cache.MetaNamespaceKeyFunc(newObj)
	if err != nil {
		return fmt.Errorf("failed to get key for ConfigMap: %w", err)
	}

	var errs []error
	currentUIDs := make([]string, 0, len(newObj.Data))

	for key, value := range newObj.Data {
		dashboard := map[string]any{}
		err := json.Unmarshal([]byte(value), &dashboard)
		if err != nil {
			// Unmarshal errors are terminal user input issues. We log them as errors
			// for visibility but skip the entry to avoid endless workqueue retries,
			// while letting other valid dashboards in the same ConfigMap proceed.
			klog.ErrorS(err, "failed to unmarshal dashboard data, skipping", "namespace", newObj.Namespace, "name", newObj.Name, "key", key)
			continue
		}

		uid, err := c.resolveDashboardUID(dashboard, newObj, key)
		if err != nil {
			// UID resolution errors are also considered terminal input errors.
			klog.ErrorS(err, "failed to resolve dashboard UID, skipping", "namespace", newObj.Namespace, "name", newObj.Name, "key", key)
			continue
		}

		// We record the UID as part of the "desired state" (currentUIDs) BEFORE the API call.
		// This ensures that if the API call fails, we still "remember" this dashboard
		// belongs to this ConfigMap, protecting it from being erroneously deleted
		// by the cleanup phase below.
		currentUIDs = append(currentUIDs, uid)
		dashboard["uid"] = uid
		delete(dashboard, "id")

		id, err := c.grafana.CreateOrUpdateDashboard(ctx, dashboard, folderID)
		if err != nil {
			var gErr *GrafanaError
			if errors.As(err, &gErr) {
				if gErr.IsNameExists() {
					errs = append(errs, fmt.Errorf("the dashboard name already existed"))
					continue
				}
			}
			errs = append(errs, err)
			continue
		}

		if homeDashboardUID != "" && uid == homeDashboardUID && id != 0 {
			klog.InfoS("Setting home dashboard", "title", dashboard["title"])
			if err := c.grafana.SetHomeDashboard(ctx, id); err != nil {
				klog.ErrorS(err, "failed to set home dashboard", "title", dashboard["title"])
				errs = append(errs, err)
			} else {
				klog.InfoS("home dashboard set successfully", "title", dashboard["title"], "id", id)
			}
		}
		klog.InfoS("dashboard created/updated successfully", "name", newObj.Name, "uid", uid)
	}

	// Immediate cleanup for keys removed from this ConfigMap.
	// We compare the UIDs we just identified as desired (currentUIDs) against
	// the UIDs we knew about from the previous successful reconcile (oldUIDs).
	oldState := c.uidMap[cmKey]

	// Find UIDs that were in the map but are not in the current ConfigMap data
	currentSet := make(map[string]struct{}, len(currentUIDs))
	for _, uid := range currentUIDs {
		currentSet[uid] = struct{}{}
	}

	var failedDeletions []string
	for _, oldUID := range oldState.uids {
		if _, exists := currentSet[oldUID]; !exists {
			klog.InfoS("dashboard key removed from ConfigMap, deleting from Grafana", "namespace", newObj.Namespace, "name", newObj.Name, "uid", oldUID)
			if err := c.grafana.DeleteDashboard(ctx, oldUID); err != nil {
				klog.ErrorS(err, "failed to delete removed dashboard", "uid", oldUID)
				errs = append(errs, err)
				// We keep failed deletions in our tracking to retry them in the next cycle
				// or let the Total Sweep handle them, ensuring we don't lose track of orphans.
				failedDeletions = append(failedDeletions, oldUID)
			}
		}
	}

	// Update the map with current state + those that failed to delete.
	c.uidMap[cmKey] = trackedState{
		uids:   append(currentUIDs, failedDeletions...),
		folder: folderTitle,
	}

	// Perform immediate folder cleanup.
	// 1. Check the CURRENT folder (in case all dashboards were removed from it but CM still exists)
	if folderTitle != "" && len(currentUIDs) == 0 {
		if err := c.cleanupFolderIfEmpty(ctx, folderTitle); err != nil {
			errs = append(errs, err)
		}
	}
	// 2. Check the OLD folder (if it changed)
	if oldState.folder != "" && oldState.folder != folderTitle {
		if err := c.cleanupFolderIfEmpty(ctx, oldState.folder); err != nil {
			errs = append(errs, err)
		}
	}
	// For immediate UX, the sweep is 10m away, which is acceptable for an empty folder.

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

func isDesiredDashboardConfigmap(cm *corev1.ConfigMap) bool {
	if cm == nil {
		return false
	}

	labels := cm.Labels
	if strings.ToLower(labels[CustomDashboardLabelKey]) == "true" {
		return true
	}
	if strings.ToLower(labels[GeneralFolderKey]) == "true" {
		return true
	}

	// Platform dashboards are created by the MCO operator and typically follow this naming convention.
	// We check both ownerRef and name prefix to avoid processing unrelated MCO resources like configs.
	owners := cm.GetOwnerReferences()
	for _, owner := range owners {
		if owner.Kind == "MultiClusterObservability" && strings.HasPrefix(cm.Name, "grafana-dashboard-") {
			return true
		}
	}

	return false
}

func getDashboardCustomFolderTitle(cm *corev1.ConfigMap) string {
	if cm == nil {
		return DefaultCustomFolder
	}

	labels := cm.Labels
	if labels[GeneralFolderKey] == "true" {
		return ""
	}

	annotations := cm.Annotations
	if annotations[CustomFolderKey] != "" {
		return annotations[CustomFolderKey]
	}

	return DefaultCustomFolder
}

// uidsMatch returns true if all UIDs in the new set are already present in the old set.
// This is used to detect if the "Desired" state from Kubernetes has changed, while
// allowing the "Old" state to contain additional UIDs (failed deletions) that
// we want to preserve.
func uidsMatch(oldUIDs, newUIDs []string) bool {
	if len(newUIDs) > len(oldUIDs) {
		return false
	}
	oldSet := make(map[string]struct{}, len(oldUIDs))
	for _, u := range oldUIDs {
		oldSet[u] = struct{}{}
	}
	for _, u := range newUIDs {
		if _, exists := oldSet[u]; !exists {
			return false
		}
	}
	return true
}
