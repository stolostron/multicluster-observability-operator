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
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

func TestGenerateAllowList(t *testing.T) {
	testCases := []struct {
		name              string
		currentAllowList  *ManagedClusterLabelList
		managedClusters   map[string]map[string]struct{}
		expectedAllowList *ManagedClusterLabelList
	}{
		{
			name: "no clusters, default allowlist",
			currentAllowList: &ManagedClusterLabelList{
				LabelList:  []string{"cloud", "vendor"},
				IgnoreList: []string{"name"},
			},
			managedClusters: map[string]map[string]struct{}{},
			expectedAllowList: &ManagedClusterLabelList{
				LabelList:  []string{"cloud", "vendor"},
				IgnoreList: []string{"name"},
			},
		},
		{
			name: "new labels from clusters are added",
			currentAllowList: &ManagedClusterLabelList{
				LabelList:  []string{"cloud"},
				IgnoreList: []string{},
			},
			managedClusters: map[string]map[string]struct{}{
				"cluster1": {"cloud": {}, "region": {}},
				"cluster2": {"vendor": {}},
			},
			expectedAllowList: &ManagedClusterLabelList{
				LabelList:  []string{"cloud", "region", "vendor"},
				IgnoreList: nil,
			},
		},
		{
			name: "ignored labels are not added",
			currentAllowList: &ManagedClusterLabelList{
				LabelList:  []string{},
				IgnoreList: []string{"region"},
			},
			managedClusters: map[string]map[string]struct{}{
				"cluster1": {"cloud": {}, "region": {}},
			},
			expectedAllowList: &ManagedClusterLabelList{
				LabelList:  []string{"cloud"},
				IgnoreList: []string{"region"},
			},
		},
		{
			name:             "empty current allowlist",
			currentAllowList: &ManagedClusterLabelList{},
			managedClusters: map[string]map[string]struct{}{
				"cluster1": {"cloud": {}, "region": {}},
				"cluster2": {"vendor": {}},
			},
			expectedAllowList: &ManagedClusterLabelList{
				LabelList:  []string{"cloud", "region", "vendor"},
				IgnoreList: nil,
			},
		},
		{
			name: "no new labels to add",
			currentAllowList: &ManagedClusterLabelList{
				LabelList:  []string{"cloud", "region", "vendor"},
				IgnoreList: []string{},
			},
			managedClusters: map[string]map[string]struct{}{
				"cluster1": {"cloud": {}, "region": {}},
				"cluster2": {"vendor": {}},
			},
			expectedAllowList: &ManagedClusterLabelList{
				LabelList:  []string{"cloud", "region", "vendor"},
				IgnoreList: nil,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := generateAllowList(tc.currentAllowList, tc.managedClusters)
			assert.Equal(t, tc.expectedAllowList, result)
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
			case <-informer.syncAllowList:
				assert.True(t, tc.expectSyncTriggered, "sync should have been triggered but was not")
			case <-time.After(100 * time.Millisecond):
				assert.False(t, tc.expectSyncTriggered, "sync should not have been triggered but was")
			}
		})
	}
}

func TestCheckForUpdate(t *testing.T) {
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
		informer.managedClusters = map[string]map[string]struct{}{
			"cluster1": {"cloud": {}, "vendor": {}, "region": {}},
		}
		informer.managedLabelAllowListConfigmap = initialCMData

		informer.checkForUpdate()

		updatedCM, err := kubeClient.CoreV1().ConfigMaps(namespace).Get(context.TODO(), cmName, metav1.GetOptions{})
		require.NoError(t, err)

		updatedList := &ManagedClusterLabelList{}
		err = yaml.Unmarshal([]byte(updatedCM.Data[cmKey]), updatedList)
		require.NoError(t, err)

		expectedLabels := []string{"cloud", "region", "vendor"}
		assert.Equal(t, expectedLabels, updatedList.LabelList)
		assert.Equal(t, []string{"name"}, updatedList.IgnoreList)

		// Check in-memory state is also updated
		informer.allowlistMtx.RLock()
		defer informer.allowlistMtx.RUnlock()
		assert.Equal(t, expectedLabels, informer.managedLabelAllowListConfigmap.LabelList)
		assert.Equal(t, []string{"cloud", "region", "vendor"}, informer.managedLabelAllowListConfigmap.RegexLabelList)
	})

	// Test case: no update is needed.
	t.Run("no update needed", func(t *testing.T) {
		kubeClient := fake.NewSimpleClientset(cm.DeepCopy())
		informer := NewManagedClusterInformer(context.TODO(), nil, kubeClient)
		// Clusters have labels that are already in the allowlist
		informer.managedClusters = map[string]map[string]struct{}{
			"cluster1": {"cloud": {}, "vendor": {}},
		}
		// In-memory state is the same as on-cluster state
		informer.managedLabelAllowListConfigmap = initialCMData

		informer.checkForUpdate()

		// Verify the ConfigMap was NOT updated by checking if Data is unchanged.
		updatedCM, err := kubeClient.CoreV1().ConfigMaps(namespace).Get(context.TODO(), cmName, metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, string(initialCMDataBytes), updatedCM.Data[cmKey])
	})
}

func TestEnsureManagedClusterLabelAllowListConfigmapExists(t *testing.T) {
	namespace := proxyconfig.ManagedClusterLabelAllowListNamespace
	cmName := proxyconfig.ManagedClusterLabelAllowListConfigMapName

	t.Run("configmap does not exist", func(t *testing.T) {
		kubeClient := fake.NewSimpleClientset()
		informer := NewManagedClusterInformer(context.TODO(), nil, kubeClient)

		err := informer.ensureManagedClusterLabelAllowListConfigmapExists()
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

		err := informer.ensureManagedClusterLabelAllowListConfigmapExists()
		require.NoError(t, err)

		cm, err := kubeClient.CoreV1().ConfigMaps(namespace).Get(context.TODO(), cmName, metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, "data", cm.Data["test"], "Existing ConfigMap should not be modified")
	})
}
