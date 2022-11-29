// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

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
	c := fake.NewFakeClient()
	_, err := GetStatefulSetList(c, namespace, map[string]string{})
	if err != nil {
		t.Fatalf("Failed to list statefulset: (%v)", err)
	}
}

func TestGetPVCList(t *testing.T) {
	c := fake.NewFakeClient()
	_, err := GetPVCList(c, namespace, map[string]string{})
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
		t.Fatalf(errMsg)
	}
	_, err = GetOrCreatePromClient()
	if (!inCluster && err == nil) || (inCluster && err != nil) {
		t.Fatalf(errMsg)
	}
	_, err = GetOrCreateKubeClient()
	if (!inCluster && err == nil) || (inCluster && err != nil) {
		t.Fatalf(errMsg)
	}
	_, err = GetOrCreateOCPClient()
	if (!inCluster && err == nil) || (inCluster && err != nil) {
		t.Fatalf(errMsg)
	}
}
