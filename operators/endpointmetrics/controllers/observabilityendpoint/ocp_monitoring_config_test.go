// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project.
package observabilityendpoint

import (
	"context"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	yamltool "github.com/ghodss/yaml"
	cmomanifests "github.com/openshift/cluster-monitoring-operator/pkg/manifests"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	operatorconfig "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/config"
)

const (
	hubInfoYAML = `
cluster-name: "test-cluster"
endpoint: "http://test-endpoint"
alertmanager-endpoint: "http://test-alertamanger-endpoint"
alertmanager-router-ca: |
    -----BEGIN CERTIFICATE-----
    xxxxxxxxxxxxxxxxxxxxxxxxxxx
    -----END CERTIFICATE-----
`
)

func TestCreateDeleteHubAmRouterCASecret(t *testing.T) {
	hubInfo := &operatorconfig.HubInfo{}
	err := yaml.Unmarshal([]byte(hubInfoYAML), &hubInfo)
	if err != nil {
		t.Fatalf("Failed to unmarshal hubInfo: (%v)", err)
	}

	hubInfoObj := newHubInfoSecret([]byte(hubInfoYAML))
	objs := []runtime.Object{hubInfoObj}

	ctx := context.TODO()
	c := fake.NewFakeClient(objs...)
	err = createHubAmRouterCASecret(ctx, hubInfo, c, promNamespace)
	if err != nil {
		t.Fatalf("Failed to create the hub-alertmanager-router-ca secret: (%v)", err)
	}
	err = deleteHubAmRouterCASecret(ctx, c, promNamespace)
	if err != nil {
		t.Fatalf("Failed to delete the hub-alertmanager-router-ca secret: (%v)", err)
	}
	err = deleteHubAmRouterCASecret(ctx, c, promNamespace)
	if err != nil {
		t.Fatalf("Run into error when try to delete hub-alertmanager-router-ca secret twice: (%v)", err)
	}
}

func TestCreateDeleteHubAmAccessorTokenSecret(t *testing.T) {
	amAccessSrt := newAMAccessorSecret()
	objs := []runtime.Object{amAccessSrt}

	ctx := context.TODO()
	c := fake.NewFakeClient(objs...)
	err := createHubAmAccessorTokenSecret(ctx, c, promNamespace)
	if err != nil {
		t.Fatalf("Failed to create the observability-alertmanager-accessor secret: (%v)", err)
	}
	err = deleteHubAmAccessorTokenSecret(ctx, c, promNamespace)
	if err != nil {
		t.Fatalf("Failed to delete the observability-alertmanager-accessor secret: (%v)", err)
	}
	err = deleteHubAmAccessorTokenSecret(ctx, c, promNamespace)
	if err != nil {
		t.Fatalf("Run into error when try to delete observability-alertmanager-accessor secret twice: (%v)", err)
	}
}

func TestClusterMonitoringConfig(t *testing.T) {
	tests := []struct {
		name                                    string
		ClusterMonitoringConfigCMExist          bool
		ClusterMonitoringConfigDataYaml         string
		ExpectedDeleteClusterMonitoringConfigCM bool
	}{
		{
			name:                                    "no cluster-monitoring-config exists",
			ClusterMonitoringConfigCMExist:          false,
			ExpectedDeleteClusterMonitoringConfigCM: true,
		},
		{
			name:                                    "cluster-monitoring-config with empty config.yaml",
			ClusterMonitoringConfigCMExist:          true,
			ClusterMonitoringConfigDataYaml:         "",
			ExpectedDeleteClusterMonitoringConfigCM: true,
		},
		{
			name:                           "cluster-monitoring-config with non-empty config.yaml and empty prometheusK8s",
			ClusterMonitoringConfigCMExist: true,
			ClusterMonitoringConfigDataYaml: `
prometheusK8s: null`,
			ExpectedDeleteClusterMonitoringConfigCM: true,
		},
		{
			name:                           "cluster-monitoring-config with non-empty config.yaml and prometheusK8s and empty additionalAlertManagerConfigs",
			ClusterMonitoringConfigCMExist: true,
			ClusterMonitoringConfigDataYaml: `
prometheusK8s:
  additionalAlertManagerConfigs: null`,
			ExpectedDeleteClusterMonitoringConfigCM: true,
		},
		{
			name:                           "cluster-monitoring-config with non-empty config.yaml and prometheusK8s and additionalAlertManagerConfigs",
			ClusterMonitoringConfigCMExist: true,
			ClusterMonitoringConfigDataYaml: `
prometheusK8s:
  additionalAlertManagerConfigs:
  - apiVersion: v2
    bearerToken:
      key: token
      name: foo
    pathPrefix: /
    scheme: https
    staticConfigs:
    - test-host.com
    tlsConfig:
      insecureSkipVerify: true`,
			ExpectedDeleteClusterMonitoringConfigCM: false,
		},
	}

	hubInfo := &operatorconfig.HubInfo{}
	err := yaml.Unmarshal([]byte(hubInfoYAML), &hubInfo)
	if err != nil {
		t.Fatalf("Failed to unmarshal hubInfo: (%v)", err)
	}
	hubInfoObj := newHubInfoSecret([]byte(hubInfoYAML))
	amAccessSrt := newAMAccessorSecret()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := []runtime.Object{hubInfoObj, amAccessSrt}
			if tt.ClusterMonitoringConfigCMExist {
				objs = append(objs, newClusterMonitoringConfigCM(tt.ClusterMonitoringConfigDataYaml))
			}
			testCreateOrUpdateClusterMonitoringConfig(t, hubInfo, fake.NewFakeClient(objs...), tt.ExpectedDeleteClusterMonitoringConfigCM)
		})
	}
}

func testCreateOrUpdateClusterMonitoringConfig(t *testing.T, hubInfo *operatorconfig.HubInfo, c client.Client, expectedCMDelete bool) {
	ctx := context.TODO()
	err := createOrUpdateClusterMonitoringConfig(ctx, hubInfo, testClusterID, c, false)
	if err != nil {
		t.Fatalf("Failed to create or update the cluster-monitoring-config configmap: (%v)", err)
	}

	foundCusterMonitoringConfigMap := &corev1.ConfigMap{}
	err = c.Get(ctx, types.NamespacedName{Name: clusterMonitoringConfigName,
		Namespace: promNamespace}, foundCusterMonitoringConfigMap)
	if err != nil {
		t.Fatalf("failed to check configmap %s: %v", clusterMonitoringConfigName, err)
	}

	foundClusterMonitoringConfigurationYAML, ok := foundCusterMonitoringConfigMap.Data[clusterMonitoringConfigDataKey]
	if !ok {
		t.Fatalf("configmap: %s doesn't contain key: %s", clusterMonitoringConfigName, clusterMonitoringConfigDataKey)
	}
	foundClusterMonitoringConfigurationJSON, err := yamltool.YAMLToJSON([]byte(foundClusterMonitoringConfigurationYAML))
	if err != nil {
		t.Fatalf("failed to transform YAML to JSON:\n%s\n", foundClusterMonitoringConfigurationYAML)
	}

	foundClusterMonitoringConfiguration := &cmomanifests.ClusterMonitoringConfiguration{}
	if err := json.Unmarshal([]byte(foundClusterMonitoringConfigurationJSON), foundClusterMonitoringConfiguration); err != nil {
		t.Fatalf("failed to marshal the cluster monitoring config: %v:\n%s\n", err, foundClusterMonitoringConfigurationJSON)
	}

	if foundClusterMonitoringConfiguration.PrometheusK8sConfig == nil {
		t.Fatalf("empty prometheusK8s in ClusterMonitoringConfiguration: %v", foundClusterMonitoringConfiguration)
	}

	if foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs == nil {
		t.Fatalf("empty AlertmanagerConfigs in ClusterMonitoringConfiguration.PrometheusK8sConfig: %v", foundClusterMonitoringConfiguration)
	}

	containsOCMAlertmanagerConfig := false
	for _, v := range foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs {
		if v.TLSConfig != (cmomanifests.TLSConfig{}) &&
			v.TLSConfig.CA != nil &&
			v.TLSConfig.CA.LocalObjectReference != (corev1.LocalObjectReference{}) &&
			v.TLSConfig.CA.LocalObjectReference.Name == hubAmRouterCASecretName &&
			v.BearerToken != nil &&
			v.BearerToken.LocalObjectReference != (corev1.LocalObjectReference{}) &&
			v.BearerToken.LocalObjectReference.Name == hubAmAccessorSecretName {
			containsOCMAlertmanagerConfig = true
			foundHubAmAccessorSecret := &corev1.Secret{}
			err = c.Get(ctx, types.NamespacedName{Name: v.BearerToken.LocalObjectReference.Name,
				Namespace: promNamespace}, foundHubAmAccessorSecret)
			if err != nil {
				t.Fatalf("failed to check the observability-alertmanager-accessor secret %s: %v", clusterMonitoringConfigName, err)
			}
			foundAmAccessorToken, ok := foundHubAmAccessorSecret.Data[hubAmAccessorSecretKey]
			if !ok {
				t.Fatalf("no key %s found in the observability-alertmanager-accessor secret", hubAmAccessorSecretKey)
			}
			if string(foundAmAccessorToken) != testBearerToken {
				t.Fatalf("incorrect token found in the observability-alertmanager-accessor secret, got token: %s, expected value %s", foundAmAccessorToken, testBearerToken)
			}
		}
	}

	if containsOCMAlertmanagerConfig == false {
		t.Fatalf("no AlertmanagerConfig for OCM in ClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs: %v", foundClusterMonitoringConfiguration)
	}

	err = revertClusterMonitoringConfig(ctx, c, false)
	if err != nil {
		t.Fatalf("Failed to revert cluster-monitoring-config configmap: (%v)", err)
	}

	err = c.Get(ctx, types.NamespacedName{Name: clusterMonitoringConfigName,
		Namespace: promNamespace}, foundCusterMonitoringConfigMap)
	if expectedCMDelete {
		if err == nil || !errors.IsNotFound(err) {
			t.Fatalf("the configmap %s should be deleted", clusterMonitoringConfigName)
		}
	}

	foundHubAmAccessorSecret := &corev1.Secret{}
	err = c.Get(ctx, types.NamespacedName{Name: hubAmAccessorSecretName,
		Namespace: promNamespace}, foundHubAmAccessorSecret)
	if err == nil || !errors.IsNotFound(err) {
		t.Fatalf("the secret %s should be deleted", hubAmAccessorSecretName)
	}

	foundHubAmRouterCASecret := &corev1.Secret{}
	err = c.Get(ctx, types.NamespacedName{Name: hubAmRouterCASecretName,
		Namespace: promNamespace}, foundHubAmRouterCASecret)
	if err == nil || !errors.IsNotFound(err) {
		t.Fatalf("the secret %s should be deleted", hubAmRouterCASecretName)
	}

	err = revertClusterMonitoringConfig(ctx, c, false)
	if err != nil {
		t.Fatalf("Run into error when try to revert cluster-monitoring-config configmap twice: (%v)", err)
	}
}
