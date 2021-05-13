// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"strings"
	"testing"

	routev1 "github.com/openshift/api/route/v1"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
)

const (
	routeHost = "test-host"
	routerCA  = "test-ca"
)

func newTestObsApiRoute() *routev1.Route {
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

func newTestAlertmanagerRoute() *routev1.Route {
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.AlertmanagerRouteName,
			Namespace: mcoNamespace,
		},
		Spec: routev1.RouteSpec{
			Host: routeHost,
		},
	}
}

func newTestRouteCA() *corev1.Secret {
	configYamlMap := map[string][]byte{}
	configYamlMap["tls.crt"] = []byte(routerCA)

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.OpenshiftRouterCASecretName,
			Namespace: config.OpenshiftIngressOperatorNamespace,
		},
		Data: configYamlMap,
	}
}

func TestNewSecret(t *testing.T) {
	initSchema(t)

	objs := []runtime.Object{newTestObsApiRoute(), newTestAlertmanagerRoute(), newTestRouteCA()}
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
	if hub.ClusterName != clusterName || !strings.HasPrefix(hub.Endpoint, "https://test-host") || !strings.HasPrefix(hub.HubAlertmanagerEndpoint, "https://test-host") || hub.HubRouterCA != routerCA {
		t.Fatalf("Wrong content in hub info secret: \ngot: (%s)\nwant: (%s)", hub.ClusterName+" "+hub.Endpoint+" "+hub.HubAlertmanagerEndpoint+" "+hub.HubRouterCA, clusterName+" "+"https://test-host"+" "+"https://test-host"+" "+routerCA)
	}
}
