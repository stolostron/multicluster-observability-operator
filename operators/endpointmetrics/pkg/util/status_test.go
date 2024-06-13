// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/util"
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

func TestReportStatus(t *testing.T) {
	s := scheme.Scheme
	assert.NoError(t, oav1beta1.AddToScheme(s))
	assert.NoError(t, addonv1alpha1.AddToScheme(s))
	assert.NoError(t, mcov1beta2.AddToScheme(s))

	testCases := map[string]struct {
		currentConditions []oav1beta1.StatusCondition
		newCondition      util.ConditionReason
		expects           func(*testing.T, []oav1beta1.StatusCondition)
	}{
		"new status should be appended": {
			currentConditions: []oav1beta1.StatusCondition{},
			newCondition:      util.Deployed,
			expects: func(t *testing.T, conditions []oav1beta1.StatusCondition) {
				assert.Len(t, conditions, 1)
				assert.EqualValues(t, util.Deployed, conditions[0].Reason)
				assert.Equal(t, metav1.ConditionTrue, conditions[0].Status)
				assert.Equal(t, "Progressing", conditions[0].Type)
				assert.InEpsilon(t, time.Now().Unix(), conditions[0].LastTransitionTime.Unix(), 1)
			},
		},
		"existing status should be updated": {
			currentConditions: []oav1beta1.StatusCondition{
				{
					Type:    "Progressing",
					Reason:  string(util.Deployed),
					Message: "Metrics collector deployed",
					Status:  metav1.ConditionTrue,
					LastTransitionTime: metav1.Time{
						Time: time.Now().Add(-time.Minute), // current state (most recent)
					},
				},
				{
					Type:    "Disabled",
					Reason:  string(util.Disabled),
					Message: "enableMetrics is set to False",
					Status:  metav1.ConditionTrue,
					LastTransitionTime: metav1.Time{
						Time: time.Now().Add(-2 * time.Minute),
					},
				},
			},
			newCondition: util.Disabled,
			expects: func(t *testing.T, conditions []oav1beta1.StatusCondition) {
				assert.Len(t, conditions, 2)
				found := false
				for _, c := range conditions {
					if c.Reason == string(util.Disabled) {
						found = true
						assert.EqualValues(t, util.Disabled, c.Reason)
						assert.Equal(t, metav1.ConditionTrue, c.Status)
						assert.Equal(t, "Disabled", c.Type)
						assert.InEpsilon(t, time.Now().Unix(), c.LastTransitionTime.Unix(), 1)
					} else {
						// other condition should not be changed
						assert.EqualValues(t, util.Deployed, c.Reason)
						assert.InEpsilon(t, time.Now().Add(-time.Minute).Unix(), c.LastTransitionTime.Unix(), 1)
					}
				}
				assert.True(t, found, "condition not found")
			},
		},
		"existing status should not be updated if same": {
			currentConditions: []oav1beta1.StatusCondition{
				{
					Type:    "Progressing",
					Reason:  string(util.Deployed),
					Message: "Metrics collector deployed",
					Status:  metav1.ConditionTrue,
					LastTransitionTime: metav1.Time{
						Time: time.Now().Add(-time.Minute), // current state (most recent)
					},
				},
				{
					Type: "Disabled",
					LastTransitionTime: metav1.Time{
						Time: time.Now().Add(-2 * time.Minute),
					},
				},
			},
			newCondition: util.Deployed,
			expects: func(t *testing.T, conditions []oav1beta1.StatusCondition) {
				assert.Len(t, conditions, 2)
				assert.EqualValues(t, util.Deployed, conditions[0].Reason)
				assert.InEpsilon(t, time.Now().Add(-time.Minute).Unix(), conditions[0].LastTransitionTime.Unix(), 1)
			},
		},
		"number of conditions should not exceed MaxStatusConditionsCount": {
			currentConditions: []oav1beta1.StatusCondition{
				{Type: "1"}, {Type: "2"}, {Type: "3"}, {Type: "4"}, {Type: "5"},
				{Type: "6"}, {Type: "7"}, {Type: "8"}, {Type: "9"}, {Type: "10"},
			},
			newCondition: util.Deployed,
			expects: func(t *testing.T, conditions []oav1beta1.StatusCondition) {
				assert.Len(t, conditions, util.MaxStatusConditionsCount)
				assert.EqualValues(t, util.Deployed, conditions[len(conditions)-1].Reason)
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
			if err := util.ReportStatus(context.Background(), client, tc.newCondition, baseAddon.Name, baseAddon.Namespace); err != nil {
				t.Fatalf("Error reporting status: %v", err)
			}
			newAddon := &oav1beta1.ObservabilityAddon{}
			if err := client.Get(context.Background(), types.NamespacedName{Name: baseAddon.Name, Namespace: baseAddon.Namespace}, newAddon); err != nil {
				t.Fatalf("Error getting observabilityaddon: (%v)", err)
			}
			tc.expects(t, newAddon.Status.Conditions)

			// cleanup
			if err := client.Delete(context.Background(), newAddon); err != nil {
				t.Fatalf("Error deleting observabilityaddon: %v", err)
			}
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

	c := newClientWithUpdateError(fakeClient, conflictErr)
	if err := util.ReportStatus(context.Background(), c, util.Deployed, name, testNamespace); err == nil {
		t.Fatalf("Conflict error should be retried and return an error if it fails")
	}
	if c.UpdateCallsCount() <= 1 {
		t.Errorf("Conflict error should be retried, called %d times", c.UpdateCallsCount())
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
