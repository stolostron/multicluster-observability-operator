// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package informer

import (
	"context"
	"testing"
	"time"

	proxyconfig "github.com/stolostron/multicluster-observability-operator/proxy/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/yaml"
)

func TestGenerateAllowList(t *testing.T) {
	baseClusters := map[string]map[string]struct{}{
		"cluster1": {"cloud": {}, "vendor": {}},
		"cluster2": {"region": {}},
	}

	testCases := []struct {
		name               string
		currentAllowList   *ManagedClusterLabelList
		lastKnownAllowlist *ManagedClusterLabelList
		managedClusters    map[string]map[string]struct{}
		expectedAllowList  *ManagedClusterLabelList
	}{
		{
			name: "newly discovered labels are added to allowlist",
			currentAllowList: &ManagedClusterLabelList{
				LabelList:  []string{},
				IgnoreList: []string{},
			},
			lastKnownAllowlist: &ManagedClusterLabelList{},
			managedClusters:    baseClusters,
			expectedAllowList: &ManagedClusterLabelList{
				LabelList:  []string{"cloud", "cluster.open-cluster-management.io/clusterset", "name", "region", "vendor"},
				IgnoreList: nil,
			},
		},
		{
			name: "label removed from ignore list is re-added (sticky)",
			currentAllowList: &ManagedClusterLabelList{
				LabelList:  []string{},
				IgnoreList: []string{}, // User removed "vendor"
			},
			lastKnownAllowlist: &ManagedClusterLabelList{
				LabelList:  []string{},
				IgnoreList: []string{"vendor"}, // It was here before
			},
			managedClusters: baseClusters,
			expectedAllowList: &ManagedClusterLabelList{
				LabelList:  []string{"cloud", "cluster.open-cluster-management.io/clusterset", "name", "region"},
				IgnoreList: []string{"vendor"},
			},
		},
		{
			name: "label removed from allow list is re-added (sticky)",
			currentAllowList: &ManagedClusterLabelList{
				LabelList:  []string{}, // User removed "vendor"
				IgnoreList: []string{},
			},
			lastKnownAllowlist: &ManagedClusterLabelList{
				LabelList:  []string{"vendor"}, // It was here before
				IgnoreList: []string{},
			},
			managedClusters: baseClusters,
			expectedAllowList: &ManagedClusterLabelList{
				LabelList:  []string{"cloud", "cluster.open-cluster-management.io/clusterset", "name", "region", "vendor"},
				IgnoreList: nil,
			},
		},
		{
			name: "un-ignoring a label requires moving it to the allowlist",
			currentAllowList: &ManagedClusterLabelList{
				LabelList:  []string{"vendor"}, // User moved "vendor" here
				IgnoreList: []string{},         // And removed it from here
			},
			lastKnownAllowlist: &ManagedClusterLabelList{
				LabelList:  []string{},
				IgnoreList: []string{"vendor"},
			},
			managedClusters: baseClusters,
			expectedAllowList: &ManagedClusterLabelList{
				LabelList:  []string{"cloud", "cluster.open-cluster-management.io/clusterset", "name", "region", "vendor"},
				IgnoreList: nil,
			},
		},
		{
			name: "removed orphaned label stays removed",
			currentAllowList: &ManagedClusterLabelList{
				LabelList:  []string{"cloud"}, // User removed "orphaned"
				IgnoreList: []string{},
			},
			lastKnownAllowlist: &ManagedClusterLabelList{
				LabelList:  []string{"cloud", "orphaned"}, // It was here before
				IgnoreList: []string{},
			},
			managedClusters: baseClusters,
			expectedAllowList: &ManagedClusterLabelList{
				LabelList:  []string{"cloud", "cluster.open-cluster-management.io/clusterset", "name", "region", "vendor"},
				IgnoreList: nil,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := generateAllowList(tc.currentAllowList, tc.lastKnownAllowlist, tc.managedClusters)
			assert.ElementsMatch(t, tc.expectedAllowList.LabelList, result.LabelList)
			if tc.expectedAllowList.IgnoreList == nil {
				assert.Empty(t, result.IgnoreList)
			} else {
				assert.ElementsMatch(t, tc.expectedAllowList.IgnoreList, result.IgnoreList)
			}
		})
	}
}

func TestManagedClusterEventHandler(t *testing.T) {
	testCases := []struct {
		name                string
		action              func(handler cache.ResourceEventHandler)
		initialClusters     map[string]map[string]struct{}
		expectedClusters    map[string]map[string]struct{}
		expectSyncTriggered bool
	}{
		{
			name: "add cluster with new label",
			action: func(handler cache.ResourceEventHandler) {
				handler.OnAdd(&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster2", Labels: map[string]string{"new_label": "true"}},
				}, false)
			},
			initialClusters:     map[string]map[string]struct{}{"cluster1": {"existing_label": {}}},
			expectedClusters:    map[string]map[string]struct{}{"cluster1": {"existing_label": {}}, "cluster2": {"new_label": {}}},
			expectSyncTriggered: true,
		},
		{
			name: "add cluster with existing label",
			action: func(handler cache.ResourceEventHandler) {
				handler.OnAdd(&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster2", Labels: map[string]string{"existing_label": "true"}},
				}, false)
			},
			initialClusters:     map[string]map[string]struct{}{"cluster1": {"existing_label": {}}},
			expectedClusters:    map[string]map[string]struct{}{"cluster1": {"existing_label": {}}, "cluster2": {"existing_label": {}}},
			expectSyncTriggered: false,
		},
		{
			name: "update cluster, no label change",
			action: func(handler cache.ResourceEventHandler) {
				old := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1", Labels: map[string]string{"label": "a"}}}
				new := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1", Labels: map[string]string{"label": "a"}}}
				handler.OnUpdate(old, new)
			},
			initialClusters:     map[string]map[string]struct{}{"cluster1": {"label": {}}},
			expectedClusters:    map[string]map[string]struct{}{"cluster1": {"label": {}}},
			expectSyncTriggered: false,
		},
		{
			name: "update cluster, add new label",
			action: func(handler cache.ResourceEventHandler) {
				old := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1", Labels: map[string]string{"label": "a"}}}
				new := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1", Labels: map[string]string{"label": "a", "new_label": "b"}}}
				handler.OnUpdate(old, new)
			},
			initialClusters:     map[string]map[string]struct{}{"cluster1": {"label": {}}},
			expectedClusters:    map[string]map[string]struct{}{"cluster1": {"label": {}, "new_label": {}}},
			expectSyncTriggered: true,
		},
		{
			name: "delete cluster, removing last instance of a label",
			action: func(handler cache.ResourceEventHandler) {
				handler.OnDelete(&clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster2"}})
			},
			initialClusters:     map[string]map[string]struct{}{"cluster1": {"shared": {}}, "cluster2": {"unique": {}}},
			expectedClusters:    map[string]map[string]struct{}{"cluster1": {"shared": {}}},
			expectSyncTriggered: true,
		},
		{
			name: "delete cluster, but its labels exist on other clusters",
			action: func(handler cache.ResourceEventHandler) {
				handler.OnDelete(&clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster2"}})
			},
			initialClusters:     map[string]map[string]struct{}{"cluster1": {"shared": {}}, "cluster2": {"shared": {}}},
			expectedClusters:    map[string]map[string]struct{}{"cluster1": {"shared": {}}},
			expectSyncTriggered: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			informer := NewManagedClusterInformer(context.TODO(), nil, nil)
			informer.managedClusters = tc.initialClusters
			handler := informer.getManagedClusterEventHandler()

			tc.action(&handler)

			assert.Equal(t, tc.expectedClusters, informer.managedClusters)

			select {
			case <-informer.syncAllowListCh:
				assert.True(t, tc.expectSyncTriggered, "sync should have been triggered but was not")
			case <-time.After(100 * time.Millisecond):
				assert.False(t, tc.expectSyncTriggered, "sync should not have been triggered but was")
			}
		})
	}
}

func TestSyncAllowlistConfigMap(t *testing.T) {
	namespace := proxyconfig.ManagedClusterLabelAllowListNamespace
	cmName := proxyconfig.ManagedClusterLabelAllowListConfigMapName
	cmKey := proxyconfig.ManagedClusterLabelAllowListConfigMapKey

	// Initial ConfigMap state
	initialCMData := &ManagedClusterLabelList{
		LabelList:  []string{"cloud", "vendor"},
		IgnoreList: []string{"name"},
	}
	initialCMDataBytes, err := yaml.Marshal(initialCMData)
	require.NoError(t, err)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: namespace},
		Data:       map[string]string{cmKey: string(initialCMDataBytes)},
	}

	// Test case: an update is required because a new label "region" was discovered.
	t.Run("update is required", func(t *testing.T) {
		kubeClient := fake.NewSimpleClientset(cm.DeepCopy())
		informer := NewManagedClusterInformer(context.TODO(), nil, kubeClient)
		informer.hasSynced.Store(true)
		informer.managedClusters = map[string]map[string]struct{}{
			"cluster1": {"cloud": {}, "vendor": {}, "region": {}},
		}
		informer.inMemoryAllowlist = initialCMData

		informer.syncAllowlistConfigMap()

		updatedCM, err := kubeClient.CoreV1().ConfigMaps(namespace).Get(context.TODO(), cmName, metav1.GetOptions{})
		require.NoError(t, err)

		updatedList := &ManagedClusterLabelList{}
		err = yaml.Unmarshal([]byte(updatedCM.Data[cmKey]), updatedList)
		require.NoError(t, err)

		expectedLabels := []string{"cloud", "cluster.open-cluster-management.io/clusterset", "region", "vendor"}
		assert.Equal(t, expectedLabels, updatedList.LabelList)
		assert.Equal(t, []string{"name"}, updatedList.IgnoreList)

		// Check in-memory state is also updated
		informer.allowlistMtx.RLock()
		defer informer.allowlistMtx.RUnlock()
		assert.Equal(t, expectedLabels, informer.inMemoryAllowlist.LabelList)
		assert.Equal(t, []string{"cloud", "cluster_open_cluster_management_io_clusterset", "region", "vendor"}, informer.inMemoryAllowlist.RegexLabelList)
	})

	// Test case: no update is needed.
	t.Run("no update needed", func(t *testing.T) {
		kubeClient := fake.NewSimpleClientset(cm.DeepCopy())
		informer := NewManagedClusterInformer(context.TODO(), nil, kubeClient)
		informer.hasSynced.Store(true)
		// Clusters have labels that are already in the allowlist
		informer.managedClusters = map[string]map[string]struct{}{
			"cluster1": {"cloud": {}, "vendor": {}},
		}
		// In-memory state is the same as on-cluster state
		informer.inMemoryAllowlist = initialCMData

		informer.syncAllowlistConfigMap()

		// Verify the ConfigMap was NOT updated by checking if Data is unchanged.
		updatedCM, err := kubeClient.CoreV1().ConfigMaps(namespace).Get(context.TODO(), cmName, metav1.GetOptions{})
		require.NoError(t, err)
		updatedList := &ManagedClusterLabelList{}
		err = yaml.Unmarshal([]byte(updatedCM.Data[cmKey]), updatedList)
		require.NoError(t, err)
		assert.Equal(t, []string{"cloud", "cluster.open-cluster-management.io/clusterset", "vendor"}, updatedList.LabelList)
	})

	t.Run("update is skipped if cache not synced", func(t *testing.T) {
		kubeClient := fake.NewSimpleClientset(cm.DeepCopy())
		informer := NewManagedClusterInformer(context.TODO(), nil, kubeClient)
		informer.hasSynced.Store(false) // Ensure cache is not synced
		informer.managedClusters = map[string]map[string]struct{}{
			"cluster1": {"new_label": {}},
		}
		informer.inMemoryAllowlist = initialCMData

		informer.syncAllowlistConfigMap()

		// Verify the ConfigMap was NOT updated
		updatedCM, err := kubeClient.CoreV1().ConfigMaps(namespace).Get(context.TODO(), cmName, metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, string(initialCMDataBytes), updatedCM.Data[cmKey])
	})

	t.Run("reverts manual incorrect change", func(t *testing.T) {
		// In-memory and managed clusters are in a correct, steady state.
		steadyStateList := &ManagedClusterLabelList{
			LabelList:  []string{"cloud", "cluster.open-cluster-management.io/clusterset", "name", "vendor"},
			IgnoreList: []string{},
		}

		informer := NewManagedClusterInformer(context.TODO(), nil, nil)
		informer.hasSynced.Store(true)
		informer.managedClusters = map[string]map[string]struct{}{
			"cluster1": {"cloud": {}, "vendor": {}},
		}
		informer.inMemoryAllowlist = steadyStateList

		// But the ConfigMap on the cluster has been manually edited to be incorrect.
		incorrectCMData := &ManagedClusterLabelList{
			LabelList:  []string{"cloud"}, // "vendor" is missing
			IgnoreList: []string{},
		}
		incorrectCMDataBytes, err := yaml.Marshal(incorrectCMData)
		require.NoError(t, err)
		incorrectCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: namespace},
			Data:       map[string]string{cmKey: string(incorrectCMDataBytes)},
		}
		kubeClient := fake.NewSimpleClientset(incorrectCM)
		informer.kubeClient = kubeClient

		// Act
		informer.syncAllowlistConfigMap()

		// Assert
		updatedCM, err := kubeClient.CoreV1().ConfigMaps(namespace).Get(context.TODO(), cmName, metav1.GetOptions{})
		require.NoError(t, err)

		finalList := &ManagedClusterLabelList{}
		err = yaml.Unmarshal([]byte(updatedCM.Data[cmKey]), finalList)
		require.NoError(t, err)

		// The informer should have added the missing labels back.
		assert.ElementsMatch(t, steadyStateList.LabelList, finalList.LabelList)
		assert.ElementsMatch(t, steadyStateList.IgnoreList, finalList.IgnoreList)
	})

	t.Run("in-memory list is updated when no-op update occurs", func(t *testing.T) {
		// On-cluster ConfigMap is ahead of the in-memory list.
		onClusterCMData := &ManagedClusterLabelList{
			LabelList:  []string{"cloud", "cluster.open-cluster-management.io/clusterset", "region", "vendor"},
			IgnoreList: []string{"name"},
		}
		onClusterCMDataBytes, err := yaml.Marshal(onClusterCMData)
		require.NoError(t, err)
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: namespace},
			Data:       map[string]string{cmKey: string(onClusterCMDataBytes)},
		}

		kubeClient := fake.NewSimpleClientset(cm.DeepCopy())
		informer := NewManagedClusterInformer(context.TODO(), nil, kubeClient)
		informer.hasSynced.Store(true)

		// Clusters have labels that match the on-cluster ConfigMap.
		informer.managedClusters = map[string]map[string]struct{}{
			"cluster1": {"cloud": {}, "vendor": {}, "region": {}},
		}

		// But the in-memory state is stale.
		staleInMemoryList := &ManagedClusterLabelList{
			LabelList:  []string{"cloud", "vendor"},
			IgnoreList: []string{"name"},
		}
		informer.inMemoryAllowlist = staleInMemoryList

		informer.syncAllowlistConfigMap()

		// Verify the ConfigMap was NOT updated.
		updatedCM, err := kubeClient.CoreV1().ConfigMaps(namespace).Get(context.TODO(), cmName, metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, string(onClusterCMDataBytes), updatedCM.Data[cmKey])

		// Verify the in-memory state IS updated.
		informer.allowlistMtx.RLock()
		defer informer.allowlistMtx.RUnlock()
		assert.ElementsMatch(t, onClusterCMData.LabelList, informer.inMemoryAllowlist.LabelList)
		assert.ElementsMatch(t, onClusterCMData.IgnoreList, informer.inMemoryAllowlist.IgnoreList)

		expectedRegexList := []string{"cloud", "cluster_open_cluster_management_io_clusterset", "region", "vendor"}
		assert.ElementsMatch(t, expectedRegexList, informer.inMemoryAllowlist.RegexLabelList)
	})
}

func TestEnsureAllowlistConfigMapExists(t *testing.T) {
	namespace := proxyconfig.ManagedClusterLabelAllowListNamespace
	cmName := proxyconfig.ManagedClusterLabelAllowListConfigMapName

	t.Run("configmap does not exist", func(t *testing.T) {
		kubeClient := fake.NewSimpleClientset()
		informer := NewManagedClusterInformer(context.TODO(), nil, kubeClient)

		err := informer.ensureAllowlistConfigMapExists()
		require.NoError(t, err)

		_, err = kubeClient.CoreV1().ConfigMaps(namespace).Get(context.TODO(), cmName, metav1.GetOptions{})
		assert.NoError(t, err, "ConfigMap should have been created")
	})

	t.Run("configmap already exists", func(t *testing.T) {
		existingCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: namespace},
			Data:       map[string]string{"test": "data"},
		}
		kubeClient := fake.NewSimpleClientset(existingCM)
		informer := NewManagedClusterInformer(context.TODO(), nil, kubeClient)

		err := informer.ensureAllowlistConfigMapExists()
		require.NoError(t, err)

		cm, err := kubeClient.CoreV1().ConfigMaps(namespace).Get(context.TODO(), cmName, metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, "data", cm.Data["test"], "Existing ConfigMap should not be modified")
	})
}
