// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

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
	hubClient := fake.NewClientBuilder().Build()
	SetHubClient(hubClient)
	_, _, err := RenewAndRetry(context.TODO(), nil)
	if err == nil {
		t.Fatal("missing error")
	}

	hubClient1 := fake.NewClientBuilder().WithRuntimeObjects(newObservabilityAddon(name, testNamespace)).Build()
	SetHubClient(hubClient1)
	_, _, err = RenewAndRetry(context.TODO(), nil)
	if err != nil {
		t.Fatalf("Error caught: %v", err)
	}
}
