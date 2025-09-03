// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package informer

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"sync"
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

type ManagedClusterLabelList struct {
	IgnoreList     []string `yaml:"ignore_labels,omitempty"`
	LabelList      []string `yaml:"labels"`
	RegexLabelList []string `yaml:"-"`
}

// ManagedClusterInformable defines the interface for accessing cached managed cluster data.
type ManagedClusterInformable interface {
	Run()
	HasSynced() bool
	GetAllManagedClusterNames() map[string]struct{}
	GetManagedClusterLabelList() []string
}

// ManagedClusterInformer caches managed cluster data and manages the label allowlist.
// It uses informers to watch for changes to ManagedCluster resources and the allowlist ConfigMap,
// keeping the cache up-to-date.
type ManagedClusterInformer struct {
	ctx                            context.Context
	clusterClient                  clusterclientset.Interface
	kubeClient                     kubernetes.Interface
	managedClusters                map[string]map[string]struct{}
	managedClustersMtx             sync.RWMutex
	managedLabelAllowListConfigmap *ManagedClusterLabelList
	allowlistMtx                   sync.RWMutex
	hasSynced                      bool
	hasSyncedMtx                   sync.RWMutex
	syncAllowList                  chan struct{}
	supervisorRestartDelay         time.Duration
	debounceTimer                  *time.Timer
	debounceDuration               time.Duration
}

// NewManagedClusterInformer creates a new ManagedClusterInformer.
func NewManagedClusterInformer(ctx context.Context, clusterClient clusterclientset.Interface,
	kubeClient kubernetes.Interface) *ManagedClusterInformer {
	return &ManagedClusterInformer{
		ctx:                            ctx,
		clusterClient:                  clusterClient,
		kubeClient:                     kubeClient,
		managedClusters:                make(map[string]map[string]struct{}),
		managedLabelAllowListConfigmap: &ManagedClusterLabelList{},
		syncAllowList:                  make(chan struct{}, 1),
		supervisorRestartDelay:         10 * time.Second,
		debounceDuration:               2 * time.Second,
	}
}

// Run starts the informer and waits for the caches to sync.
func (i *ManagedClusterInformer) Run() {
	if err := i.ensureManagedClusterLabelAllowListConfigmapExists(); err != nil {
		klog.Fatalf("Failed to ensure managed cluster label allowlist configmap exists: %v", err)
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
		proxyconfig.ManagedClusterLabelAllowListNamespace, fields.Everything())
	cmOptions := cache.InformerOptions{
		ListerWatcher: cmWatchlist,
		ObjectType:    &v1.ConfigMap{},
		Handler:       i.getManagedClusterLabelAllowListEventHandler(),
	}
	_, cmController := cache.NewInformerWithOptions(cmOptions)

	go clusterController.Run(i.ctx.Done())
	go cmController.Run(i.ctx.Done())

	if !cache.WaitForCacheSync(i.ctx.Done(), clusterController.HasSynced, cmController.HasSynced) {
		klog.Error("Failed to sync informer caches")
		return
	}

	i.hasSyncedMtx.Lock()
	i.hasSynced = true
	i.hasSyncedMtx.Unlock()
	klog.Info("Informer caches have successfully synced")

	i.managedClustersMtx.RLock()
	klog.Infof("allManagedClusterNames: %v", slices.Sorted(maps.Keys(i.managedClusters)))
	i.managedClustersMtx.RUnlock()

	i.allowlistMtx.RLock()
	klog.Infof("managedLabelList.RegexLabelList: %v", i.managedLabelAllowListConfigmap.RegexLabelList)
	i.allowlistMtx.RUnlock()

	go i.runConfigmapSync()
}

// GetAllManagedClusterNames returns all managed cluster names.
func (i *ManagedClusterInformer) GetAllManagedClusterNames() map[string]struct{} {
	i.managedClustersMtx.RLock()
	defer i.managedClustersMtx.RUnlock()
	return extractMapKeysSet(i.managedClusters)
}

// GetManagedClusterLabelList returns the managed cluster label list.
func (i *ManagedClusterInformer) GetManagedClusterLabelList() []string {
	i.allowlistMtx.RLock()
	defer i.allowlistMtx.RUnlock()
	return slices.Clone(i.managedLabelAllowListConfigmap.RegexLabelList)
}

// HasSynced returns true if the informer has successfully synced.
func (i *ManagedClusterInformer) HasSynced() bool {
	i.hasSyncedMtx.RLock()
	defer i.hasSyncedMtx.RUnlock()
	return i.hasSynced
}

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

// getManagedClusterEventHandler is the hendler for the ManagedClusters resources informer.
func (i *ManagedClusterInformer) getManagedClusterEventHandler() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			managedCluster := obj.(*clusterv1.ManagedCluster)
			klog.Infof("added a managedcluster: %s \n", managedCluster.Name)

			beforeLabels := i.getAllLabels()

			i.managedClustersMtx.Lock()
			i.managedClusters[managedCluster.Name] = extractMapKeysSet(managedCluster.Labels)
			i.managedClustersMtx.Unlock()

			afterLabels := i.getAllLabels()
			if !maps.Equal(beforeLabels, afterLabels) {
				i.syncAllowList <- struct{}{}
			}
		},

		DeleteFunc: func(obj any) {
			managedCluster := obj.(*clusterv1.ManagedCluster)
			klog.Infof("deleted a managedcluster: %s \n", managedCluster.Name)

			beforeLabels := i.getAllLabels()

			i.managedClustersMtx.Lock()
			delete(i.managedClusters, managedCluster.Name)
			i.managedClustersMtx.Unlock()

			afterLabels := i.getAllLabels()
			if !maps.Equal(beforeLabels, afterLabels) {
				i.syncAllowList <- struct{}{}
			}
		},

		UpdateFunc: func(oldObj, newObj any) {
			oldCluster := oldObj.(*clusterv1.ManagedCluster)
			newCluster := newObj.(*clusterv1.ManagedCluster)

			if maps.Equal(oldCluster.Labels, newCluster.Labels) {
				return
			}

			klog.Infof("changed a managedcluster labels: %s \n", newCluster.Name)

			beforeLabels := i.getAllLabels()

			i.managedClustersMtx.Lock()
			i.managedClusters[newCluster.Name] = extractMapKeysSet(newCluster.Labels)
			i.managedClustersMtx.Unlock()

			afterLabels := i.getAllLabels()
			if !maps.Equal(beforeLabels, afterLabels) {
				i.syncAllowList <- struct{}{}
			}
		},
	}
}

func extractMapKeysSet[K comparable, V any](inputMap map[K]V) map[K]struct{} {
	ret := make(map[K]struct{}, len(inputMap))
	for key := range inputMap {
		ret[key] = struct{}{}
	}
	return ret
}

// getManagedClusterLabelAllowListEventHandler is the handler for the ConfigMap informer.
func (i *ManagedClusterInformer) getManagedClusterLabelAllowListEventHandler() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			if obj.(*v1.ConfigMap).Name == proxyconfig.ManagedClusterLabelAllowListConfigMapName {
				klog.V(1).Infof("added configmap: %s", proxyconfig.ManagedClusterLabelAllowListConfigMapName)
				i.syncAllowList <- struct{}{}
			}
		},

		DeleteFunc: func(obj any) {
			if obj.(*v1.ConfigMap).Name == proxyconfig.ManagedClusterLabelAllowListConfigMapName {
				klog.Warningf("ConfigMap %s was deleted, recreating it.", proxyconfig.ManagedClusterLabelAllowListConfigMapName)
				if err := i.ensureManagedClusterLabelAllowListConfigmapExists(); err != nil {
					klog.Errorf("Failed to recreate deleted configmap %s: %v", proxyconfig.ManagedClusterLabelAllowListConfigMapName, err)
				}
			}
		},

		UpdateFunc: func(oldObj, newObj any) {
			if newObj.(*v1.ConfigMap).Name != proxyconfig.ManagedClusterLabelAllowListConfigMapName {
				return
			}
			newConfig := newObj.(*v1.ConfigMap)
			oldConfig := oldObj.(*v1.ConfigMap)

			if reflect.DeepEqual(newConfig.Data, oldConfig.Data) {
				return
			}
			klog.V(1).Infof("updated configmap: %s", proxyconfig.ManagedClusterLabelAllowListConfigMapName)

			i.syncAllowList <- struct{}{}

		},
	}
}

func generateAllowList(currentAllowList *ManagedClusterLabelList, managedClusters map[string]map[string]struct{}) *ManagedClusterLabelList {
	// Extract the labels set from the managed clusters
	labelsSet := map[string]struct{}{}
	for _, labels := range managedClusters {
		for label := range labels {
			labelsSet[label] = struct{}{}
		}
	}

	allowedLabels := map[string]struct{}{}
	for _, label := range currentAllowList.LabelList {
		allowedLabels[label] = struct{}{}
	}

	ignoredLabels := map[string]struct{}{}
	for _, label := range currentAllowList.IgnoreList {
		ignoredLabels[label] = struct{}{}

	}

	// Add missing label in the allowedLabels list
	for label := range ignoredLabels {
		delete(labelsSet, label)
	}
	for label := range labelsSet {
		allowedLabels[label] = struct{}{}
	}

	return &ManagedClusterLabelList{
		IgnoreList: slices.Sorted(maps.Keys(ignoredLabels)),
		LabelList:  slices.Sorted(maps.Keys(allowedLabels)),
	}
}

func (i *ManagedClusterInformer) runConfigmapSync() {
	for {
		func() {
			defer func() {
				if r := recover(); r != nil {
					klog.Errorf("Recovered from panic in configmap sync: %v", r)
				}
			}()
			i.syncLoop()
		}()

		// Check if the watcher was closed intentionally.
		select {
		case <-i.ctx.Done():
			klog.Info("Configmap sync supervisor shutting down.")
			return
		default:
			// The watcher stopped unexpectedly. Log, wait, and restart.
			klog.Warningf("Configmap sync loop stopped unexpectedly. Restarting in %v...", i.supervisorRestartDelay)
			time.Sleep(i.supervisorRestartDelay)
		}
	}
}

func (i *ManagedClusterInformer) syncLoop() {
	for {
		select {
		case <-i.syncAllowList:
			i.triggerCheck()
		case <-i.ctx.Done():
			return
		}
	}
}

func (i *ManagedClusterInformer) triggerCheck() {
	if i.debounceTimer != nil {
		i.debounceTimer.Stop()
	}

	i.debounceTimer = time.AfterFunc(i.debounceDuration, i.checkForUpdate)
}

func (i *ManagedClusterInformer) checkForUpdate() {
	i.managedClustersMtx.RLock()
	managedClustersCopy := make(map[string]map[string]struct{}, len(i.managedClusters))
	for k, v := range i.managedClusters {
		managedClustersCopy[k] = v
	}
	i.managedClustersMtx.RUnlock()

	var mergedList *ManagedClusterLabelList
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
		if err := unmarshalDataToManagedClusterLabelList(allowListCM.Data,
			proxyconfig.ManagedClusterLabelAllowListConfigMapKey, currentOnClusterList); err != nil {
			klog.Errorf("Failed to unmarshal configmap, will not retry: %v", err)
			return nil
		}

		mergedList = generateAllowList(currentOnClusterList, managedClustersCopy)

		i.allowlistMtx.RLock()
		inMemoryList := i.managedLabelAllowListConfigmap
		i.allowlistMtx.RUnlock()

		if slices.Equal(mergedList.LabelList, inMemoryList.LabelList) && slices.Equal(mergedList.IgnoreList, inMemoryList.IgnoreList) {
			klog.V(4).Info("Managed cluster label allowlist is already up-to-date")
			mergedList = nil // Ensure mergedList is nil if no update is needed
			return nil
		}

		if err := marshalLabelListToConfigMap(allowListCM,
			proxyconfig.ManagedClusterLabelAllowListConfigMapKey, mergedList); err != nil {
			return err
		}

		_, err = i.kubeClient.CoreV1().ConfigMaps(proxyconfig.ManagedClusterLabelAllowListNamespace).Update(
			i.ctx,
			allowListCM,
			metav1.UpdateOptions{},
		)
		return err
	})

	if err != nil {
		klog.Errorf("Failed to update managedcluster label allowlist configmap after retries: %v", err)
		return
	}

	if mergedList != nil {
		klog.Info("Successfully updated managedcluster label allowlist configmap")
		mergedList.RegexLabelList = make([]string, 0, len(mergedList.LabelList))
		for _, label := range mergedList.LabelList {
			mergedList.RegexLabelList = append(mergedList.RegexLabelList, promLabelRegex.ReplaceAllString(label, "_"))
		}
		i.allowlistMtx.Lock()
		i.managedLabelAllowListConfigmap = mergedList
		i.allowlistMtx.Unlock()
	}
}

func marshalLabelListToConfigMap(obj *v1.ConfigMap, key string,
	labelList *ManagedClusterLabelList) error {
	data, err := yaml.Marshal(labelList)
	if err != nil {
		return fmt.Errorf("failed to marshal managedLabelList data: %w", err)
	}

	if obj.Data == nil {
		obj.Data = map[string]string{}
	}
	obj.Data[key] = string(data)

	return nil
}

func unmarshalDataToManagedClusterLabelList(data map[string]string, key string,
	labelList *ManagedClusterLabelList) error {
	if err := yaml.Unmarshal([]byte(data[key]), labelList); err != nil {
		return fmt.Errorf("failed to unmarshal configmap %s data to the managedLabelList: %w", key, err)
	}

	return nil
}

// ensureManagedClusterLabelAllowListConfigmapExists checks if the allowlist ConfigMap exists and creates it if it doesn't.
func (i *ManagedClusterInformer) ensureManagedClusterLabelAllowListConfigmapExists() error {
	_, err := i.kubeClient.CoreV1().ConfigMaps(proxyconfig.ManagedClusterLabelAllowListNamespace).Get(
		i.ctx,
		proxyconfig.ManagedClusterLabelAllowListConfigMapName,
		metav1.GetOptions{},
	)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			klog.Info("managedcluster label allowlist configmap not found, creating it")
			cm := proxyconfig.CreateManagedClusterLabelAllowListCM(
				proxyconfig.ManagedClusterLabelAllowListNamespace,
			)
			_, err := i.kubeClient.CoreV1().ConfigMaps(proxyconfig.ManagedClusterLabelAllowListNamespace).Create(i.ctx, cm, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create managedcluster label allowlist configmap: %w", err)
			}
		} else {
			return fmt.Errorf("failed to get managedcluster label allowlist configmap: %w", err)
		}
	}
	return nil
}
