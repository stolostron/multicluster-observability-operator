// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project.
package util

import (
	"context"
	"os"
	"testing"

	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
)

func init() {
	os.Setenv("UNIT_TEST", "true")
	os.Setenv("HUB_NAMESPACE", testNamespace)
	s := scheme.Scheme
	oav1beta1.AddToScheme(s)
}

func TestRenewAndRetry(t *testing.T) {
	hubClient := fake.NewFakeClient()
	SetHubClient(hubClient)
	_, _, err := RenewAndRetry(context.TODO())
	if err == nil {
		t.Fatal("missing error")
	}

	hubClient1 := fake.NewFakeClient(newObservabilityAddon(name, testNamespace))
	SetHubClient(hubClient1)
	_, _, err = RenewAndRetry(context.TODO())
	if err != nil {
		t.Fatalf("Error caught: %v", err)
	}
}
