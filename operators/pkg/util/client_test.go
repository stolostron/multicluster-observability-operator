// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"testing"

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
	_, err := GetOrCreateCRDClient()
	if err == nil {
		t.Fatalf("Failed to catch error")
	}
	_, err = GetOrCreatePromClient()
	if err == nil {
		t.Fatalf("Failed to catch error")
	}
	_, err = GetOrCreateKubeClient()
	if err == nil {
		t.Fatalf("Failed to catch error")
	}
	_, err = GetOrCreateOCPClient()
	if err == nil {
		t.Fatalf("Failed to catch error")
	}
}
