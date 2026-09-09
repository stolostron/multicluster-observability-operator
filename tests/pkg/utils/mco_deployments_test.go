// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"strings"
	"testing"
	"time"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFormatDeploymentNotReadyError(t *testing.T) {
	t.Run("not ready with conditions and explicit replicas", func(t *testing.T) {
		replicas := int32(2)
		dep := &appv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-dep",
				Namespace: "test-ns",
			},
			Spec: appv1.DeploymentSpec{
				Replicas: &replicas,
			},
			Status: appv1.DeploymentStatus{
				ReadyReplicas: 1,
				Conditions: []appv1.DeploymentCondition{
					{
						Type:    appv1.DeploymentAvailable,
						Status:  corev1.ConditionFalse,
						Reason:  "MinimumReplicasUnavailable",
						Message: "Deployment does not have minimum availability.",
					},
					{
						Type:    appv1.DeploymentProgressing,
						Status:  corev1.ConditionTrue,
						Reason:  "ReplicaSetUpdated",
						Message: "ReplicaSet updated",
					},
				},
			},
		}

		err := FormatDeploymentNotReadyError(dep, "test-ns", "test-dep")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		errMsg := err.Error()
		if !strings.Contains(errMsg, "deployment test-ns/test-dep is not ready: 1/2 ready replicas") {
			t.Errorf("unexpected error prefix: %s", errMsg)
		}
		if !strings.Contains(errMsg, "Available=False(MinimumReplicasUnavailable: Deployment does not have minimum availability.)") {
			t.Errorf("expected negative condition in error: %s", errMsg)
		}
		if strings.Contains(errMsg, "Progressing=True") {
			t.Errorf("positive condition should not be included: %s", errMsg)
		}
	})

	t.Run("nil deployment", func(t *testing.T) {
		err := FormatDeploymentNotReadyError(nil, "test-ns", "test-dep")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "deployment test-ns/test-dep is nil") {
			t.Errorf("expected nil error message, got: %s", err.Error())
		}
	})

	t.Run("deployment replica failure condition status", func(t *testing.T) {
		dep := &appv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-dep",
				Namespace: "test-ns",
			},
			Status: appv1.DeploymentStatus{
				ReadyReplicas: 0,
				Conditions: []appv1.DeploymentCondition{
					{
						Type:    appv1.DeploymentReplicaFailure,
						Status:  corev1.ConditionTrue,
						Reason:  "FailedCreate",
						Message: "quota exceeded",
					},
					{
						Type:    appv1.DeploymentReplicaFailure,
						Status:  corev1.ConditionFalse,
						Reason:  "QuotaRestored",
						Message: "quota restored",
					},
				},
			},
		}

		err := FormatDeploymentNotReadyError(dep, "test-ns", "test-dep")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		errMsg := err.Error()
		if !strings.Contains(errMsg, "ReplicaFailure=True(FailedCreate: quota exceeded)") {
			t.Errorf("expected ReplicaFailure=True condition in error: %s", errMsg)
		}
		if strings.Contains(errMsg, "ReplicaFailure=False") {
			t.Errorf("ReplicaFailure=False should not be included: %s", errMsg)
		}
	})

	t.Run("not ready with nil replicas defaults to 1", func(t *testing.T) {
		dep := &appv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-dep",
				Namespace: "test-ns",
			},
			Status: appv1.DeploymentStatus{
				ReadyReplicas: 0,
			},
		}

		err := FormatDeploymentNotReadyError(dep, "test-ns", "test-dep")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "0/1 ready replicas") {
			t.Errorf("expected 0/1 ready replicas: %s", err.Error())
		}
	})
}

func TestFormatDeploymentStillExistsError(t *testing.T) {
	t.Run("nil deployment", func(t *testing.T) {
		err := FormatDeploymentStillExistsError(nil, "test-ns", "test-dep")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "deployment test-ns/test-dep is nil") {
			t.Errorf("expected nil error message, got: %s", err.Error())
		}
	})

	t.Run("terminating deployment with finalizers", func(t *testing.T) {
		now := metav1.NewTime(time.Date(2026, 9, 9, 16, 0, 0, 0, time.UTC))
		dep := &appv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "test-dep",
				Namespace:         "test-ns",
				DeletionTimestamp: &now,
				Finalizers:        []string{"foregroundDeletion", "test-finalizer"},
			},
		}

		err := FormatDeploymentStillExistsError(dep, "test-ns", "test-dep")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		errMsg := err.Error()
		if !strings.Contains(errMsg, "terminating since") {
			t.Errorf("expected terminating in error: %s", errMsg)
		}
		if !strings.Contains(errMsg, "[foregroundDeletion test-finalizer]") {
			t.Errorf("expected finalizers in error: %s", errMsg)
		}
	})

	t.Run("active deployment still exists", func(t *testing.T) {
		replicas := int32(1)
		dep := &appv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-dep",
				Namespace:  "test-ns",
				Generation: 3,
			},
			Spec: appv1.DeploymentSpec{
				Replicas: &replicas,
			},
			Status: appv1.DeploymentStatus{
				ReadyReplicas:      1,
				ObservedGeneration: 3,
			},
		}

		err := FormatDeploymentStillExistsError(dep, "test-ns", "test-dep")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		errMsg := err.Error()
		if !strings.Contains(errMsg, "deployment test-ns/test-dep still exists (replicas: 1/1, generation: 3, observedGeneration: 3)") {
			t.Errorf("unexpected error format: %s", errMsg)
		}
	})
}
