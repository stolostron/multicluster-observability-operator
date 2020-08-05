// Copyright (c) 2020 Red Hat, Inc.

package multiclusterobservability

import (
	"strings"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	fakeconfigclient "github.com/openshift/client-go/config/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCreateHubClusterMonitoringConfig(t *testing.T) {

	route := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obsAPIGateway,
			Namespace: "test",
		},
		Spec: routev1.RouteSpec{
			Host: "apiServerURL",
		},
	}
	s := scheme.Scheme
	s.AddKnownTypes(routev1.GroupVersion, route)
	client := fake.NewFakeClientWithScheme(s, route)

	version := &configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{Name: "version"},
		Spec: configv1.ClusterVersionSpec{
			ClusterID: configv1.ClusterID("xxxx-xxxx"),
		},
	}
	ocpClient := fakeconfigclient.NewSimpleClientset(version)

	_, err := UpdateHubClusterMonitoringConfig(client, ocpClient, "test")
	if err != nil {
		t.Errorf("Update configmap has error: %v", err)
	}

	cm, _ := getConfigMap(client)
	if cm.Data == nil {
		t.Errorf("Update configmap is failed")
	}
}

func TestUpdateHubClusterMonitoringConfig(t *testing.T) {

	cases := []struct {
		name    string
		rawData []byte
	}{
		{
			name: "update configmap with empty prometheusK8s",
			rawData: []byte(`
"http":
  "httpProxy": "test"
  "httpsProxy": "test"
`),
		},
		{
			name: "update configmap with empty remoteWrite",
			rawData: []byte(`
"http":
  "httpProxy": "test"
  "httpsProxy": "test"
prometheusK8s:
  externalLabels: null
  hostport: ""
  nodeSelector: null
  remoteWrite: null
`),
		},
		{
			name: "update configmap with the full values",
			rawData: []byte(`
"http":
  "httpProxy": "test"
  "httpsProxy": "test"
prometheusK8s:
  externalLabels: null
  hostport: ""
  nodeSelector: null
  remoteWrite:
  - url: http://observatorium/api/metrics/v1/write
    writeRelabelConfigs:
    - replacement: hub_cluster
      sourceLabels:
      - __name__
      targetLabel: cluster
    - replacement: xxxx-xxxx
      sourceLabels:
      - __name__
      targetLabel: cluster_id
  - url: http://apiServerURL/api/metrics/v1/write
    writeRelabelConfigs:
    - replacement: spoke_cluster
      sourceLabels:
      - __name__
      targetLabel: cluster
    - replacement: xxxx-xxxx
      sourceLabels:
      - __name__
      targetLabel: cluster_id
  resources: null
  retention: ""
  tolerations: null
  volumeClaimTemplate: null
`),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cm := &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cmName,
					Namespace: cmNamespace,
				},
				Data: map[string]string{
					configKey: string(c.rawData),
				},
			}
			route := &routev1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      obsAPIGateway,
					Namespace: "test",
				},
				Spec: routev1.RouteSpec{
					Host: "apiServerURL",
				},
			}
			s := scheme.Scheme
			s.AddKnownTypes(routev1.GroupVersion, route)
			client := fake.NewFakeClientWithScheme(s, route, cm)

			version := &configv1.ClusterVersion{
				ObjectMeta: metav1.ObjectMeta{Name: "version"},
				Spec: configv1.ClusterVersionSpec{
					ClusterID: configv1.ClusterID("xxxx-xxxx"),
				},
			}
			ocpClient := fakeconfigclient.NewSimpleClientset(version)

			_, err := UpdateHubClusterMonitoringConfig(client, ocpClient, "test")
			if err != nil {
				t.Errorf("Update configmap has error: %v", err)
			}

			configmap, _ := getConfigMap(client)
			if !strings.Contains(configmap.Data[configKey], "httpsProxy: test") {
				t.Errorf("Missed the original data in configmap %v", configmap.Data[configKey])
			}
		})
	}
}
