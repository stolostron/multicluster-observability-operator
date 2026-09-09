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
	"unicode/utf8"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
		{"logfmt info containing error in key/val", `caller=main.go:50 level=info msg="sync complete" error_count=0`, false},
		{"logfmt debug containing failed in key/val", `caller=main.go:50 level=debug msg="evaluation status" failed=false`, false},
		{"logfmt quoted info containing error", `caller=main.go:50 level="info" msg="handled error gracefully"`, false},
		{"logfmt quoted debug containing failed", `caller=main.go:50 level="debug" msg="failed condition handled"`, false},
		{"json info containing error in message", `{"level":"info","ts":"2026-09-09T10:00:00Z","msg":"recovered from error"}`, false},
		{"json debug containing failed in message", `{"level":"debug","ts":"2026-09-09T10:00:00Z","msg":"no failed jobs"}`, false},
		{
			"prom benign out of order samples warning",
			`ts=2026-09-09T13:12:00Z caller=scrape.go:1712 level=warn component="scrape manager" msg="Error on ingesting out-of-order samples" num_dropped=1`,
			false,
		},
		{"logfmt warn containing genuine error", `ts=2026-09-09T13:12:00Z level=warn component="remote-write" msg="failed to send batch"`, true},
		{"klog info containing panic is still caught", "I0904 13:16:22.123456 reconciler.go:42] Caught panic during handler", true},
		{
			"client-go benign inClusterConfig warning",
			"2026-09-09T13:48:39.485786389Z W0909 13:48:39.485663       1 client_config.go:682] Neither --kubeconfig nor --master was specified.  Using the inClusterConfig.  This might not work.",
			false,
		},
		{
			"controller-runtime benign authorization disabled warning",
			"2026-09-09T13:48:32.729619491Z W0909 13:48:32.729570       1 authorization.go:59] Authorization is disabled",
			false,
		},
		{
			"controller-runtime benign authentication disabled warning",
			"2026-09-09T13:48:32.729652612Z W0909 13:48:32.729618       1 authentication.go:52] Authentication is disabled",
			false,
		},
		{
			"controller blocking clusters info log is treated as info by isErrorLine (captured via tail)",
			`2026-09-09T14:45:10.630088826Z I0909 14:45:10.630051 1 multiclusterobservability_controller.go:1251] "Waiting for MCOA ManifestWorks and ManagedClusterAddOns to be deleted" logger="controller_multiclustermonitoring" Request.Namespace="" Request.Name="observability" blockingClusters=["local-cluster"]`,
			false,
		},
		{
			"controller waiting for mcoa cleanup is treated as info by isErrorLine (captured via tail)",
			`I0909 14:45:10.630051 1 multiclusterobservability_controller.go:1251] Waiting for MCOA teardown to complete`,
			false,
		},
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
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "pod-multicontainer-restarts",
				CreationTimestamp: metav1.NewTime(now.Add(-20 * time.Minute)),
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "c1", Ready: true, RestartCount: 2},
					{Name: "c2", Ready: true, RestartCount: 3},
				},
				InitContainerStatuses: []corev1.ContainerStatus{
					{Name: "init", Ready: true, RestartCount: 1},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "pod-ancient-succeeded",
				CreationTimestamp: metav1.NewTime(now.Add(-3 * time.Hour)),
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodSucceeded,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "pod-recent-succeeded",
				CreationTimestamp: metav1.NewTime(now.Add(-15 * time.Minute)),
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodSucceeded,
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
	// Verify multi-container restart count is summed (2 + 3 + 1 = 6)
	if !strings.Contains(out, "pod-multicontainer-restarts") || !strings.Contains(out, " 6 ") {
		t.Errorf("formatPodsStatuses missing summed restarts (expected 6), got:\n%s", out)
	}
	// Verify ancient succeeded pod is omitted and summarized
	if strings.Contains(out, "pod-ancient-succeeded") {
		t.Errorf("formatPodsStatuses should have omitted pod-ancient-succeeded, got:\n%s", out)
	}
	if !strings.Contains(out, "(+ 1 ancient Succeeded pods omitted for brevity)") {
		t.Errorf("formatPodsStatuses missing ancient succeeded summary line, got:\n%s", out)
	}
	// Verify recent succeeded pod is kept in table
	if !strings.Contains(out, "pod-recent-succeeded") {
		t.Errorf("formatPodsStatuses should keep recent succeeded pod in table, got:\n%s", out)
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

	// 4. Stale events (> 1h) filtering
	staleEvents := []corev1.Event{
		{
			Reason:        "AncientEvent",
			Message:       "Happened 2 hours ago",
			Count:         1,
			LastTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Hour)),
		},
	}
	if out := formatObjectEvents("Pod", staleEvents); out != "" {
		t.Errorf("expected empty string when all events are older than 1 hour, got:\n%s", out)
	}

	mixedEvents := []corev1.Event{
		{
			Reason:        "AncientEvent",
			Message:       "Happened 2 hours ago",
			Count:         1,
			LastTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Hour)),
		},
		{
			Reason:        "RecentEvent",
			Message:       "Happened 5 minutes ago",
			Count:         1,
			LastTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
		},
	}
	mixedOut := formatObjectEvents("Pod", mixedEvents)
	if strings.Contains(mixedOut, "AncientEvent") {
		t.Errorf("expected ancient event to be filtered out, got:\n%s", mixedOut)
	}
	if !strings.Contains(mixedOut, "RecentEvent") {
		t.Errorf("expected recent event to be present, got:\n%s", mixedOut)
	}
	if !strings.Contains(mixedOut, "(+ 1 older events omitted for brevity)") {
		t.Errorf("expected omitted note for 1 older event, got:\n%s", mixedOut)
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

	mcaMismatching := unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "addon.open-cluster-management.io/v1beta1",
			"kind":       "ManagedClusterAddOn",
			"metadata": map[string]any{
				"name":              "multicluster-observability-addon",
				"namespace":         "local-cluster",
				"deletionTimestamp": now.Format(time.RFC3339),
				"finalizers": []any{
					"addon.open-cluster-management.io/addon-pre-delete",
				},
			},
			"status": map[string]any{
				"conditions": []any{
					map[string]any{
						"type":    "Progressing",
						"status":  "True",
						"reason":  "Progressing",
						"message": "progressing... mca and work configs mismatch",
					},
					map[string]any{
						"type":    "ManifestApplied",
						"status":  "False",
						"reason":  "AddonManifestAppliedFailed",
						"message": "failed to apply the manifests of addon",
					},
					map[string]any{
						"type":    "Available",
						"status":  "Unknown",
						"reason":  "NoProbeResult",
						"message": "Probe results are not returned",
					},
					map[string]any{
						"type":    "HookManifestCompleted",
						"status":  "False",
						"reason":  "HookManifestIsNotCompleted",
						"message": "hook manifestWork addon-multicluster-observability-addon-pre-delete is not completed.",
					},
				},
				"configReferences": []any{
					map[string]any{
						"group":    "addon.open-cluster-management.io",
						"resource": "addondeploymentconfigs",
						"desiredConfig": map[string]any{
							"name":      "multicluster-observability-addon",
							"namespace": "open-cluster-management-observability",
							"specHash":  "dcce13790009ad6fae8113d6d4c63a0b4233408726095cf6199d60e76ef3090a",
						},
						"lastAppliedConfig": map[string]any{
							"name":      "multicluster-observability-addon",
							"namespace": "open-cluster-management-observability",
							"specHash":  "78c764d7a531cf7ed54a2fc134bc54ab91eb4771ddef3018c09fdcb88fd8ef58",
						},
					},
					map[string]any{
						"group":    "monitoring.rhobs",
						"resource": "scrapeconfigs",
						"desiredConfig": map[string]any{
							"name":      "platform-metrics",
							"namespace": "open-cluster-management-observability",
							"specHash":  "",
						},
						"lastAppliedConfig": map[string]any{
							"name":      "platform-metrics",
							"namespace": "open-cluster-management-observability",
							"specHash":  "0fe88872cc9af84e3079c3a6445851c0d34bb8f3a1f77cc8accaa4bbc48befc0",
						},
					},
					map[string]any{
						"group":    "monitoring.rhobs",
						"resource": "prometheusagents",
						"desiredConfig": map[string]any{
							"name":      "mcoa-default-platform-metrics-collector-global-default",
							"namespace": "open-cluster-management-observability",
							"specHash":  "f84fc6cbdb348fe97c05c9447fc91568ac9066f7922410951956ab751654cba2",
						},
						"lastAppliedConfig": map[string]any{
							"name":      "mcoa-default-platform-metrics-collector-global-default",
							"namespace": "open-cluster-management-observability",
							"specHash":  "f84fc6cbdb348fe97c05c9447fc91568ac9066f7922410951956ab751654cba2",
						},
					},
					map[string]any{
						"group":    "custom.io",
						"resource": "unnamedconfigs",
						"desiredConfig": map[string]any{
							"specHash": "abcdef1234567890",
						},
					},
				},
			},
		},
	}

	client := &mockDynamicClient{
		itemsByGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
			gvr: {mcaHealthy, mcaTerminating, mcaMismatching, mcaOther},
		},
	}

	// Verify executing LogManagedClusterAddOns runs cleanly without error or panic
	LogManagedClusterAddOns(client)

	// Verify detailed formatting of formatManagedClusterAddOns
	output := formatManagedClusterAddOns([]unstructured.Unstructured{mcaHealthy, mcaMismatching})
	if !strings.Contains(output, "CONFIGS") {
		t.Error("expected table header to include CONFIGS column")
	}
	if !strings.Contains(output, "[1/4 sync, 3 mismatch]") {
		t.Errorf("expected [1/4 sync, 3 mismatch] in output, got: %s", output)
	}
	if !strings.Contains(
		output,
		"ConfigReference mismatch: addondeploymentconfigs.addon.open-cluster-management.io open-cluster-management-observability/multicluster-observability-addon (desiredHash: dcce1379, appliedHash: 78c764d7)",
	) {
		t.Errorf("expected addondeploymentconfig mismatch detail in output, got: %s", output)
	}
	if !strings.Contains(output, "ConfigReference mismatch: scrapeconfigs.monitoring.rhobs open-cluster-management-observability/platform-metrics (desiredHash: <empty>, appliedHash: 0fe88872)") {
		t.Errorf("expected scrapeconfigs mismatch detail in output, got: %s", output)
	}
	if !strings.Contains(output, "ConfigReference mismatch: unnamedconfigs.custom.io <unnamed> (desiredHash: abcdef12, applied: <none>)") {
		t.Errorf("expected unnamed configReference mismatch detail in output, got: %s", output)
	}
	if !strings.Contains(output, "[ManifestApplied=False (AddonManifestAppliedFailed): failed to apply the manifests of addon]") {
		t.Errorf("expected ManifestApplied=False condition in output, got: %s", output)
	}
	if !strings.Contains(output, "[HookManifestCompleted=False (HookManifestIsNotCompleted): hook manifestWork addon-multicluster-observability-addon-pre-delete is not completed.]") {
		t.Errorf("expected HookManifestCompleted=False condition in output, got: %s", output)
	}
	if !strings.Contains(output, "[Available=Unknown (NoProbeResult): Probe results are not returned]") {
		t.Errorf("expected Available=Unknown condition in output, got: %s", output)
	}
}

func TestShortHash(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "empty", input: "", expected: "<empty>"},
		{name: "short", input: "abc", expected: "abc"},
		{name: "exact-8", input: "12345678", expected: "12345678"},
		{name: "long-sha256", input: "dcce13790009ad6fae8113d6d4c63a0b4233408726095cf6199d60e76ef3090a", expected: "dcce1379"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shortHash(tc.input)
			if got != tc.expected {
				t.Errorf("shortHash(%q) = %q, expected %q", tc.input, got, tc.expected)
			}
		})
	}
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

	out := formatManifestWorks([]unstructured.Unstructured{mwTerminating, mwLegacy, mwOther})
	if !strings.Contains(out, "addon-multicluster-observability-addon-deploy-0") {
		t.Errorf("expected addon-multicluster-observability-addon-deploy-0 in output, got:\n%s", out)
	}
	if !strings.Contains(out, "cluster1-observability") {
		t.Errorf("expected cluster1-observability in output, got:\n%s", out)
	}
	if strings.Contains(out, "klusterlet-crds") {
		t.Errorf("expected non-observability ManifestWork klusterlet-crds to be filtered out, got:\n%s", out)
	}
	if !strings.Contains(out, "Terminating with finalizers [cluster.open-cluster-management.io/manifest-work-cleanup]") {
		t.Errorf("expected terminating finalizer detail in output, got:\n%s", out)
	}
	if !strings.Contains(out, "AppliedManifestWorkFailed") {
		t.Errorf("expected AppliedManifestWorkFailed in output, got:\n%s", out)
	}
	if !strings.Contains(out, "ImagePullBackOff: failed to pull image") {
		t.Errorf("expected manifest-level ImagePullBackOff error in output, got:\n%s", out)
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

	t.Run("pod evicted 2h40m ago is ancient with 1h cutoff", func(t *testing.T) {
		pod := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "evicted-pod",
				CreationTimestamp: metav1.NewTime(now.Add(-5 * time.Hour)),
			},
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{
					{
						Type:               corev1.DisruptionTarget,
						Status:             corev1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now.Add(-160 * time.Minute)),
					},
				},
			},
		}
		if !isPodAncientFailure(pod, 1*time.Hour) {
			t.Errorf("expected pod evicted 2h40m ago to be ancient with 1h cutoff")
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

	t.Run("all lines fit within tail limit without omission", func(t *testing.T) {
		t1 := now.Add(-3 * time.Minute).Format(time.RFC3339)
		t2 := now.Add(-2 * time.Minute).Format(time.RFC3339)
		t3 := now.Add(-1 * time.Minute).Format(time.RFC3339)
		raw := fmt.Sprintf("%s I0909 10:00:00.000000 1 main.go:10] starting server\n"+
			"%s E0909 10:01:00.000000 1 main.go:20] failed to connect to database\n"+
			"%s I0909 10:02:00.000000 1 main.go:30] retrying connection\n"+
			"%s E0909 10:03:00.000000 1 main.go:40] fatal error: connection refused\n",
			t1, t2, t2, t3)

		lines, msg := formatContainerLogs(raw, cutoff)
		if len(lines) != 4 {
			t.Fatalf("expected 4 lines without omission, got %d: %v", len(lines), lines)
		}
		if !strings.Contains(lines[0], "starting server") {
			t.Errorf("expected first line in lines[0], got %q", lines[0])
		}
		if !strings.Contains(lines[1], "failed to connect") {
			t.Errorf("expected error in lines[1], got %q", lines[1])
		}
		if !strings.Contains(lines[3], "connection refused") {
			t.Errorf("expected second error in lines[3], got %q", lines[3])
		}
		if !strings.Contains(msg, "all 4 log lines") {
			t.Errorf("expected msg to mention all 4 log lines, got %q", msg)
		}
	})

	t.Run("preceding errors and tail lines captured including reconciler status", func(t *testing.T) {
		var rawBuilder strings.Builder
		// 5 older errors
		for i := 0; i < 5; i++ {
			ts := now.Add(-time.Duration(180-i) * time.Second).Format(time.RFC3339)
			rawBuilder.WriteString(fmt.Sprintf("%s E0909 10:00:00.000000 1 main.go:10] preceding error %d\n", ts, i))
		}
		// 15 older routine info lines
		for i := 0; i < 15; i++ {
			ts := now.Add(-time.Duration(120-i) * time.Second).Format(time.RFC3339)
			rawBuilder.WriteString(fmt.Sprintf("%s I0909 10:00:00.000000 1 main.go:10] routine info %d\n", ts, i))
		}
		// 19 tail routine info lines
		for i := 0; i < 19; i++ {
			ts := now.Add(-time.Duration(40-i) * time.Second).Format(time.RFC3339)
			rawBuilder.WriteString(fmt.Sprintf("%s I0909 10:00:00.000000 1 main.go:10] tail line %d\n", ts, i))
		}
		// Final tail line: MCO controller waiting on blockingClusters
		tsFinal := now.Add(-1 * time.Second).Format(time.RFC3339)
		rawBuilder.WriteString(
			fmt.Sprintf(
				"%s I0909 10:00:00.000000 1 multiclusterobservability_controller.go:1251] \"Waiting for MCOA ManifestWorks and ManagedClusterAddOns to be deleted\" blockingClusters=[\"local-cluster\"]\n",
				tsFinal,
			),
		)

		lines, msg := formatContainerLogs(rawBuilder.String(), cutoff)
		// Expected: 1 notice + 5 preceding errors + 1 tail divider + 20 tail lines = 27 lines
		if len(lines) != 27 {
			t.Fatalf("expected 27 lines, got %d: %v", len(lines), lines)
		}
		expectedNotice := "  (+ 15 earlier non-error lines omitted for brevity)"
		if lines[0] != expectedNotice {
			t.Errorf("expected notice %q, got %q", expectedNotice, lines[0])
		}
		if !strings.Contains(lines[1], "preceding error 0") {
			t.Errorf("expected first error at lines[1], got %q", lines[1])
		}
		if lines[6] != "  --- [tail: last 20 log lines] ---" {
			t.Errorf("expected tail divider at lines[6], got %q", lines[6])
		}
		// Last line must be the controller blockingClusters signal
		if !strings.Contains(lines[26], "blockingClusters=[\"local-cluster\"]") {
			t.Errorf("expected controller blocking signal preserved at tail, got %q", lines[26])
		}
		if !strings.Contains(msg, "last 20 log lines + 5 preceding error/warning line(s)") {
			t.Errorf("expected msg to mention 20 tail lines and 5 preceding errors, got %q", msg)
		}
	})

	t.Run("capped preceding errors and non-error lines omitted", func(t *testing.T) {
		var rawBuilder strings.Builder
		for i := 0; i < 45; i++ {
			ts := now.Add(-time.Duration(120-i) * time.Second).Format(time.RFC3339)
			rawBuilder.WriteString(fmt.Sprintf("%s E0909 10:00:00.000000 1 main.go:10] error event %d\n", ts, i))
		}
		for i := 0; i < 10; i++ {
			ts := now.Add(-time.Duration(50-i) * time.Second).Format(time.RFC3339)
			rawBuilder.WriteString(fmt.Sprintf("%s I0909 10:00:00.000000 1 main.go:10] info event %d\n", ts, i))
		}
		for i := 0; i < 20; i++ {
			ts := now.Add(-time.Duration(20-i) * time.Second).Format(time.RFC3339)
			rawBuilder.WriteString(fmt.Sprintf("%s I0909 10:00:00.000000 1 main.go:10] tail event %d\n", ts, i))
		}

		lines, _ := formatContainerLogs(rawBuilder.String(), cutoff)
		// Expected: 1 notice + 30 capped preceding errors + 1 divider + 20 tail lines = 52 lines
		if len(lines) != 52 {
			t.Fatalf("expected 52 lines (1 notice + 30 errors + 1 divider + 20 tail), got %d", len(lines))
		}
		expectedNotice := "  (+ 15 older error lines and 10 non-error lines omitted for brevity)"
		if lines[0] != expectedNotice {
			t.Errorf("expected notice %q, got %q", expectedNotice, lines[0])
		}
		if lines[31] != "  --- [tail: last 20 log lines] ---" {
			t.Errorf("expected tail divider at lines[31], got %q", lines[31])
		}
	})

	t.Run("no errors with capped window lines", func(t *testing.T) {
		var rawBuilder strings.Builder
		for i := 0; i < 25; i++ {
			ts := now.Add(-time.Duration(25-i) * time.Second).Format(time.RFC3339)
			rawBuilder.WriteString(fmt.Sprintf("%s I0909 10:00:00.000000 1 main.go:10] routine line %d\n", ts, i))
		}

		lines, _ := formatContainerLogs(rawBuilder.String(), cutoff)
		if len(lines) != 21 { // 1 omitted notice + 20 capped tail lines
			t.Fatalf("expected 21 lines, got %d", len(lines))
		}
		expectedNotice := "  (+ 5 older log lines omitted for brevity; no errors detected in preceding 6m window)"
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

	t.Run("long line truncation", func(t *testing.T) {
		t1 := now.Add(-1 * time.Minute).Format(time.RFC3339)
		longMsg := strings.Repeat("a", 1200)
		raw := fmt.Sprintf("%s E0909 10:00:00.000000 1 main.go:10] error: %s\n", t1, longMsg)
		lines, _ := formatContainerLogs(raw, cutoff)
		if len(lines) != 1 {
			t.Fatalf("expected 1 line, got %d", len(lines))
		}
		expectedLen := maxContainerLogLineLength + len(" ... [truncated]")
		if len(lines[0]) != expectedLen {
			t.Errorf("expected line length %d, got %d", expectedLen, len(lines[0]))
		}
		if !strings.HasSuffix(lines[0], " ... [truncated]") {
			t.Errorf("expected line to end with truncation suffix, got %q", lines[0])
		}
	})
}

func TestTruncateLogLine(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{"short line", "hello world", 20, "hello world"},
		{"exact length", "12345", 5, "12345"},
		{"truncated line", "1234567890", 5, "12345 ... [truncated]"},
		{"utf8 multi-byte straddle", "hello \u20ac world", 8, "hello  ... [truncated]"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := truncateLogLine(tc.input, tc.maxLen)
			if actual != tc.expected {
				t.Errorf("truncateLogLine(%q, %d) = %q; want %q", tc.input, tc.maxLen, actual, tc.expected)
			}
		})
	}
}

func TestFormatContainerState(t *testing.T) {
	startTime := metav1.Date(2026, 9, 9, 10, 30, 0, 0, time.UTC)

	runningState := corev1.ContainerState{
		Running: &corev1.ContainerStateRunning{
			StartedAt: startTime,
		},
	}
	if got := formatContainerState(runningState); got != "Running (started 2026-09-09 10:30:00)" {
		t.Errorf("formatContainerState(running) = %q; want %q", got, "Running (started 2026-09-09 10:30:00)")
	}

	waitingState := corev1.ContainerState{
		Waiting: &corev1.ContainerStateWaiting{
			Reason:  "CrashLoopBackOff",
			Message: "back-off 5m0s restarting failed container",
		},
	}
	if got := formatContainerState(waitingState); got != "Waiting (CrashLoopBackOff: back-off 5m0s restarting failed container)" {
		t.Errorf("formatContainerState(waiting) = %q; want %q", got, "Waiting (CrashLoopBackOff: back-off 5m0s restarting failed container)")
	}

	terminatedState := corev1.ContainerState{
		Terminated: &corev1.ContainerStateTerminated{
			ExitCode: 137,
			Reason:   "ContainerStatusUnknown",
			Message:  "The container could not be located",
		},
	}
	if got := formatContainerState(terminatedState); got != "Terminated (exit code 137, reason: ContainerStatusUnknown: The container could not be located)" {
		t.Errorf("formatContainerState(terminated) = %q; want %q", got, "Terminated (exit code 137, reason: ContainerStatusUnknown: The container could not be located)")
	}
}

func TestFormatAddOnDeploymentConfig(t *testing.T) {
	adc := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name":       "multicluster-observability-addon",
				"generation": int64(3),
			},
			"spec": map[string]any{
				"agentInstallNamespace": "open-cluster-management-agent-addon",
				"customizedVariables": []any{
					map[string]any{
						"name":  "COLLECTOR_IMAGE",
						"value": "quay.io/stolostron/metrics-collector:latest",
					},
				},
				"proxyConfig": map[string]any{
					"noProxy":    "localhost",
					"httpsProxy": "https://proxy.example.com:8443",
					"httpProxy":  "http://proxy.example.com:8080",
				},
			},
		},
	}

	output := formatAddOnDeploymentConfig(adc)
	if !strings.Contains(output, "multicluster-observability-addon (generation 3)") {
		t.Errorf("expected name and generation in output, got: %s", output)
	}
	if !strings.Contains(output, "agentInstallNamespace: open-cluster-management-agent-addon") {
		t.Errorf("expected agentInstallNamespace in output, got: %s", output)
	}
	if !strings.Contains(output, "COLLECTOR_IMAGE: quay.io/stolostron/metrics-collector:latest") {
		t.Errorf("expected customizedVariable in output, got: %s", output)
	}
	expectedProxy := "      proxyConfig:\n        httpProxy: http://proxy.example.com:8080\n        httpsProxy: https://proxy.example.com:8443\n        noProxy: localhost\n"
	if !strings.Contains(output, expectedProxy) {
		t.Errorf("expected sorted proxyConfig in output, got: %s", output)
	}
}

func TestLogClusterMonitoringConfigStatus(t *testing.T) {
	cmoCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-monitoring-config",
			Namespace: "openshift-monitoring",
		},
		Data: map[string]string{
			"config.yaml": `enableUserWorkload: true
prometheusK8s:
  additionalAlertmanagerConfigs:
  - scheme: https
    pathPrefix: /
`,
		},
	}
	uwmCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "user-workload-monitoring-config",
			Namespace: "openshift-user-workload-monitoring",
		},
		Data: map[string]string{
			"config.yaml": `prometheus:
  logLevel: info
`,
		},
	}

	client := kubefake.NewClientset(cmoCM, uwmCM)
	// Should execute cleanly without error or panic
	logClusterMonitoringConfigStatus(client, "Hub")
}

func TestDefensiveNilClientGuards(t *testing.T) {
	// None of these should panic when passed nil clients
	printMCOACustomResources(nil, "some-ns")
	logClusterMonitoringConfigStatus(nil, "Hub")
	logSpokeClusterDebugInfo(nil, nil, "cluster1", true)
	logSpokeClusterDebugInfo(nil, nil, "cluster1", false)
}

func TestFormatMCOCapabilities(t *testing.T) {
	capabilities := map[string]any{
		"platform": map[string]any{
			"analytics": map[string]any{
				"namespaceRightSizingRecommendation":      map[string]any{"enabled": true},
				"virtualizationRightSizingRecommendation": map[string]any{"enabled": true},
			},
			"metrics": map[string]any{
				"alerts":  map[string]any{"enabled": false},
				"default": map[string]any{"enabled": true},
			},
		},
		"userWorkloads": map[string]any{
			"metrics": map[string]any{
				"alerts":  map[string]any{"enabled": false},
				"default": map[string]any{"enabled": false},
			},
		},
	}
	out := formatMCOCapabilities(capabilities)
	expectedPlatform := "    Platform: metrics=true, alerts=false, rightSizing(ns=true, virt=true)\n"
	if !strings.Contains(out, expectedPlatform) {
		t.Errorf("expected platform capabilities summary %q, got:\n%s", expectedPlatform, out)
	}
	expectedUWL := "    UserWorkloads: metrics=false, alerts=false\n"
	if !strings.Contains(out, expectedUWL) {
		t.Errorf("expected userWorkloads capabilities summary %q, got:\n%s", expectedUWL, out)
	}

	// Fallback to compact JSON on unrecognized structure
	unknownCap := map[string]any{"custom": "value"}
	fallbackOut := formatMCOCapabilities(unknownCap)
	if !strings.Contains(fallbackOut, `Capabilities: {"custom":"value"}`) {
		t.Errorf("expected compact JSON fallback, got:\n%s", fallbackOut)
	}
}

func TestFormatPodStatus(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "0/1 nodes available: untolerated taint",
				},
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionFalse,
					Reason: "ContainersNotReady",
				},
				{
					Type:   corev1.PodInitialized,
					Status: corev1.ConditionTrue,
				},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "container-1",
					Ready:        false,
					RestartCount: 2,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "back-off 5m0s restarting failed container",
						},
					},
				},
			},
		},
	}

	out := formatPodStatus(pod)
	if !strings.Contains(out, "PodScheduled: False (Unschedulable: 0/1 nodes available: untolerated taint)") {
		t.Errorf("expected condition with reason and message, got:\n%s", out)
	}
	if !strings.Contains(out, "Ready: False (ContainersNotReady)") {
		t.Errorf("expected condition with reason only, got:\n%s", out)
	}
	if !strings.Contains(out, "Initialized: True") {
		t.Errorf("expected condition with status only, got:\n%s", out)
	}
	if !strings.Contains(out, "container-1: Ready=false, Restarts=2, State: Waiting (CrashLoopBackOff: back-off 5m0s restarting failed container)") {
		t.Errorf("expected waiting container state, got:\n%s", out)
	}
}

func TestSanitizeManifestError(t *testing.T) {
	shortMsg := "Failed to apply manifest: Job is invalid"
	if sanitizeManifestError(shortMsg) != shortMsg {
		t.Fatalf("expected short message unchanged, got: %s", sanitizeManifestError(shortMsg))
	}

	longMsg := strings.Repeat("A", 800)
	sanitized := sanitizeManifestError(longMsg)
	if len(sanitized) > 620 || !strings.HasSuffix(sanitized, "... [truncated]") {
		t.Fatalf("expected truncated message, got len %d: %s", len(sanitized), sanitized)
	}

	// Multi-byte UTF-8 straddle verification
	multiBytePrefix := strings.Repeat("a", 599) + "€€€"
	multiByteSanitized := sanitizeManifestError(multiBytePrefix)
	if !utf8.ValidString(multiByteSanitized) {
		t.Errorf("expected valid utf-8 string, got invalid bytes: %q", multiByteSanitized)
	}
}

func TestCheckJobsInNamespace(t *testing.T) {
	one := int32(1)
	jobCleanup := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "observability-monitoring-cleanup",
			Namespace:         MCO_AGENT_ADDON_NAMESPACE,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
		},
		Spec: batchv1.JobSpec{
			Completions: &one,
		},
		Status: batchv1.JobStatus{
			Succeeded: 0,
			Failed:    1,
			Conditions: []batchv1.JobCondition{
				{
					Type:    batchv1.JobFailed,
					Status:  corev1.ConditionTrue,
					Reason:  "DeadlineExceeded",
					Message: "Job was active longer than specified deadline",
				},
			},
		},
	}
	jobOther := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "submariner-cleanup",
			Namespace:         MCO_AGENT_ADDON_NAMESPACE,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
		},
		Spec: batchv1.JobSpec{
			Completions: &one,
		},
	}
	jobWithFailedPods := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "observability-addon-prune",
			Namespace:         MCO_AGENT_ADDON_NAMESPACE,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Minute)),
		},
		Spec: batchv1.JobSpec{
			Completions: &one,
		},
		Status: batchv1.JobStatus{
			Succeeded: 0,
			Failed:    2,
			Conditions: []batchv1.JobCondition{
				{
					Type:   batchv1.JobSuspended,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	client := kubefake.NewSimpleClientset(jobCleanup, jobOther, jobWithFailedPods)

	// Verify CheckJobsInNamespace filters out non-observability jobs and runs cleanly
	CheckJobsInNamespace(client, MCO_AGENT_ADDON_NAMESPACE)

	// Test formatJobsStatuses directly
	out, err := formatJobsStatuses(client, MCO_AGENT_ADDON_NAMESPACE)
	if err != nil {
		t.Fatalf("unexpected error formatting jobs: %v", err)
	}
	if !strings.Contains(out, "observability-monitoring-cleanup") {
		t.Errorf("expected observability-monitoring-cleanup in output, got: %s", out)
	}
	if strings.Contains(out, "submariner-cleanup") {
		t.Errorf("expected non-observability job submariner-cleanup to be filtered out, got: %s", out)
	}
	if !strings.Contains(out, "Failed") {
		t.Errorf("expected Failed condition in output, got: %s", out)
	}
	if !strings.Contains(out, "DeadlineExceeded") {
		t.Errorf("expected DeadlineExceeded in output, got: %s", out)
	}
	if !strings.Contains(out, "Job was active longer than specified deadline") {
		t.Errorf("expected condition message in output, got: %s", out)
	}
	if !strings.Contains(out, "[Failed pods: 2]") {
		t.Errorf("expected [Failed pods: 2] for job with non-failure condition, got: %s", out)
	}
}

func TestFormatRecentWarningEvents(t *testing.T) {
	now := time.Now()
	event1 := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "event-1",
			Namespace: MCO_AGENT_ADDON_NAMESPACE,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "observability-monitoring-cleanup-abcde",
		},
		Type:          corev1.EventTypeWarning,
		Reason:        "FailedScheduling",
		Message:       "0/1 nodes are available: 1 node(s) had untolerated taint {node.kubernetes.io/disk-pressure: }",
		Count:         4,
		LastTimestamp: metav1.NewTime(now.Add(-2 * time.Minute)),
	}
	eventNormal := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "event-normal",
			Namespace: MCO_AGENT_ADDON_NAMESPACE,
		},
		Type:          corev1.EventTypeNormal,
		Reason:        "Scheduled",
		LastTimestamp: metav1.NewTime(now.Add(-1 * time.Minute)),
	}
	eventOld := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "event-old",
			Namespace: MCO_AGENT_ADDON_NAMESPACE,
		},
		Type:          corev1.EventTypeWarning,
		Reason:        "FailedScheduling",
		Message:       "Old event",
		LastTimestamp: metav1.NewTime(now.Add(-30 * time.Minute)),
	}
	eventSeries := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "event-series",
			Namespace: MCO_AGENT_ADDON_NAMESPACE,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "observability-monitoring-series-pod",
		},
		Type:    corev1.EventTypeWarning,
		Reason:  "BackOff",
		Message: "Back-off restarting failed container",
		Series: &corev1.EventSeries{
			Count:            3,
			LastObservedTime: metav1.NewMicroTime(now.Add(-1 * time.Minute)),
		},
	}

	client := kubefake.NewSimpleClientset(event1, eventNormal, eventOld, eventSeries)

	out, err := formatRecentWarningEvents(client, []string{MCO_AGENT_ADDON_NAMESPACE}, 10*time.Minute, 15)
	if err != nil {
		t.Fatalf("unexpected error formatting warning events: %v", err)
	}

	if !strings.Contains(out, "FailedScheduling (x4)") {
		t.Errorf("expected FailedScheduling with count in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Pod/observability-monitoring-cleanup-abcde") {
		t.Errorf("expected object kind/name in output, got:\n%s", out)
	}
	if !strings.Contains(out, "disk-pressure") {
		t.Errorf("expected disk-pressure message in output, got:\n%s", out)
	}
	if !strings.Contains(out, "BackOff (x3)") {
		t.Errorf("expected BackOff (x3) from series, got:\n%s", out)
	}
	if !strings.Contains(out, "Pod/observability-monitoring-series-pod") {
		t.Errorf("expected series pod in output, got:\n%s", out)
	}
	if strings.Contains(out, "Old event") {
		t.Errorf("expected old event beyond window to be excluded, got:\n%s", out)
	}
	if strings.Contains(out, "Scheduled") {
		t.Errorf("expected Normal event to be excluded, got:\n%s", out)
	}

	// Test when no events match
	emptyClient := kubefake.NewSimpleClientset()
	emptyOut, err := formatRecentWarningEvents(emptyClient, []string{MCO_AGENT_ADDON_NAMESPACE}, 10*time.Minute, 15)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(emptyOut, "No recent Warning events found") {
		t.Errorf("expected empty message, got: %s", emptyOut)
	}
}

func TestGetPodWorkloadKey(t *testing.T) {
	isController := true
	podWithController := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "metrics-collector-deployment-bf4cc564f-62jjd",
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "ReplicaSet",
					Name:       "metrics-collector-deployment-bf4cc564f",
					Controller: &isController,
				},
			},
		},
	}
	if key := getPodWorkloadKey(podWithController); key != "metrics-collector-deployment-bf4cc564f" {
		t.Errorf("expected controller name, got %q", key)
	}

	podWithoutController := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-custom-workload-abcde",
		},
	}
	if key := getPodWorkloadKey(podWithoutController); key != "my-custom-workload" {
		t.Errorf("expected stripped suffix prefix, got %q", key)
	}

	podBare := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "standalone",
		},
	}
	if key := getPodWorkloadKey(podBare); key != "standalone" {
		t.Errorf("expected full pod name, got %q", key)
	}
}

func TestFormatRecentWarningEvents_Consolidation(t *testing.T) {
	now := time.Now()
	event1 := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "evicted-1",
			Namespace: "open-cluster-management-observability",
		},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "metrics-collector-deployment-bf4cc564f-62jjd",
		},
		Type:          corev1.EventTypeWarning,
		Reason:        "Evicted",
		Message:       "The node had condition: [DiskPressure].",
		Count:         1,
		LastTimestamp: metav1.NewTime(now.Add(-5 * time.Minute)),
	}
	event2 := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "evicted-2",
			Namespace: "open-cluster-management-observability",
		},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "metrics-collector-deployment-bf4cc564f-696h5",
		},
		Type:          corev1.EventTypeWarning,
		Reason:        "Evicted",
		Message:       "The node had condition: [DiskPressure].",
		Count:         1,
		LastTimestamp: metav1.NewTime(now.Add(-4 * time.Minute)),
	}
	event3 := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "failed-scheduling",
			Namespace: "open-cluster-management-observability",
		},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "metrics-collector-deployment-bf4cc564f-8c52l",
		},
		Type:          corev1.EventTypeWarning,
		Reason:        "FailedScheduling",
		Message:       "0/1 nodes are available: untolerated taint",
		Count:         1,
		LastTimestamp: metav1.NewTime(now.Add(-3 * time.Minute)),
	}

	client := kubefake.NewSimpleClientset(event1, event2, event3)
	out, err := formatRecentWarningEvents(client, []string{"open-cluster-management-observability"}, 20*time.Minute, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Evicted (x2)") {
		t.Errorf("expected Evicted (x2) consolidation, got:\n%s", out)
	}
	if !strings.Contains(out, "Pod/metrics-collector-deployment-bf4cc564f-*") {
		t.Errorf("expected wildcard object name, got:\n%s", out)
	}
	if !strings.Contains(out, "FailedScheduling") {
		t.Errorf("expected FailedScheduling event, got:\n%s", out)
	}
}

func TestCheckPodsInNamespace_WorkloadCapping(t *testing.T) {
	isController := true
	now := metav1.Now()
	var pods []runtime.Object
	// 5 evicted failed pods
	for i := 1; i <= 5; i++ {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              fmt.Sprintf("metrics-collector-deployment-bf4cc564f-%d", i),
				Namespace:         "test-ns",
				CreationTimestamp: now,
				OwnerReferences: []metav1.OwnerReference{
					{
						Kind:       "ReplicaSet",
						Name:       "metrics-collector-deployment-bf4cc564f",
						Controller: &isController,
					},
				},
			},
			Status: corev1.PodStatus{
				Phase:  corev1.PodFailed,
				Reason: "Evicted",
			},
		}
		pods = append(pods, pod)
	}
	// 1 running pod for the same workload (active replacement)
	pods = append(pods, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "metrics-collector-deployment-bf4cc564f-running",
			Namespace:         "test-ns",
			CreationTimestamp: now,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "ReplicaSet",
					Name:       "metrics-collector-deployment-bf4cc564f",
					Controller: &isController,
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	})

	client := kubefake.NewSimpleClientset(pods...)
	// Calling CheckPodsInNamespace should execute cleanly with duplicate failed pods capped
	// and recognize that the workload has an active running replacement.
	CheckPodsInNamespace(client, "test-ns", nil, nil)
}

func TestFormatPodsStatuses_WorkloadCapping(t *testing.T) {
	isController := true
	now := metav1.Now()
	var pods []corev1.Pod

	// 5 failed pods for workload-a
	for i := 1; i <= 5; i++ {
		pods = append(pods, corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              fmt.Sprintf("workload-a-replica-%d", i),
				CreationTimestamp: now,
				OwnerReferences: []metav1.OwnerReference{
					{
						Kind:       "ReplicaSet",
						Name:       "workload-a-rs",
						Controller: &isController,
					},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodFailed,
			},
		})
	}

	// 3 failed pods for workload-b (no controller ownerRef, suffix stripped)
	for i := 1; i <= 3; i++ {
		pods = append(pods, corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              fmt.Sprintf("workload-b-%d", i),
				CreationTimestamp: now,
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodFailed,
			},
		})
	}

	// 1 running pod for workload-a
	pods = append(pods, corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "workload-a-active",
			CreationTimestamp: now,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "ReplicaSet",
					Name:       "workload-a-rs",
					Controller: &isController,
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	})

	out := formatPodsStatuses(pods)

	// Verify workload-a only has 2 sample failed rows + 1 running row
	if !strings.Contains(out, "workload-a-replica-1") || !strings.Contains(out, "workload-a-replica-2") {
		t.Errorf("expected first 2 failed samples for workload-a, got:\n%s", out)
	}
	if strings.Contains(out, "workload-a-replica-3") || strings.Contains(out, "workload-a-replica-4") || strings.Contains(out, "workload-a-replica-5") {
		t.Errorf("expected failed samples 3-5 for workload-a to be omitted, got:\n%s", out)
	}
	if !strings.Contains(out, "workload-a-active") {
		t.Errorf("expected running pod to be present, got:\n%s", out)
	}
	if !strings.Contains(out, `(+ 3 additional non-running pod(s) for "workload-a-rs" omitted for brevity)`) {
		t.Errorf("expected workload-a omitted summary, got:\n%s", out)
	}

	// Verify workload-b only has 2 sample failed rows
	if !strings.Contains(out, "workload-b-1") || !strings.Contains(out, "workload-b-2") {
		t.Errorf("expected first 2 failed samples for workload-b, got:\n%s", out)
	}
	if strings.Contains(out, "workload-b-3") {
		t.Errorf("expected failed sample 3 for workload-b to be omitted, got:\n%s", out)
	}
	if !strings.Contains(out, `(+ 1 additional non-running pod(s) for "workload-b" omitted for brevity)`) {
		t.Errorf("expected workload-b omitted summary, got:\n%s", out)
	}
}

func TestFormatNamespacePodHealthSummary(t *testing.T) {
	tests := []struct {
		name                 string
		ns                   string
		notRunningCount      int
		activeUnhealthyCount int
		skippedAncientCount  int
		wantSummary          string
		wantError            bool
	}{
		{
			name:                 "active unhealthy pods present without running replicas",
			ns:                   "test-ns",
			notRunningCount:      3,
			activeUnhealthyCount: 2,
			skippedAncientCount:  0,
			wantSummary:          `Found 2 active unhealthy pod(s) without running replicas in namespace "test-ns"`,
			wantError:            true,
		},
		{
			name:                 "active unhealthy pods with ancient pods skipped",
			ns:                   "test-ns",
			notRunningCount:      3,
			activeUnhealthyCount: 2,
			skippedAncientCount:  1,
			wantSummary:          `Found 2 active unhealthy pod(s) without running replicas in namespace "test-ns" (1 ancient failed pod(s) skipped)`,
			wantError:            true,
		},
		{
			name:                 "all workloads running with historical evicted replicas",
			ns:                   "open-cluster-management-observability",
			notRunningCount:      38,
			activeUnhealthyCount: 0,
			skippedAncientCount:  0,
			wantSummary:          `All active workloads are running in namespace "open-cluster-management-observability" (38 historical evicted/failed pod(s) with active replacements)`,
			wantError:            false,
		},
		{
			name:                 "all workloads running with historical evicted replicas and ancient skipped",
			ns:                   "open-cluster-management-observability",
			notRunningCount:      38,
			activeUnhealthyCount: 0,
			skippedAncientCount:  2,
			wantSummary:          `All active workloads are running in namespace "open-cluster-management-observability" (38 historical evicted/failed pod(s) with active replacements, 2 ancient failed pod(s) skipped)`,
			wantError:            false,
		},
		{
			name:                 "only ancient pods skipped",
			ns:                   "open-cluster-management",
			notRunningCount:      0,
			activeUnhealthyCount: 0,
			skippedAncientCount:  1,
			wantSummary:          `All active pods are healthy in namespace "open-cluster-management" (1 ancient failed pod(s) skipped)`,
			wantError:            false,
		},
		{
			name:                 "completely healthy namespace",
			ns:                   "open-cluster-management",
			notRunningCount:      0,
			activeUnhealthyCount: 0,
			skippedAncientCount:  0,
			wantSummary:          `All pods are running in namespace "open-cluster-management"`,
			wantError:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSummary, gotError := formatNamespacePodHealthSummary(tt.ns, tt.notRunningCount, tt.activeUnhealthyCount, tt.skippedAncientCount)
			if gotSummary != tt.wantSummary {
				t.Errorf("formatNamespacePodHealthSummary() gotSummary = %q, want %q", gotSummary, tt.wantSummary)
			}
			if gotError != tt.wantError {
				t.Errorf("formatNamespacePodHealthSummary() gotError = %v, want %v", gotError, tt.wantError)
			}
		})
	}
}
