// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"strings"
	"testing"

	operatorv1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	mcoshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
)

const (
	routeHost        = "observatorium-api-open-cluster-management-observability.apps.test-host.test.com"
	routerCA         = "test-ca"
	routerBYOCA      = "test-ca"
	routerBYOCert    = "test-cert"
	routerBYOCertKey = "test-key"
	routerDefaultCA  = "test-ca"
)

func newTestObsApiRoute() *routev1.Route {
	return &routev1.Route{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Route",
			APIVersion: "v1",
		},
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
		TypeMeta: metav1.TypeMeta{
			Kind:       "Route",
			APIVersion: "v1",
		},
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
		TypeMeta: metav1.TypeMeta{
			Kind:       "IngressController",
			APIVersion: "v1",
		},
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
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
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
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
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
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.AlertmanagerRouteBYOCERTName,
			Namespace: mcoNamespace,
		},
		Data: configYamlMap,
	}
}

func newTestAmDefaultCA() *corev1.ConfigMap {
	configYamlMap := map[string]string{"service-ca.crt": routerDefaultCA}

	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.AlertmanagersDefaultCaBundleName,
			Namespace: mcoNamespace,
		},
		Data: configYamlMap,
	}
}

func newMultiClusterObservability() *mcov1beta2.MultiClusterObservability {
	return &mcov1beta2.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta2.StorageConfig{
				MetricObjectStorage: &mcoshared.PreConfiguredStorage{
					Key:  "test",
					Name: "test",
				},
				AlertmanagerStorageSize: "2Gi",
			},
		},
	}
}

func TestNewSecret(t *testing.T) {
	initSchema(t)

	mco := newMultiClusterObservability()
	config.SetMonitoringCRName(mco.Name)
	objs := []runtime.Object{
		newTestObsApiRoute(),
		newTestAlertmanagerRoute(),
		newTestIngressController(),
		newTestRouteCASecret(),
		mco,
	}
	c := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
	crdMap := map[string]bool{config.IngressControllerCRD: true}

	hubInfo, err := generateHubInfoSecret(c, mcoNamespace, namespace, crdMap, config.IsUWMAlertingDisabledInSpec(mco))
	if err != nil {
		t.Fatalf("Failed to initial the hub info secret: (%v)", err)
	}
	if hubInfo == nil {
		t.Fatal("Generated hub info secret is nil")
	}
	hub := &operatorconfig.HubInfo{}
	err = yaml.Unmarshal(hubInfo.Data[operatorconfig.HubInfoSecretKey], &hub)
	if err != nil {
		t.Fatalf("Failed to unmarshal data in hub info secret (%v)", err)
	}
	if !strings.HasPrefix(hub.ObservatoriumAPIEndpoint, "https://observatorium-api-open-cluster-management-observability.apps.test-host") ||
		hub.AlertmanagerEndpoint != "https://"+routeHost || hub.AlertmanagerRouterCA != routerCA || hub.HubClusterID != "test-host-test" {
		t.Fatalf("Wrong content in hub info secret: \ngot: "+hub.ObservatoriumAPIEndpoint+" "+hub.AlertmanagerEndpoint+" "+hub.AlertmanagerRouterCA, clusterName+" "+"https://test-host"+" "+"test-host"+" "+routerCA)
	}

	// Test UWM alerting disabled
	mco.Annotations = map[string]string{config.AnnotationDisableUWMAlerting: "true"}
	hubInfo, err = generateHubInfoSecret(c, mcoNamespace, namespace, crdMap, config.IsUWMAlertingDisabledInSpec(mco))
	if err != nil {
		t.Fatalf("Failed to generate hub info secret with UWM alerting disabled: %v", err)
	}
	if hubInfo == nil {
		t.Fatal("Generated hub info secret is nil")
	}
	hub = &operatorconfig.HubInfo{}
	err = yaml.Unmarshal(hubInfo.Data[operatorconfig.HubInfoSecretKey], &hub)
	if err != nil {
		t.Fatalf("Failed to unmarshal data in hub info secret (%v)", err)
	}
	if !hub.UWMAlertingDisabled {
		t.Fatalf("UWM alerting should be disabled, but UWMAlertingDisabled is false")
	}

	// Test UWM alerting enabled
	mco.Annotations = map[string]string{config.AnnotationDisableUWMAlerting: "false"}
	hubInfo, err = generateHubInfoSecret(c, mcoNamespace, namespace, crdMap, config.IsUWMAlertingDisabledInSpec(mco))
	if err != nil {
		t.Fatalf("Failed to generate hub info secret with UWM alerting enabled: %v", err)
	}
	if hubInfo == nil {
		t.Fatal("Generated hub info secret is nil")
	}
	hub = &operatorconfig.HubInfo{}
	err = yaml.Unmarshal(hubInfo.Data[operatorconfig.HubInfoSecretKey], &hub)
	if err != nil {
		t.Fatalf("Failed to unmarshal data in hub info secret (%v)", err)
	}
	if hub.UWMAlertingDisabled {
		t.Fatalf("UWM alerting should be enabled, but UWMAlertingDisabled is true")
	}

	// Test UWM alerting disabled but general alerting enabled
	mco.Annotations = map[string]string{config.AnnotationDisableUWMAlerting: "true"}
	config.SetAlertingDisabled(false) // Enable general alerting
	hubInfo, err = generateHubInfoSecret(c, mcoNamespace, namespace, crdMap, config.IsUWMAlertingDisabledInSpec(mco))
	if err != nil {
		t.Fatalf("Failed to generate hub info secret with UWM alerting disabled but general alerting enabled: %v", err)
	}
	if hubInfo == nil {
		t.Fatal("Generated hub info secret is nil")
	}
	hub = &operatorconfig.HubInfo{}
	err = yaml.Unmarshal(hubInfo.Data[operatorconfig.HubInfoSecretKey], &hub)
	if err != nil {
		t.Fatalf("Failed to unmarshal data in hub info secret (%v)", err)
	}
	if !hub.UWMAlertingDisabled {
		t.Fatalf("UWM alerting should be disabled, but UWMAlertingDisabled is false")
	}
	if hub.AlertmanagerEndpoint == "" {
		t.Fatalf("AlertmanagerEndpoint should be set when general alerting is enabled")
	}

	mco.Spec.AdvancedConfig = &mcov1beta2.AdvancedConfig{
		CustomObservabilityHubURL: "https://custom-obs:8080",
		CustomAlertmanagerHubURL:  "https://custom-am",
	}
	c = fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
	hubInfo, err = generateHubInfoSecret(c, mcoNamespace, namespace, crdMap, config.IsUWMAlertingDisabledInSpec(mco))
	if err != nil {
		t.Fatalf("Failed to generate hub info secret: %v", err)
	}
	if hubInfo == nil {
		t.Fatal("Generated hub info secret is nil")
	}
	hub = &operatorconfig.HubInfo{}
	err = yaml.Unmarshal(hubInfo.Data[operatorconfig.HubInfoSecretKey], &hub)
	if err != nil {
		t.Fatalf("Failed to unmarshal data in hub info secret (%v)", err)
	}
	if !strings.HasPrefix(hub.ObservatoriumAPIEndpoint, "https://custom-obs:8080") || !strings.HasPrefix(hub.AlertmanagerEndpoint, "https://custom-am") || hub.AlertmanagerRouterCA != routerCA {
		t.Fatalf("Wrong content in hub info secret: \ngot: "+hub.ObservatoriumAPIEndpoint+" "+hub.AlertmanagerEndpoint+" "+hub.AlertmanagerRouterCA, clusterName+" "+"https://custom-obs"+" "+"custom-obs"+" "+routerCA)
	}

}

func TestNewBYOSecret(t *testing.T) {
	initSchema(t)

	mco := newMultiClusterObservability()
	objs := []runtime.Object{newTestObsApiRoute(), newTestAlertmanagerRoute(), newTestAmRouteBYOCA(), newTestAmRouteBYOCert()}
	c := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()

	crdMap := map[string]bool{config.IngressControllerCRD: true}
	hubInfo, err := generateHubInfoSecret(c, mcoNamespace, namespace, crdMap, config.IsUWMAlertingDisabledInSpec(mco))
	if err != nil {
		t.Fatalf("Failed to initial the hub info secret: %v", err)
	}
	if hubInfo == nil {
		t.Fatal("Generated hub info secret is nil")
	}
	hub := &operatorconfig.HubInfo{}
	err = yaml.Unmarshal(hubInfo.Data[operatorconfig.HubInfoSecretKey], &hub)
	if err != nil {
		t.Fatalf("Failed to unmarshal data in hub info secret (%v)", err)
	}
	if !strings.HasPrefix(hub.ObservatoriumAPIEndpoint, "https://observatorium-api-open-cluster-management-observability.apps.test-host") ||
		hub.AlertmanagerEndpoint != "https://"+routeHost || hub.AlertmanagerRouterCA != routerBYOCA {
		t.Fatalf("Wrong content in hub info secret: \ngot: "+hub.ObservatoriumAPIEndpoint+" "+hub.AlertmanagerEndpoint+" "+hub.AlertmanagerRouterCA, clusterName+" "+"https://test-host"+" "+"test-host"+" "+routerBYOCA)
	}
}
