// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package observabilityendpoint

import (
	"context"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	yamltool "github.com/ghodss/yaml"
	cmomanifests "github.com/openshift/cluster-monitoring-operator/pkg/manifests"

	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"gopkg.in/yaml.v2"
)

const (
	testClusterID = "kind-cluster-id"
	hubInfoYAML   = `
cluster-name: "test-cluster"
endpoint: "http://test-endpoint"
alertmanager-endpoint: "http://test-alertamanger-endpoint"
alertmanager-router-ca: |
    -----BEGIN CERTIFICATE-----
    xxxxxxxxxxxxxxxxxxxxxxxxxxx
    -----END CERTIFICATE-----
`
	hubInfoYAMLAlertsDisabled = `
cluster-name: "test-cluster"
endpoint: "http://test-endpoint"
alertmanager-endpoint: ""
alertmanager-router-ca: |
    -----BEGIN CERTIFICATE-----
    xxxxxxxxxxxxxxxxxxxxxxxxxxx
    -----END CERTIFICATE-----
`
	clusterMonitoringConfigDataYaml = `
prometheusK8s:
  externalLabels:
    managed_cluster: kind-cluster-id
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
      ServerName: ""
      ca:
        key: service-ca.crt
        name: hub-alertmanager-router-ca
      insecureSkipVerify: true`
)

func TestClusterMonitoringConfig(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zap.Options{Development: true})))
	tests := []struct {
		name                                    string
		ClusterMonitoringConfigCMExist          bool
		ClusterMonitoringConfigDataYaml         string
		Manager                                 string
		ExpectedDeleteClusterMonitoringConfigCM bool
	}{
		{
			name:                                    "no cluster-monitoring-config exists",
			ClusterMonitoringConfigCMExist:          false,
			ExpectedDeleteClusterMonitoringConfigCM: false,
		},
		{
			name:                                    "cluster-monitoring-config with empty config.yaml",
			ClusterMonitoringConfigCMExist:          true,
			ClusterMonitoringConfigDataYaml:         "",
			Manager:                                 endpointMonitoringOperatorMgr,
			ExpectedDeleteClusterMonitoringConfigCM: false,
		},
		{
			name:                           "cluster-monitoring-config with non-empty config.yaml and empty prometheusK8s",
			ClusterMonitoringConfigCMExist: true,
			Manager:                        "some-other-manager",
			ClusterMonitoringConfigDataYaml: `
prometheusK8s: null`,
			ExpectedDeleteClusterMonitoringConfigCM: false,
		},
		{
			name:                           "cluster-monitoring-config with non-empty config.yaml and prometheusK8s and empty additionalAlertManagerConfigs",
			ClusterMonitoringConfigCMExist: true,
			ClusterMonitoringConfigDataYaml: `
prometheusK8s:
  additionalAlertManagerConfigs: null`,
			ExpectedDeleteClusterMonitoringConfigCM: false,
		},
		{
			name:                           "cluster-monitoring-config with non-empty config.yaml and empty prometheusK8s with endpoint-monitoring-operator manager",
			ClusterMonitoringConfigCMExist: true,
			Manager:                        endpointMonitoringOperatorMgr,
			ClusterMonitoringConfigDataYaml: `
prometheusK8s: null`,
			ExpectedDeleteClusterMonitoringConfigCM: false,
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
	hubInfoObj := newHubInfoSecret([]byte(hubInfoYAML), testNamespace)
	amAccessSrt := newAMAccessorSecret(testNamespace)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := []runtime.Object{hubInfoObj, amAccessSrt}
			if tt.ClusterMonitoringConfigCMExist {
				objs = append(objs, newClusterMonitoringConfigCM(tt.ClusterMonitoringConfigDataYaml, tt.Manager))
			}
			testCreateOrUpdateClusterMonitoringConfig(t, hubInfo, fake.NewClientBuilder().WithRuntimeObjects(objs...).Build(), tt.ExpectedDeleteClusterMonitoringConfigCM)
		})
	}
}

func TestClusterMonitoringConfigAlertsDisabled(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zap.Options{Development: true})))
	ctx := context.TODO()

	hubInfo := &operatorconfig.HubInfo{}
	err := yaml.Unmarshal([]byte(hubInfoYAMLAlertsDisabled), &hubInfo)
	if err != nil {
		t.Fatalf("Failed to unmarshal hubInfo: (%v)", err)
	}
	hubInfoObj := newHubInfoSecret([]byte(hubInfoYAMLAlertsDisabled), testNamespace)
	amAccessSrt := newAMAccessorSecret(testNamespace)

	// Scenario 1:
	//   create cluster-monitoring-config configmap with "manager: endpoint-monitoring-operator"
	//   Disable alert forwarding
	//   clusterMonitoringRevertedName should be created
	//   cluster-monitoring-config should be removed
	cmc := newClusterMonitoringConfigCM(clusterMonitoringConfigDataYaml, "endpoint-monitoring-operator")
	objs := []runtime.Object{hubInfoObj, amAccessSrt, cmc}
	c := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
	// t.Run("disable alert forwarding as manager", func(t *testing.T) {
	// 	err = createOrUpdateClusterMonitoringConfig(ctx, hubInfo, testClusterID, c, false)
	// 	if err != nil {
	// 		t.Fatalf("Failed to create or update the cluster-monitoring-config configmap: (%v)", err)
	// 	}

	// 	foundclusterMonitoringRevertedCM := &corev1.ConfigMap{}
	// 	err = c.Get(ctx, types.NamespacedName{Name: clusterMonitoringRevertedName,
	// 		Namespace: namespace}, foundclusterMonitoringRevertedCM)
	// 	if err != nil {
	// 		t.Fatalf("failed to retrieve configmap %s: %v", clusterMonitoringRevertedName, err)
	// 	}

	// 	foundCusterMonitoringConfigMap := &corev1.ConfigMap{}
	// 	err = c.Get(ctx, types.NamespacedName{Name: clusterMonitoringConfigName,
	// 		Namespace: promNamespace}, foundCusterMonitoringConfigMap)
	// 	if err != nil {
	// 		t.Fatalf("could not retrieve configmap %s: %v", clusterMonitoringConfigName, err)
	// 	}

	// 	foundClusterMonitoringConfigurationYAML, ok := foundCusterMonitoringConfigMap.Data[clusterMonitoringConfigDataKey]
	// 	if !ok {
	// 		t.Fatalf("configmap: %s doesn't contain key: %s", clusterMonitoringConfigName, clusterMonitoringConfigDataKey)
	// 	}
	// 	foundClusterMonitoringConfigurationJSON, err := yamltool.YAMLToJSON([]byte(foundClusterMonitoringConfigurationYAML))
	// 	if err != nil {
	// 		t.Fatalf("failed to transform YAML to JSON:\n%s\n", foundClusterMonitoringConfigurationYAML)
	// 	}

	// 	foundClusterMonitoringConfiguration := &cmomanifests.ClusterMonitoringConfiguration{}
	// 	if err := json.Unmarshal([]byte(foundClusterMonitoringConfigurationJSON), foundClusterMonitoringConfiguration); err != nil {
	// 		t.Fatalf("failed to marshal the cluster monitoring config: %v:\n%s\n", err, foundClusterMonitoringConfigurationJSON)
	// 	}

	// 	if foundClusterMonitoringConfiguration.PrometheusK8sConfig == nil {
	// 		t.Fatalf("empty prometheusK8s in ClusterMonitoringConfiguration: %v", foundClusterMonitoringConfiguration)
	// 	}

	// 	if label := foundClusterMonitoringConfiguration.PrometheusK8sConfig.ExternalLabels[operatorconfig.ClusterLabelKeyForAlerts]; label != "" {
	// 		t.Fatalf("managed cluster label not deleted on revert: %s:%s",
	// 			operatorconfig.ClusterLabelKeyForAlerts,
	// 			label)
	// 	}

	// 	if foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs != nil {
	// 		t.Fatalf("AlertmanagerConfigs in ClusterMonitoringConfiguration.PrometheusK8sConfig: is not null")
	// 	}
	// })

	// // Scenario 2:
	// //    cluster-monitoring-config created externally (e.g, policy)
	// //    disable alert forwarding
	// //    verify cluster-monitoring-config is not reverted
	// // err = c.Delete(ctx, cmc)
	// // if err != nil {
	// // 	t.Fatalf("could not delete existing cluster-monitoring-config")
	// // }
	// err = c.Create(ctx, newClusterMonitoringConfigCM(clusterMonitoringConfigDataYaml, "some-other-manager"))
	// if err != nil {
	// 	t.Fatalf("could not recreate cluster-monitoring-config with alerts enabled")
	// }
	// t.Run("disable alert forwarding as non-manager", func(t *testing.T) {
	// 	err = createOrUpdateClusterMonitoringConfig(ctx, hubInfo, testClusterID, c, false)
	// 	if err != nil {
	// 		t.Fatalf("Failed to create or update the cluster-monitoring-config configmap: (%v)", err)
	// 	}

	// 	foundclusterMonitoringRevertedCM := &corev1.ConfigMap{}
	// 	err = c.Get(ctx, types.NamespacedName{Name: clusterMonitoringRevertedName,
	// 		Namespace: namespace}, foundclusterMonitoringRevertedCM)
	// 	if err != nil {
	// 		t.Fatalf("failed to retrieve configmap %s: %v", clusterMonitoringRevertedName, err)
	// 	}

	// 	foundCusterMonitoringConfigMap := &corev1.ConfigMap{}
	// 	err = c.Get(ctx, types.NamespacedName{Name: clusterMonitoringConfigName,
	// 		Namespace: promNamespace}, foundCusterMonitoringConfigMap)
	// 	if err != nil {
	// 		t.Fatalf("could not retrieve configmap %s: %v", clusterMonitoringConfigName, err)
	// 	}

	// 	foundClusterMonitoringConfigurationYAML, ok := foundCusterMonitoringConfigMap.Data[clusterMonitoringConfigDataKey]
	// 	if !ok {
	// 		t.Fatalf("configmap: %s doesn't contain key: %s", clusterMonitoringConfigName, clusterMonitoringConfigDataKey)
	// 	}
	// 	foundClusterMonitoringConfigurationJSON, err := yamltool.YAMLToJSON([]byte(foundClusterMonitoringConfigurationYAML))
	// 	if err != nil {
	// 		t.Fatalf("failed to transform YAML to JSON:\n%s\n", foundClusterMonitoringConfigurationYAML)
	// 	}

	// 	foundClusterMonitoringConfiguration := &cmomanifests.ClusterMonitoringConfiguration{}
	// 	if err := json.Unmarshal([]byte(foundClusterMonitoringConfigurationJSON), foundClusterMonitoringConfiguration); err != nil {
	// 		t.Fatalf("failed to marshal the cluster monitoring config: %v:\n%s\n", err, foundClusterMonitoringConfigurationJSON)
	// 	}

	// 	if foundClusterMonitoringConfiguration.PrometheusK8sConfig == nil {
	// 		t.Fatalf("empty prometheusK8s in ClusterMonitoringConfiguration: %v, expected to preserve content", foundClusterMonitoringConfiguration)
	// 	}

	// 	if label := foundClusterMonitoringConfiguration.PrometheusK8sConfig.ExternalLabels[operatorconfig.ClusterLabelKeyForAlerts]; label != "" {
	// 		t.Fatalf("managed cluster label not deleted on revert: %s:%s",
	// 			operatorconfig.ClusterLabelKeyForAlerts,
	// 			label)
	// 	}

	// 	if foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs == nil {
	// 		t.Fatalf("AlertmanagerConfigs in ClusterMonitoringConfiguration.PrometheusK8sConfig reverted, expected to be preserved")
	// 	}
	// })

	// Scenario 3:
	//   Reenable alert forwarding.
	//   verify clusterMonitoringRevertedName is deleted
	//   cluster-monitoring-config restored
	err = c.Delete(ctx, hubInfoObj)
	if err != nil {
		t.Fatalf("could not delete existing hubInfoObj")
	}
	err = yaml.Unmarshal([]byte(hubInfoYAML), &hubInfo)
	if err != nil {
		t.Fatalf("failed to unmarshal hubInfo: (%v)", err)
	}
	hubInfoObj = newHubInfoSecret([]byte(hubInfoYAML), testNamespace)
	err = c.Create(ctx, hubInfoObj)
	if err != nil {
		t.Fatalf("could not recreate hubInfoObject to enable alerts again")
	}
	t.Run("Reenable alert forwarding", func(t *testing.T) {
		err = createOrUpdateClusterMonitoringConfig(ctx, hubInfo, testClusterID, c, false)
		if err != nil {
			t.Fatalf("Failed to create or update the cluster-monitoring-config configmap: (%v)", err)
		}

		foundclusterMonitoringRevertedCM := &corev1.ConfigMap{}
		err = c.Get(ctx, types.NamespacedName{Name: clusterMonitoringRevertedName,
			Namespace: namespace}, foundclusterMonitoringRevertedCM)
		if err == nil {
			t.Fatalf("configmap %s still present after reenabling alerts", clusterMonitoringRevertedName)
		}

		foundCusterMonitoringConfigMap := &corev1.ConfigMap{}
		err = c.Get(ctx, types.NamespacedName{Name: clusterMonitoringConfigName,
			Namespace: promNamespace}, foundCusterMonitoringConfigMap)
		if err != nil {
			t.Fatalf("could not retrieve configmap %s: %v", clusterMonitoringConfigName, err)
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

		label := foundClusterMonitoringConfiguration.PrometheusK8sConfig.ExternalLabels[operatorconfig.ClusterLabelKeyForAlerts]
		if label != testClusterID {
			t.Fatalf("label %s not set to %s", operatorconfig.ClusterLabelKeyForAlerts, testClusterID)
		}

		if foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs == nil {
			t.Fatalf("AlertmanagerConfigs is nil after reenabling alerts")
		}
	})
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

	err = revertClusterMonitoringConfig(ctx, c)
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
	if err != nil {
		t.Fatalf("the secret %s should not be deleted", hubAmAccessorSecretName)
	}

	foundHubAmRouterCASecret := &corev1.Secret{}
	err = c.Get(ctx, types.NamespacedName{Name: hubAmRouterCASecretName,
		Namespace: promNamespace}, foundHubAmRouterCASecret)
	if err != nil {
		t.Fatalf("the secret %s should not be deleted", hubAmRouterCASecretName)
	}

	err = revertClusterMonitoringConfig(ctx, c)
	if err != nil {
		t.Fatalf("Run into error when try to revert cluster-monitoring-config configmap twice: (%v)", err)
	}
}
