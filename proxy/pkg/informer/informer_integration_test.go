// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

//go:build integration

// Package informer_test contains integration tests for the informer package.
//
// These tests use the controller-runtime's envtest package to set up a real, temporary Kubernetes API server
// and etcd instance. This approach was chosen because the standard fake clientsets (e.g., fake.NewSimpleClientset)
// are not sufficiently thread-safe to handle the concurrent ListWatch operations initiated by the multiple
// informers in the ManagedClusterInformer. Using fake clients resulted in panics and race conditions during
// the initial cache sync.
//
// This test suite specifically validates:
//   - The entire startup flow of the ManagedClusterInformer's Run() function.
//   - That the initial cache synchronization completes successfully without deadlocks or race conditions,
//     which was the primary issue this test was created to solve.
//   - The successful reconciliation of the allowlist ConfigMap after the initial data from the API server
//     has been processed.
package informer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	proxyconfig "github.com/stolostron/multicluster-observability-operator/proxy/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	clusterclientset "open-cluster-management.io/api/client/cluster/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	cfg *rest.Config
)

func TestMain(m *testing.M) {
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "vendor", "open-cluster-management.io", "api", "cluster", "v1")},
	}

	var err error
	cfg, err = testEnv.Start()
	if err != nil {
		panic(err)
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}

	_, err = kubeClient.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: proxyconfig.ManagedClusterLabelAllowListNamespace},
	}, metav1.CreateOptions{})
	if err != nil {
		panic(err)
	}

	code := m.Run()

	err = testEnv.Stop()
	if err != nil {
		panic(err)
	}

	os.Exit(code)
}

func TestRun_InitialSync(t *testing.T) {
	namespace := proxyconfig.ManagedClusterLabelAllowListNamespace
	cmName := proxyconfig.ManagedClusterLabelAllowListConfigMapName
	cmKey := proxyconfig.ManagedClusterLabelAllowListConfigMapKey

	kubeClient, err := kubernetes.NewForConfig(cfg)
	require.NoError(t, err)
	clusterClient, err := clusterclientset.NewForConfig(cfg)
	require.NoError(t, err)

	// Create a ConfigMap that will exist at startup.
	initialCMData := &ManagedClusterLabelList{
		LabelList:  []string{"initial_label"},
		IgnoreList: []string{},
	}
	initialCMDataBytes, err := yaml.Marshal(initialCMData)
	require.NoError(t, err)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: namespace},
		Data:       map[string]string{cmKey: string(initialCMDataBytes)},
	}
	_, err = kubeClient.CoreV1().ConfigMaps(namespace).Create(context.TODO(), cm, metav1.CreateOptions{})
	require.NoError(t, err)
	t.Cleanup(func() {
		kubeClient.CoreV1().ConfigMaps(namespace).Delete(context.TODO(), cmName, metav1.DeleteOptions{})
	})

	// Create a ManagedCluster that will exist at startup.
	cluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "cluster1",
			Labels: map[string]string{"cloud": "aws", "vendor": "redhat"},
		},
	}
	_, err = clusterClient.ClusterV1().ManagedClusters().Create(context.TODO(), cluster, metav1.CreateOptions{})
	require.NoError(t, err)
	t.Cleanup(func() {
		clusterClient.ClusterV1().ManagedClusters().Delete(context.TODO(), "cluster1", metav1.DeleteOptions{})
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	informer := NewManagedClusterInformer(ctx, clusterClient, kubeClient)

	// Run the informer in a goroutine.
	go informer.Run()

	// Wait for the caches to sync.
	err = wait.PollUntilContextTimeout(context.Background(), 100*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		return informer.HasSynced(), nil
	})
	require.NoError(t, err, "informer did not sync within the timeout period")

	// After sync, the initial load should have populated the in-memory allowlist
	// with the content of the ConfigMap that existed at startup.
	informer.allowlistMtx.RLock()
	assert.ElementsMatch(t, []string{"initial_label"}, informer.inMemoryAllowlist.LabelList)
	assert.ElementsMatch(t, []string{"initial_label"}, informer.inMemoryAllowlist.RegexLabelList)
	informer.allowlistMtx.RUnlock()

	// After sync, wait for the allowlist to be updated based on the initial resources.
	err = wait.PollUntilContextTimeout(context.Background(), 100*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		updatedCM, err := kubeClient.CoreV1().ConfigMaps(namespace).Get(context.TODO(), cmName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		updatedList := &ManagedClusterLabelList{}
		if err := yaml.Unmarshal([]byte(updatedCM.Data[cmKey]), updatedList); err != nil {
			return false, err
		}

		return len(updatedList.LabelList) > 1, nil
	})
	require.NoError(t, err, "ConfigMap was not updated with cluster labels after sync")

	// Final check of the ConfigMap content.
	updatedCM, err := kubeClient.CoreV1().ConfigMaps(namespace).Get(context.TODO(), cmName, metav1.GetOptions{})
	require.NoError(t, err)
	updatedList := &ManagedClusterLabelList{}
	err = yaml.Unmarshal([]byte(updatedCM.Data[cmKey]), updatedList)
	require.NoError(t, err)

	expectedLabels := []string{"cloud", "vendor", "name", "cluster.open-cluster-management.io/clusterset", "initial_label"}
	assert.ElementsMatch(t, expectedLabels, updatedList.LabelList)

	// Also check the in-memory representation.
	informer.allowlistMtx.RLock()
	defer informer.allowlistMtx.RUnlock()
	assert.ElementsMatch(t, expectedLabels, informer.inMemoryAllowlist.LabelList)
	assert.ElementsMatch(t, []string{"cloud", "vendor", "name", "cluster_open_cluster_management_io_clusterset", "initial_label"}, informer.inMemoryAllowlist.RegexLabelList)
}
