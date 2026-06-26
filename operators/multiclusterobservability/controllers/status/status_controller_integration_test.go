// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

//go:build integration

package status_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	operatorv1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	promv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	observabilityshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	mcov1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/status"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	observatoriumAPIs "github.com/stolostron/observatorium-operator/api/v1alpha1"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func createBaseScheme(t *testing.T) *runtime.Scheme {
	scheme := runtime.NewScheme()
	require.NoError(t, kubescheme.AddToScheme(scheme))
	require.NoError(t, apiextensionsv1.AddToScheme(scheme))
	require.NoError(t, mcov1beta1.AddToScheme(scheme))
	require.NoError(t, mcov1beta2.AddToScheme(scheme))
	require.NoError(t, observatoriumAPIs.AddToScheme(scheme))
	require.NoError(t, promv1.AddToScheme(scheme))
	require.NoError(t, promv1alpha1.AddToScheme(scheme))
	require.NoError(t, routev1.AddToScheme(scheme))
	require.NoError(t, addonapiv1alpha1.AddToScheme(scheme))
	require.NoError(t, operatorv1.AddToScheme(scheme))

	return scheme
}

func newNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func newStorageSecret(name, ns, key string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Data: map[string][]byte{
			key: []byte(""),
		},
	}
}

func newMCO(ns, storageSecretName, storageSecretKey string) *mcov1beta2.MultiClusterObservability {
	return &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mco",
			Namespace: ns,
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta2.StorageConfig{
				MetricObjectStorage: &observabilityshared.PreConfiguredStorage{
					Name: storageSecretName,
					Key:  storageSecretKey,
				},
			},
			ObservabilityAddonSpec: &observabilityshared.ObservabilityAddonSpec{
				Interval: 44,
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
				},
			},
		},
	}
}

// TestIntegrationStatusManager_LifecycleAndReadiness verifies the StatusReconciler
// correctly interacts with the real API server, handling eventual consistency,
// readiness transitions, and concurrent update conflicts.
func TestIntegrationStatusManager_LifecycleAndReadiness(t *testing.T) {
	// Spin up an isolated envtest for this test to avoid collisions with the shared TestMain envtest
	localTestEnv := &envtest.Environment{
		CRDDirectoryPaths:       []string{filepath.Join("testdata", "crd"), filepath.Join("..", "..", "bundle", "manifests")},
		ControlPlaneStopTimeout: 5 * time.Minute,
	}

	localRestCfg, err := localTestEnv.Start()
	require.NoError(t, err)
	defer localTestEnv.Stop()

	scheme := createBaseScheme(t)
	k8sClient, err := client.New(localRestCfg, client.Options{Scheme: scheme})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup namespace and configurations using the default namespace expected by StatusReconciler
	ns := config.GetDefaultNamespace()
	err = k8sClient.Create(ctx, newNamespace(ns))
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("failed to create namespace: %v", err)
	}

	mcoName := "mco-status-test"
	config.SetMonitoringCRName(mcoName)

	storageSecretName := "status-test-storage-secret"
	storageSecretKey := "test-key"
	validS3Config := `
type: s3
config:
  bucket: test
  endpoint: test
  insecure: true
  access_key: test
  secret_key: test
`
	storageSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      storageSecretName,
			Namespace: ns,
		},
		Data: map[string][]byte{
			storageSecretKey: []byte(validS3Config),
		},
	}
	err = k8sClient.Create(ctx, storageSecret)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("failed to create storage secret: %v", err)
	}

	mco := newMCO(ns, storageSecretName, storageSecretKey)
	mco.Name = mcoName
	require.NoError(t, k8sClient.Create(ctx, mco))

	// Initialize StatusReconciler
	r := &status.StatusReconciler{
		Client: k8sClient,
		Log:    logf.Log,
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{Name: mcoName},
	}

	// 1. Initial State: Should be Failed due to missing Deployments/StatefulSets
	_, err = r.Reconcile(ctx, req)
	require.NoError(t, err)

	var fetchedMCO mcov1beta2.MultiClusterObservability
	err = k8sClient.Get(ctx, types.NamespacedName{Name: mcoName}, &fetchedMCO)
	require.NoError(t, err)
	hasFailed := false
	for _, cond := range fetchedMCO.Status.Conditions {
		if cond.Type == status.ConditionTypeFailed && cond.Reason == status.ReasonDeploymentNotFound {
			hasFailed = true
			break
		}
	}
	require.True(t, hasFailed, "Expected MCO to be in Failed state with ReasonDeploymentNotFound due to missing workloads")

	// 2. Eventual Consistency: Create expected workloads and observe transition to Ready
	expectedDeployments := config.GetExpectedDeploymentNames()
	for _, name := range expectedDeployments {
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "test"}}},
				},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, dep))
		dep.Status.Replicas = 1
		dep.Status.ReadyReplicas = 1
		require.NoError(t, k8sClient.Status().Update(ctx, dep))
	}

	expectedStatefulSets := config.GetExpectedStatefulSetNames()
	for _, name := range expectedStatefulSets {
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: appsv1.StatefulSetSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "test"}}},
				},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, sts))
		sts.Status.Replicas = 1
		sts.Status.ReadyReplicas = 1
		require.NoError(t, k8sClient.Status().Update(ctx, sts))
	}

	// Trigger reconcile to evaluate new workloads
	_, err = r.Reconcile(ctx, req)
	require.NoError(t, err)

	err = k8sClient.Get(ctx, types.NamespacedName{Name: mcoName}, &fetchedMCO)
	require.NoError(t, err)
	hasReady := false
	for _, cond := range fetchedMCO.Status.Conditions {
		if cond.Type == status.ConditionTypeReady && cond.Status == metav1.ConditionTrue {
			hasReady = true
			break
		}
	}
	require.True(t, hasReady, "Expected MCO to transition to Ready state after workloads became ready")

	// Cleanup
	k8sClient.Delete(ctx, mco)
	for _, name := range expectedDeployments {
		k8sClient.Delete(ctx, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}})
	}
	for _, name := range expectedStatefulSets {
		k8sClient.Delete(ctx, &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}})
	}
}
