// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"strings"
	"testing"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestCheckNonOCPPrometheusAgentReady(t *testing.T) {
	tests := []struct {
		name    string
		change  func(*unstructured.Unstructured, *appv1.StatefulSet)
		wantErr string
	}{
		{name: "non-OCP rollout ready"},
		{
			name: "non-OCP scrape class has not been applied",
			change: func(pa *unstructured.Unstructured, _ *appv1.StatefulSet) {
				pa.Object["spec"].(map[string]any)["scrapeClasses"] = []any{}
			},
			wantErr: "does not have the default non-ocp-monitoring scrape class",
		},
		{
			name: "non-OCP scrape class is not the default",
			change: func(pa *unstructured.Unstructured, _ *appv1.StatefulSet) {
				pa.Object["spec"].(map[string]any)["scrapeClasses"] = []any{
					map[string]any{"name": "non-ocp-monitoring", "default": false},
				}
			},
			wantErr: "does not have the default non-ocp-monitoring scrape class",
		},
		{
			name: "PrometheusAgent is not available",
			change: func(pa *unstructured.Unstructured, _ *appv1.StatefulSet) {
				pa.Object["status"].(map[string]any)["conditions"] = []any{
					map[string]any{"type": "Available", "status": "False"},
				}
			},
			wantErr: "is not Available",
		},
		{
			name: "old OpenShift StatefulSet is still present",
			change: func(_ *unstructured.Unstructured, sts *appv1.StatefulSet) {
				sts.Spec.Template.Spec.Containers = append(sts.Spec.Template.Spec.Containers,
					corev1.Container{Name: "kube-rbac-proxy"})
			},
			wantErr: "still has the OpenShift-only kube-rbac-proxy sidecar",
		},
		{
			name: "StatefulSet generation has not been observed",
			change: func(_ *unstructured.Unstructured, sts *appv1.StatefulSet) {
				sts.Status.ObservedGeneration--
			},
			wantErr: "non-OCP rollout is not ready",
		},
		{
			name: "replacement pod has not been updated",
			change: func(_ *unstructured.Unstructured, sts *appv1.StatefulSet) {
				sts.Status.UpdatedReplicas = 0
			},
			wantErr: "updated 0/1",
		},
		{
			name: "replacement pod is running but not ready",
			change: func(_ *unstructured.Unstructured, sts *appv1.StatefulSet) {
				sts.Status.ReadyReplicas = 0
			},
			wantErr: "ready 0/1",
		},
		{
			name: "StatefulSet revision has not caught up",
			change: func(_ *unstructured.Unstructured, sts *appv1.StatefulSet) {
				sts.Status.CurrentRevision = "previous-revision"
			},
			wantErr: "revisions \"previous-revision\"/\"current-revision\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pa, sts := readyNonOCPPrometheusAgentAndStatefulSet()
			if tt.change != nil {
				tt.change(pa, sts)
			}

			err := checkNonOCPPrometheusAgentReady(pa, sts)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected ready rollout, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func readyNonOCPPrometheusAgentAndStatefulSet() (*unstructured.Unstructured, *appv1.StatefulSet) {
	pa := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "platform-metrics-collector"},
		"spec": map[string]any{
			"scrapeClasses": []any{
				map[string]any{"name": "non-ocp-monitoring", "default": true},
			},
		},
		"status": map[string]any{
			"conditions": []any{
				map[string]any{"type": "Available", "status": "True"},
			},
		},
	}}

	replicas := int32(1)
	sts := &appv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "prom-agent-platform-metrics-collector",
			Generation: 2,
		},
		Spec: appv1.StatefulSetSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "prometheus"}},
			}},
		},
		Status: appv1.StatefulSetStatus{
			ObservedGeneration: 2,
			UpdatedReplicas:    1,
			ReadyReplicas:      1,
			CurrentRevision:    "current-revision",
			UpdateRevision:     "current-revision",
		},
	}
	return pa, sts
}
