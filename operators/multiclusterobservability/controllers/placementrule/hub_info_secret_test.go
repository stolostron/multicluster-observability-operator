// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"strings"
	"testing"

	operatorv1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	operatorconfig "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/config"
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

func newTestIngressController() *operatorv1.IngressController {
	return &operatorv1.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.OpenshiftIngressOperatorCRName,
			Namespace: config.OpenshiftIngressOperatorNamespace,
		},
		Spec: operatorv1.IngressControllerSpec{
			DefaultCertificate: &corev1.LocalObjectReference{
				Name: "custom-certs-default",
			},
		},
	}

}

func newTestRouteCASecret() *corev1.Secret {
	configYamlMap := map[string][]byte{}
	configYamlMap["tls.crt"] = []byte(routerCA)

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "custom-certs-default",
			Namespace: config.OpenshiftIngressNamespace,
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

	objs := []runtime.Object{newTestObsApiRoute(), newTestAlertmanagerRoute(), newTestIngressController(), newTestRouteCASecret()}
	c := fake.NewFakeClient(objs...)

	hubInfo, err := generateHubInfoSecret(c, mcoNamespace, namespace, true)
	if err != nil {
		t.Fatalf("Failed to initial the hub info secret: (%v)", err)
	}
	hub := &operatorconfig.HubInfo{}
	err = yaml.Unmarshal(hubInfo.Data[operatorconfig.HubInfoSecretKey], &hub)
	if err != nil {
		t.Fatalf("Failed to unmarshal data in hub info secret (%v)", err)
	}
	if !strings.HasPrefix(hub.ObservatoriumAPIEndpoint, "https://test-host") || hub.AlertmanagerEndpoint != routeHost || hub.AlertmanagerRouterCA != routerCA {
		t.Fatalf("Wrong content in hub info secret: \ngot: "+hub.ObservatoriumAPIEndpoint+" "+hub.AlertmanagerEndpoint+" "+hub.AlertmanagerRouterCA, clusterName+" "+"https://test-host"+" "+"test-host"+" "+routerCA)
	}
}

func TestNewBYOSecret(t *testing.T) {
	initSchema(t)

	objs := []runtime.Object{newTestObsApiRoute(), newTestAlertmanagerRoute(), newTestAmRouteBYOCA(), newTestAmRouteBYOCert()}
	c := fake.NewFakeClient(objs...)

	hubInfo, err := generateHubInfoSecret(c, mcoNamespace, namespace, true)
	if err != nil {
		t.Fatalf("Failed to initial the hub info secret: (%v)", err)
	}
	hub := &operatorconfig.HubInfo{}
	err = yaml.Unmarshal(hubInfo.Data[operatorconfig.HubInfoSecretKey], &hub)
	if err != nil {
		t.Fatalf("Failed to unmarshal data in hub info secret (%v)", err)
	}
	if !strings.HasPrefix(hub.ObservatoriumAPIEndpoint, "https://test-host") || hub.AlertmanagerEndpoint != routeHost || hub.AlertmanagerRouterCA != routerBYOCA {
		t.Fatalf("Wrong content in hub info secret: \ngot: "+hub.ObservatoriumAPIEndpoint+" "+hub.AlertmanagerEndpoint+" "+hub.AlertmanagerRouterCA, clusterName+" "+"https://test-host"+" "+"test-host"+" "+routerBYOCA)
	}
}
