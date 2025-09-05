// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

// Package informer provides a controller that watches ManagedCluster resources to keep a label allowlist synchronized.
//
// The primary role of this package is to automatically discover all labels present on ManagedCluster
// resources and ensure they are added to the 'observability-managed-cluster-label-allowlist' ConfigMap.
// This allowlist is then used by the rbac-query-proxy to generate a synthetic metric that powers
// dynamic, multi-level filtering of clusters in Grafana.
package informer

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	proxyconfig "github.com/stolostron/multicluster-observability-operator/proxy/pkg/config"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	clusterclientset "open-cluster-management.io/api/client/cluster/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

var promLabelRegex = regexp.MustCompile(`[^\w]+`)

// ManagedClusterLabelList defines the structure of the label allowlist data stored in the ConfigMap.
type ManagedClusterLabelList struct {
	IgnoreList     []string `yaml:"ignore_labels,omitempty"`
	LabelList      []string `yaml:"labels"`
	RegexLabelList []string `yaml:"-"`
}

// Clone creates a deep copy of the ManagedClusterLabelList.
func (l *ManagedClusterLabelList) Clone() *ManagedClusterLabelList {
	if l == nil {
		return nil
	}
	return &ManagedClusterLabelList{
		IgnoreList:     slices.Clone(l.IgnoreList),
		LabelList:      slices.Clone(l.LabelList),
		RegexLabelList: slices.Clone(l.RegexLabelList),
	}
}

// ManagedClusterInformable defines the interface for accessing cached managed cluster data.
type ManagedClusterInformable interface {
	Run()
	HasSynced() bool
	GetAllManagedClusterNames() map[string]struct{}
	GetManagedClusterLabelList() []string
}

// ManagedClusterInformer caches ManagedCluster data and manages the label allowlist ConfigMap.
type ManagedClusterInformer struct {
	ctx                    context.Context
	clusterClient          clusterclientset.Interface
	kubeClient             kubernetes.Interface
	managedClusters        map[string]map[string]struct{}
	managedClustersMtx     sync.RWMutex
	inMemoryAllowlist      *ManagedClusterLabelList
	allowlistMtx           sync.RWMutex
	hasSynced              atomic.Bool
	syncAllowListCh        chan struct{}
	supervisorRestartDelay time.Duration
	debounceTimer          *time.Timer
	debounceDuration       time.Duration
}

// NewManagedClusterInformer creates a new ManagedClusterInformer.
func NewManagedClusterInformer(ctx context.Context, clusterClient clusterclientset.Interface,
	kubeClient kubernetes.Interface) *ManagedClusterInformer {
	return &ManagedClusterInformer{
		ctx:                    ctx,
		clusterClient:          clusterClient,
		kubeClient:             kubeClient,
		managedClusters:        make(map[string]map[string]struct{}),
		inMemoryAllowlist:      &ManagedClusterLabelList{},
		syncAllowListCh:        make(chan struct{}, 1),
		supervisorRestartDelay: 10 * time.Second,
		debounceDuration:       2 * time.Second,
	}
}

// Run starts the informers and worker goroutines.
func (i *ManagedClusterInformer) Run() {
	if err := i.ensureAllowlistConfigMapExists(); err != nil {
		klog.Fatalf("Failed to ensure managed cluster label allowlist configmap exists: %v", err)
	}

	if err := i.initialAllowlistLoad(); err != nil {
		klog.Warningf("Failed to perform initial load of allowlist: %v", err)
	}

	clusterWatchlist := cache.NewListWatchFromClient(
		i.clusterClient.ClusterV1().RESTClient(),
		"managedclusters",
		v1.NamespaceAll,
		fields.Everything(),
	)
	clusterOptions := cache.InformerOptions{
		ListerWatcher: clusterWatchlist,
		ObjectType:    &clusterv1.ManagedCluster{},
		Handler:       i.getManagedClusterEventHandler(),
	}
	_, clusterController := cache.NewInformerWithOptions(clusterOptions)

	cmWatchlist := cache.NewListWatchFromClient(i.kubeClient.CoreV1().RESTClient(), "configmaps",
		proxyconfig.ManagedClusterLabelAllowListNamespace,
		fields.OneTermEqualSelector("metadata.name", proxyconfig.ManagedClusterLabelAllowListConfigMapName))
	cmOptions := cache.InformerOptions{
		ListerWatcher: cmWatchlist,
		ObjectType:    &v1.ConfigMap{},
		Handler:       i.getAllowlistConfigMapEventHandler(),
	}
	_, cmController := cache.NewInformerWithOptions(cmOptions)

	go i.runConfigMapSyncSupervisor()

	go clusterController.Run(i.ctx.Done())
	go cmController.Run(i.ctx.Done())

	if !cache.WaitForCacheSync(i.ctx.Done(), clusterController.HasSynced, cmController.HasSynced) {
		klog.Error("Failed to sync informer caches")
		return
	}

	i.hasSynced.Store(true)
	klog.Info("Informer caches have successfully synced")

	i.managedClustersMtx.RLock()
	klog.Infof("Initial list of managed clusters: %v", slices.Sorted(maps.Keys(i.managedClusters)))
	i.managedClustersMtx.RUnlock()

	i.allowlistMtx.RLock()
	klog.Infof("Initial regex label list: %v", i.inMemoryAllowlist.RegexLabelList)
	i.allowlistMtx.RUnlock()

	// Trigger a sync after the initial cache sync to process all existing resources.
	i.syncAllowListCh <- struct{}{}
}

// GetAllManagedClusterNames returns a set of all cached managed cluster names.
func (i *ManagedClusterInformer) GetAllManagedClusterNames() map[string]struct{} {
	i.managedClustersMtx.RLock()
	defer i.managedClustersMtx.RUnlock()
	return extractMapKeysSet(i.managedClusters)
}

// GetManagedClusterLabelList returns a slice of all cached and sanitized label names.
func (i *ManagedClusterInformer) GetManagedClusterLabelList() []string {
	i.allowlistMtx.RLock()
	defer i.allowlistMtx.RUnlock()
	return slices.Clone(i.inMemoryAllowlist.RegexLabelList)
}

// HasSynced returns true if the informer caches have successfully synced.
func (i *ManagedClusterInformer) HasSynced() bool {
	return i.hasSynced.Load()
}

// getAllLabels returns a set of all unique labels across all cached managed clusters.
func (i *ManagedClusterInformer) getAllLabels() map[string]struct{} {
	i.managedClustersMtx.RLock()
	defer i.managedClustersMtx.RUnlock()
	allLabels := make(map[string]struct{})
	for _, labels := range i.managedClusters {
		for label := range labels {
			allLabels[label] = struct{}{}
		}
	}
	return allLabels
}

// getManagedClusterEventHandler creates the event handler for the ManagedCluster informer.
func (i *ManagedClusterInformer) getManagedClusterEventHandler() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			managedCluster := obj.(*clusterv1.ManagedCluster)
			klog.Infof("Added managed cluster: %s", managedCluster.Name)

			beforeLabels := i.getAllLabels()

			i.managedClustersMtx.Lock()
			i.managedClusters[managedCluster.Name] = extractMapKeysSet(managedCluster.Labels)
			i.managedClustersMtx.Unlock()

			afterLabels := i.getAllLabels()
			if !maps.Equal(beforeLabels, afterLabels) {
				i.syncAllowListCh <- struct{}{}
			}
		},

		DeleteFunc: func(obj any) {
			managedCluster := obj.(*clusterv1.ManagedCluster)
			klog.Infof("Deleted managed cluster: %s", managedCluster.Name)

			beforeLabels := i.getAllLabels()

			i.managedClustersMtx.Lock()
			delete(i.managedClusters, managedCluster.Name)
			i.managedClustersMtx.Unlock()

			afterLabels := i.getAllLabels()
			if !maps.Equal(beforeLabels, afterLabels) {
				i.syncAllowListCh <- struct{}{}
			}
		},

		UpdateFunc: func(oldObj, newObj any) {
			oldCluster := oldObj.(*clusterv1.ManagedCluster)
			newCluster := newObj.(*clusterv1.ManagedCluster)

			if maps.Equal(oldCluster.Labels, newCluster.Labels) {
				return
			}

			klog.Infof("Updated managed cluster labels: %s", newCluster.Name)

			beforeLabels := i.getAllLabels()

			i.managedClustersMtx.Lock()
			i.managedClusters[newCluster.Name] = extractMapKeysSet(newCluster.Labels)
			i.managedClustersMtx.Unlock()

			afterLabels := i.getAllLabels()
			if !maps.Equal(beforeLabels, afterLabels) {
				i.syncAllowListCh <- struct{}{}
			}
		},
	}
}

// getAllowlistConfigMapEventHandler creates the event handler for the allowlist ConfigMap informer.
func (i *ManagedClusterInformer) getAllowlistConfigMapEventHandler() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			klog.V(1).Infof("Observed addition of ConfigMap: %s", proxyconfig.ManagedClusterLabelAllowListConfigMapName)
			i.syncAllowListCh <- struct{}{}
		},

		DeleteFunc: func(obj any) {
			klog.Warningf("ConfigMap %s was deleted, recreating it", proxyconfig.ManagedClusterLabelAllowListConfigMapName)
			if err := i.ensureAllowlistConfigMapExists(); err != nil {
				klog.Errorf("Failed to recreate deleted ConfigMap %s: %v", proxyconfig.ManagedClusterLabelAllowListConfigMapName, err)
			}
		},

		UpdateFunc: func(oldObj, newObj any) {
			newConfig := newObj.(*v1.ConfigMap)
			oldConfig := oldObj.(*v1.ConfigMap)

			if reflect.DeepEqual(newConfig.Data, oldConfig.Data) {
				return
			}
			klog.V(1).Infof("Observed update of ConfigMap: %s", proxyconfig.ManagedClusterLabelAllowListConfigMapName)

			i.syncAllowListCh <- struct{}{}
		},
	}
}

// runConfigMapSyncSupervisor is a resilient worker that runs the sync loop and restarts it on unexpected failures.
func (i *ManagedClusterInformer) runConfigMapSyncSupervisor() {
	for {
		func() {
			defer func() {
				if r := recover(); r != nil {
					klog.Errorf("Recovered from panic in ConfigMap sync loop: %v", r)
				}
			}()
			i.syncLoop()
		}()

		select {
		case <-i.ctx.Done():
			klog.Info("ConfigMap sync supervisor shutting down.")
			return
		default:
			klog.Warningf("ConfigMap sync loop stopped unexpectedly. Restarting in %v...", i.supervisorRestartDelay)
			time.Sleep(i.supervisorRestartDelay)
		}
	}
}

// syncLoop waits for signals to process allowlist updates.
func (i *ManagedClusterInformer) syncLoop() {
	for {
		select {
		case <-i.syncAllowListCh:
			i.debounceSync()
		case <-i.ctx.Done():
			return
		}
	}
}

// debounceSync waits for a brief period of inactivity before triggering a sync to avoid redundant updates.
func (i *ManagedClusterInformer) debounceSync() {
	if i.debounceTimer != nil {
		i.debounceTimer.Stop()
	}

	i.debounceTimer = time.AfterFunc(i.debounceDuration, i.syncAllowlistConfigMap)
}

// syncAllowlistConfigMap is the core reconciliation function that updates the ConfigMap if needed.
func (i *ManagedClusterInformer) syncAllowlistConfigMap() {
	if !i.HasSynced() {
		klog.V(4).Info("Cache not synced yet, skipping ConfigMap update check")
		return
	}

	i.managedClustersMtx.RLock()
	managedClustersCopy := maps.Clone(i.managedClusters)
	i.managedClustersMtx.RUnlock()

	var newList *ManagedClusterLabelList
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		allowListCM, err := i.kubeClient.CoreV1().ConfigMaps(proxyconfig.ManagedClusterLabelAllowListNamespace).Get(
			i.ctx,
			proxyconfig.ManagedClusterLabelAllowListConfigMapName,
			metav1.GetOptions{},
		)
		if err != nil {
			return err
		}

		currentOnClusterList := &ManagedClusterLabelList{}
		if err := unmarshalData(allowListCM.Data,
			proxyconfig.ManagedClusterLabelAllowListConfigMapKey, currentOnClusterList); err != nil {
			// If unmarshalling fails, we cannot proceed. This is not a conflict, so we return nil.
			klog.Errorf("Failed to unmarshal data from ConfigMap, will not retry: %v", err)
			return nil
		}

		newList = generateAllowList(currentOnClusterList, managedClustersCopy)

		// If the generated list is identical to what's already on the cluster, no update is needed.
		if slices.Equal(newList.LabelList, currentOnClusterList.LabelList) && slices.Equal(newList.IgnoreList, currentOnClusterList.IgnoreList) {
			klog.V(4).Info("Managed cluster label allowlist is already up-to-date")
			newList = nil // Signal that no update was performed.
			return nil
		}

		if err := marshalData(allowListCM,
			proxyconfig.ManagedClusterLabelAllowListConfigMapKey, newList); err != nil {
			return err
		}

		_, err = i.kubeClient.CoreV1().ConfigMaps(proxyconfig.ManagedClusterLabelAllowListNamespace).Update(
			i.ctx,
			allowListCM,
			metav1.UpdateOptions{},
		)
		if err == nil {
			logAllowlistChanges(currentOnClusterList, newList)
		}
		return err
	})

	if err != nil {
		klog.Errorf("Failed to update managed cluster label allowlist ConfigMap after retries: %v", err)
		return
	}

	// If newList is not nil, an update was successfully performed.
	if newList != nil {
		i.setInMemoryAllowlist(newList)
	}
}

// setInMemoryAllowlist updates the in-memory allowlist and sanitizes the labels for Prometheus.
func (i *ManagedClusterInformer) setInMemoryAllowlist(list *ManagedClusterLabelList) {
	list.RegexLabelList = make([]string, 0, len(list.LabelList))
	for _, label := range list.LabelList {
		list.RegexLabelList = append(list.RegexLabelList, promLabelRegex.ReplaceAllString(label, "_"))
	}
	i.allowlistMtx.Lock()
	i.inMemoryAllowlist = list
	i.allowlistMtx.Unlock()
}

// initialAllowlistLoad reads the allowlist ConfigMap from the cluster and populates the in-memory cache.
// This is intended to be called once at startup.
func (i *ManagedClusterInformer) initialAllowlistLoad() error {
	allowListCM, err := i.kubeClient.CoreV1().ConfigMaps(proxyconfig.ManagedClusterLabelAllowListNamespace).Get(
		i.ctx,
		proxyconfig.ManagedClusterLabelAllowListConfigMapName,
		metav1.GetOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to get allowlist ConfigMap for initial load: %w", err)
	}

	list := &ManagedClusterLabelList{}
	if err := unmarshalData(allowListCM.Data, proxyconfig.ManagedClusterLabelAllowListConfigMapKey, list); err != nil {
		// An error here might mean the ConfigMap is empty or malformed.
		// We can proceed with an empty list and let the sync loop correct it.
		klog.Warningf("Failed to unmarshal data from ConfigMap for initial load: %v", err)
	}

	i.setInMemoryAllowlist(list)
	return nil
}

// ensureAllowlistConfigMapExists checks if the allowlist ConfigMap exists and creates it if it doesn't.
func (i *ManagedClusterInformer) ensureAllowlistConfigMapExists() error {
	_, err := i.kubeClient.CoreV1().ConfigMaps(proxyconfig.ManagedClusterLabelAllowListNamespace).Get(
		i.ctx,
		proxyconfig.ManagedClusterLabelAllowListConfigMapName,
		metav1.GetOptions{},
	)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			klog.Info("Managed cluster label allowlist ConfigMap not found, creating it")
			cm := proxyconfig.CreateManagedClusterLabelAllowListCM(
				proxyconfig.ManagedClusterLabelAllowListNamespace,
			)
			_, createErr := i.kubeClient.CoreV1().ConfigMaps(proxyconfig.ManagedClusterLabelAllowListNamespace).Create(i.ctx, cm, metav1.CreateOptions{})
			if createErr != nil {
				return fmt.Errorf("failed to create allowlist ConfigMap: %w", createErr)
			}
		} else {
			return fmt.Errorf("failed to get allowlist ConfigMap: %w", err)
		}
	}
	return nil
}

// generateAllowList creates a new allowlist by merging newly discovered labels with the existing ones.
func generateAllowList(currentAllowList *ManagedClusterLabelList, managedClusters map[string]map[string]struct{}) *ManagedClusterLabelList {
	labelsFromClusters := make(map[string]struct{})
	for _, labels := range managedClusters {
		for label := range labels {
			labelsFromClusters[label] = struct{}{}
		}
	}

	allowedLabels := make(map[string]struct{})
	for _, label := range currentAllowList.LabelList {
		allowedLabels[label] = struct{}{}
	}

	ignoredLabels := make(map[string]struct{})
	for _, label := range currentAllowList.IgnoreList {
		ignoredLabels[label] = struct{}{}
	}

	// Add labels from clusters to the allowed list, skipping any that are in the ignore list.
	for label := range labelsFromClusters {
		if _, isIgnored := ignoredLabels[label]; !isIgnored {
			allowedLabels[label] = struct{}{}
		}
	}

	// Ensure required labels are present.
	for _, requiredLabel := range proxyconfig.RequiredLabelList {
		if _, isIgnored := ignoredLabels[requiredLabel]; !isIgnored {
			allowedLabels[requiredLabel] = struct{}{}
		}
	}

	// Remove any labels that are in the ignore list from the allowed list.
	for label := range ignoredLabels {
		delete(allowedLabels, label)
	}

	return &ManagedClusterLabelList{
		IgnoreList: slices.Sorted(maps.Keys(ignoredLabels)),
		LabelList:  slices.Sorted(maps.Keys(allowedLabels)),
	}
}

// logAllowlistChanges calculates and logs the differences between two allowlists.
func logAllowlistChanges(before, after *ManagedClusterLabelList) {
	labelsAdded, labelsRemoved := getDiff(before.LabelList, after.LabelList)
	ignoredAdded, ignoredRemoved := getDiff(before.IgnoreList, after.IgnoreList)

	diffs := []struct {
		name string
		data []string
	}{
		{"labels_added", labelsAdded},
		{"labels_removed", labelsRemoved},
		{"ignored_added", ignoredAdded},
		{"ignored_removed", ignoredRemoved},
	}

	var logParts []string
	for _, d := range diffs {
		if len(d.data) > 0 {
			logParts = append(logParts, fmt.Sprintf("%s=%v", d.name, d.data))
		}
	}

	if len(logParts) > 0 {
		klog.Infof("Successfully updated managed cluster label allowlist ConfigMap: %s", strings.Join(logParts, ", "))
	}
}

// getDiff calculates the items added to or removed from a string slice.
func getDiff(before, after []string) ([]string, []string) {
	beforeMap := make(map[string]struct{}, len(before))
	for _, s := range before {
		beforeMap[s] = struct{}{}
	}

	added := []string{}
	for _, s := range after {
		if _, found := beforeMap[s]; found {
			// If it's in both, it's not a removed item. Delete it from the map.
			delete(beforeMap, s)
		} else {
			// If it's in after but not before, it was added.
			added = append(added, s)
		}
	}

	slices.Sort(added)
	return added, slices.Sorted(maps.Keys(beforeMap))
}

func extractMapKeysSet[K comparable, V any](inputMap map[K]V) map[K]struct{} {
	ret := make(map[K]struct{}, len(inputMap))
	for key := range inputMap {
		ret[key] = struct{}{}
	}
	return ret
}

func marshalData(obj *v1.ConfigMap, key string,
	labelList *ManagedClusterLabelList) error {
	data, err := yaml.Marshal(labelList)
	if err != nil {
		return fmt.Errorf("failed to marshal allowlist data: %w", err)
	}

	if obj.Data == nil {
		obj.Data = make(map[string]string)
	}
	obj.Data[key] = string(data)

	return nil
}

func unmarshalData(data map[string]string, key string,
	labelList *ManagedClusterLabelList) error {
	if err := yaml.Unmarshal([]byte(data[key]), labelList); err != nil {
		return fmt.Errorf("failed to unmarshal data for key %s: %w", key, err)
	}

	return nil
}
