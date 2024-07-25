// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package status_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/status"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/util"
	oashared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
)

func TestStatusController_HubNominalCase(t *testing.T) {
	addonName := "observability-addon"
	addonNamespace := "test-ns"
	spokeOba := newObservabilityAddon(addonName, addonNamespace)
	spokeClient := newClient(spokeOba)

	addonHubNamespace := "test-ns"
	hubOba := newObservabilityAddon(addonName, addonHubNamespace)
	hubOba.Spec.Interval = 12341 // add variation in the spec, not status
	custumHubClient := newClientWithUpdateError(newClient(hubOba), nil, nil)
	reloadableHubClient, err := util.NewReloadableHubClientWithReloadFunc(func() (client.Client, error) { return custumHubClient, nil })
	if err != nil {
		t.Fatalf("Failed to create reloadable hub client: %v", err)
	}
	statusReconciler := &status.StatusReconciler{
		Client:       spokeClient,
		HubClient:    reloadableHubClient,
		Namespace:    addonNamespace,
		HubNamespace: addonHubNamespace,
		ObsAddonName: addonName,
		Logger:       logr.Discard(),
	}

	// no status difference triggers no update
	resp, err := statusReconciler.Reconcile(context.Background(), ctrl.Request{})
	if err != nil {
		t.Fatalf("Failed to reconcile: %v", err)
	}
	if !resp.IsZero() {
		t.Fatalf("Expected no requeue")
	}
	if custumHubClient.UpdateCallsCount() > 0 {
		t.Fatalf("Expected no update")
	}

	// update status in spoke
	spokeOba.Status.Conditions = append(spokeOba.Status.Conditions, oav1beta1.StatusCondition{
		Type: "Available",
	})
	err = spokeClient.Status().Update(context.Background(), spokeOba)

	if err != nil {
		t.Fatalf("Failed to update status in spoke: %v", err)
	}

	// status difference should trigger update in hub
	resp, err = statusReconciler.Reconcile(context.Background(), ctrl.Request{})
	if err != nil {
		t.Fatalf("Failed to reconcile: %v", err)
	}
	if !resp.IsZero() {
		t.Fatalf("Expected no requeue")
	}
	if custumHubClient.UpdateCallsCount() != 1 {
		t.Fatalf("Expected update")
	}

	// check status in hub
	hubObsAddon := &oav1beta1.ObservabilityAddon{}
	err = custumHubClient.Get(context.Background(), types.NamespacedName{Name: addonName, Namespace: addonHubNamespace}, hubObsAddon)
	if err != nil {
		t.Fatalf("Failed to get oba in hub: %v", err)
	}
	if !reflect.DeepEqual(hubObsAddon.Status.Conditions, spokeOba.Status.Conditions) {
		t.Fatalf("Status not updated in hub: %v", hubObsAddon.Status)
	}
}

func TestStatusController_UpdateHubAddonFailures(t *testing.T) {
	addonName := "observability-addon"
	addonNamespace := "test-ns"
	spokeOba := newObservabilityAddon(addonName, addonNamespace)
	// add status to trigger update
	spokeOba.Status.Conditions = append(spokeOba.Status.Conditions, oav1beta1.StatusCondition{
		Type: "Available",
	})
	spokeClient := newClient(spokeOba)

	addonHubNamespace := "test-ns"
	hubOba := newObservabilityAddon(addonName, addonHubNamespace)
	var updateErr error
	hubClientWithConflict := newClientWithUpdateError(newClient(hubOba), updateErr, nil)
	reloadableHubClient, err := util.NewReloadableHubClientWithReloadFunc(func() (client.Client, error) { return hubClientWithConflict, nil })
	if err != nil {
		t.Fatalf("Failed to create reloadable hub client: %v", err)
	}
	statusReconciler := &status.StatusReconciler{
		Client:       spokeClient,
		HubClient:    reloadableHubClient,
		Namespace:    addonNamespace,
		HubNamespace: addonHubNamespace,
		ObsAddonName: addonName,
		Logger:       logr.Discard(),
	}

	testCases := map[string]struct {
		updateErr       error
		terminalErr     bool
		requeue         bool
		requeueAfterVal int
		updateCallsMin  int
		updateCallsMax  int
	}{
		"Conflict": {
			updateErr:      apiErrors.NewConflict(schema.GroupResource{Group: oav1beta1.GroupVersion.Group, Resource: "FakeResource"}, addonName, fmt.Errorf("fake conflict")),
			requeue:        true,
			updateCallsMin: 1,
		},
		"Server unavailable": {
			updateErr:      apiErrors.NewServiceUnavailable("service unavailable"),
			requeue:        true,
			updateCallsMax: 1,
		},
		"internal error": {
			updateErr:      apiErrors.NewInternalError(fmt.Errorf("internal error")),
			updateCallsMax: 1,
			requeue:        true,
		},
		"Permanent error": {
			updateErr:      apiErrors.NewBadRequest("bad request"),
			terminalErr:    true,
			updateCallsMax: 1,
		},
		"Too many requests": {
			updateErr:       apiErrors.NewTooManyRequests("too many requests", 10),
			requeue:         true,
			requeueAfterVal: 10,
			updateCallsMax:  1,
		},
		"Network error": {
			updateErr: &net.DNSError{
				Err: "network error",
			},
			requeue:        true,
			updateCallsMax: 1,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			hubClientWithConflict.UpdateError = tc.updateErr
			hubClientWithConflict.Reset()
			resp, err := statusReconciler.Reconcile(context.Background(), ctrl.Request{})
			isTerminalErr := errors.Is(err, reconcile.TerminalError(nil))
			if tc.terminalErr != isTerminalErr {
				t.Fatalf("Invalid reconcile error: got %v, expected %v", err, tc.terminalErr)
			}

			isRequeued := (!resp.IsZero() && err == nil) || (err != nil && !errors.Is(err, reconcile.TerminalError(nil)))
			if tc.requeue != isRequeued {
				t.Fatalf("Expected requeue")
			}

			if tc.requeueAfterVal > 0 && int(resp.RequeueAfter.Seconds()) != tc.requeueAfterVal {
				t.Fatalf("Invalid requeue after value: got %v, expected %v", int(resp.RequeueAfter.Seconds()), tc.requeueAfterVal)
			}
			if tc.updateCallsMin > 0 && hubClientWithConflict.UpdateCallsCount() < tc.updateCallsMin {
				t.Fatalf("Expected update retry min %d times, got %d", tc.updateCallsMin, hubClientWithConflict.UpdateCallsCount())
			}
			if tc.updateCallsMax > 0 && hubClientWithConflict.UpdateCallsCount() > tc.updateCallsMax {
				t.Fatalf("Expected update retry at most %d times, got %d", tc.updateCallsMax, hubClientWithConflict.UpdateCallsCount())
			}
		})
	}
}

func TestStatusController_GetHubAddonFailures(t *testing.T) {
	addonName := "observability-addon"
	addonNamespace := "test-ns"
	spokeOba := newObservabilityAddon(addonName, addonNamespace)
	// add status to trigger update
	spokeOba.Status.Conditions = append(spokeOba.Status.Conditions, oav1beta1.StatusCondition{
		Type: "Available",
	})
	spokeClient := newClient(spokeOba)

	addonHubNamespace := "test-ns"
	hubOba := newObservabilityAddon(addonName, addonHubNamespace)
	hubClientWithConflict := newClientWithUpdateError(newClient(hubOba), nil, nil)

	var reloadCount int
	reloadableHubClient, err := util.NewReloadableHubClientWithReloadFunc(func() (client.Client, error) {
		reloadCount++
		return hubClientWithConflict, nil
	})
	if err != nil {
		t.Fatalf("Failed to create reloadable hub client: %v", err)
	}
	statusReconciler := &status.StatusReconciler{
		Client:       spokeClient,
		HubClient:    reloadableHubClient,
		Namespace:    addonNamespace,
		HubNamespace: addonHubNamespace,
		ObsAddonName: addonName,
		Logger:       logr.Discard(),
	}

	testCases := map[string]struct {
		getErr          error
		terminalErr     bool
		requeue         bool
		requeueAfterVal int
		reloadCount     int
	}{
		"Unauthorized": {
			getErr:      apiErrors.NewUnauthorized("unauthorized"),
			requeue:     true,
			reloadCount: 1,
		},
		"Permanent error": {
			getErr:      apiErrors.NewBadRequest("bad request"),
			terminalErr: true,
		},
		"Servers unavailable": {
			getErr:  apiErrors.NewServiceUnavailable("service unavailable"),
			requeue: true,
		},
		"Too many requests": {
			getErr:          apiErrors.NewTooManyRequests("too many requests", 10),
			requeue:         true,
			requeueAfterVal: 10,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			hubClientWithConflict.GetError = tc.getErr
			reloadCount = 0
			resp, err := statusReconciler.Reconcile(context.Background(), ctrl.Request{})
			isTerminalErr := errors.Is(err, reconcile.TerminalError(nil))
			if tc.terminalErr != isTerminalErr {
				t.Fatalf("Invalid reconcile error: got %v, expected %v", err, tc.terminalErr)
			}
			isRequeued := (!resp.IsZero() && err == nil) || (err != nil && !errors.Is(err, reconcile.TerminalError(nil)))
			if tc.requeue != isRequeued {
				t.Fatalf("Expected requeue")
			}
			if tc.requeueAfterVal > 0 && int(resp.RequeueAfter.Seconds()) != tc.requeueAfterVal {
				t.Fatalf("Invalid requeue after value: got %v, expected %v", int(resp.RequeueAfter.Seconds()), tc.requeueAfterVal)
			}
			if tc.reloadCount != reloadCount {
				t.Fatalf("Expected reload %d times, got %d", tc.reloadCount, reloadCount)
			}
		})
	}
}

func TestStatusController_UpdateSpokeAddon(t *testing.T) {
	addonName := "observability-addon"
	addonNamespace := "test-ns"

	addonHubNamespace := "test-ns"
	hubOba := newObservabilityAddon(addonName, addonHubNamespace)
	var updateErr error
	hubClientWithConflict := newClientWithUpdateError(newClient(hubOba), updateErr, nil)
	reloadableHubClient, err := util.NewReloadableHubClientWithReloadFunc(func() (client.Client, error) { return hubClientWithConflict, nil })
	if err != nil {
		t.Fatalf("Failed to create reloadable hub client: %v", err)
	}
	availableMsg := "observability-controller add-on is available."

	newCondition := func(t, r, m string, status metav1.ConditionStatus, lastTransitionTime time.Time) oav1beta1.StatusCondition {
		return oav1beta1.StatusCondition{
			Type:               t,
			Reason:             r,
			Message:            m,
			Status:             status,
			LastTransitionTime: metav1.NewTime(lastTransitionTime),
		}
	}

	testCases := map[string]struct {
		spokeAddonConditions []oav1beta1.StatusCondition
		expectConditions     []oav1beta1.StatusCondition
	}{
		"no condition": {
			spokeAddonConditions: []oav1beta1.StatusCondition{},
			expectConditions:     []oav1beta1.StatusCondition{},
		},
		"no component condition": {
			spokeAddonConditions: []oav1beta1.StatusCondition{
				newCondition("Available", "ForwardSuccessful", "MetricsCollector: Metrics sent", metav1.ConditionTrue, time.Now().Add(-time.Minute)),
			},
			expectConditions: []oav1beta1.StatusCondition{
				newCondition("Available", "ForwardSuccessful", "MetricsCollector: Metrics sent", metav1.ConditionTrue, time.Now().Add(-time.Minute)),
			},
		},
		"single component aggregation": {
			spokeAddonConditions: []oav1beta1.StatusCondition{
				newCondition("MetricsCollector", "ForwardSuccessful", "Metrics sent", metav1.ConditionTrue, time.Now()),
			},
			expectConditions: []oav1beta1.StatusCondition{
				newCondition("MetricsCollector", "ForwardSuccessful", "Metrics sent", metav1.ConditionTrue, time.Now()),
				newCondition("Available", "ForwardSuccessful", availableMsg, metav1.ConditionTrue, time.Now()),
			},
		},
		"multi aggregation with same reason": {
			spokeAddonConditions: []oav1beta1.StatusCondition{
				newCondition("MetricsCollector", "ForwardSuccessful", "Metrics sent", metav1.ConditionTrue, time.Now()),
				newCondition("UwlMetricsCollector", "ForwardSuccessful", "Metrics sent", metav1.ConditionTrue, time.Now()),
			},
			expectConditions: []oav1beta1.StatusCondition{
				newCondition("MetricsCollector", "ForwardSuccessful", "Metrics sent", metav1.ConditionTrue, time.Now()),
				newCondition("UwlMetricsCollector", "ForwardSuccessful", "Metrics sent", metav1.ConditionTrue, time.Now()),
				newCondition("Available", "ForwardSuccessful", availableMsg, metav1.ConditionTrue, time.Now()),
			},
		},
		"multi aggregation with highest priority reason": {
			spokeAddonConditions: []oav1beta1.StatusCondition{
				newCondition("MetricsCollector", "ForwardSuccessful", "Metrics sent", metav1.ConditionTrue, time.Now()),
				newCondition("UwlMetricsCollector", "ForwardFailed", "Metrics failed", metav1.ConditionTrue, time.Now()),
			},
			expectConditions: []oav1beta1.StatusCondition{
				newCondition("MetricsCollector", "ForwardSuccessful", "Metrics sent", metav1.ConditionTrue, time.Now()),
				newCondition("UwlMetricsCollector", "ForwardFailed", "Metrics failed", metav1.ConditionTrue, time.Now()),
				newCondition("Degraded", "ForwardFailed", "UwlMetricsCollector: Metrics failed; MetricsCollector: Metrics sent", metav1.ConditionTrue, time.Now()),
			},
		},
		"conditions are not updated if they are the same": {
			spokeAddonConditions: []oav1beta1.StatusCondition{
				newCondition("MetricsCollector", "ForwardSuccessful", "Metrics sent", metav1.ConditionTrue, time.Now()),
				newCondition("UwlMetricsCollector", "ForwardFailed", "Metrics failed", metav1.ConditionTrue, time.Now()),
				newCondition("Degraded", "ForwardFailed", "UwlMetricsCollector: Metrics failed; MetricsCollector: Metrics sent", metav1.ConditionTrue, time.Now().Add(-time.Minute)),
				newCondition("Available", "ForwardSuccessful", "", metav1.ConditionFalse, time.Now()),
			},
			expectConditions: []oav1beta1.StatusCondition{
				newCondition("MetricsCollector", "ForwardSuccessful", "Metrics sent", metav1.ConditionTrue, time.Now()),
				newCondition("UwlMetricsCollector", "ForwardFailed", "Metrics failed", metav1.ConditionTrue, time.Now()),
				newCondition("Available", "ForwardSuccessful", "", metav1.ConditionFalse, time.Now()),
				newCondition("Degraded", "ForwardFailed", "UwlMetricsCollector: Metrics failed; MetricsCollector: Metrics sent", metav1.ConditionTrue, time.Now().Add(-time.Minute)),
			},
		},
		"status is updated if the condition is different": {
			spokeAddonConditions: []oav1beta1.StatusCondition{
				newCondition("MetricsCollector", "ForwardFailed", "Metrics failed", metav1.ConditionTrue, time.Now()),
				newCondition("Available", "ForwardSuccessful", "MetricsCollector: Metrics sent", metav1.ConditionTrue, time.Now().Add(-time.Minute)),
			},
			expectConditions: []oav1beta1.StatusCondition{
				newCondition("MetricsCollector", "ForwardFailed", "Metrics failed", metav1.ConditionTrue, time.Now()),
				newCondition("Available", "ForwardSuccessful", "MetricsCollector: Metrics sent", metav1.ConditionFalse, time.Now().Add(-time.Minute)),
				newCondition("Degraded", "ForwardFailed", "MetricsCollector: Metrics failed", metav1.ConditionTrue, time.Now()),
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			spokeOba := newObservabilityAddon(addonName, addonNamespace)
			spokeOba.Status.Conditions = tc.spokeAddonConditions
			spokeClient := newClient(spokeOba)
			statusReconciler := &status.StatusReconciler{
				Client:       spokeClient,
				HubClient:    reloadableHubClient,
				Namespace:    addonNamespace,
				HubNamespace: addonHubNamespace,
				ObsAddonName: addonName,
				Logger:       logr.Discard(),
			}

			resp, err := statusReconciler.Reconcile(context.Background(), ctrl.Request{})
			assert.NoError(t, err)
			assert.True(t, resp.IsZero())

			newSpokeOba := &oav1beta1.ObservabilityAddon{}
			err = spokeClient.Get(context.Background(), types.NamespacedName{Name: addonName, Namespace: addonNamespace}, newSpokeOba)
			if err != nil {
				t.Fatalf("Failed to get oba in spoke: %v", err)
			}

			assert.Equal(t, len(tc.expectConditions), len(newSpokeOba.Status.Conditions))

			sort.Slice(newSpokeOba.Status.Conditions, func(i, j int) bool {
				return newSpokeOba.Status.Conditions[i].Type < newSpokeOba.Status.Conditions[j].Type
			})
			sort.Slice(tc.expectConditions, func(i, j int) bool {
				return tc.expectConditions[i].Type < tc.expectConditions[j].Type
			})
			for i := range tc.expectConditions {
				assert.Equal(t, tc.expectConditions[i].Type, newSpokeOba.Status.Conditions[i].Type)
				assert.Equal(t, tc.expectConditions[i].Reason, newSpokeOba.Status.Conditions[i].Reason)
				assert.Equal(t, tc.expectConditions[i].Message, newSpokeOba.Status.Conditions[i].Message)
				assert.Equal(t, tc.expectConditions[i].Status, newSpokeOba.Status.Conditions[i].Status)
				assert.WithinDuration(t, tc.expectConditions[i].LastTransitionTime.Time, newSpokeOba.Status.Conditions[i].LastTransitionTime.Time, time.Second)
			}
		})
	}
}

func newClient(objs ...runtime.Object) client.Client {
	s := scheme.Scheme
	addonv1alpha1.AddToScheme(s)
	oav1beta1.AddToScheme(s)

	return fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(objs...).
		WithStatusSubresource(
			&addonv1alpha1.ManagedClusterAddOn{},
			&oav1beta1.MultiClusterObservability{},
			&oav1beta1.ObservabilityAddon{},
		).
		Build()
}

// TestClient wraps a client.Client to customize operations for testing
type TestClient struct {
	client.Client
	UpdateError      error
	GetError         error
	updateCallsCount int
	statusWriter     *TestStatusWriter
}

func newClientWithUpdateError(c client.Client, updateError, getError error) *TestClient {
	ret := &TestClient{
		Client:      c,
		UpdateError: updateError,
		GetError:    getError,
	}
	ret.statusWriter = &TestStatusWriter{SubResourceWriter: c.Status(), updateError: &ret.UpdateError, callsCount: &ret.updateCallsCount}
	return ret
}

func (c *TestClient) Status() client.StatusWriter {
	return c.statusWriter
}

func (c *TestClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if c.GetError != nil {
		return c.GetError
	}
	return c.Client.Get(ctx, key, obj)
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

func newObservabilityAddon(name string, ns string) *oav1beta1.ObservabilityAddon {
	return &oav1beta1.ObservabilityAddon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: oashared.ObservabilityAddonSpec{
			EnableMetrics: true,
			Interval:      60,
		},
	}
}
