// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"context"
	"testing"

	ocinfrav1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis"
	epv1alpha1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/monitoring/v1alpha1"
)

func TestEndpointConfigCR(t *testing.T) {
	s := scheme.Scheme
	if err := apis.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add monitoringv1alpha1 scheme: (%v)", err)
	}
	if err := routev1.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add routev1 scheme: (%v)", err)
	}
	if err := ocinfrav1.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add ocinfrav1 scheme: (%v)", err)
	}

	routeHost := "test-host"
	route := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "observatorium-api",
			Namespace: "test-obs-ns",
		},
		Spec: routev1.RouteSpec{
			Host: routeHost,
		},
	}

	infra := &ocinfrav1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Status: ocinfrav1.InfrastructureStatus{
			APIServerURL: "test-api-url",
		},
	}

	objs := []runtime.Object{route, infra}
	c := fake.NewFakeClient(objs...)
	err := createEndpointConfigCR(c, "test-obs-ns", namespace, "test-cluster")
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

	err = createEndpointConfigCR(c, "test-obs-ns", namespace, "test-cluster")
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
