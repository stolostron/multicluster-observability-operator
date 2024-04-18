// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package status_test

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/go-logr/logr"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/status"
	oashared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
)

const (
	name             = "observability-addon"
	testNamespace    = "test-ns"
	testHubNamespace = "test-hub-ns"
	obAddonName      = "observability-addon"
)

func TestStatusController_NominalCase(t *testing.T) {
	spokeOba := newObservabilityAddon(name, testNamespace)
	c := newClient(spokeOba)

	hubOba := newObservabilityAddon(name, testHubNamespace)
	hubOba.Spec.Interval = 12341 // add variation in the spec, not status
	custumHubClient := newClientWithUpdateError(newClient(hubOba), nil, nil)
	hubClient := &status.ClientWithReload{
		Client:    custumHubClient,
		Generator: func() (client.Client, error) { return nil, nil }, // no reload
	}
	r := newStatusReconciler(c, hubClient)

	// no status difference triggers no update
	resp, err := r.Reconcile(context.Background(), newRequest())
	if err != nil {
		t.Fatalf("Failed to reconcile: %v", err)
	}
	if !reflect.DeepEqual(resp, ctrl.Result{}) {
		t.Fatalf("Expected no requeue")
	}
	if custumHubClient.UpdateCallsCount() > 0 {
		t.Fatalf("Expected no update")
	}

	// update status in spoke
	addCondition(spokeOba, "Deployed", metav1.ConditionTrue)
	err = c.Update(context.Background(), spokeOba)
	if err != nil {
		t.Fatalf("Failed to update status in spoke: %v", err)
	}

	// status difference should trigger update in hub
	resp, err = r.Reconcile(context.Background(), newRequest())
	if err != nil {
		t.Fatalf("Failed to reconcile: %v", err)
	}
	if !reflect.DeepEqual(resp, ctrl.Result{}) {
		t.Fatalf("Expected no requeue")
	}
	if custumHubClient.UpdateCallsCount() != 1 {
		t.Fatalf("Expected update")
	}

	// check status in hub
	hubObsAddon := &oav1beta1.ObservabilityAddon{}
	err = hubClient.Get(context.Background(), types.NamespacedName{Name: obAddonName, Namespace: testHubNamespace}, hubObsAddon)
	if err != nil {
		t.Fatalf("Failed to get oba in hub: %v", err)
	}
	if !reflect.DeepEqual(hubObsAddon.Status.Conditions, spokeOba.Status.Conditions) {
		t.Fatalf("Status not updated in hub: %v", hubObsAddon.Status)
	}
}

func TestStatusController_UpdateHubAddonFailures(t *testing.T) {
	spokeOba := newObservabilityAddon(name, testNamespace)
	addCondition(spokeOba, "Deployed", metav1.ConditionTrue) // add status to trigger update
	c := newClient(spokeOba)

	hubOba := newObservabilityAddon(name, testHubNamespace)
	var updateErr error
	hubClientWithConflict := newClientWithUpdateError(newClient(hubOba), updateErr, nil)
	hubClient := &status.ClientWithReload{
		Client:    hubClientWithConflict,
		Generator: func() (client.Client, error) { return nil, nil }, // no reload
	}
	r := newStatusReconciler(c, hubClient)

	testCases := map[string]struct {
		updateErr       error
		reconcileErr    error
		requeue         bool
		requeueAfter    bool
		requeueAfterVal int
		updateCallsMin  int
		updateCallsMax  int
	}{
		"Conflict": {
			updateErr:      errors.NewConflict(schema.GroupResource{Group: oav1beta1.GroupVersion.Group, Resource: "FakeResource"}, name, fmt.Errorf("fake conflict")),
			requeueAfter:   true,
			updateCallsMin: 1,
		},
		"Server unavailable": {
			updateErr:      errors.NewServiceUnavailable("service unavailable"),
			requeueAfter:   true,
			updateCallsMax: 1,
		},
		"internal error": {
			updateErr: errors.NewInternalError(fmt.Errorf("internal error")),
			// reconcileErr:   errors.NewInternalError(fmt.Errorf("fake internal error")),
			updateCallsMax: 1,
			requeueAfter:   true,
		},
		"Permanent error": {
			updateErr:      errors.NewBadRequest("bad request"),
			reconcileErr:   errors.NewBadRequest("bad request"),
			updateCallsMax: 1,
		},
		"Too many requests": {
			updateErr:       errors.NewTooManyRequests("too many requests", 10),
			requeueAfter:    true,
			requeueAfterVal: 10,
			updateCallsMax:  1,
		},
		"Network error": {
			updateErr: &net.DNSError{
				Err: "network error",
			},
			requeueAfter:   true,
			updateCallsMax: 1,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			hubClientWithConflict.UpdateError = tc.updateErr
			hubClientWithConflict.Reset()
			resp, err := r.Reconcile(context.Background(), newRequest())
			if (tc.reconcileErr != nil && err == nil) || (tc.reconcileErr == nil && err != nil) {
				t.Fatalf("Invalid reconcile error: got %v, expected %v", err, tc.reconcileErr)
			}
			if tc.requeue != resp.Requeue {
				t.Fatalf("Invalid requeue: got %v, expected %v", resp.Requeue, tc.requeue)
			}
			if tc.requeueAfter != (resp.RequeueAfter > 0) {
				t.Fatalf("Invalid requeue after: got %v, expected %v", resp.RequeueAfter > 0, tc.requeueAfter)
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
	spokeOba := newObservabilityAddon(name, testNamespace)
	addCondition(spokeOba, "Deployed", metav1.ConditionTrue) // add status to trigger update
	c := newClient(spokeOba)

	hubOba := newObservabilityAddon(name, testHubNamespace)
	hubClientWithConflict := newClientWithUpdateError(newClient(hubOba), nil, nil)
	var reloadCount int
	hubClient := &status.ClientWithReload{
		Client: hubClientWithConflict,
		Generator: func() (client.Client, error) {
			reloadCount++
			return hubClientWithConflict, nil
		},
	}
	r := newStatusReconciler(c, hubClient)

	testCases := map[string]struct {
		getErr          error
		reconcileErr    error
		requeue         bool
		requeueAfter    bool
		requeueAfterVal int
		reloadCount     int
	}{
		"Unauthorized": {
			getErr:       errors.NewUnauthorized("unauthorized"),
			requeueAfter: true,
			reloadCount:  1,
		},
		"Permanent error": {
			getErr:       errors.NewBadRequest("bad request"),
			reconcileErr: errors.NewBadRequest("bad request"),
		},
		"Servers unavailable": {
			getErr:       errors.NewServiceUnavailable("service unavailable"),
			requeueAfter: true,
		},
		"Too many requests": {
			getErr:          errors.NewTooManyRequests("too many requests", 10),
			requeueAfter:    true,
			requeueAfterVal: 10,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			hubClientWithConflict.GetError = tc.getErr
			reloadCount = 0
			// hubClientWithConflict.Reset()
			resp, err := r.Reconcile(context.Background(), newRequest())
			if (tc.reconcileErr != nil && err == nil) || (tc.reconcileErr == nil && err != nil) {
				t.Fatalf("Invalid reconcile error: got %v, expected %v", err, tc.reconcileErr)
			}
			if tc.requeue != resp.Requeue {
				t.Fatalf("Invalid requeue: got %v, expected %v", resp.Requeue, tc.requeue)
			}
			if tc.requeueAfter != (resp.RequeueAfter > 0) {
				t.Fatalf("Invalid requeue after: got %v, expected %v", resp.RequeueAfter > 0, tc.requeueAfter)
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

func newClient(objs ...runtime.Object) client.Client {
	s := scheme.Scheme
	addonv1alpha1.AddToScheme(s)
	oav1beta1.AddToScheme(s)

	return fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objs...).Build()
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
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: oashared.ObservabilityAddonSpec{
			EnableMetrics: true,
			Interval:      60,
		},
	}
}

func addCondition(oba *oav1beta1.ObservabilityAddon, statusType string, status metav1.ConditionStatus) {
	condition := oav1beta1.StatusCondition{
		Type:    statusType,
		Status:  status,
		Reason:  "DummyReason",
		Message: "DummyMessage",
	}
	oba.Status.Conditions = append(oba.Status.Conditions, condition)
}

func newRequest() ctrl.Request {
	return ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "install",
			Namespace: testNamespace,
		},
	}
}

func newStatusReconciler(c client.Client, hubClient *status.ClientWithReload) *status.StatusReconciler {
	return &status.StatusReconciler{
		Client:       c,
		HubClient:    hubClient,
		Namespace:    testNamespace,
		HubNamespace: testHubNamespace,
		ObsAddonName: obAddonName,
		Logger:       logr.Discard(),
	}
}
