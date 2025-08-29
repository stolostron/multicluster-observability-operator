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
	"sort"
	"sync"
	"time"

	proxyconfig "github.com/stolostron/multicluster-observability-operator/proxy/pkg/config"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	"k8s.io/kubectl/pkg/util/slice"
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
	GetAllManagedClusterNames() map[string]string
	GetAllManagedClusterLabelNames() map[string]bool
	GetManagedClusterLabelList() []string
}

// ManagedClusterInformer caches managed cluster data and manages the label allowlist.
// It uses informers to watch for changes to ManagedCluster resources and the allowlist ConfigMap,
// keeping the cache up-to-date.
type ManagedClusterInformer struct {
	ctx                            context.Context
	clusterClient                  clusterclientset.Interface
	kubeClient                     kubernetes.Interface
	allManagedClusterNames         map[string]string
	allManagedClusterNamesMtx      sync.RWMutex
	allManagedClusterLabelNames    map[string]bool
	allManagedClusterLabelNamesMtx sync.RWMutex
	// managedLabelList is the in-memory representation of the historical label list.
	// It accumulates all labels ever seen on any managed cluster and is the source of truth for the informer.
	// It is never cleared, ensuring that even if a label is removed from all clusters,
	// it remains available for historical queries in Grafana.
	managedLabelList *ManagedClusterLabelList
	// syncLabelList is a temporary snapshot of the label list as read from the allowlist ConfigMap.
	// It is used for comparison against the in-memory managedLabelList to detect changes
	// (either new labels discovered by the informer or manual user edits to the ConfigMap)
	// that need to be persisted.
	syncLabelList     *ManagedClusterLabelList
	labelListMtx      sync.RWMutex
	hasSynced         bool
	hasSyncedMtx      sync.RWMutex
	resyncStopCh      chan struct{}
	resyncMtx         sync.Mutex
}

// NewManagedClusterInformer creates a new ManagedClusterInformer.
func NewManagedClusterInformer(ctx context.Context, clusterClient clusterclientset.Interface,
	kubeClient kubernetes.Interface) *ManagedClusterInformer {
	return &ManagedClusterInformer{
		ctx:                         ctx,
		clusterClient:               clusterClient,
		kubeClient:                  kubeClient,
		allManagedClusterNames:      make(map[string]string),
		allManagedClusterLabelNames: make(map[string]bool),
		managedLabelList:            &ManagedClusterLabelList{},
		syncLabelList:               &ManagedClusterLabelList{},
	}
}

// Run starts the informer and waits for the caches to sync.
func (i *ManagedClusterInformer) Run() {
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

	go i.scheduleManagedClusterLabelAllowlistResync()
}

// GetAllManagedClusterNames returns all managed cluster names.
func (i *ManagedClusterInformer) GetAllManagedClusterNames() map[string]string {
	i.allManagedClusterNamesMtx.RLock()
	defer i.allManagedClusterNamesMtx.RUnlock()
	return maps.Clone(i.allManagedClusterNames)
}

// GetAllManagedClusterLabelNames returns all managed cluster labels.
func (i *ManagedClusterInformer) GetAllManagedClusterLabelNames() map[string]bool {
	i.allManagedClusterLabelNamesMtx.RLock()
	defer i.allManagedClusterLabelNamesMtx.RUnlock()
	return maps.Clone(i.allManagedClusterLabelNames)
}

// GetManagedClusterLabelList returns the managed cluster label list.
func (i *ManagedClusterInformer) GetManagedClusterLabelList() []string {
	i.labelListMtx.RLock()
	defer i.labelListMtx.RUnlock()
	return slices.Clone(i.managedLabelList.RegexLabelList)
}

// HasSynced returns true if the informer has successfully synced.
func (i *ManagedClusterInformer) HasSynced() bool {
	i.hasSyncedMtx.RLock()
	defer i.hasSyncedMtx.RUnlock()
	return i.hasSynced
}

// getManagedClusterEventHandler is the hendler for the ManagedClusters resources informer.
func (i *ManagedClusterInformer) getManagedClusterEventHandler() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			clusterName := obj.(*clusterv1.ManagedCluster).Name
			klog.Infof("added a managedcluster: %s \n", obj.(*clusterv1.ManagedCluster).Name)

			i.allManagedClusterNamesMtx.Lock()
			i.allManagedClusterNames[clusterName] = clusterName
			i.allManagedClusterNamesMtx.Unlock()

			clusterLabels := obj.(*clusterv1.ManagedCluster).Labels
			i.updateManagedLabelList(clusterLabels)
		},


		DeleteFunc: func(obj any) {
			clusterName := obj.(*clusterv1.ManagedCluster).Name
			klog.Infof("deleted a managedcluster: %s \n", obj.(*clusterv1.ManagedCluster).Name)

			i.allManagedClusterNamesMtx.Lock()
			delete(i.allManagedClusterNames, clusterName)
			i.allManagedClusterNamesMtx.Unlock()
		},

		UpdateFunc: func(oldObj, newObj any) {
			oldCluster := oldObj.(*clusterv1.ManagedCluster)
			newCluster := newObj.(*clusterv1.ManagedCluster)

			if reflect.DeepEqual(oldCluster.Labels, newCluster.Labels) {
				return
			}

			clusterName := newCluster.Name
			klog.Infof("changed a managedcluster: %s \n", newCluster.Name)

			i.allManagedClusterNamesMtx.Lock()
			i.allManagedClusterNames[clusterName] = clusterName
			i.allManagedClusterNamesMtx.Unlock()

			i.updateManagedLabelList(newCluster.Labels)
		},
	}
}

// updateManagedLabelList updated the managedLabelList by adding missing label keys from the clusterLabels parameter.
// This function only adds new labels and never removes them. This ensures that once a label is observed,
// it is permanently retained in the list, allowing users to query historical data for that label
// even after it has been removed from all ManagedClusters.
func (i *ManagedClusterInformer) updateManagedLabelList(clusterLabels map[string]string) {
	i.labelListMtx.Lock()
	defer i.labelListMtx.Unlock()

	updated := false
	for key := range clusterLabels {
		if !slice.ContainsString(i.managedLabelList.LabelList, key, nil) {
			i.managedLabelList.LabelList = append(i.managedLabelList.LabelList, key)
			updated = true
		}
	}

	if updated {
		i.addManagedClusterLabelNames()
	}
}

func (i *ManagedClusterInformer) addManagedClusterLabelNames() {
	for _, key := range i.managedLabelList.LabelList {
		if slice.ContainsString(i.managedLabelList.IgnoreList, key, nil) {
			// Ignored labels are handled in the updateAllManagedClusterLabelNames->ignoreManagedClusterLabelNames call
			continue
		}

		isEnabled, ok := i.allManagedClusterLabelNames[key]
		if !ok || !isEnabled {
			klog.Infof("adding managedcluster label: %s", key)
			i.allManagedClusterLabelNamesMtx.Lock()
			i.allManagedClusterLabelNames[key] = true
			i.allManagedClusterLabelNamesMtx.Unlock()
		}
	}

	i.updateRegexLabelList()
}

// getManagedClusterLabelAllowListEventHandler is the handler for the ConfigMap informer.
func (i *ManagedClusterInformer) getManagedClusterLabelAllowListEventHandler() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			if obj.(*v1.ConfigMap).Name == proxyconfig.ManagedClusterLabelAllowListConfigMapName {
				klog.Infof("added configmap: %s", proxyconfig.ManagedClusterLabelAllowListConfigMapName)
				i.labelListMtx.Lock()
				defer i.labelListMtx.Unlock()

				_ = unmarshalDataToManagedClusterLabelList(obj.(*v1.ConfigMap).Data,
					proxyconfig.ManagedClusterLabelAllowListConfigMapKey, i.managedLabelList)
				*i.syncLabelList = *i.managedLabelList
				i.updateAllManagedClusterLabelNames()
				go i.scheduleManagedClusterLabelAllowlistResync()
			}
		},

		DeleteFunc: func(obj any) {
			if obj.(*v1.ConfigMap).Name == proxyconfig.ManagedClusterLabelAllowListConfigMapName {
				klog.Warningf("deleted configmap: %s", proxyconfig.ManagedClusterLabelAllowListConfigMapName)
				i.stopScheduleManagedClusterLabelAllowlistResync()
			}
		},

		UpdateFunc: func(oldObj, newObj any) {
			if newObj.(*v1.ConfigMap).Name == proxyconfig.ManagedClusterLabelAllowListConfigMapName {
				klog.Infof("updated configmap: %s", proxyconfig.ManagedClusterLabelAllowListConfigMapName)
				i.labelListMtx.Lock()
				defer i.labelListMtx.Unlock()

				_ = unmarshalDataToManagedClusterLabelList(newObj.(*v1.ConfigMap).Data,
					proxyconfig.ManagedClusterLabelAllowListConfigMapKey, i.syncLabelList)

				sortManagedLabelList(i.managedLabelList)
				sortManagedLabelList(i.syncLabelList)

				if ok := reflect.DeepEqual(i.syncLabelList, i.managedLabelList); !ok {
					i.managedLabelList.IgnoreList = i.syncLabelList.IgnoreList
					*i.syncLabelList = *i.managedLabelList
				}

				i.updateAllManagedClusterLabelNames()
			}
		},
	}
}

// scheduleManagedClusterLabelAllowlistResync schedules a periodic task to reconcile the in-memory label list
// with the persisted version in the `observability-managed-cluster-label-allowlist` ConfigMap.
// This is critical for two reasons:
//  1. Persistence: It saves newly discovered labels from the in-memory `managedLabelList` into the
//     ConfigMap. This ensures that if the pod restarts, the historical list of labels is not lost.
//  2. User Configuration: It detects and applies changes made directly to the ConfigMap by users,
//     such as adding a label to the `ignore_labels` list.
func (i *ManagedClusterInformer) scheduleManagedClusterLabelAllowlistResync() {
	i.resyncMtx.Lock()
	if i.resyncStopCh != nil {
		i.resyncMtx.Unlock()
		return
	}
	i.resyncStopCh = make(chan struct{})
	stopCh := i.resyncStopCh
	i.resyncMtx.Unlock()

	go func() {
		klog.Info("starting scheduler for managedcluster allowlist resync")
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := i.resyncManagedClusterLabelAllowList(); err != nil {
					klog.Errorf("failed to resync managedcluster allowlist: %v", err)
				}
			case <-stopCh:
				return
			case <-i.ctx.Done():
				klog.Info("context cancelled, stopping scheduler for managedcluster allowlist resync")
				return
			}
		}
	}()
}

// StopScheduleManagedClusterLabelAllowlistResync stops the managed cluster label allowlist resync.
func (i *ManagedClusterInformer) stopScheduleManagedClusterLabelAllowlistResync() {
	i.resyncMtx.Lock()
	defer i.resyncMtx.Unlock()

	if i.resyncStopCh != nil {
		klog.Info("stopping scheduler for managedcluster allowlist resync")
		close(i.resyncStopCh)
		i.resyncStopCh = nil
	}
}

func (i *ManagedClusterInformer) resyncManagedClusterLabelAllowList() error {
	i.labelListMtx.Lock()
	defer i.labelListMtx.Unlock()

	found, err := proxyconfig.GetManagedClusterLabelAllowListConfigmap(i.ctx, i.kubeClient,
		proxyconfig.ManagedClusterLabelAllowListNamespace)

	if err != nil {
		return err
	}

	err = unmarshalDataToManagedClusterLabelList(found.Data,
		proxyconfig.ManagedClusterLabelAllowListConfigMapKey, i.syncLabelList)

	if err != nil {
		return err
	}

	sortManagedLabelList(i.managedLabelList)
	sortManagedLabelList(i.syncLabelList)

	syncIgnoreList := []string{}

	for _, label := range i.syncLabelList.IgnoreList {
		if slice.ContainsString(proxyconfig.RequiredLabelList, label, nil) {
			klog.Infof("detected required managedcluster label in ignorelist. resetting label: %s", label)
			continue
		}

		syncIgnoreList = append(syncIgnoreList, label)
	}

	sort.Strings(syncIgnoreList)
	i.syncLabelList.IgnoreList = syncIgnoreList

	if ok := reflect.DeepEqual(i.syncLabelList, i.managedLabelList); !ok {
		klog.Infof("resyncing required for managedcluster label allowlist: %v",
			proxyconfig.ManagedClusterLabelAllowListConfigMapName)

		i.managedLabelList.IgnoreList = i.syncLabelList.IgnoreList
		i.ignoreManagedClusterLabelNames()

		*i.syncLabelList = *i.managedLabelList
		if err := marshalLabelListToConfigMap(found,
			proxyconfig.ManagedClusterLabelAllowListConfigMapKey, i.syncLabelList); err != nil {
			return err
		}

		_, err := i.kubeClient.CoreV1().ConfigMaps(proxyconfig.ManagedClusterLabelAllowListNamespace).Update(
			i.ctx,
			found,
			metav1.UpdateOptions{},
		)
		if err != nil {
			return fmt.Errorf("failed to update managedcluster label allowlist configmap: %w", err)
		}
	}

	return nil
}

func (i *ManagedClusterInformer) ignoreManagedClusterLabelNames() {
	for _, key := range i.managedLabelList.IgnoreList {
		if _, ok := i.allManagedClusterLabelNames[key]; !ok {
			klog.Infof("ignoring managedcluster label: %s", key)

		} else if isEnabled := i.allManagedClusterLabelNames[key]; isEnabled {
			klog.Infof("disabled managedcluster label: %s", key)
		}

		i.allManagedClusterLabelNames[key] = false
	}

	i.updateRegexLabelList()
}

// updateRegexLabelList sanitizes the managed cluster label keys to be compatible with PromQL and updates the regex label list.
func (i *ManagedClusterInformer) updateRegexLabelList() {
	// The RegexLabelList holds a sanitized version of the Kubernetes label keys, making them
	// compatible with the Prometheus query language (PromQL). Kubernetes labels can contain characters
	// like '.', '-', and '/' which are invalid in PromQL label names. This code replaces all
	// non-alphanumeric characters with underscores to create a valid PromQL-compatible label name.
	// For example, `cluster.open-cluster-management.io/clusterset` becomes
	// `cluster_open_cluster_management_io_clusterset`.
	i.managedLabelList.RegexLabelList = []string{}
	i.allManagedClusterLabelNamesMtx.RLock()
	for key, isEnabled := range i.allManagedClusterLabelNames {
		if isEnabled {
			i.managedLabelList.RegexLabelList = append(
				i.managedLabelList.RegexLabelList,
				promLabelRegex.ReplaceAllString(key, "_"),
			)
		}
	}
	i.allManagedClusterLabelNamesMtx.RUnlock()
	i.syncLabelList.RegexLabelList = i.managedLabelList.RegexLabelList
}

func (i *ManagedClusterInformer) updateAllManagedClusterLabelNames() {
	if i.managedLabelList.LabelList != nil {
		i.addManagedClusterLabelNames()
	} else {
		klog.Infof("managed label list is empty")
	}

	if i.managedLabelList.IgnoreList != nil {
		i.ignoreManagedClusterLabelNames()
	} else {
		klog.Infof("managed ignore list is empty")
	}
}

func sortManagedLabelList(labelList *ManagedClusterLabelList) {
	if labelList != nil {
		sort.Strings(labelList.IgnoreList)
		sort.Strings(labelList.LabelList)
		sort.Strings(labelList.RegexLabelList)
	} else {
		klog.Infof("managedLabelList is empty: %v", labelList)
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
