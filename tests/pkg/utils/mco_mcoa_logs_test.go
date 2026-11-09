// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestCheckClusterLoggingOperatorStatus(t *testing.T) {
	csv := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "cluster-logging.v6.7.0"},
		"status":   map[string]any{"phase": "Succeeded"},
	}}
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: clusterLoggingDeploymentName, Namespace: clusterLoggingNamespace, Generation: 2},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status:     appsv1.DeploymentStatus{ObservedGeneration: 2, ReadyReplicas: 1, AvailableReplicas: 1},
	}

	tests := []struct {
		name      string
		change    func(*unstructured.Unstructured, *appsv1.Deployment)
		wantError string
	}{
		{name: "healthy"},
		{
			name: "CSV failed",
			change: func(csv *unstructured.Unstructured, _ *appsv1.Deployment) {
				csv.Object["status"] = map[string]any{"phase": "Failed", "reason": "NoOperatorGroup", "message": "no matching OperatorGroup"}
			},
			wantError: "NoOperatorGroup",
		},
		{
			name: "deployment has not observed current generation",
			change: func(_ *unstructured.Unstructured, deployment *appsv1.Deployment) {
				deployment.Status.ObservedGeneration = 1
			},
			wantError: "has not observed generation 2",
		},
		{
			name: "operator pod is not ready",
			change: func(_ *unstructured.Unstructured, deployment *appsv1.Deployment) {
				deployment.Status.ReadyReplicas = 0
			},
			wantError: "0/1 ready replicas",
		},
		{
			name: "operator pod is not available",
			change: func(_ *unstructured.Unstructured, deployment *appsv1.Deployment) {
				deployment.Status.AvailableReplicas = 0
			},
			wantError: "0/1 available replicas",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotCSV, gotDeployment := csv.DeepCopy(), deployment.DeepCopy()
			if test.change != nil {
				test.change(gotCSV, gotDeployment)
			}
			err := checkClusterLoggingCSVStatus(gotCSV)
			if err == nil {
				err = checkClusterLoggingDeploymentStatus(gotDeployment)
			}
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("expected healthy operator, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error = %v, want text %q", err, test.wantError)
			}
		})
	}
}

func TestCheckManagedLokiStackStatus(t *testing.T) {
	stack := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name":       managedLokiStackName,
			"namespace":  clusterLoggingNamespace,
			"generation": int64(2),
		},
		"status": map[string]any{"conditions": []any{map[string]any{
			"type": "Ready", "status": "True", "observedGeneration": int64(2), "reason": "ReadyComponents",
		}}},
	}}

	tests := []struct {
		name      string
		change    func(*unstructured.Unstructured)
		wantError string
	}{
		{name: "healthy"},
		{
			name: "no conditions yet",
			change: func(stack *unstructured.Unstructured) {
				delete(stack.Object, "status")
			},
			wantError: "has no status conditions yet",
		},
		{
			name: "components pending",
			change: func(stack *unstructured.Unstructured) {
				stack.Object["status"] = map[string]any{"conditions": []any{map[string]any{
					"type": "Ready", "status": "False", "observedGeneration": int64(2),
					"reason": "PendingComponents", "message": "ingester pod is pending",
				}}}
			},
			wantError: "ingester pod is pending",
		},
		{
			name: "stale ready condition",
			change: func(stack *unstructured.Unstructured) {
				stack.Object["status"] = map[string]any{"conditions": []any{map[string]any{
					"type": "Ready", "status": "True", "observedGeneration": int64(1),
				}}}
			},
			wantError: "Ready condition is stale",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotStack := stack.DeepCopy()
			if test.change != nil {
				test.change(gotStack)
			}
			err := checkManagedLokiStackStatus(gotStack)
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("expected healthy LokiStack, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error = %v, want text %q", err, test.wantError)
			}
		})
	}
}
