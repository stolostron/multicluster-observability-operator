// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package informer

import (
	"context"
	"reflect"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/ghodss/yaml"
	"github.com/go-co-op/gocron"
	proxyconfig "github.com/stolostron/multicluster-observability-operator/proxy/pkg/config"
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

var (
	AllManagedClusterNames    map[string]string
	AllManagedClusterNamesMtx sync.RWMutex

	AllManagedClusterLabelNames    map[string]bool
	AllManagedClusterLabelNamesMtx sync.RWMutex

	managedLabelList = proxyconfig.GetManagedClusterLabelList()
	syncLabelList    = proxyconfig.GetSyncLabelList()
	resyncTag        = "managed-cluster-label-allowlist-resync"
	scheduler        *gocron.Scheduler
)

// InitAllManagedClusterNames initializes all managed cluster names map.
func InitAllManagedClusterNames() {
	AllManagedClusterNames = map[string]string{}
}

// InitAllManagedClusterLabelNames initializes all managed cluster labels map.
func InitAllManagedClusterLabelNames() {
	AllManagedClusterLabelNames = map[string]bool{}
}

func InitScheduler() {
	scheduler = gocron.NewScheduler(time.UTC)
}

// GetAllManagedClusterNames returns all managed cluster names.
func GetAllManagedClusterNames() map[string]string {
	return AllManagedClusterNames
}

// GetAllManagedClusterLabelNames returns all managed cluster labels.
func GetAllManagedClusterLabelNames() map[string]bool {
	return AllManagedClusterLabelNames
}

// WatchManagedCluster will watch and save managedcluster when create/update/delete managedcluster.
func WatchManagedCluster(clusterClient clusterclientset.Interface, kubeClient kubernetes.Interface) {
	InitAllManagedClusterNames()
	InitAllManagedClusterLabelNames()
	watchlist := cache.NewListWatchFromClient(
		clusterClient.ClusterV1().RESTClient(),
		"managedclusters",
		v1.NamespaceAll,
		fields.Everything(),
	)

	options := cache.InformerOptions{
		ListerWatcher: watchlist,
		ObjectType:    &clusterv1.ManagedCluster{},
		Handler:       GetManagedClusterEventHandler(),
	}
	_, controller := cache.NewInformerWithOptions(options)

	stop := make(chan struct{})
	go controller.Run(stop)
	for {
		time.Sleep(time.Second * 30)
		klog.V(1).Infof("found %v clusters", len(AllManagedClusterNames))
	}
}

// GetManagedClusterEventHandler return event handler functions for managed cluster watch events.
func GetManagedClusterEventHandler() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			clusterName := obj.(*clusterv1.ManagedCluster).Name
			klog.Infof("added a managedcluster: %s \n", obj.(*clusterv1.ManagedCluster).Name)

			AllManagedClusterNamesMtx.Lock()
			AllManagedClusterNames[clusterName] = clusterName
			AllManagedClusterNamesMtx.Unlock()

			clusterLabels := obj.(*clusterv1.ManagedCluster).Labels
			if ok := shouldUpdateManagedClusterLabelNames(clusterLabels, managedLabelList); ok {
				addManagedClusterLabelNames(managedLabelList)
			}
		},

		DeleteFunc: func(obj interface{}) {
			clusterName := obj.(*clusterv1.ManagedCluster).Name
			klog.Infof("deleted a managedcluster: %s \n", obj.(*clusterv1.ManagedCluster).Name)

			AllManagedClusterNamesMtx.Lock()
			delete(AllManagedClusterNames, clusterName)
			AllManagedClusterNamesMtx.Unlock()
		},

		UpdateFunc: func(oldObj, newObj interface{}) {
			clusterName := newObj.(*clusterv1.ManagedCluster).Name
			klog.Infof("changed a managedcluster: %s \n", newObj.(*clusterv1.ManagedCluster).Name)

			AllManagedClusterNamesMtx.Lock()
			AllManagedClusterNames[clusterName] = clusterName
			AllManagedClusterNamesMtx.Unlock()

			clusterLabels := newObj.(*clusterv1.ManagedCluster).Labels
			if ok := shouldUpdateManagedClusterLabelNames(clusterLabels, managedLabelList); ok {
				addManagedClusterLabelNames(managedLabelList)
			}
		},
	}
}

// shouldUpdateManagedClusterLabelNames determine whether the managedcluster label names map should be updated.
func shouldUpdateManagedClusterLabelNames(clusterLabels map[string]string,
	managedLabelList *proxyconfig.ManagedClusterLabelList) bool {
	updateRequired := false

	for key := range clusterLabels {
		if !slice.ContainsString(managedLabelList.LabelList, key, nil) {
			managedLabelList.LabelList = append(managedLabelList.LabelList, key)
			updateRequired = true
		}
	}

	klog.Infof("managedcluster label names update required: %v", updateRequired)
	return updateRequired
}

// addManagedClusterLabelNames set key to enable within the managedcluster label names map.
func addManagedClusterLabelNames(managedLabelList *proxyconfig.ManagedClusterLabelList) {
	for _, key := range managedLabelList.LabelList {
		if _, ok := AllManagedClusterLabelNames[key]; !ok {
			klog.Infof("added managedcluster label: %s", key)

			AllManagedClusterLabelNamesMtx.Lock()
			AllManagedClusterLabelNames[key] = true
			AllManagedClusterLabelNamesMtx.Unlock()

		} else if slice.ContainsString(managedLabelList.IgnoreList, key, nil) {
			klog.V(2).Infof("managedcluster label <%s> set to ignore, remove label from ignore list to enable.", key)

		} else if isEnabled := AllManagedClusterLabelNames[key]; !isEnabled {
			klog.Infof("enabled managedcluster label: %s", key)

			AllManagedClusterLabelNamesMtx.Lock()
			AllManagedClusterLabelNames[key] = true
			AllManagedClusterLabelNamesMtx.Unlock()

		}
	}

	managedLabelList.RegexLabelList = []string{}
	regex := regexp.MustCompile(`[^\w]+`)

	AllManagedClusterLabelNamesMtx.RLock()
	defer AllManagedClusterLabelNamesMtx.RUnlock()
	for key, isEnabled := range AllManagedClusterLabelNames {
		if isEnabled {
			managedLabelList.RegexLabelList = append(
				managedLabelList.RegexLabelList,
				regex.ReplaceAllString(key, "_"),
			)
		}
	}
	syncLabelList.RegexLabelList = managedLabelList.RegexLabelList
}

// WatchManagedClusterLabelAllowList will watch and save managedcluster label allowlist configmap
// when create/update/delete.
func WatchManagedClusterLabelAllowList(kubeClient kubernetes.Interface) {
	watchlist := cache.NewListWatchFromClient(kubeClient.CoreV1().RESTClient(), "configmaps",
		proxyconfig.ManagedClusterLabelAllowListNamespace, fields.Everything())

	options := cache.InformerOptions{
		ListerWatcher: watchlist,
		ObjectType:    &v1.ConfigMap{},
		Handler:       GetManagedClusterLabelAllowListEventHandler(kubeClient),
	}
	_, controller := cache.NewInformerWithOptions(options)

	stop := make(chan struct{})
	go controller.Run(stop)
	for {
		time.Sleep(time.Second * 30)
		klog.V(1).Infof("found %v labels", len(AllManagedClusterLabelNames))
	}
}

// GetManagedClusterLabelAllowListEventHandler return event handler for managedcluster label allow list watch event.
func GetManagedClusterLabelAllowListEventHandler(kubeClient kubernetes.Interface) cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if obj.(*v1.ConfigMap).Name == proxyconfig.GetManagedClusterLabelAllowListConfigMapName() {
				klog.Infof("added configmap: %s", proxyconfig.GetManagedClusterLabelAllowListConfigMapName())

				if ok := scheduler != nil; ok {
					if ok := scheduler.IsRunning(); !ok {
						go ScheduleManagedClusterLabelAllowlistResync(kubeClient)
					}
				}
			}
		},

		DeleteFunc: func(obj interface{}) {
			if obj.(*v1.ConfigMap).Name == proxyconfig.GetManagedClusterLabelAllowListConfigMapName() {
				klog.Warningf("deleted configmap: %s", proxyconfig.GetManagedClusterLabelAllowListConfigMapName())
				StopScheduleManagedClusterLabelAllowlistResync()
			}
		},

		UpdateFunc: func(oldObj, newObj interface{}) {
			if newObj.(*v1.ConfigMap).Name == proxyconfig.GetManagedClusterLabelAllowListConfigMapName() {
				klog.Infof("updated configmap: %s", proxyconfig.GetManagedClusterLabelAllowListConfigMapName())

				_ = unmarshalDataToManagedClusterLabelList(newObj.(*v1.ConfigMap).Data,
					proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(), syncLabelList)

				sortManagedLabelList(managedLabelList)
				sortManagedLabelList(syncLabelList)

				if ok := reflect.DeepEqual(syncLabelList, managedLabelList); !ok {
					managedLabelList.IgnoreList = syncLabelList.IgnoreList
					*syncLabelList = *managedLabelList
				}

				updateAllManagedClusterLabelNames(managedLabelList)
			}
		},
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

// resyncManagedClusterLabelAllowList resync the managedcluster Label allowlist configmap data.
func resyncManagedClusterLabelAllowList(kubeClient kubernetes.Interface) error {
	found, err := proxyconfig.GetManagedClusterLabelAllowListConfigmap(kubeClient,
		proxyconfig.ManagedClusterLabelAllowListNamespace)

	if err != nil {
		return err
	}

	err = unmarshalDataToManagedClusterLabelList(found.Data,
		proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(), syncLabelList)

	if err != nil {
		return err
	}

	sortManagedLabelList(managedLabelList)
	sortManagedLabelList(syncLabelList)

	syncIgnoreList := []string{}
	syncUpdate := false

	for _, label := range syncLabelList.IgnoreList {
		if slice.ContainsString(proxyconfig.GetRequiredLabelList(), label, nil) {
			klog.Infof("detected required managedcluster label in ignorelist. resetting label: %s", label)
		} else {
			syncIgnoreList = append(syncIgnoreList, label)
			syncUpdate = true
		}
	}

	sort.Strings(syncIgnoreList)
	if syncUpdate {
		syncLabelList.IgnoreList = syncIgnoreList
	}

	if ok := reflect.DeepEqual(syncLabelList, managedLabelList); !ok {
		klog.Infof("resyncing required for managedcluster label allowlist: %v",
			proxyconfig.GetManagedClusterLabelAllowListConfigMapName())

		managedLabelList.IgnoreList = syncLabelList.IgnoreList
		ignoreManagedClusterLabelNames(managedLabelList)

		*syncLabelList = *managedLabelList
		_ = marshalLabelListToConfigMap(found,
			proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(), syncLabelList)

		_, err := kubeClient.CoreV1().ConfigMaps(proxyconfig.ManagedClusterLabelAllowListNamespace).Update(
			context.TODO(),
			found,
			metav1.UpdateOptions{},
		)
		if err != nil {
			klog.Errorf("failed to update managedcluster label allowlist configmap: %v", err)
			return err
		}
	}

	return nil
}

func ScheduleManagedClusterLabelAllowlistResync(kubeClient kubernetes.Interface) {
	if scheduler == nil {
		InitScheduler()
	}

	_, err := scheduler.Tag(resyncTag).Every(30).Second().Do(resyncManagedClusterLabelAllowList, kubeClient)
	if err != nil {
		klog.Errorf("failed to schedule job for managedcluster allowlist resync: %v", err)
	}

	klog.Info("starting scheduler for managedcluster allowlist resync")
	scheduler.StartAsync()
}

func StopScheduleManagedClusterLabelAllowlistResync() {
	klog.Info("stopping scheduler for managedcluster allowlist resync")
	scheduler.Stop()

	if ok := scheduler.IsRunning(); !ok {
		InitScheduler()
	}
}

// ignoreManagedClusterLabelNames set key to ignore within the managedcluster label names map.
func ignoreManagedClusterLabelNames(managedLabelList *proxyconfig.ManagedClusterLabelList) {
	for _, key := range managedLabelList.IgnoreList {
		if _, ok := AllManagedClusterLabelNames[key]; !ok {
			klog.Infof("ignoring managedcluster label: %s", key)

		} else if isEnabled := AllManagedClusterLabelNames[key]; isEnabled {
			klog.Infof("disabled managedcluster label: %s", key)
		}

		AllManagedClusterLabelNames[key] = false
	}

	managedLabelList.RegexLabelList = []string{}
	regex := regexp.MustCompile(`[^\w]+`)
	AllManagedClusterLabelNamesMtx.RLock()
	defer AllManagedClusterLabelNamesMtx.RUnlock()
	for key, isEnabled := range AllManagedClusterLabelNames {
		if isEnabled {
			managedLabelList.RegexLabelList = append(
				managedLabelList.RegexLabelList,
				regex.ReplaceAllString(key, "_"),
			)
		}
	}
	syncLabelList.RegexLabelList = managedLabelList.RegexLabelList
}

// updateAllManagedClusterLabelNames updates all managed cluster label names status within the map.
func updateAllManagedClusterLabelNames(managedLabelList *proxyconfig.ManagedClusterLabelList) {
	if managedLabelList.LabelList != nil {
		addManagedClusterLabelNames(managedLabelList)
	} else {
		klog.Infof("managed label list is empty")
	}

	if managedLabelList.IgnoreList != nil {
		ignoreManagedClusterLabelNames(managedLabelList)
	} else {
		klog.Infof("managed ignore list is empty")
	}
}

// marshalLabelListToConfigMap marshal managedcluster label list data to configmap data key.
func marshalLabelListToConfigMap(obj interface{}, key string,
	managedLabelList *proxyconfig.ManagedClusterLabelList) error {
	data, err := yaml.Marshal(managedLabelList)

	if err != nil {
		klog.Errorf("failed to marshal managedLabelList data: %v", err)
		return err
	}
	obj.(*v1.ConfigMap).Data[key] = string(data)
	return nil
}

// unmarshalDataToManagedClusterLabelList unmarshal managedcluster label allowlist.
func unmarshalDataToManagedClusterLabelList(data map[string]string, key string,
	managedLabelList *proxyconfig.ManagedClusterLabelList) error {
	err := yaml.Unmarshal([]byte(data[key]), managedLabelList)

	if err != nil {
		klog.Errorf("failed to unmarshal configmap <%s> data to the managedLabelList: %v", key, err)
		return err
	}

	return nil
}
