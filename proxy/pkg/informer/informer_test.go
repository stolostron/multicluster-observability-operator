// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package informer

import (
	"context"
	"regexp"
	"sort"
	"testing"
	"time"

	proxyconfig "github.com/stolostron/multicluster-observability-operator/proxy/pkg/config"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakekube "k8s.io/client-go/kubernetes/fake"
	fakecluster "open-cluster-management.io/api/client/cluster/clientset/versioned/fake"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

func TestMarshalLabelListToConfigMap(t *testing.T) {
	managedClusterLabelAllowlist := proxyconfig.CreateManagedClusterLabelAllowListCM("ns1").Data
	managedClusterLabelList := &proxyconfig.ManagedClusterLabelList{}
	err := unmarshalDataToManagedClusterLabelList(managedClusterLabelAllowlist,
		proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(), managedClusterLabelList)
	assert.NoError(t, err)
	assert.NotEmpty(t, managedClusterLabelList.LabelList)
	assert.NotEmpty(t, managedClusterLabelList.IgnoreList)

	cm := &corev1.ConfigMap{}
	err = marshalLabelListToConfigMap(cm, proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(),
		managedClusterLabelList)
	assert.NoError(t, err)
	assert.NotEmpty(t, cm.Data[proxyconfig.GetManagedClusterLabelAllowListConfigMapKey()])
}

func TestGetManagedClusterEventHandler(t *testing.T) {
	cluster1 := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster1",
			Labels: map[string]string{
				"name":        "cluster1",
				"environment": "dev",
			},
		},
	}
	cluster2 := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster2",
			Labels: map[string]string{
				"name":        "cluster2",
				"environment": "dev",
				"cloud":       "Amazon",
			},
		},
	}

	informer := NewManagedClusterInformer(
		fakecluster.NewSimpleClientset(),
		fakekube.NewSimpleClientset(),
	)

	eventHandler := informer.getManagedClusterEventHandler()

	// Add cluster1
	eventHandler.AddFunc(cluster1)
	assert.Equal(t, map[string]string{"cluster1": "cluster1"}, informer.GetAllManagedClusterNames())
	assert.True(t, informer.GetAllManagedClusterLabelNames()["name"])
	assert.True(t, informer.GetAllManagedClusterLabelNames()["environment"])
	assert.False(t, informer.GetAllManagedClusterLabelNames()["cloud"])

	// Update with cluster2. In informer logic, this is like adding a new cluster.
	eventHandler.UpdateFunc(cluster1, cluster2)
	assert.Equal(t, map[string]string{"cluster1": "cluster1", "cluster2": "cluster2"}, informer.GetAllManagedClusterNames())
	assert.True(t, informer.GetAllManagedClusterLabelNames()["name"])
	assert.True(t, informer.GetAllManagedClusterLabelNames()["environment"])
	assert.True(t, informer.GetAllManagedClusterLabelNames()["cloud"])

	// Delete cluster1
	eventHandler.DeleteFunc(cluster1)
	assert.Equal(t, map[string]string{"cluster2": "cluster2"}, informer.GetAllManagedClusterNames())
	// Labels are not removed on delete
	assert.True(t, informer.GetAllManagedClusterLabelNames()["name"])
	assert.True(t, informer.GetAllManagedClusterLabelNames()["environment"])
	assert.True(t, informer.GetAllManagedClusterLabelNames()["cloud"])
}

func TestGetManagedClusterLabelAllowListEventHandler(t *testing.T) {
	cm := proxyconfig.CreateManagedClusterLabelAllowListCM("open-cluster-management-observability")

	kubeClient := fakekube.NewSimpleClientset(cm)
	informer := NewManagedClusterInformer(
		fakecluster.NewSimpleClientset(),
		kubeClient,
	)
	// Isolate test from global singletons
	informer.managedLabelList = &proxyconfig.ManagedClusterLabelList{
		LabelList:  []string{},
		IgnoreList: []string{},
	}
	informer.syncLabelList = &proxyconfig.ManagedClusterLabelList{
		LabelList:  []string{},
		IgnoreList: []string{},
	}

	// Add some labels to the informer state first
	informer.allManagedClusterLabelNames["vendor"] = true
	informer.allManagedClusterLabelNames["cloud"] = true
	informer.managedLabelList.LabelList = []string{"vendor", "cloud"}
	informer.addManagedClusterLabelNames() // To populate RegexLabelList

	eventHandler := informer.getManagedClusterLabelAllowListEventHandler()

	// Test AddFunc
	eventHandler.AddFunc(cm)
	assert.Eventually(t, func() bool { return informer.scheduler.IsRunning() }, time.Second*5, time.Millisecond*100)
	informer.StopScheduleManagedClusterLabelAllowlistResync()

	// Test UpdateFunc
	updatedCm := cm.DeepCopy()
	updatedCm.Data[proxyconfig.GetManagedClusterLabelAllowListConfigMapKey()] = `
label_list:
  - cloud
  - vendor
ignore_list:
  - name
  - clusterID
  - vendor
`

	eventHandler.UpdateFunc(cm, updatedCm)

	// assert.False(t, informer.GetAllManagedClusterLabelNames()["vendor"], "Label 'vendor' should be disabled")
	assert.True(t, informer.GetAllManagedClusterLabelNames()["cloud"], "Label 'cloud' should be enabled")

	// Test DeleteFunc
	informer.ScheduleManagedClusterLabelAllowlistResync()
	assert.Eventually(t, func() bool { return informer.scheduler.IsRunning() }, time.Second*5, time.Millisecond*100)
	eventHandler.DeleteFunc(cm)
	assert.False(t, informer.scheduler.IsRunning())
}

func TestStopScheduleManagedClusterLabelAllowlistResync(t *testing.T) {
	informer := NewManagedClusterInformer(
		fakecluster.NewSimpleClientset(),
		fakekube.NewSimpleClientset(),
	)

	_, err := informer.scheduler.Every(1).Seconds().Do(func() {})
	assert.NoError(t, err)

	informer.scheduler.StartAsync()
	time.Sleep(2 * time.Second)
	assert.True(t, informer.scheduler.IsRunning())

	informer.StopScheduleManagedClusterLabelAllowlistResync()
	assert.False(t, informer.scheduler.IsRunning())
}

func TestScheduleManagedClusterLabelAllowlistResync(t *testing.T) {
	namespace := proxyconfig.ManagedClusterLabelAllowListNamespace
	cm := proxyconfig.CreateManagedClusterLabelAllowListCM(namespace)
	kubeClient := fakekube.NewSimpleClientset(cm)

	informer := NewManagedClusterInformer(
		fakecluster.NewSimpleClientset(),
		kubeClient,
	)

	informer.managedLabelList.LabelList = []string{"cloud", "environment"}
	informer.updateAllManagedClusterLabelNames()

	informer.ScheduleManagedClusterLabelAllowlistResync()
	time.Sleep(2 * time.Second)
	assert.True(t, informer.scheduler.IsRunning())

	informer.StopScheduleManagedClusterLabelAllowlistResync()
	assert.False(t, informer.scheduler.IsRunning())

	informer.ScheduleManagedClusterLabelAllowlistResync()
	time.Sleep(2 * time.Second)
	assert.True(t, informer.scheduler.IsRunning())

	informer.StopScheduleManagedClusterLabelAllowlistResync()
	assert.False(t, informer.scheduler.IsRunning())
}

// TODO
func TestResyncManagedClusterLabelAllowList(t *testing.T) {
	namespace := proxyconfig.ManagedClusterLabelAllowListNamespace
	cm := proxyconfig.CreateManagedClusterLabelAllowListCM(namespace)
	kubeClient := fakekube.NewSimpleClientset(cm)

	informer := NewManagedClusterInformer(
		fakecluster.NewSimpleClientset(),
		kubeClient,
	)

	informer.managedLabelList.LabelList = []string{"cloud", "environment"}
	informer.updateAllManagedClusterLabelNames()

	err := informer.resyncManagedClusterLabelAllowList()
	assert.NoError(t, err)

	updatedCm, err := kubeClient.CoreV1().ConfigMaps(namespace).Get(context.TODO(), cm.Name, metav1.GetOptions{})
	assert.NoError(t, err)

	syncedList := &proxyconfig.ManagedClusterLabelList{}
	err = unmarshalDataToManagedClusterLabelList(updatedCm.Data,
		proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(), syncedList)
	assert.NoError(t, err)

	sort.Strings(syncedList.LabelList)
	assert.Contains(t, syncedList.LabelList, "cloud")
	assert.Contains(t, syncedList.LabelList, "environment")
}

func TestUpdateAllManagedClusterLabelNames(t *testing.T) {
	tests := []struct {
		name           string
		labelList      []string
		ignoreList     []string
		initialLabels  map[string]bool
		expectedLabels map[string]bool
	}{
		{
			name:           "Add new labels",
			labelList:      []string{"label1", "label2"},
			ignoreList:     nil,
			initialLabels:  map[string]bool{},
			expectedLabels: map[string]bool{"label1": true, "label2": true},
		},
		{
			name:           "Ignore labels",
			labelList:      nil,
			ignoreList:     []string{"label3"},
			initialLabels:  map[string]bool{"label3": true},
			expectedLabels: map[string]bool{"label3": false},
		},
		{
			name:           "Add and ignore labels",
			labelList:      []string{"label4"},
			ignoreList:     []string{"label5"},
			initialLabels:  map[string]bool{"label5": true},
			expectedLabels: map[string]bool{"label4": true, "label5": false},
		},
		{
			name:           "Ignore non-existing labels",
			labelList:      nil,
			ignoreList:     []string{"label6"},
			initialLabels:  map[string]bool{},
			expectedLabels: map[string]bool{"label6": false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			informer := NewManagedClusterInformer(
				fakecluster.NewSimpleClientset(),
				fakekube.NewSimpleClientset(),
			)

			informer.allManagedClusterLabelNames = tt.initialLabels
			informer.managedLabelList = &proxyconfig.ManagedClusterLabelList{
				LabelList:  tt.labelList,
				IgnoreList: tt.ignoreList,
			}

			informer.updateAllManagedClusterLabelNames()

			assert.Equal(t, tt.expectedLabels, informer.allManagedClusterLabelNames)

			regex := regexp.MustCompile(`[^\w]+`)
			expectedRegexList := []string{}
			for key, isEnabled := range tt.expectedLabels {
				if isEnabled {
					expectedRegexList = append(expectedRegexList, regex.ReplaceAllString(key, "_"))
				}
			}

			sort.Strings(informer.syncLabelList.RegexLabelList)
			sort.Strings(expectedRegexList)
			assert.Equal(t, expectedRegexList, informer.syncLabelList.RegexLabelList)
		})
	}
}

func TestSortManagedLabelList(t *testing.T) {
	sortManagedLabelList(nil)

	managedLabelList := &proxyconfig.ManagedClusterLabelList{
		IgnoreList:     []string{"c", "a", "b"},
		LabelList:      []string{"z", "y", "x"},
		RegexLabelList: []string{"foo", "bar"},
	}

	sortManagedLabelList(managedLabelList)

	assert.Equal(t, []string{"a", "b", "c"}, managedLabelList.IgnoreList)
	assert.Equal(t, []string{"x", "y", "z"}, managedLabelList.LabelList)
	assert.Equal(t, []string{"bar", "foo"}, managedLabelList.RegexLabelList)
}

func TestGetAllManagedClusterLabelNames(t *testing.T) {
	informer := NewManagedClusterInformer(
		fakecluster.NewSimpleClientset(),
		fakekube.NewSimpleClientset(),
	)

	informer.managedLabelList = &proxyconfig.ManagedClusterLabelList{
		IgnoreList: []string{"clusterID", "name", "environment"},
		LabelList:  []string{"cloud", "vendor"},
	}
	informer.updateAllManagedClusterLabelNames()

	labels := informer.GetAllManagedClusterLabelNames()
	assert.True(t, labels["cloud"])
	assert.True(t, labels["vendor"])
	assert.False(t, labels["name"])
	assert.False(t, labels["environment"])

	informer.managedLabelList.IgnoreList = []string{"clusterID", "vendor", "environment"}
	informer.managedLabelList.LabelList = []string{"cloud", "name", "environment"}
	informer.updateAllManagedClusterLabelNames()

	labels = informer.GetAllManagedClusterLabelNames()
	assert.True(t, labels["cloud"])
	assert.True(t, labels["name"])
	assert.False(t, labels["vendor"])
	assert.False(t, labels["environment"])
}
