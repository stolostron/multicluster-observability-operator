// Copyright (c) 2021 Red Hat, Inc.

package placementrule

import (
	"strings"
	"testing"

	routev1 "github.com/openshift/api/route/v1"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	routeHost = "test-host"
)

func newTestRoute() *routev1.Route {
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "observatorium-api",
			Namespace: mcoNamespace,
		},
		Spec: routev1.RouteSpec{
			Host: routeHost,
		},
	}
}

func TestNewSecret(t *testing.T) {
	initSchema(t)

	objs := []runtime.Object{newTestRoute()}
	c := fake.NewFakeClient(objs...)

	hubInfo, err := newHubInfoSecret(c, mcoNamespace, namespace, clusterName, newTestMCO())
	if err != nil {
		t.Fatalf("Failed to initial the hub info secret: (%v)", err)
	}
	hub := &HubInfo{}
	err = yaml.Unmarshal(hubInfo.Data[hubInfoKey], &hub)
	if err != nil {
		t.Fatalf("Failed to unmarshal data in hub info secret (%v)", err)
	}
	if hub.ClusterName != clusterName || !strings.HasPrefix(hub.Endpoint, "https://test-host") {
		t.Fatalf("Wrong content in hub info secret: (%s)", hub.ClusterName+" "+hub.Endpoint)
	}
}
