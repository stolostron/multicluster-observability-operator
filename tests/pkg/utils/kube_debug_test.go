// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestIsErrorLine(t *testing.T) {
	testCases := []struct {
		name     string
		line     string
		expected bool
	}{
		{"empty string", "", false},
		{"routine klog info", "I0904 13:16:22.123456 reconciler.go:42] Successfully synced", false},
		{"routine controller log", "2026-09-09T10:00:00Z INFO Reconciling MultiClusterObservability", false},
		{"klog error header", "E0904 13:16:22.123456 client.go:100] Connection refused", true},
		{"klog warning header", "W0904 13:16:22.123456 client.go:100] Retrying request", true},
		{"klog fatal header", "F0904 13:16:22.123456 main.go:20] Unable to start server", true},
		{"keyword error", "failed to list resources: error contacting apiserver", true},
		{"keyword failed", "Deployment local-cluster/mcoa failed to progress", true},
		{"keyword timeout", "Context deadline exceeded (timeout waiting for pod)", true},
		{"keyword panic", "panic: runtime error: invalid memory address or nil pointer dereference", true},
		{"keyword fatal", "FATAL: shutdown initiated", true},
		{"keyword exception", "Unhandled exception in collector thread", true},
		{"word starting with W not klog", "Waiting for endpoint to be available...", false},
		{"word starting with E not klog", "Every reconciliation loop took 10ms", false},
		{"word starting with F not klog", "Finished sync of all resources", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := isErrorLine(tc.line)
			if actual != tc.expected {
				t.Errorf("isErrorLine(%q) = %v; want %v", tc.line, actual, tc.expected)
			}
		})
	}
}

func TestCleanUnstructuredForLogging(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "observability.open-cluster-management.io/v1beta2",
			"kind":       "MultiClusterObservability",
			"metadata": map[string]any{
				"name": "observability",
				"managedFields": []any{
					map[string]any{"manager": "kubectl", "operation": "Update"},
					map[string]any{"manager": "mco-operator", "operation": "Apply"},
				},
				"annotations": map[string]any{
					"kubectl.kubernetes.io/last-applied-configuration": "{\"apiVersion\":\"...\"}",
					"mco-keep-annotation":                              "important-value",
				},
			},
			"spec": map[string]any{
				"storageConfig": map[string]any{
					"metricObjectStorage": map[string]any{"name": "thanos-storage"},
				},
			},
		},
	}

	cleanUnstructuredForLogging(obj)

	// Verify managedFields are stripped
	if obj.GetManagedFields() != nil {
		t.Errorf("expected managedFields to be stripped, but found: %+v", obj.GetManagedFields())
	}
	metadata, _, _ := unstructured.NestedMap(obj.Object, "metadata")
	if _, ok := metadata["managedFields"]; ok {
		t.Errorf("expected managedFields key to be completely removed from metadata map, but found: %+v", metadata["managedFields"])
	}

	// Verify kubectl last-applied is stripped
	annotations := obj.GetAnnotations()
	if _, found := annotations["kubectl.kubernetes.io/last-applied-configuration"]; found {
		t.Errorf("expected kubectl.kubernetes.io/last-applied-configuration to be removed")
	}

	// Verify other annotations are preserved
	if val, ok := annotations["mco-keep-annotation"]; !ok || val != "important-value" {
		t.Errorf("expected mco-keep-annotation to be preserved, got: %v", val)
	}

	// Verify spec is untouched
	storageConfig, found, _ := unstructured.NestedMap(obj.Object, "spec", "storageConfig")
	if !found || storageConfig == nil {
		t.Errorf("expected spec.storageConfig to remain intact")
	}
}

func TestCleanUnstructuredForLogging_NilSafe(t *testing.T) {
	// Must not panic on nil
	cleanUnstructuredForLogging(nil)
}

func TestFormatPodsStatuses(t *testing.T) {
	now := time.Now()
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "pod-running",
				CreationTimestamp: metav1.NewTime(now.Add(-10 * time.Minute)),
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "app", Ready: true, RestartCount: 0},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "pod-crashloop",
				CreationTimestamp: metav1.NewTime(now.Add(-2 * time.Hour)),
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodFailed,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "app", Ready: false, RestartCount: 15},
				},
			},
		},
	}

	out := formatPodsStatuses(pods)

	if !strings.Contains(out, "NAME") || !strings.Contains(out, "STATUS") || !strings.Contains(out, "RESTARTS") {
		t.Errorf("formatPodsStatuses header missing expected columns, got:\n%s", out)
	}
	if !strings.Contains(out, "pod-running") || !strings.Contains(out, "Running") {
		t.Errorf("formatPodsStatuses missing running pod info, got:\n%s", out)
	}
	if !strings.Contains(out, "pod-crashloop") || !strings.Contains(out, "Failed") || !strings.Contains(out, "15") {
		t.Errorf("formatPodsStatuses missing crashlooping pod info, got:\n%s", out)
	}
}

func TestFormatDeploymentsStatuses(t *testing.T) {
	one := int32(1)
	three := int32(3)
	now := time.Now()

	client := kubefake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "dep-healthy",
				Namespace:         "open-cluster-management-observability",
				CreationTimestamp: metav1.NewTime(now.Add(-30 * time.Minute)),
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &one,
			},
			Status: appsv1.DeploymentStatus{
				ReadyReplicas:     1,
				UpdatedReplicas:   1,
				AvailableReplicas: 1,
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "dep-degraded",
				Namespace:         "open-cluster-management-observability",
				CreationTimestamp: metav1.NewTime(now.Add(-15 * time.Minute)),
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &three,
			},
			Status: appsv1.DeploymentStatus{
				ReadyReplicas:     0,
				UpdatedReplicas:   3,
				AvailableReplicas: 0,
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "dep-nil-replicas",
				Namespace:         "open-cluster-management-observability",
				CreationTimestamp: metav1.NewTime(now.Add(-5 * time.Minute)),
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: nil, // Must safely default to 1 without panic
			},
			Status: appsv1.DeploymentStatus{
				ReadyReplicas:     1,
				UpdatedReplicas:   1,
				AvailableReplicas: 1,
			},
		},
	)

	out, err := formatDeploymentsStatuses(client, "open-cluster-management-observability")
	if err != nil {
		t.Fatalf("unexpected error formatting deployments: %v", err)
	}

	if !strings.Contains(out, "dep-healthy") || !strings.Contains(out, "1/1") {
		t.Errorf("expected dep-healthy 1/1, got:\n%s", out)
	}
	if !strings.Contains(out, "dep-degraded") || !strings.Contains(out, "0/3") {
		t.Errorf("expected dep-degraded 0/3, got:\n%s", out)
	}
	if !strings.Contains(out, "dep-nil-replicas") || !strings.Contains(out, "1/1") {
		t.Errorf("expected dep-nil-replicas safely formatted as 1/1, got:\n%s", out)
	}
}

func TestFormatStatefulSetsStatuses(t *testing.T) {
	two := int32(2)
	now := time.Now()

	client := kubefake.NewSimpleClientset(
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "thanos-receive-default",
				Namespace:         "open-cluster-management-observability",
				CreationTimestamp: metav1.NewTime(now.Add(-40 * time.Minute)),
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: &two,
			},
			Status: appsv1.StatefulSetStatus{
				ReadyReplicas:   2,
				UpdatedReplicas: 2,
			},
		},
	)

	out, err := formatStatefulSetsStatuses(client, "open-cluster-management-observability")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "thanos-receive-default") || !strings.Contains(out, "2/2") {
		t.Errorf("expected thanos-receive-default 2/2, got:\n%s", out)
	}
}

func TestFormatDaemonSetsStatuses(t *testing.T) {
	now := time.Now()

	client := kubefake.NewSimpleClientset(
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "node-exporter",
				Namespace:         "open-cluster-management-observability",
				CreationTimestamp: metav1.NewTime(now.Add(-50 * time.Minute)),
			},
			Status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: 5,
				CurrentNumberScheduled: 5,
				NumberReady:            5,
			},
		},
	)

	out, err := formatDaemonSetsStatuses(client, "open-cluster-management-observability")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "node-exporter") {
		t.Fatalf("expected node-exporter row, got:\n%s", out)
	}
	// DESIRED, CURRENT, READY columns must all report 5.
	if !regexp.MustCompile(`node-exporter\s+5\s+5\s+5\s`).MatchString(out) {
		t.Errorf("expected node-exporter 5/5/5 counts, got:\n%s", out)
	}
}

// Lightweight mock for dynamic.Interface to avoid unvendored external packages
type mockResourceClient struct {
	dynamic.ResourceInterface
	items []unstructured.Unstructured
}

func (m *mockResourceClient) Namespace(ns string) dynamic.ResourceInterface {
	return m
}

func (m *mockResourceClient) List(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return &unstructured.UnstructuredList{Items: m.items}, nil
}

func (m *mockResourceClient) Get(ctx context.Context, name string, opts metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
	for _, item := range m.items {
		if item.GetName() == name {
			return &item, nil
		}
	}
	return nil, fmt.Errorf("not found: %s", name)
}

type mockDynamicClient struct {
	dynamic.Interface
	itemsByGVR map[schema.GroupVersionResource][]unstructured.Unstructured
}

func (m *mockDynamicClient) Resource(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return &mockResourceClient{items: m.itemsByGVR[resource]}
}

func TestLogManagedClusterAddOns(t *testing.T) {
	gvr := NewMCOManagedClusterAddonsGVR()
	now := metav1.Now()

	mcaHealthy := unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "addon.open-cluster-management.io/v1beta1",
			"kind":       "ManagedClusterAddOn",
			"metadata": map[string]any{
				"name":      "multicluster-observability-addon",
				"namespace": "cluster1",
			},
			"status": map[string]any{
				"conditions": []any{
					map[string]any{
						"type":    "Available",
						"status":  "True",
						"reason":  "AddonAvailable",
						"message": "Addon is ready",
					},
					map[string]any{
						"type":    "Degraded",
						"status":  "False",
						"reason":  "AddonNotDegraded",
						"message": "",
					},
				},
			},
		},
	}

	mcaTerminating := unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "addon.open-cluster-management.io/v1beta1",
			"kind":       "ManagedClusterAddOn",
			"metadata": map[string]any{
				"name":              "multicluster-observability-addon",
				"namespace":         "cluster2",
				"deletionTimestamp": now.Format(time.RFC3339),
				"finalizers": []any{
					"open-cluster-management.io/manifest-work-cleanup",
				},
			},
			"status": map[string]any{
				"conditions": []any{
					map[string]any{
						"type":    "Available",
						"status":  "True",
						"reason":  "AddonAvailable",
						"message": "Addon is shutting down",
					},
					map[string]any{
						"type":    "Degraded",
						"status":  "False",
						"reason":  "",
						"message": "",
					},
				},
			},
		},
	}

	mcaOther := unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "addon.open-cluster-management.io/v1beta1",
			"kind":       "ManagedClusterAddOn",
			"metadata": map[string]any{
				"name":      "work-manager",
				"namespace": "cluster1",
			},
		},
	}

	client := &mockDynamicClient{
		itemsByGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
			gvr: {mcaHealthy, mcaTerminating, mcaOther},
		},
	}

	// Verify executing LogManagedClusterAddOns runs cleanly without error or panic
	LogManagedClusterAddOns(client)
}

func TestPrintManifestWorks(t *testing.T) {
	gvr := NewOCMManifestworksGVR()
	now := metav1.Now()

	mwTerminating := unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]any{
				"name":              "addon-multicluster-observability-addon-deploy-0",
				"namespace":         "cluster1",
				"deletionTimestamp": now.Format(time.RFC3339),
				"finalizers": []any{
					"cluster.open-cluster-management.io/manifest-work-cleanup",
				},
			},
			"status": map[string]any{
				"conditions": []any{
					map[string]any{
						"type":    "Applied",
						"status":  "True",
						"reason":  "AppliedManifestWorkComplete",
						"message": "Apply manifest work complete",
					},
					map[string]any{
						"type":    "Available",
						"status":  "True",
						"reason":  "ResourceAvailable",
						"message": "All resources available",
					},
					map[string]any{
						"type":    "Degraded",
						"status":  "False",
						"reason":  "",
						"message": "",
					},
				},
			},
		},
	}

	mwLegacy := unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]any{
				"name":      "cluster1-observability",
				"namespace": "cluster1",
			},
			"status": map[string]any{
				"conditions": []any{
					map[string]any{
						"type":    "Applied",
						"status":  "False",
						"reason":  "AppliedManifestWorkFailed",
						"message": "failed to apply manifest",
					},
					map[string]any{
						"type":    "Available",
						"status":  "False",
						"reason":  "ResourceNotAvailable",
						"message": "not all resources available",
					},
				},
				"resourceStatus": map[string]any{
					"manifests": []any{
						map[string]any{
							"resourceMeta": map[string]any{
								"kind":      "Deployment",
								"name":      "endpoint-observability-operator",
								"namespace": "open-cluster-management-addon-observability",
							},
							"conditions": []any{
								map[string]any{
									"type":    "Applied",
									"status":  "False",
									"reason":  "ApplyFailed",
									"message": "ImagePullBackOff: failed to pull image",
								},
							},
						},
					},
				},
			},
		},
	}

	mwOther := unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]any{
				"name":      "klusterlet-crds",
				"namespace": "cluster1",
			},
		},
	}

	client := &mockDynamicClient{
		itemsByGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
			gvr: {mwTerminating, mwLegacy, mwOther},
		},
	}

	// Verify executing printManifestWorks runs cleanly without error or panic
	printManifestWorks(client)
}

func TestLogClusterManagementAddOn(t *testing.T) {
	gvr := NewMCOClusterManagementAddonsGVR()

	cmao := unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "addon.open-cluster-management.io/v1beta1",
			"kind":       "ClusterManagementAddOn",
			"metadata": map[string]any{
				"name": "multicluster-observability-addon",
			},
			"spec": map[string]any{
				"installStrategy": map[string]any{
					"type": "Manual",
				},
			},
			"status": map[string]any{
				"conditions": []any{
					map[string]any{
						"type":    "Progressing",
						"status":  "False",
						"reason":  "AddonInstalled",
						"message": "Addon is installed",
					},
				},
			},
		},
	}

	client := &mockDynamicClient{
		itemsByGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
			gvr: {cmao},
		},
	}

	LogClusterManagementAddOn(client)
}

func TestGetDiscoveredManagedClusterNames(t *testing.T) {
	gvr := NewOCMManagedClustersGVR()
	c1 := unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name": "local-cluster",
			},
		},
	}
	c2 := unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name": "spoke1",
			},
		},
	}

	client := &mockDynamicClient{
		itemsByGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
			gvr: {c1, c2},
		},
	}

	names, err := GetDiscoveredManagedClusterNames(context.Background(), client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names) != 2 || names[0] != "local-cluster" || names[1] != "spoke1" {
		t.Errorf("unexpected discovered names: %v", names)
	}
}

func TestResolveSpokeClientsFromHub(t *testing.T) {
	validKubeconfig := `apiVersion: v1
clusters:
- cluster:
    server: https://127.0.0.1:6443
  name: spoke
contexts:
- context:
    cluster: spoke
    user: admin
  name: spoke
current-context: spoke
kind: Config
users:
- name: admin
  user:
    token: dummy
`

	t.Run("resolves via Hive ClusterDeployment", func(t *testing.T) {
		cdGVR := NewHiveClusterDeploymentGVR()
		cd := unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "hive.openshift.io/v1",
				"kind":       "ClusterDeployment",
				"metadata": map[string]any{
					"name":      "spoke-hive",
					"namespace": "spoke-hive",
				},
				"spec": map[string]any{
					"clusterMetadata": map[string]any{
						"adminKubeconfigSecretRef": map[string]any{
							"name": "custom-hive-secret",
						},
					},
				},
			},
		}

		dynClient := &mockDynamicClient{
			itemsByGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
				cdGVR: {cd},
			},
		}

		kubeClient := kubefake.NewSimpleClientset(
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "custom-hive-secret",
					Namespace: "spoke-hive",
				},
				Data: map[string][]byte{
					"kubeconfig": []byte(validKubeconfig),
				},
			},
		)

		spokeKube, spokeDyn, err := resolveSpokeClientsFromHub(context.Background(), kubeClient, dynClient, "spoke-hive")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if spokeKube == nil || spokeDyn == nil {
			t.Errorf("expected non-nil spoke clients, got spokeKube=%v, spokeDyn=%v", spokeKube, spokeDyn)
		}
	})

	t.Run("resolves via standard admin kubeconfig naming convention", func(t *testing.T) {
		dynClient := &mockDynamicClient{
			itemsByGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{},
		}

		kubeClient := kubefake.NewSimpleClientset(
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "spoke-std-admin-kubeconfig",
					Namespace: "spoke-std",
				},
				Data: map[string][]byte{
					"kubeconfig": []byte(validKubeconfig),
				},
			},
		)

		spokeKube, spokeDyn, err := resolveSpokeClientsFromHub(context.Background(), kubeClient, dynClient, "spoke-std")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if spokeKube == nil || spokeDyn == nil {
			t.Errorf("expected non-nil spoke clients, got spokeKube=%v, spokeDyn=%v", spokeKube, spokeDyn)
		}
	})

	t.Run("resolves via hive secret-type label", func(t *testing.T) {
		dynClient := &mockDynamicClient{
			itemsByGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{},
		}

		kubeClient := kubefake.NewSimpleClientset(
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-auto-generated-name",
					Namespace: "spoke-labeled",
					Labels: map[string]string{
						"hive.openshift.io/secret-type": "kubeconfig",
					},
				},
				Data: map[string][]byte{
					"kubeconfig": []byte(validKubeconfig),
				},
			},
		)

		spokeKube, spokeDyn, err := resolveSpokeClientsFromHub(context.Background(), kubeClient, dynClient, "spoke-labeled")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if spokeKube == nil || spokeDyn == nil {
			t.Errorf("expected non-nil spoke clients, got spokeKube=%v, spokeDyn=%v", spokeKube, spokeDyn)
		}
	})

	t.Run("returns error when no admin kubeconfig secret found", func(t *testing.T) {
		dynClient := &mockDynamicClient{
			itemsByGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{},
		}
		kubeClient := kubefake.NewSimpleClientset()

		_, _, err := resolveSpokeClientsFromHub(context.Background(), kubeClient, dynClient, "unknown-cluster")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "no admin kubeconfig secret found") {
			t.Errorf("expected 'no admin kubeconfig secret found' error, got: %v", err)
		}
	})
}

func TestIsObservabilityPodInSharedNamespace(t *testing.T) {
	testCases := []struct {
		podName  string
		expected bool
	}{
		{"prom-agent-platform-metrics-collector-0", true},
		{"prometheus-k8s-0", true},
		{"endpoint-monitoring-operator-6486c75d7c-mldpd", true},
		{"observability-monitoring-cleanup-1234", true},
		{"observability-addon-abcde", true},
		{"alertmanager-alertmanager-0", true},
		{"metrics-collector-deployment-123", true},
		{"uwl-metrics-collector-456", true},
		{"hypershift-addon-agent-6c94cf7f58-6s9qz", false},
		{"cluster-proxy-proxy-agent-bbc99f777-gpb28", false},
		{"klusterlet-addon-workmgr-68f844fbfd-qknkh", false},
		{"managed-serviceaccount-addon-agent-8699975dd-bcgl2", false},
	}

	for _, tc := range testCases {
		actual := isObservabilityPodInSharedNamespace(tc.podName)
		if actual != tc.expected {
			t.Errorf("isObservabilityPodInSharedNamespace(%q) = %v; want %v", tc.podName, actual, tc.expected)
		}
	}
}

func TestIsPodAncientFailure(t *testing.T) {
	now := time.Now()

	t.Run("fresh pod created 10m ago", func(t *testing.T) {
		pod := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "fresh-pod",
				CreationTimestamp: metav1.NewTime(now.Add(-10 * time.Minute)),
			},
		}
		if isPodAncientFailure(pod, 4*time.Hour) {
			t.Errorf("expected fresh pod not to be ancient")
		}
	})

	t.Run("ancient pod created 24h ago with conditions 24h ago", func(t *testing.T) {
		pod := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "ancient-pod",
				CreationTimestamp: metav1.NewTime(now.Add(-24 * time.Hour)),
			},
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{
					{
						Type:               corev1.PodReady,
						Status:             corev1.ConditionFalse,
						LastTransitionTime: metav1.NewTime(now.Add(-20 * time.Hour)),
					},
				},
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name: "container-1",
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								FinishedAt: metav1.NewTime(now.Add(-20 * time.Hour)),
							},
						},
					},
				},
			},
		}
		if !isPodAncientFailure(pod, 4*time.Hour) {
			t.Errorf("expected ancient pod to be ancient")
		}
	})

	t.Run("old pod created 10h ago with condition transitioned 30m ago", func(t *testing.T) {
		pod := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "recently-failed-old-pod",
				CreationTimestamp: metav1.NewTime(now.Add(-10 * time.Hour)),
			},
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{
					{
						Type:               corev1.PodReady,
						Status:             corev1.ConditionFalse,
						LastTransitionTime: metav1.NewTime(now.Add(-30 * time.Minute)),
					},
				},
			},
		}
		if isPodAncientFailure(pod, 4*time.Hour) {
			t.Errorf("expected recently transitioned pod not to be ancient")
		}
	})

	t.Run("pod with zero creationTimestamp is not ancient", func(t *testing.T) {
		pod := corev1.Pod{}
		if isPodAncientFailure(pod, 4*time.Hour) {
			t.Errorf("expected pod with zero timestamp not to be ancient")
		}
	})
}

func TestFormatNodesStatuses(t *testing.T) {
	t.Run("empty nodes", func(t *testing.T) {
		client := kubefake.NewSimpleClientset()
		out, err := formatNodesStatuses(client)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "No nodes found") {
			t.Errorf("expected 'No nodes found', got: %s", out)
		}
	})

	t.Run("healthy node", func(t *testing.T) {
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "worker-1",
				CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Hour)),
				Labels: map[string]string{
					"node-role.kubernetes.io/worker": "",
				},
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
					{
						Type:   corev1.NodeDiskPressure,
						Status: corev1.ConditionFalse,
					},
				},
			},
		}
		client := kubefake.NewSimpleClientset(node)
		out, err := formatNodesStatuses(client)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "worker-1") || !strings.Contains(out, "Ready") || !strings.Contains(out, "None") {
			t.Errorf("expected healthy node summary, got:\n%s", out)
		}
		if strings.Contains(out, "Node Issues / Taints:") {
			t.Errorf("did not expect issues block for healthy node, got:\n%s", out)
		}
	})

	t.Run("node with disk pressure and taints", func(t *testing.T) {
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "master-1",
				CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * 24 * time.Hour)),
				Labels: map[string]string{
					"node-role.kubernetes.io/control-plane": "",
					"node-role.kubernetes.io/master":        "",
				},
			},
			Spec: corev1.NodeSpec{
				Taints: []corev1.Taint{
					{
						Key:    "node.kubernetes.io/disk-pressure",
						Value:  "",
						Effect: corev1.TaintEffectNoSchedule,
					},
				},
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:    corev1.NodeReady,
						Status:  corev1.ConditionTrue,
						Reason:  "KubeletReady",
						Message: "kubelet is posting ready status",
					},
					{
						Type:    corev1.NodeDiskPressure,
						Status:  corev1.ConditionTrue,
						Reason:  "KubeletHasDiskPressure",
						Message: "The node had condition: [DiskPressure]",
					},
				},
			},
		}
		client := kubefake.NewSimpleClientset(node)
		out, err := formatNodesStatuses(client)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "DiskPressure") {
			t.Errorf("expected DiskPressure in table, got:\n%s", out)
		}
		if !strings.Contains(out, "Taint:node.kubernetes.io/disk-pressure") {
			t.Errorf("expected Taint in table, got:\n%s", out)
		}
		if !strings.Contains(out, "Node Issues / Taints:") {
			t.Errorf("expected issues block, got:\n%s", out)
		}
		if !strings.Contains(out, "KubeletHasDiskPressure") {
			t.Errorf("expected KubeletHasDiskPressure reason in details, got:\n%s", out)
		}
	})
}

func TestLogPodLogs_EarlyReturn(t *testing.T) {
	client := kubefake.NewSimpleClientset()

	// Should not panic or make requests for pending pod
	pendingPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pending-pod", Namespace: "default"},
		Status:     corev1.PodStatus{Phase: corev1.PodPending},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "c1"}},
		},
	}
	LogPodLogs(client, "default", pendingPod)

	// Should not panic or make requests for evicted pod
	evictedPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "evicted-pod", Namespace: "default"},
		Status:     corev1.PodStatus{Phase: corev1.PodFailed, Reason: "Evicted"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "c1"}},
		},
	}
	LogPodLogs(client, "default", evictedPod)
}
