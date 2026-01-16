// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package multiclusterobservability

import (
	"reflect"
	"testing"

	routev1 "github.com/openshift/api/route/v1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGenerateGrafanaRoute(t *testing.T) {
	instance := &mcov1beta2.MultiClusterObservability{}
	s := scheme.Scheme
	s.AddKnownTypes(mcov1beta2.GroupVersion)
	if err := mcov1beta2.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add scheme: (%v)", err)
	}

	clientScheme := runtime.NewScheme()
	if err := routev1.AddToScheme(clientScheme); err != nil {
		t.Fatalf("Unable to add route scheme: (%v)", err)
	}

	tests := []struct {
		name string
		want routev1.Route
		c    client.WithWatch
	}{
		{
			name: "Test create a Route if it does not exist",
			want: routev1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      config.GrafanaRouteName,
					Namespace: config.GetDefaultNamespace(),
					Annotations: map[string]string{
						haProxyRouterTimeoutKey: defaultHaProxyRouterTimeout,
					},
				},
				Spec: routev1.RouteSpec{
					Port: &routev1.RoutePort{
						TargetPort: intstr.FromString("oauth-proxy"),
					},
					To: routev1.RouteTargetReference{
						Kind: "Service",
						Name: config.GrafanaServiceName,
					},
					TLS: &routev1.TLSConfig{
						Termination:                   routev1.TLSTerminationReencrypt,
						InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
					},
				},
			},
			c: fake.NewClientBuilder().WithScheme(clientScheme).Build(),
		},
		{
			name: "Test update a Route if it has been modified",
			want: routev1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      config.GrafanaRouteName,
					Namespace: config.GetDefaultNamespace(),
					Annotations: map[string]string{
						haProxyRouterTimeoutKey: defaultHaProxyRouterTimeout,
					},
				},
				Spec: routev1.RouteSpec{
					Port: &routev1.RoutePort{
						TargetPort: intstr.FromString("oauth-proxy"),
					},
					To: routev1.RouteTargetReference{
						Kind: "Service",
						Name: config.GrafanaServiceName,
					},
					TLS: &routev1.TLSConfig{
						Termination:                   routev1.TLSTerminationReencrypt,
						InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
					},
				},
			},
			c: fake.NewClientBuilder().WithScheme(clientScheme).WithObjects(&routev1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:        config.GrafanaRouteName,
					Namespace:   config.GetDefaultNamespace(),
					Annotations: map[string]string{},
				},
				Spec: routev1.RouteSpec{
					Port: &routev1.RoutePort{
						TargetPort: intstr.FromString("oauth-proxy"),
					},
					To: routev1.RouteTargetReference{
						Kind: "Service",
						Name: "modified",
					},
					TLS: &routev1.TLSConfig{
						Termination:                   routev1.TLSTerminationReencrypt,
						InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
					},
				},
			}).Build(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GenerateGrafanaRoute(t.Context(), tt.c, s, instance)
			if err != nil {
				t.Errorf("GenerateGrafanaDataSource() error = %v", err)
				return
			}
			list := &routev1.RouteList{}
			if err := tt.c.List(t.Context(), list); err != nil {
				t.Fatalf("Unable to list routes: (%v)", err)
			}
			if len(list.Items) != 1 {
				t.Fatalf("Expected 1 route, got %d", len(list.Items))
			}
			if !reflect.DeepEqual(list.Items[0].Spec, tt.want.Spec) {
				t.Fatalf("Expected route spec: %v, got %v", tt.want.Spec, list.Items[0].Spec)
			}
		})
	}
}
