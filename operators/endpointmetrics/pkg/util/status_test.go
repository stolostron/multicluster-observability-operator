// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project.
package util

import (
	"context"
	"fmt"
	"testing"

	oav1beta1 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
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

	expectedStatus := []oav1beta1.StatusCondition{
		{
			Type:    "NotSupported",
			Status:  metav1.ConditionTrue,
			Reason:  "NotSupported",
			Message: "No Prometheus service found in this cluster",
		},
		{
			Type:    "Progressing",
			Status:  metav1.ConditionTrue,
			Reason:  "Deployed",
			Message: "Metrics collector deployed",
		},
		{
			Type:    "Disabled",
			Status:  metav1.ConditionTrue,
			Reason:  "Disabled",
			Message: "enableMetrics is set to False",
		},
	}

	statusList := []string{"NotSupported", "Deployed", "Disabled"}
	s.AddKnownTypes(oav1beta1.GroupVersion, oa)
	c := fake.NewFakeClient(objs...)
	for i := range statusList {
		ReportStatus(context.TODO(), c, oa, statusList[i])
		if oa.Status.Conditions[0].Message != expectedStatus[i].Message || oa.Status.Conditions[0].Reason != expectedStatus[i].Reason || oa.Status.Conditions[0].Status != expectedStatus[i].Status || oa.Status.Conditions[0].Type != expectedStatus[i].Type {
			t.Errorf("Error: Status not updated. Expected: %s, Actual: %s", expectedStatus[i], fmt.Sprintf("%+v\n", oa.Status.Conditions[0]))
		}
	}

}
