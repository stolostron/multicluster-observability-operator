// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	"testing"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
)

func TestClusterManagmentAddon(t *testing.T) {
	s := scheme.Scheme
	addonv1alpha1.AddToScheme(s)
	routev1.AddToScheme(s)

	consoleRoute := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multicloud-console",
			Namespace: config.GetMCONamespace(),
		},
		Spec: routev1.RouteSpec{
			Host: "console",
		},
	}

	c := fake.NewClientBuilder().WithRuntimeObjects(consoleRoute).Build()
	_, err := CreateClusterManagementAddon(c)
	if err != nil {
		t.Fatalf("Failed to create clustermanagementaddon: (%v)", err)
	}
	_, err = CreateClusterManagementAddon(c)
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

	if selfMgmt, found := addon.ObjectMeta.Annotations[addonv1alpha1.AddonLifecycleAnnotationKey]; found == false {
		t.Fatalf("no AddonLifecycle included")
	} else {
		if selfMgmt != addonv1alpha1.AddonLifecycleSelfManageAnnotationValue {
			t.Fatalf("Wrong AddonLifecycle annotation: %s", selfMgmt)
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

	// Create a clustermanagement addon without the requires selfmgmt annotation
	// and then make sure it's set during updates.
	clusterManagementAddon, err := newClusterManagementAddon(c)
	if err != nil {
		t.Fatalf("Failed to create new clustermanagementaddon: (%v)", err)
	}

	delete(clusterManagementAddon.ObjectMeta.Annotations, addonv1alpha1.AddonLifecycleAnnotationKey)

	if err := c.Create(context.TODO(), clusterManagementAddon); err != nil {
		t.Fatalf("Failed to create clustermanagementaddon: (%v)", err)
	}

	_, err = CreateClusterManagementAddon(c)
	if err != nil {
		t.Fatalf("Failed to create clustermanagementaddon: (%v)", err)
	}

	err = c.Get(context.TODO(),
		types.NamespacedName{
			Name: ObservabilityController,
		},
		addon,
	)

	if err != nil {
		t.Fatalf("Failed to get clustermanagementaddon: (%v)", err)
	}
	if selfMgmt, found := addon.ObjectMeta.Annotations[addonv1alpha1.AddonLifecycleAnnotationKey]; found == false {
		t.Fatalf("no AddonLifecycle included")
	} else {
		if selfMgmt != addonv1alpha1.AddonLifecycleSelfManageAnnotationValue {
			t.Fatalf("Wrong AddonLifecycle annotation: %s", selfMgmt)
		}
	}

	// delete it again for good measure
	err = DeleteClusterManagementAddon(c)
	if err != nil {
		t.Fatalf("Failed to delete clustermanagementaddon: (%v)", err)
	}

}
