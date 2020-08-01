// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"context"
	"testing"

	routev1 "github.com/openshift/api/route/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	epv1alpha1 "github.com/open-cluster-management/multicluster-observability-operator/pkg/apis/monitoring/v1alpha1"
)

const (
	routeHost = "test-host"
)

func newTestRoute() *routev1.Route {
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "observatorium-api",
			Namespace: mcmNameSpace,
		},
		Spec: routev1.RouteSpec{
			Host: routeHost,
		},
	}
}

func TestEndpointConfigCR(t *testing.T) {
	initSchema(t)

	objs := []runtime.Object{newTestRoute(), newTestInfra()}
	c := fake.NewFakeClient(objs...)

	err := createEndpointConfigCR(c, mcmNameSpace, namespace, "test-cluster")
	if err != nil {
		t.Fatalf("Failed to create EndpointMonitoring: (%v)", err)
	}
	found := &epv1alpha1.EndpointMonitoring{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: epConfigName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get EndpointMonitoring: (%v)", err)
	}
	if found.Spec.GlobalConfig.SeverURL != routeHost {
		t.Log(found.Spec.GlobalConfig.SeverURL)
		t.Fatal("Endpointmonitoring has wrong configurations")
	}

	err = createEndpointConfigCR(c, mcmNameSpace, namespace, "test-cluster")
	if err != nil {
		t.Fatalf("Failed to create EndpointMonitoring: (%v)", err)
	}

	err = deleteEndpointConfigCR(c, namespace)
	if err != nil {
		t.Fatalf("Failed to delete EndpointMonitoring: (%v)", err)
	}
	err = c.Get(context.TODO(), types.NamespacedName{Name: epConfigName, Namespace: namespace}, found)
	if err == nil || !errors.IsNotFound(err) {
		t.Fatalf("Failed to delete EndpointMonitoring: (%v)", err)
	}

	err = deleteEndpointConfigCR(c, namespace)
	if err != nil {
		t.Fatalf("Failed to delete EndpointMonitoring: (%v)", err)
	}

}
