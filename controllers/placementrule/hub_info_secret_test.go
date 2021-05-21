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
	routeHost        = "test-host"
	routerCA         = "test-ca"
	routerBYOCA      = "test-ca"
	routerBYOCert    = "test-cert"
	routerBYOCertKey = "test-key"
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

func newTestAmRouteBYOCA() *corev1.Secret {
	configYamlMap := map[string][]byte{}
	configYamlMap["tls.crt"] = []byte(routerBYOCA)

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.AlertmanagerRouteBYOCAName,
			Namespace: mcoNamespace,
		},
		Data: configYamlMap,
	}
}

func newTestAmRouteBYOCert() *corev1.Secret {
	configYamlMap := map[string][]byte{}
	configYamlMap["tls.crt"] = []byte(routerBYOCert)
	configYamlMap["tls.key"] = []byte(routerBYOCertKey)

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.AlertmanagerRouteBYOCERTName,
			Namespace: mcoNamespace,
		},
		Data: configYamlMap,
	}
}

func TestNewSecret(t *testing.T) {
	initSchema(t)

	objs := []runtime.Object{newTestObsApiRoute(), newTestAlertmanagerRoute(), newTestRouteCA()}
	c := fake.NewFakeClient(objs...)

	hubInfo, err := newHubInfoSecret(c, mcoNamespace, namespace, newTestMCO())
	if err != nil {
		t.Fatalf("Failed to initial the hub info secret: (%v)", err)
	}
	hub := &HubInfo{}
	err = yaml.Unmarshal(hubInfo.Data[hubInfoKey], &hub)
	if err != nil {
		t.Fatalf("Failed to unmarshal data in hub info secret (%v)", err)
	}
	if !strings.HasPrefix(hub.Endpoint, "https://test-host") || !strings.HasPrefix(hub.HubAlertmanagerEndpoint, "https://test-host") || hub.HubAlertmanagerRouterCA != routerCA {
		t.Fatalf("Wrong content in hub info secret: \ngot: "+hub.Endpoint+" "+hub.HubAlertmanagerEndpoint+" "+hub.HubAlertmanagerRouterCA, clusterName+" "+"https://test-host"+" "+"https://test-host"+" "+routerCA)
	}
}

func TestNewBYOSecret(t *testing.T) {
	initSchema(t)

	objs := []runtime.Object{newTestObsApiRoute(), newTestAlertmanagerRoute(), newTestAmRouteBYOCA(), newTestAmRouteBYOCert()}
	c := fake.NewFakeClient(objs...)

	hubInfo, err := newHubInfoSecret(c, mcoNamespace, namespace, newTestMCO())
	if err != nil {
		t.Fatalf("Failed to initial the hub info secret: (%v)", err)
	}
	hub := &HubInfo{}
	err = yaml.Unmarshal(hubInfo.Data[hubInfoKey], &hub)
	if err != nil {
		t.Fatalf("Failed to unmarshal data in hub info secret (%v)", err)
	}
	if !strings.HasPrefix(hub.Endpoint, "https://test-host") || !strings.HasPrefix(hub.HubAlertmanagerEndpoint, "https://test-host") || hub.HubAlertmanagerRouterCA != routerBYOCA {
		t.Fatalf("Wrong content in hub info secret: \ngot: "+hub.Endpoint+" "+hub.HubAlertmanagerEndpoint+" "+hub.HubAlertmanagerRouterCA, clusterName+" "+"https://test-host"+" "+"https://test-host"+" "+routerBYOCA)
	}
}
