// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package informer

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/go-co-op/gocron"
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

const (
	resyncTag = "managed-cluster-label-allowlist-resync"
)

// ManagedClusterInformable defines the interface for accessing cached managed cluster data.
type ManagedClusterInformable interface {
	Run()
	GetAllManagedClusterNames() map[string]string
	GetAllManagedClusterLabelNames() map[string]bool
	GetManagedClusterLabelList() *proxyconfig.ManagedClusterLabelList
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
	managedLabelList               *proxyconfig.ManagedClusterLabelList
	syncLabelList                  *proxyconfig.ManagedClusterLabelList
	scheduler                      *gocron.Scheduler
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
		managedLabelList:            proxyconfig.GetManagedClusterLabelList(),
		syncLabelList:               proxyconfig.GetSyncLabelList(),
		scheduler:                   gocron.NewScheduler(time.UTC),
	}
}

// Run starts the informer.
func (i *ManagedClusterInformer) Run() {
	go i.watchManagedCluster()
	go i.watchManagedClusterLabelAllowList()
	go i.scheduleManagedClusterLabelAllowlistResync()
}

// GetAllManagedClusterNames returns all managed cluster names.
func (i *ManagedClusterInformer) GetAllManagedClusterNames() map[string]string {
	i.allManagedClusterNamesMtx.RLock()
	defer i.allManagedClusterNamesMtx.RUnlock()
	return i.allManagedClusterNames
}

// GetAllManagedClusterLabelNames returns all managed cluster labels.
func (i *ManagedClusterInformer) GetAllManagedClusterLabelNames() map[string]bool {
	i.allManagedClusterLabelNamesMtx.RLock()
	defer i.allManagedClusterLabelNamesMtx.RUnlock()
	return i.allManagedClusterLabelNames
}

// GetManagedClusterLabelList returns the managed cluster label list.
func (i *ManagedClusterInformer) GetManagedClusterLabelList() *proxyconfig.ManagedClusterLabelList {
	return i.managedLabelList
}

func (i *ManagedClusterInformer) watchManagedCluster() {
	watchlist := cache.NewListWatchFromClient(
		i.clusterClient.ClusterV1().RESTClient(),
		"managedclusters",
		v1.NamespaceAll,
		fields.Everything(),
	)

	options := cache.InformerOptions{
		ListerWatcher: watchlist,
		ObjectType:    &clusterv1.ManagedCluster{},
		Handler:       i.getManagedClusterEventHandler(),
	}
	_, controller := cache.NewInformerWithOptions(options)

	go controller.Run(i.ctx.Done())

	<-i.ctx.Done()
	klog.Info("context cancelled, stopping managed cluster watcher")
}

// getManagedClusterEventHandler is the hendler for the ManagedClusters resources informer.
func (i *ManagedClusterInformer) getManagedClusterEventHandler() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			clusterName := obj.(*clusterv1.ManagedCluster).Name
			klog.Infof("added a managedcluster: %s \n", obj.(*clusterv1.ManagedCluster).Name)

			i.allManagedClusterNamesMtx.Lock()
			i.allManagedClusterNames[clusterName] = clusterName
			i.allManagedClusterNamesMtx.Unlock()

			clusterLabels := obj.(*clusterv1.ManagedCluster).Labels
			if ok := i.updateManagedLabelList(clusterLabels); ok {
				i.addManagedClusterLabelNames()
			}
		},

		DeleteFunc: func(obj interface{}) {
			clusterName := obj.(*clusterv1.ManagedCluster).Name
			klog.Infof("deleted a managedcluster: %s \n", obj.(*clusterv1.ManagedCluster).Name)

			i.allManagedClusterNamesMtx.Lock()
			delete(i.allManagedClusterNames, clusterName)
			i.allManagedClusterNamesMtx.Unlock()
		},

		UpdateFunc: func(oldObj, newObj interface{}) {
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

			clusterLabels := newCluster.Labels
			if ok := i.updateManagedLabelList(clusterLabels); ok {
				i.addManagedClusterLabelNames()
			}
		},
	}
}

// updateManagedLabelList updated the managedLabelList by adding missing label keys from the clusterLabels parameter.
// Returns true is the list has been updated
func (i *ManagedClusterInformer) updateManagedLabelList(clusterLabels map[string]string) bool {
	updated := false

	for key := range clusterLabels {
		if !slice.ContainsString(i.managedLabelList.LabelList, key, nil) {
			i.managedLabelList.LabelList = append(i.managedLabelList.LabelList, key)
			updated = true
		}
	}

	return updated
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

	i.managedLabelList.RegexLabelList = []string{}
	regex := regexp.MustCompile(`[^\w]+`)

	i.allManagedClusterLabelNamesMtx.RLock()
	defer i.allManagedClusterLabelNamesMtx.RUnlock()
	for key, isEnabled := range i.allManagedClusterLabelNames {
		if isEnabled {
			i.managedLabelList.RegexLabelList = append(
				i.managedLabelList.RegexLabelList,
				regex.ReplaceAllString(key, "_"),
			)
		}
	}
	i.syncLabelList.RegexLabelList = i.managedLabelList.RegexLabelList
}

func (i *ManagedClusterInformer) watchManagedClusterLabelAllowList() {
	watchlist := cache.NewListWatchFromClient(i.kubeClient.CoreV1().RESTClient(), "configmaps",
		proxyconfig.ManagedClusterLabelAllowListNamespace, fields.Everything())

	options := cache.InformerOptions{
		ListerWatcher: watchlist,
		ObjectType:    &v1.ConfigMap{},
		Handler:       i.getManagedClusterLabelAllowListEventHandler(),
	}
	_, controller := cache.NewInformerWithOptions(options)

	go controller.Run(i.ctx.Done())

	<-i.ctx.Done()
	klog.Info("context cancelled, stopping managed cluster label allowlist watcher")
}

func (i *ManagedClusterInformer) getManagedClusterLabelAllowListEventHandler() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if obj.(*v1.ConfigMap).Name == proxyconfig.GetManagedClusterLabelAllowListConfigMapName() {
				klog.Infof("added configmap: %s", proxyconfig.GetManagedClusterLabelAllowListConfigMapName())

				if ok := i.scheduler != nil; ok {
					if ok := i.scheduler.IsRunning(); !ok {
						go i.scheduleManagedClusterLabelAllowlistResync()
					}
				}
			}
		},

		DeleteFunc: func(obj interface{}) {
			if obj.(*v1.ConfigMap).Name == proxyconfig.GetManagedClusterLabelAllowListConfigMapName() {
				klog.Warningf("deleted configmap: %s", proxyconfig.GetManagedClusterLabelAllowListConfigMapName())
				i.stopScheduleManagedClusterLabelAllowlistResync()
			}
		},

		UpdateFunc: func(oldObj, newObj interface{}) {
			if newObj.(*v1.ConfigMap).Name == proxyconfig.GetManagedClusterLabelAllowListConfigMapName() {
				klog.Infof("updated configmap: %s", proxyconfig.GetManagedClusterLabelAllowListConfigMapName())

				_ = unmarshalDataToManagedClusterLabelList(newObj.(*v1.ConfigMap).Data,
					proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(), i.syncLabelList)

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

// ScheduleManagedClusterLabelAllowlistResync schedules the managed cluster label allowlist resync.
func (i *ManagedClusterInformer) scheduleManagedClusterLabelAllowlistResync() {
	if i.scheduler == nil {
		i.scheduler = gocron.NewScheduler(time.UTC)
	}

	_, err := i.scheduler.Tag(resyncTag).Every(30).Second().Do(i.resyncManagedClusterLabelAllowList)
	if err != nil {
		klog.Errorf("failed to schedule job for managedcluster allowlist resync: %v", err)
	}

	klog.Info("starting scheduler for managedcluster allowlist resync")
	i.scheduler.StartAsync()

	go func() {
		<-i.ctx.Done()
		klog.Info("context cancelled, stopping scheduler for managedcluster allowlist resync")
		i.scheduler.Stop()
	}()
}

// StopScheduleManagedClusterLabelAllowlistResync stops the managed cluster label allowlist resync.
func (i *ManagedClusterInformer) stopScheduleManagedClusterLabelAllowlistResync() {
	klog.Info("stopping scheduler for managedcluster allowlist resync")
	i.scheduler.Stop()

	if ok := i.scheduler.IsRunning(); !ok {
		i.scheduler = gocron.NewScheduler(time.UTC)
	}
}

func (i *ManagedClusterInformer) resyncManagedClusterLabelAllowList() error {
	found, err := proxyconfig.GetManagedClusterLabelAllowListConfigmap(i.ctx, i.kubeClient,
		proxyconfig.ManagedClusterLabelAllowListNamespace)

	if err != nil {
		return err
	}

	err = unmarshalDataToManagedClusterLabelList(found.Data,
		proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(), i.syncLabelList)

	if err != nil {
		return err
	}

	sortManagedLabelList(i.managedLabelList)
	sortManagedLabelList(i.syncLabelList)

	syncIgnoreList := []string{}

	for _, label := range i.syncLabelList.IgnoreList {
		if slice.ContainsString(proxyconfig.GetRequiredLabelList(), label, nil) {
			klog.Infof("detected required managedcluster label in ignorelist. resetting label: %s", label)
			continue
		}

		syncIgnoreList = append(syncIgnoreList, label)
	}

	sort.Strings(syncIgnoreList)
	i.syncLabelList.IgnoreList = syncIgnoreList

	if ok := reflect.DeepEqual(i.syncLabelList, i.managedLabelList); !ok {
		klog.Infof("resyncing required for managedcluster label allowlist: %v",
			proxyconfig.GetManagedClusterLabelAllowListConfigMapName())

		i.managedLabelList.IgnoreList = i.syncLabelList.IgnoreList
		i.ignoreManagedClusterLabelNames()

		*i.syncLabelList = *i.managedLabelList
		_ = marshalLabelListToConfigMap(found,
			proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(), i.syncLabelList)

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

	i.managedLabelList.RegexLabelList = []string{}
	regex := regexp.MustCompile(`[^\w]+`)
	i.allManagedClusterLabelNamesMtx.RLock()
	defer i.allManagedClusterLabelNamesMtx.RUnlock()
	for key, isEnabled := range i.allManagedClusterLabelNames {
		if isEnabled {
			i.managedLabelList.RegexLabelList = append(
				i.managedLabelList.RegexLabelList,
				regex.ReplaceAllString(key, "_"),
			)
		}
	}
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

func sortManagedLabelList(managedLabelList *proxyconfig.ManagedClusterLabelList) {
	if managedLabelList != nil {
		sort.Strings(managedLabelList.IgnoreList)
		sort.Strings(managedLabelList.LabelList)
		sort.Strings(managedLabelList.RegexLabelList)
	} else {
		klog.Infof("managedLabelList is empty: %v", managedLabelList)
	}
}

func marshalLabelListToConfigMap(obj *v1.ConfigMap, key string,
	managedLabelList *proxyconfig.ManagedClusterLabelList) error {
	data, err := yaml.Marshal(managedLabelList)
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
	managedLabelList *proxyconfig.ManagedClusterLabelList) error {
	if err := yaml.Unmarshal([]byte(data[key]), managedLabelList); err != nil {
		return fmt.Errorf("failed to unmarshal configmap %s data to the managedLabelList: %w", key, err)
	}

	return nil
}
