// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"strings"
	"testing"
	"time"

	appv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFormatStatefulSetStillExistsError(t *testing.T) {
	t.Run("nil statefulset", func(t *testing.T) {
		err := FormatStatefulSetStillExistsError(nil, "test-ns", "test-sts")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "statefulset test-ns/test-sts is nil") {
			t.Errorf("expected nil error message, got: %s", err.Error())
		}
	})

	t.Run("terminating statefulset with finalizers", func(t *testing.T) {
		now := metav1.NewTime(time.Date(2026, 9, 9, 16, 0, 0, 0, time.UTC))
		sts := &appv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "test-sts",
				Namespace:         "test-ns",
				DeletionTimestamp: &now,
				Finalizers:        []string{"test-finalizer"},
			},
		}

		err := FormatStatefulSetStillExistsError(sts, "test-ns", "test-sts")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		errMsg := err.Error()
		if !strings.Contains(errMsg, "terminating since") {
			t.Errorf("expected terminating in error: %s", errMsg)
		}
		if !strings.Contains(errMsg, "[test-finalizer]") {
			t.Errorf("expected finalizers in error: %s", errMsg)
		}
	})

	t.Run("active statefulset still exists", func(t *testing.T) {
		replicas := int32(1)
		sts := &appv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-sts",
				Namespace:  "test-ns",
				Generation: 2,
			},
			Spec: appv1.StatefulSetSpec{
				Replicas: &replicas,
			},
			Status: appv1.StatefulSetStatus{
				ReadyReplicas:      1,
				ObservedGeneration: 2,
			},
		}

		err := FormatStatefulSetStillExistsError(sts, "test-ns", "test-sts")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		errMsg := err.Error()
		if !strings.Contains(errMsg, "statefulset test-ns/test-sts still exists (replicas: 1/1, generation: 2, observedGeneration: 2)") {
			t.Errorf("unexpected error format: %s", errMsg)
		}
	})
}
