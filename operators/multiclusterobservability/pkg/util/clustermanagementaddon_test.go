// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
)

func TestClusterManagmentAddon(t *testing.T) {
	s := scheme.Scheme
	addonv1alpha1.AddToScheme(s)
	c := fake.NewFakeClient()
	err := CreateClusterManagementAddon(c)
	if err != nil {
		t.Fatalf("Failed to create clustermanagementaddon: (%v)", err)
	}
	err = CreateClusterManagementAddon(c)
	if err != nil {
		t.Fatalf("Failed to create clustermanagementaddon twice: (%v)", err)
	}
	addon := &addonv1alpha1.ClusterManagementAddOn{}
	err = c.Get(context.TODO(),
		types.NamespacedName{
			Name: ObservabilityController,
		},
		addon,
	)
	if err != nil {
		t.Fatalf("Failed to get clustermanagementaddon: (%v)", err)
	}
	if addon.Spec.AddOnConfiguration.CRDName != "observabilityaddons.observability.open-cluster-management.io" {
		t.Fatalf("Wrong CRD name included: %s", addon.Spec.AddOnConfiguration.CRDName)
	}
	if linkTxt, found := addon.ObjectMeta.Annotations["console.open-cluster-management.io/launch-link-text"]; found == false {
		t.Fatalf("No launch-link-text annotation included")
	} else {
		if linkTxt != "Grafana" {
			t.Fatalf("Wrong launch-link-text annotation: %s", linkTxt)
		}
	}

	err = DeleteClusterManagementAddon(c)
	if err != nil {
		t.Fatalf("Failed to delete clustermanagementaddon: (%v)", err)
	}
	err = c.Get(context.TODO(),
		types.NamespacedName{
			Name: ObservabilityController,
		},
		addon,
	)
	if err == nil || !errors.IsNotFound(err) {
		t.Fatalf("Failed to delete clustermanagementaddon: (%v)", err)
	}
}
