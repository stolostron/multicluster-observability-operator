// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package status

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReportStatus(t *testing.T) {
	s := scheme.Scheme
	assert.NoError(t, oav1beta1.AddToScheme(s))
	assert.NoError(t, addonv1alpha1.AddToScheme(s))
	assert.NoError(t, mcov1beta2.AddToScheme(s))

	type updateParams struct {
		component Component
		reason    Reason
		message   string
	}

	testCases := map[string]struct {
		currentConditions []oav1beta1.StatusCondition
		updateParams      updateParams
		expects           func(*testing.T, bool, error, []oav1beta1.StatusCondition)
	}{
		"new status should be appended": {
			currentConditions: []oav1beta1.StatusCondition{},
			updateParams: updateParams{
				component: "MetricsCollector",
				reason:    UpdateSuccessful,
				message:   "Metrics collector updated",
			},
			expects: func(t *testing.T, wasUpdated bool, updateErr error, conditions []oav1beta1.StatusCondition) {
				assert.NoError(t, updateErr)
				assert.Len(t, conditions, 1)
				assert.EqualValues(t, UpdateSuccessful, conditions[0].Reason)
				assert.Equal(t, metav1.ConditionTrue, conditions[0].Status)
				assert.Equal(t, "MetricsCollector", conditions[0].Type)
				assert.InEpsilon(t, time.Now().Unix(), conditions[0].LastTransitionTime.Unix(), 1)
				assert.True(t, wasUpdated)
			},
		},
		"existing status should be updated": {
			currentConditions: []oav1beta1.StatusCondition{
				{
					Type:    "MetricsCollector",
					Reason:  string(ForwardSuccessful),
					Message: "Metrics collector deployed",
					Status:  metav1.ConditionTrue,
					LastTransitionTime: metav1.Time{
						Time: time.Now().Add(-time.Minute), // current state (most recent)
					},
				},
				{
					Type:    "Available",
					Reason:  string(ForwardSuccessful),
					Message: "Metrics collector available",
					Status:  metav1.ConditionTrue,
					LastTransitionTime: metav1.Time{
						Time: time.Now().Add(-2 * time.Minute),
					},
				},
			},
			updateParams: updateParams{
				component: "MetricsCollector",
				reason:    UpdateFailed,
				message:   "Metrics collector disabled",
			},
			expects: func(t *testing.T, wasUpdated bool, updateErr error, conditions []oav1beta1.StatusCondition) {
				assert.NoError(t, updateErr)
				condMap := make(map[string]oav1beta1.StatusCondition)
				for _, c := range conditions {
					condMap[c.Type] = c
				}
				assert.Len(t, condMap, 2)
				mcCond := condMap["MetricsCollector"]
				assert.EqualValues(t, UpdateFailed, mcCond.Reason)
				assert.Equal(t, metav1.ConditionTrue, mcCond.Status)
				assert.InEpsilon(t, time.Now().Unix(), mcCond.LastTransitionTime.Unix(), 1)
				availCond := condMap["Available"]
				assert.EqualValues(t, ForwardSuccessful, availCond.Reason)
				assert.InEpsilon(t, time.Now().Add(-2*time.Minute).Unix(), availCond.LastTransitionTime.Unix(), 1)
				assert.True(t, wasUpdated)
			},
		},
		"existing status should not be updated if same": {
			currentConditions: []oav1beta1.StatusCondition{
				{
					Type:    "MetricsCollector",
					Reason:  string(ForwardSuccessful),
					Message: "Metrics collector deployed",
					Status:  metav1.ConditionTrue,
					LastTransitionTime: metav1.Time{
						Time: time.Now().Add(-3 * time.Minute), // current state (most recent)
					},
				},
				{
					Type:    "Available",
					Reason:  string(ForwardSuccessful),
					Message: "Metrics collector available",
					Status:  metav1.ConditionTrue,
					LastTransitionTime: metav1.Time{
						Time: time.Now().Add(-2 * time.Minute),
					},
				},
			},
			updateParams: updateParams{
				component: "MetricsCollector",
				reason:    ForwardSuccessful,
				message:   "Metrics collector deployed",
			},
			expects: func(t *testing.T, wasUpdated bool, updateErr error, conditions []oav1beta1.StatusCondition) {
				assert.NoError(t, updateErr)
				condMap := make(map[string]oav1beta1.StatusCondition)
				for _, c := range conditions {
					condMap[c.Type] = c
				}
				assert.Len(t, condMap, 2)
				mcCond := condMap["MetricsCollector"]
				// check that the time has not been updated
				assert.InEpsilon(t, time.Now().Add(-3*time.Minute).Unix(), mcCond.LastTransitionTime.Unix(), 1)
				assert.False(t, wasUpdated)
			},
		},
		"invalid transitions should be rejected": {
			currentConditions: []oav1beta1.StatusCondition{
				{
					Type:    "MetricsCollector",
					Reason:  string(UpdateFailed),
					Message: "Metrics collector broken",
					Status:  metav1.ConditionTrue,
					LastTransitionTime: metav1.Time{
						Time: time.Now().Add(-time.Minute), // current state (most recent)
					},
				},
			},
			updateParams: updateParams{
				component: "MetricsCollector",
				reason:    ForwardSuccessful,
				message:   "Metrics collector is now working",
			},
			expects: func(t *testing.T, wasUpdated bool, resultErr error, conditions []oav1beta1.StatusCondition) {
				assert.Len(t, conditions, 1)
				assert.Error(t, resultErr)
				assert.ErrorIs(t, resultErr, ErrInvalidTransition)
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// setup
			client := fake.NewClientBuilder().WithStatusSubresource(
				&oav1beta1.ObservabilityAddon{},
			).WithScheme(s).Build()
			baseAddon := newObservabilityAddon("observability-addon", "test-ns")
			baseAddon.Status.Conditions = tc.currentConditions
			if err := client.Create(context.Background(), baseAddon); err != nil {
				t.Fatalf("Error creating observabilityaddon: %v", err)
			}

			// test
			statusUpdater := NewStatus(client, baseAddon.Name, baseAddon.Namespace, logr.Logger{})
			wasUpdated, updateErr := statusUpdater.UpdateComponentCondition(context.Background(), tc.updateParams.component, tc.updateParams.reason, tc.updateParams.message)

			newAddon := &oav1beta1.ObservabilityAddon{}
			if err := client.Get(context.Background(), types.NamespacedName{Name: baseAddon.Name, Namespace: baseAddon.Namespace}, newAddon); err != nil {
				t.Fatalf("Error getting observabilityaddon: (%v)", err)
			}
			tc.expects(t, wasUpdated, updateErr, newAddon.Status.Conditions)
		})
	}
}

func TestReportStatus_Conflict(t *testing.T) {
	// Conflict on update should be retried
	name := "observability-addon"
	testNamespace := "test-ns"
	oa := newObservabilityAddon(name, testNamespace)
	s := scheme.Scheme
	oav1beta1.AddToScheme(s)
	fakeClient := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(oa).Build()
	conflictErr := errors.NewConflict(schema.GroupResource{Group: oav1beta1.GroupVersion.Group, Resource: "resource"}, name, fmt.Errorf("conflict"))

	client := newClientWithUpdateError(fakeClient, conflictErr)
	statusUpdater := NewStatus(client, name, testNamespace, logr.Logger{})
	if _, err := statusUpdater.UpdateComponentCondition(context.Background(), "MetricsCollector", UpdateSuccessful, "Metrics collector updated"); err == nil {
		t.Fatalf("Conflict error should be retried and return no error if it succeeds")
	}

	if client.UpdateCallsCount() <= 1 {
		t.Errorf("Conflict error should be retried, called %d times", client.UpdateCallsCount())
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

// TestClient wraps a client.Client to customize operations for testing
type TestClient struct {
	client.Client
	UpdateError      error
	updateCallsCount int
	statusWriter     *TestStatusWriter
}

func newClientWithUpdateError(c client.Client, updateError error) *TestClient {
	ret := &TestClient{
		Client:      c,
		UpdateError: updateError,
	}
	ret.statusWriter = &TestStatusWriter{SubResourceWriter: c.Status(), updateError: &ret.UpdateError, callsCount: &ret.updateCallsCount}
	return ret
}

func (c *TestClient) Status() client.StatusWriter {
	return c.statusWriter
}

func (c *TestClient) UpdateCallsCount() int {
	return c.updateCallsCount
}

func (c *TestClient) Reset() {
	c.updateCallsCount = 0
}

type TestStatusWriter struct {
	client.SubResourceWriter
	updateError *error
	callsCount  *int
}

func (f *TestStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	*f.callsCount++

	if *f.updateError != nil {
		return *f.updateError
	}

	return f.SubResourceWriter.Update(ctx, obj, opts...)
}
