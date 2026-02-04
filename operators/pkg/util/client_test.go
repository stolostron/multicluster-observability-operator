// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"testing"

	restclient "k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	namespace = "namespace"
)

func TestGetStatefulSetList(t *testing.T) {
	c := fake.NewClientBuilder().Build()
	_, err := GetStatefulSetList(t.Context(), c, namespace, map[string]string{})
	if err != nil {
		t.Fatalf("Failed to list statefulset: (%v)", err)
	}
}

func TestGetPVCList(t *testing.T) {
	c := fake.NewClientBuilder().Build()
	_, err := GetPVCList(t.Context(), c, namespace, map[string]string{})
	if err != nil {
		t.Fatalf("Failed to list pvc: (%v)", err)
	}
}

func TestCreateClient(t *testing.T) {
	inCluster := false
	errMsg := "Failed to catch error"
	_, err := restclient.InClusterConfig()
	if err == nil {
		inCluster = true
		errMsg = "Failed to create client"
	}

	_, err = GetOrCreateCRDClient()
	if (!inCluster && err == nil) || (inCluster && err != nil) {
		t.Fatal(errMsg)
	}
	_, err = GetOrCreatePromClient()
	if (!inCluster && err == nil) || (inCluster && err != nil) {
		t.Fatal(errMsg)
	}
	_, err = GetOrCreateKubeClient()
	if (!inCluster && err == nil) || (inCluster && err != nil) {
		t.Fatal(errMsg)
	}
	_, err = GetOrCreateOCPClient()
	if (!inCluster && err == nil) || (inCluster && err != nil) {
		t.Fatal(errMsg)
	}
}
