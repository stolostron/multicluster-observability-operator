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
		{"timestamped klog info header", "2026-09-09T13:16:22.123456789Z I0904 13:16:22.123456 client.go:100] Successfully synced", false},
		{"timestamped klog error header", "2026-09-09T13:16:22.123456789Z E0904 13:16:22.123456 client.go:100] Connection refused", true},
		{"timestamped klog warning header", "2026-09-09T13:16:22.123456789Z W0904 13:16:22.123456 client.go:100] Retrying request", true},
		{"timestamped klog fatal header", "2026-09-09T13:16:22.123456789Z F0904 13:16:22.123456 main.go:20] Unable to start server", true},
		{
			"timestamped klog info containing 'failed' condition name",
			`2026-09-09T11:49:27.999120911Z I0909 11:49:27.999008 1 status_controller.go:145] "Updating MCO status conditions" logger="controllers.MultiClusterObservabilityStatus" Request.Namespace="" Request.Name="observability" changes=["Added: Failed (Status: True, Reason: DeploymentNotReady)"]`,
			false,
		},
		{"timestamped klog info containing 'error' in message", "2026-09-09T11:49:27.999120911Z I0909 11:49:27.999008 1 controller.go:100] Reconcile finished with error handled", false},
		{"timestamped zap info containing 'failed'", "2026-09-09T10:00:00Z INFO Controller checking failed jobs", false},
		{"timestamped zap debug containing 'error'", "2026-09-09T10:00:00Z DEBUG Skipping ignored error on cleanup", false},
		{"klog info containing panic is still caught", "I0904 13:16:22.123456 reconciler.go:42] Caught panic during handler", true},
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

	// Verify filtering in shared agent namespace
	sharedClient := kubefake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "endpoint-monitoring-operator",
				Namespace: MCO_AGENT_ADDON_NAMESPACE,
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "hypershift-addon-agent",
				Namespace: MCO_AGENT_ADDON_NAMESPACE,
			},
		},
	)
	sharedOut, err := formatDeploymentsStatuses(sharedClient, MCO_AGENT_ADDON_NAMESPACE)
	if err != nil {
		t.Fatalf("unexpected error formatting shared deployments: %v", err)
	}
	if !strings.Contains(sharedOut, "endpoint-monitoring-operator") {
		t.Errorf("expected endpoint-monitoring-operator in shared namespace, got:\n%s", sharedOut)
	}
	if strings.Contains(sharedOut, "hypershift-addon-agent") {
		t.Errorf("expected hypershift-addon-agent to be filtered out in shared namespace, got:\n%s", sharedOut)
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

	// Verify filtering in shared agent namespace
	sharedClient := kubefake.NewSimpleClientset(
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "prom-agent-platform-metrics-collector",
				Namespace: MCO_AGENT_ADDON_NAMESPACE,
			},
		},
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "unrelated-addon-statefulset",
				Namespace: MCO_AGENT_ADDON_NAMESPACE,
			},
		},
	)
	sharedOut, err := formatStatefulSetsStatuses(sharedClient, MCO_AGENT_ADDON_NAMESPACE)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(sharedOut, "prom-agent-platform-metrics-collector") {
		t.Errorf("expected prom-agent-platform-metrics-collector in shared namespace, got:\n%s", sharedOut)
	}
	if strings.Contains(sharedOut, "unrelated-addon-statefulset") {
		t.Errorf("expected unrelated-addon-statefulset to be filtered out, got:\n%s", sharedOut)
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

	// Verify filtering in shared agent namespace
	sharedClient := kubefake.NewSimpleClientset(
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "node-exporter",
				Namespace: MCO_AGENT_ADDON_NAMESPACE,
			},
		},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "unrelated-daemonset",
				Namespace: MCO_AGENT_ADDON_NAMESPACE,
			},
		},
	)
	sharedOut, err := formatDaemonSetsStatuses(sharedClient, MCO_AGENT_ADDON_NAMESPACE)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(sharedOut, "node-exporter") {
		t.Errorf("expected node-exporter in shared namespace, got:\n%s", sharedOut)
	}
	if strings.Contains(sharedOut, "unrelated-daemonset") {
		t.Errorf("expected unrelated-daemonset to be filtered out, got:\n%s", sharedOut)
	}
}

func TestFormatObjectEvents(t *testing.T) {
	// 1. Empty events
	if out := formatObjectEvents("Pod", nil); out != "" {
		t.Errorf("expected empty string for empty events, got %q", out)
	}

	// 2. Sorting chronologically
	t0 := time.Now().Add(-10 * time.Minute)
	t1 := time.Now().Add(-5 * time.Minute)
	t2 := time.Now().Add(-1 * time.Minute)
	events := []corev1.Event{
		{
			Reason:        "FailedScheduling",
			Message:       "0/1 nodes available",
			Count:         2,
			LastTimestamp: metav1.NewTime(t1),
		},
		{
			Reason:        "Scheduled",
			Message:       "Assigned to node-1",
			Count:         1,
			LastTimestamp: metav1.NewTime(t0),
		},
		{
			Reason:        "Evicted",
			Message:       "The node was low on resource: ephemeral-storage",
			Count:         1,
			LastTimestamp: metav1.NewTime(t2),
		},
	}

	out := formatObjectEvents("Pod", events)
	schedIdx := strings.Index(out, "Scheduled")
	failedIdx := strings.Index(out, "FailedScheduling")
	evictedIdx := strings.Index(out, "Evicted")

	if schedIdx == -1 || failedIdx == -1 || evictedIdx == -1 {
		t.Fatalf("expected all events to be present, got:\n%s", out)
	}
	if !(schedIdx < failedIdx && failedIdx < evictedIdx) {
		t.Errorf("expected events to be chronologically ordered (Scheduled < FailedScheduling < Evicted), got indices %d, %d, %d", schedIdx, failedIdx, evictedIdx)
	}

	// 3. Capping at 25 events with omitted summary
	var manyEvents []corev1.Event
	for i := 0; i < 35; i++ {
		manyEvents = append(manyEvents, corev1.Event{
			Reason:        fmt.Sprintf("Reason%02d", i),
			Message:       fmt.Sprintf("Message %d", i),
			Count:         1,
			LastTimestamp: metav1.NewTime(t0.Add(time.Duration(i) * time.Minute)),
		})
	}
	cappedOut := formatObjectEvents("Pod", manyEvents)
	if !strings.Contains(cappedOut, "(+ 10 older events omitted for brevity)") {
		t.Errorf("expected '(+ 10 older events omitted for brevity)' in output, got:\n%s", cappedOut)
	}
	if strings.Contains(cappedOut, "Reason00") {
		t.Errorf("expected earliest event Reason00 to be omitted, but it was found in:\n%s", cappedOut)
	}
	if !strings.Contains(cappedOut, "Reason34") {
		t.Errorf("expected latest event Reason34 to be present in:\n%s", cappedOut)
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
		var foundRow bool
		for _, line := range strings.Split(out, "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 5 && fields[0] == "worker-1" {
				foundRow = true
				if fields[1] != "Ready" {
					t.Errorf("expected worker-1 status to be Ready, got: %s", fields[1])
				}
				if fields[len(fields)-1] != "None" {
					t.Errorf("expected worker-1 issues to be None, got: %s", fields[len(fields)-1])
				}
			}
		}
		if !foundRow {
			t.Errorf("expected worker-1 row in output, got:\n%s", out)
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
		if !strings.Contains(out, "Scheduling taint") {
			t.Errorf("expected Scheduling taint in details, got:\n%s", out)
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

func TestIsObservabilityWorkloadInSharedNamespace(t *testing.T) {
	testCases := []struct {
		name     string
		expected bool
	}{
		{"endpoint-monitoring-operator", true},
		{"endpoint-monitoring-operator-6486c75d7c-mldpd", true},
		{"observability-monitoring-cleanup", true},
		{"observability-monitoring-cleanup-1234", true},
		{"observability-addon", true},
		{"observability-addon-abcde", true},
		{"prom-agent-platform-metrics-collector", true},
		{"prom-agent-platform-metrics-collector-0", true},
		{"prometheus-k8s", true},
		{"prometheus-k8s-0", true},
		{"alertmanager-canary", true},
		{"alertmanager-alertmanager-0", true},
		{"metrics-collector", true},
		{"metrics-collector-deployment-123", true},
		{"uwl-metrics-collector", true},
		{"uwl-metrics-collector-456", true},
		{"cluster-proxy-proxy-agent", false},
		{"cluster-proxy-proxy-agent-bbc99f777-gpb28", false},
		{"hypershift-addon-agent", false},
		{"hypershift-addon-agent-6c94cf7f58-6s9qz", false},
		{"klusterlet-addon-workmgr", false},
		{"klusterlet-addon-workmgr-68f844fbfd-qknkh", false},
		{"managed-serviceaccount-addon-agent", false},
		{"managed-serviceaccount-addon-agent-8699975dd-bcgl2", false},
	}

	for _, tc := range testCases {
		actual := isObservabilityWorkloadInSharedNamespace(tc.name)
		if actual != tc.expected {
			t.Errorf("isObservabilityWorkloadInSharedNamespace(%q) = %v; want %v", tc.name, actual, tc.expected)
		}
	}
}

func TestFormatSecretsStatuses(t *testing.T) {
	now := time.Now()
	secrets := []corev1.Secret{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "thanos-object-storage",
				CreationTimestamp: metav1.NewTime(now.Add(-10 * time.Minute)),
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{"thanos.yaml": []byte("config")},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "observability-server-certs",
				CreationTimestamp: metav1.NewTime(now.Add(-2 * time.Hour)),
			},
			Type: corev1.SecretTypeTLS,
			Data: map[string][]byte{"tls.crt": []byte("cert"), "tls.key": []byte("key")},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "multiclusterhub-operator-pull-secret",
				CreationTimestamp: metav1.NewTime(now.Add(-5 * time.Hour)),
			},
			Type: corev1.SecretTypeDockerConfigJson,
			Data: map[string][]byte{corev1.DockerConfigJsonKey: []byte("{}")},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "alertmanager-dockercfg-c9gxj",
				CreationTimestamp: metav1.NewTime(now.Add(-24 * time.Hour)),
			},
			Type: corev1.SecretTypeDockercfg,
			Data: map[string][]byte{corev1.DockerConfigKey: []byte("{}")},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "default-token-xyz12",
				CreationTimestamp: metav1.NewTime(now.Add(-48 * time.Hour)),
			},
			Type: corev1.SecretTypeServiceAccountToken,
			Data: map[string][]byte{"token": []byte("token")},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "thanos-dockercfg-98765",
				CreationTimestamp: metav1.NewTime(now.Add(-48 * time.Hour)),
			},
			Type: corev1.SecretTypeDockerConfigJson,
			Data: map[string][]byte{corev1.DockerConfigJsonKey: []byte("{}")},
		},
	}

	out := formatSecretsStatuses(secrets, "open-cluster-management-observability")

	// Must include real application secrets
	if !strings.Contains(out, "thanos-object-storage") {
		t.Errorf("expected thanos-object-storage in output, got:\n%s", out)
	}
	if !strings.Contains(out, "observability-server-certs") {
		t.Errorf("expected observability-server-certs in output, got:\n%s", out)
	}
	if !strings.Contains(out, "multiclusterhub-operator-pull-secret") {
		t.Errorf("expected multiclusterhub-operator-pull-secret in output, got:\n%s", out)
	}

	// Must omit internal service account secrets
	if strings.Contains(out, "alertmanager-dockercfg-c9gxj") {
		t.Errorf("expected alertmanager-dockercfg-c9gxj to be omitted, got:\n%s", out)
	}
	if strings.Contains(out, "default-token-xyz12") {
		t.Errorf("expected default-token-xyz12 to be omitted, got:\n%s", out)
	}
	if strings.Contains(out, "thanos-dockercfg-98765") {
		t.Errorf("expected thanos-dockercfg-98765 to be omitted, got:\n%s", out)
	}

	// Must contain the summary line
	expectedSummary := "(+ 3 service account dockercfg/token Secrets omitted for brevity)"
	if !strings.Contains(out, expectedSummary) {
		t.Errorf("expected summary %q in output, got:\n%s", expectedSummary, out)
	}
}

func TestCheckDeploymentsInNamespace_SharedNamespaceScoping(t *testing.T) {
	replicas := int32(1)
	unreadyProxyDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-proxy-proxy-agent",
			Namespace: MCO_AGENT_ADDON_NAMESPACE,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas:     0,
			AvailableReplicas: 0,
			UpdatedReplicas:   1,
		},
	}
	readyEndpointOperator := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "endpoint-monitoring-operator",
			Namespace: MCO_AGENT_ADDON_NAMESPACE,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas:     1,
			AvailableReplicas: 1,
			UpdatedReplicas:   1,
		},
	}

	client := kubefake.NewSimpleClientset(unreadyProxyDeployment, readyEndpointOperator)
	// Should not panic or dump unready non-observability deployments
	CheckDeploymentsInNamespace(client, MCO_AGENT_ADDON_NAMESPACE)
}

func TestFormatContainerLogs(t *testing.T) {
	now := time.Now()
	cutoff := now.Add(-6 * time.Minute)

	t.Run("empty logs", func(t *testing.T) {
		lines, msg := formatContainerLogs("", cutoff)
		if len(lines) != 0 {
			t.Errorf("expected 0 lines, got %d", len(lines))
		}
		if msg == "" {
			t.Error("expected non-empty message")
		}
	})

	t.Run("non-error lines omitted when errors present", func(t *testing.T) {
		t1 := now.Add(-3 * time.Minute).Format(time.RFC3339)
		t2 := now.Add(-2 * time.Minute).Format(time.RFC3339)
		t3 := now.Add(-1 * time.Minute).Format(time.RFC3339)
		raw := fmt.Sprintf("%s I0909 10:00:00.000000 1 main.go:10] starting server\n"+
			"%s E0909 10:01:00.000000 1 main.go:20] failed to connect to database\n"+
			"%s I0909 10:02:00.000000 1 main.go:30] retrying connection\n"+
			"%s E0909 10:03:00.000000 1 main.go:40] fatal error: connection refused\n",
			t1, t2, t2, t3)

		lines, msg := formatContainerLogs(raw, cutoff)
		if len(lines) != 3 { // 1 omitted notice + 2 errors
			t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
		}
		expectedNotice := "  (+ 2 non-error log lines omitted for brevity)"
		if lines[0] != expectedNotice {
			t.Errorf("expected notice %q, got %q", expectedNotice, lines[0])
		}
		if !strings.Contains(lines[1], "failed to connect") {
			t.Errorf("expected first error in lines[1], got %q", lines[1])
		}
		if !strings.Contains(lines[2], "connection refused") {
			t.Errorf("expected second error in lines[2], got %q", lines[2])
		}
		if !strings.Contains(msg, "2 error/warning lines") {
			t.Errorf("expected msg to mention 2 error/warning lines, got %q", msg)
		}
	})

	t.Run("capped error lines and non-error lines omitted", func(t *testing.T) {
		var rawBuilder strings.Builder
		for i := 0; i < 60; i++ {
			ts := now.Add(-time.Duration(60-i) * time.Second).Format(time.RFC3339)
			rawBuilder.WriteString(fmt.Sprintf("%s E0909 10:00:00.000000 1 main.go:10] error event %d\n", ts, i))
		}
		for i := 0; i < 10; i++ {
			ts := now.Add(-time.Duration(10-i) * time.Second).Format(time.RFC3339)
			rawBuilder.WriteString(fmt.Sprintf("%s I0909 10:00:00.000000 1 main.go:10] info event %d\n", ts, i))
		}

		lines, _ := formatContainerLogs(rawBuilder.String(), cutoff)
		if len(lines) != 51 { // 1 omitted notice + 50 capped errors
			t.Fatalf("expected 51 lines (1 notice + 50 errors), got %d", len(lines))
		}
		expectedNotice := "  (+ 10 older error lines and 10 non-error lines omitted for brevity)"
		if lines[0] != expectedNotice {
			t.Errorf("expected notice %q, got %q", expectedNotice, lines[0])
		}
	})

	t.Run("no errors with capped window lines", func(t *testing.T) {
		var rawBuilder strings.Builder
		for i := 0; i < 25; i++ {
			ts := now.Add(-time.Duration(25-i) * time.Second).Format(time.RFC3339)
			rawBuilder.WriteString(fmt.Sprintf("%s I0909 10:00:00.000000 1 main.go:10] routine line %d\n", ts, i))
		}

		lines, _ := formatContainerLogs(rawBuilder.String(), cutoff)
		if len(lines) != 21 { // 1 omitted notice + 20 capped window lines
			t.Fatalf("expected 21 lines, got %d", len(lines))
		}
		expectedNotice := "  (+ 5 older log lines omitted for brevity)"
		if lines[0] != expectedNotice {
			t.Errorf("expected notice %q, got %q", expectedNotice, lines[0])
		}
	})

	t.Run("no omitted notice when all lines fit", func(t *testing.T) {
		t1 := now.Add(-1 * time.Minute).Format(time.RFC3339)
		raw := fmt.Sprintf("%s E0909 10:00:00.000000 1 main.go:10] error line 1\n%s E0909 10:00:01.000000 1 main.go:11] error line 2\n", t1, t1)
		lines, _ := formatContainerLogs(raw, cutoff)
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines without notice, got %d: %v", len(lines), lines)
		}
		if strings.HasPrefix(lines[0], "  (+ ") {
			t.Errorf("did not expect omitted notice when all lines fit, got %q", lines[0])
		}
	})
}
