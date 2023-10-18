// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package status

import (
	"context"
	"os"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/util"
	oashared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
)

const (
	name            = "observability-addon"
	testNamespace   = "test-ns"
	testHubNamspace = "test-hub-ns"
)

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

func init() {
	os.Setenv("UNIT_TEST", "true")
	s := scheme.Scheme
	addonv1alpha1.AddToScheme(s)
	oav1beta1.AddToScheme(s)

	namespace = testNamespace
	hubNamespace = testHubNamspace
}

func TestStatusController(t *testing.T) {

	hubClient := fake.NewClientBuilder().Build()
	util.SetHubClient(hubClient)
	c := fake.NewClientBuilder().Build()

	r := &StatusReconciler{
		Client:    c,
		HubClient: hubClient,
	}

	// test error in reconcile if missing obervabilityaddon
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "install",
			Namespace: testNamespace,
		},
	}
	ctx := context.TODO()
	_, err := r.Reconcile(ctx, req)
	if err == nil {
		t.Fatalf("reconcile: miss the error for missing obervabilityaddon")
	}

	// test status in local pushed to hub
	err = hubClient.Create(ctx, newObservabilityAddon(name, testHubNamspace))
	if err != nil {
		t.Fatalf("failed to create hub oba to install: (%v)", err)
	}

	oba := newObservabilityAddon(name, testNamespace)
	oba.Status = oav1beta1.ObservabilityAddonStatus{
		Conditions: []oav1beta1.StatusCondition{
			{
				Type:    "Deployed",
				Status:  metav1.ConditionTrue,
				Reason:  "Deployed",
				Message: "Metrics collector deployed",
			},
		},
	}
	err = c.Create(ctx, oba)
	if err != nil {
		t.Fatalf("failed to create oba to install: (%v)", err)
	}
	req = ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "install",
			Namespace: testNamespace,
		},
	}
	_, err = r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("Failed to reconcile: (%v)", err)
	}
	hubObsAddon := &oav1beta1.ObservabilityAddon{}
	err = hubClient.Get(ctx, types.NamespacedName{Name: obAddonName, Namespace: testHubNamspace}, hubObsAddon)
	if err != nil {
		t.Fatalf("Failed to get oba in hub: (%v)", err)
	}

	if hubObsAddon.Status.Conditions == nil || len(hubObsAddon.Status.Conditions) != 1 {
		t.Fatalf("No correct status set in hub observabilityaddon: (%v)", hubObsAddon)
	} else if hubObsAddon.Status.Conditions[0].Type != "Deployed" {
		t.Fatalf("Wrong status type: (%v)", hubObsAddon.Status)
	}
}
