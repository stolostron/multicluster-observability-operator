// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package openshift

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/status"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCmoConfigWatcher(t *testing.T) {
	namespace := "my-namespace"
	addon := newObservabilityAddon("observability-addon", namespace)
	s := runtime.NewScheme()
	oav1beta1.AddToScheme(s)
	c := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(addon).Build()
	statusReporter := &statusReporterMock{}
	capacity := 5
	leakPeriod := 100 * time.Millisecond
	cmoWatcher := NewCmoConfigChangesWatcher(c, logr.Logger{}, statusReporter, capacity, leakPeriod, 0.5)
	req := ctrl.Request{NamespacedName: types.NamespacedName{
		Namespace: config.OCPClusterMonitoringNamespace,
		Name:      config.OCPClusterMonitoringConfigMapName,
	}}

	// Ensnure that status is reset on first run, even if not triggered by cmo update to avoid locked state
	statusReporter.Reason = status.CmoReconcileLoopDetected
	res, err := cmoWatcher.CheckRequest(context.Background(), ctrl.Request{}, false)
	assert.NoError(t, err)
	assert.True(t, res.IsZero())
	assert.Equal(t, status.CmoReconcileLoopStopped, statusReporter.Reason)

	// Reset status and cmo watcher
	statusReporter = &statusReporterMock{}
	statusResetFillRatio := 0.6
	cmoWatcher = NewCmoConfigChangesWatcher(c, logr.Logger{}, statusReporter, capacity, leakPeriod, statusResetFillRatio)

	// Reconciles trigerred by other sources are ignored
	for range capacity {
		res, err := cmoWatcher.CheckRequest(context.Background(), ctrl.Request{}, true)
		assert.NoError(t, err)
		assert.True(t, res.IsZero())
	}

	// Reconciles without cmo update are ignored
	for range capacity {
		res, err := cmoWatcher.CheckRequest(context.Background(), req, false)
		assert.NoError(t, err)
		assert.True(t, res.IsZero())
	}

	// Fill the bucket. Unchanged status.
	for range capacity - 1 {
		res, err := cmoWatcher.CheckRequest(context.Background(), req, true)
		assert.NoError(t, err)
		assert.True(t, res.IsZero())
		assert.Len(t, statusReporter.Reason, 0)
	}

	// Next trigger degrades status and returns requeueAfter
	res, err = cmoWatcher.CheckRequest(context.Background(), req, true)
	assert.NoError(t, err)
	assert.False(t, res.IsZero())
	assert.InEpsilon(t, 200*time.Millisecond, res.RequeueAfter, 0.1, fmt.Sprintf("requeue after is: %v", res.RequeueAfter))
	assert.Equal(t, status.CmoReconcileLoopDetected, statusReporter.Reason)

	// Additional checks without updates of the configMap return same requeueAfter
	res, err = cmoWatcher.CheckRequest(context.Background(), req, false)
	assert.NoError(t, err)
	assert.False(t, res.IsZero())
	assert.InEpsilon(t, 200*time.Millisecond, res.RequeueAfter, 0.1, fmt.Sprintf("requeue after is: %v", res.RequeueAfter))

	// Wait for returned requeue after (ensure it is not zero)
	time.Sleep(res.RequeueAfter + 10*time.Millisecond) // Add some time to avoid flacky tests

	// Trigger one reconcile => status should be resolved
	res, err = cmoWatcher.CheckRequest(context.Background(), req, false)
	assert.NoError(t, err)
	assert.True(t, res.IsZero())
	assert.Equal(t, status.CmoReconcileLoopStopped, statusReporter.Reason)
}

func TestLeakyBucket_Add(t *testing.T) {
	capacity := 3
	leakPeriod := 100 * time.Millisecond

	bucket := newLeakyBucket(capacity, leakPeriod)

	// Should be able to add up to capacity
	for i := 0; i < capacity; i++ {
		success := bucket.Add()
		if !success {
			t.Errorf("Failed to add item %d to bucket with capacity %d", i+1, capacity)
		}
	}

	// Should fail to add when full
	success := bucket.Add()
	if success {
		t.Errorf("Expected Add to return false when bucket is full")
	}
}

func TestLeakyBucket_FillRatio(t *testing.T) {
	capacity := 4
	leakPeriod := 100 * time.Millisecond

	bucket := newLeakyBucket(capacity, leakPeriod)

	// Empty bucket
	if ratio := bucket.FillRatio(); ratio != 0.0 {
		t.Errorf("Expected fill ratio of empty bucket to be 0.0, got %f", ratio)
	}

	// Add 2 items
	bucket.Add()
	bucket.Add()

	expectedRatio := 2.0 / 4.0
	if ratio := bucket.FillRatio(); ratio != expectedRatio {
		t.Errorf("Expected fill ratio to be %f, got %f", expectedRatio, ratio)
	}

	// Fill bucket
	bucket.Add()
	bucket.Add()

	if ratio := bucket.FillRatio(); ratio != 1.0 {
		t.Errorf("Expected fill ratio of full bucket to be 1.0, got %f", ratio)
	}
}

func TestLeakyBucket_Leaking(t *testing.T) {
	capacity := 3
	leakPeriod := 50 * time.Millisecond

	bucket := newLeakyBucket(capacity, leakPeriod)

	// Fill the bucket
	for i := 0; i < capacity; i++ {
		bucket.Add()
	}

	if bucket.FillRatio() != 1.0 {
		t.Errorf("Expected bucket to be full")
	}

	// Wait for at least one leak to occur
	time.Sleep(leakPeriod + 100*time.Millisecond)

	// Should be able to add one more
	success := bucket.Add()
	if !success {
		t.Errorf("Expected to be able to add after leaking")
	}
}

func newObservabilityAddon(name string, ns string) *oav1beta1.ObservabilityAddon {
	return &oav1beta1.ObservabilityAddon{
		TypeMeta: metav1.TypeMeta{
			APIVersion: oav1beta1.GroupVersion.String(),
			Kind:       "ObservabilityAddon",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
}

type statusReporterMock struct {
	Reason status.Reason
}

func (s *statusReporterMock) UpdateComponentCondition(ctx context.Context, componentName status.Component, newReason status.Reason, newMessage string) (bool, error) {
	s.Reason = newReason
	return true, nil
}

func (s *statusReporterMock) GetConditionReason(ctx context.Context, componentName status.Component) (status.Reason, error) {
	return s.Reason, nil
}
