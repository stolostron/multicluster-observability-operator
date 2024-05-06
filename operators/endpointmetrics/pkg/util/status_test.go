// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/util"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	name          = "observability-addon"
	testNamespace = "test-ns"
)

func newObservabilityAddon(name string, ns string) *oav1beta1.ObservabilityAddon {
	return &oav1beta1.ObservabilityAddon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
}

func TestReportStatus(t *testing.T) {
	oa := newObservabilityAddon(name, testNamespace)
	objs := []runtime.Object{oa}
	s := scheme.Scheme
	if err := oav1beta1.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add oav1beta1 scheme: (%v)", err)
	}

	// New status should be appended
	statusList := []util.StatusConditionName{util.NotSupportedStatus, util.DeployedStatus, util.DisabledStatus}
	s.AddKnownTypes(oav1beta1.GroupVersion, oa)
	c := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
	for i := range statusList {
		if err := util.ReportStatus(context.Background(), c, statusList[i], oa.Name, oa.Namespace); err != nil {
			t.Fatalf("Error reporting status: %v", err)
		}
		runtimeAddon := &oav1beta1.ObservabilityAddon{}
		if err := c.Get(context.Background(), types.NamespacedName{Name: name, Namespace: testNamespace}, runtimeAddon); err != nil {
			t.Fatalf("Error getting observabilityaddon: (%v)", err)
		}

		if len(runtimeAddon.Status.Conditions) != i+1 {
			t.Errorf("Status not updated. Expected: %s, Actual: %s", statusList[i], fmt.Sprintf("%+v\n", runtimeAddon.Status.Conditions))
		}

		if runtimeAddon.Status.Conditions[i].Reason != string(statusList[i]) {
			t.Errorf("Status not updated. Expected: %s, Actual: %s", statusList[i], runtimeAddon.Status.Conditions[i].Type)
		}
	}

	// Same status than current one should not be appended
	if err := util.ReportStatus(context.Background(), c, util.DisabledStatus, oa.Name, oa.Namespace); err != nil {
		t.Fatalf("Error reporting status: %v", err)
	}
	runtimeAddon := &oav1beta1.ObservabilityAddon{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: name, Namespace: testNamespace}, runtimeAddon); err != nil {
		t.Fatalf("Error getting observabilityaddon: (%v)", err)
	}

	if len(runtimeAddon.Status.Conditions) != len(statusList) {
		t.Errorf("Status should not be appended. Expected: %d, Actual: %d", len(statusList), len(runtimeAddon.Status.Conditions))
	}

	// Number of conditions should not exceed MaxStatusConditionsCount
	statusList = []util.StatusConditionName{util.DeployedStatus, util.DisabledStatus, util.DegradedStatus}
	for i := 0; i < util.MaxStatusConditionsCount+3; i++ {
		status := statusList[i%len(statusList)]
		if err := util.ReportStatus(context.Background(), c, status, oa.Name, oa.Namespace); err != nil {
			t.Fatalf("Error reporting status: %v", err)
		}
	}

	runtimeAddon = &oav1beta1.ObservabilityAddon{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: name, Namespace: testNamespace}, runtimeAddon); err != nil {
		t.Fatalf("Error getting observabilityaddon: (%v)", err)
	}

	if len(runtimeAddon.Status.Conditions) != util.MaxStatusConditionsCount {
		t.Errorf("Number of conditions should not exceed MaxStatusConditionsCount. Expected: %d, Actual: %d", util.MaxStatusConditionsCount, len(runtimeAddon.Status.Conditions))
	}
}

func TestReportStatus_Conflict(t *testing.T) {
	// Conflict on update should be retried
	oa := newObservabilityAddon(name, testNamespace)
	s := scheme.Scheme
	oav1beta1.AddToScheme(s)
	fakeClient := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(oa).Build()
	conflictErr := errors.NewConflict(schema.GroupResource{Group: oav1beta1.GroupVersion.Group, Resource: "resource"}, name, fmt.Errorf("conflict"))

	c := newClientWithUpdateError(fakeClient, conflictErr)
	if err := util.ReportStatus(context.Background(), c, util.DeployedStatus, name, testNamespace); err == nil {
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
