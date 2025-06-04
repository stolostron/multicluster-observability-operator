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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	yamltool "github.com/ghodss/yaml"
	cmomanifests "github.com/openshift/cluster-monitoring-operator/pkg/manifests"
	"github.com/stretchr/testify/assert"

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
	testNamespace := "test-ns"
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
	tokenValue := "test-token"
	amAccessSrt := newAMAccessorSecret(testNamespace, tokenValue)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := []runtime.Object{hubInfoObj, amAccessSrt}
			if tt.ClusterMonitoringConfigCMExist {
				objs = append(objs, newClusterMonitoringConfigCM(tt.ClusterMonitoringConfigDataYaml, tt.Manager))
			}
			testCreateOrUpdateClusterMonitoringConfig(t, hubInfo, fake.NewClientBuilder().WithRuntimeObjects(objs...).Build(), tt.ExpectedDeleteClusterMonitoringConfigCM, tokenValue, testNamespace)
		})
	}
}

// When the cluster-monitoring-config is unchanged, no need to update it
func TestClusterMonitoringConfigUnchanged(t *testing.T) {
	testNamespace := "test-ns"
	amAccessSrt := newAMAccessorSecret(testNamespace, "test-token")
	hubInfo := &operatorconfig.HubInfo{
		ClusterName:              "test-cluster",
		ObservatoriumAPIEndpoint: "http://test-endpoint",
		AlertmanagerEndpoint:     "http://test-alertamanger-endpoint",
	}
	cmoCfg := newClusterMonitoringConfigCM(clusterMonitoringConfigDataYaml, endpointMonitoringOperatorMgr)
	client := fake.NewClientBuilder().WithRuntimeObjects(newHubInfoSecret([]byte(hubInfoYAML), testNamespace), cmoCfg, amAccessSrt).Build()
	wasUpdated, err := createOrUpdateClusterMonitoringConfig(context.Background(), hubInfo, testClusterID, client, false, testNamespace)
	if err != nil {
		t.Fatalf("Failed to create or update the cluster-monitoring-config configmap: (%v)", err)
	}
	assert.True(t, wasUpdated)

	cmoCfgBeforeUpdate := &corev1.ConfigMap{}
	err = client.Get(context.Background(), types.NamespacedName{Name: clusterMonitoringConfigName, Namespace: promNamespace}, cmoCfgBeforeUpdate)
	if err != nil {
		t.Fatalf("failed to check configmap %s: %v", clusterMonitoringConfigName, err)
	}

	wasUpdated, err = createOrUpdateClusterMonitoringConfig(context.Background(), hubInfo, testClusterID, client, false, testNamespace)
	if err != nil {
		t.Fatalf("Failed to create or update the cluster-monitoring-config configmap: (%v)", err)
	}
	assert.False(t, wasUpdated)

	cmoCfgAfterUpdate := &corev1.ConfigMap{}
	err = client.Get(context.Background(), types.NamespacedName{Name: clusterMonitoringConfigName, Namespace: promNamespace}, cmoCfgAfterUpdate)
	if err != nil {
		t.Fatalf("failed to check configmap %s: %v", clusterMonitoringConfigName, err)
	}

	if cmoCfgBeforeUpdate.ResourceVersion != cmoCfgAfterUpdate.ResourceVersion {
		t.Fatalf("The cluster-monitoring-config configmap should not be updated")
	}
}

func TestClusterMonitoringConfigAlertsDisabled(t *testing.T) {
	testNamespace := "test-ns"
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zap.Options{Development: true})))
	ctx := context.TODO()

	hubInfo := &operatorconfig.HubInfo{}
	err := yaml.Unmarshal([]byte(hubInfoYAMLAlertsDisabled), &hubInfo)
	if err != nil {
		t.Fatalf("Failed to unmarshal hubInfo: (%v)", err)
	}
	hubInfoObj := newHubInfoSecret([]byte(hubInfoYAMLAlertsDisabled), testNamespace)
	amAccessSrt := newAMAccessorSecret(testNamespace, "test-token")

	// Scenario 1:
	//   create cluster-monitoring-config configmap with "manager: endpoint-monitoring-operator"
	//   Disable alert forwarding
	//   clusterMonitoringRevertedName should be created
	//   cluster-monitoring-config should be removed
	cmc := newClusterMonitoringConfigCM(clusterMonitoringConfigDataYaml, "endpoint-monitoring-operator")
	objs := []runtime.Object{hubInfoObj, amAccessSrt, cmc}
	c := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()

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
		wasUpdated, err := createOrUpdateClusterMonitoringConfig(ctx, hubInfo, testClusterID, c, false, testNamespace)
		if err != nil {
			t.Fatalf("Failed to create or update the cluster-monitoring-config configmap: (%v)", err)
		}
		assert.True(t, wasUpdated)

		foundclusterMonitoringRevertedCM := &corev1.ConfigMap{}
		err = c.Get(ctx, types.NamespacedName{Name: clusterMonitoringRevertedName,
			Namespace: testNamespace}, foundclusterMonitoringRevertedCM)
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

func testCreateOrUpdateClusterMonitoringConfig(t *testing.T, hubInfo *operatorconfig.HubInfo, c client.Client, expectedCMDelete bool, tokenValue, ns string) {
	ctx := context.TODO()
	wasUpdated, err := createOrUpdateClusterMonitoringConfig(ctx, hubInfo, testClusterID, c, false, ns)
	if err != nil {
		t.Fatalf("Failed to create or update the cluster-monitoring-config configmap: (%v)", err)
	}
	assert.True(t, wasUpdated)

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
			if string(foundAmAccessorToken) != tokenValue {
				t.Fatalf("incorrect token found in the observability-alertmanager-accessor secret, got token: %s, expected value %s", foundAmAccessorToken, tokenValue)
			}
		}
	}

	if containsOCMAlertmanagerConfig == false {
		t.Fatalf("no AlertmanagerConfig for OCM in ClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs: %v", foundClusterMonitoringConfiguration)
	}

	err = RevertClusterMonitoringConfig(ctx, c)
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

	err = RevertClusterMonitoringConfig(ctx, c)
	if err != nil {
		t.Fatalf("Run into error when try to revert cluster-monitoring-config configmap twice: (%v)", err)
	}
}

func TestUserWorkloadMonitoringConfig(t *testing.T) {
	testNamespace := operatorconfig.OCPUserWorkloadMonitoringNamespace
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zap.Options{Development: true})))
	tests := []struct {
		name                                   string
		UserWorkloadMonitoringConfigCMExist    bool
		UserWorkloadMonitoringConfigDataYaml   string
		Manager                                string
		ExpectedDeleteUserWorkloadMonitoringCM bool
	}{
		{
			name:                                   "no user-workload-monitoring-config exists",
			UserWorkloadMonitoringConfigCMExist:    false,
			ExpectedDeleteUserWorkloadMonitoringCM: false,
		},
		{
			name:                                   "user-workload-monitoring-config with empty config.yaml",
			UserWorkloadMonitoringConfigCMExist:    true,
			UserWorkloadMonitoringConfigDataYaml:   "",
			Manager:                                endpointMonitoringOperatorMgr,
			ExpectedDeleteUserWorkloadMonitoringCM: false,
		},
		{
			name:                                "user-workload-monitoring-config with non-empty config.yaml and empty prometheus",
			UserWorkloadMonitoringConfigCMExist: true,
			Manager:                             "some-other-manager",
			UserWorkloadMonitoringConfigDataYaml: `
prometheus: null`,
			ExpectedDeleteUserWorkloadMonitoringCM: false,
		},
		{
			name:                                "user-workload-monitoring-config with non-empty config.yaml and prometheus and empty alertmanagerConfigs",
			UserWorkloadMonitoringConfigCMExist: true,
			UserWorkloadMonitoringConfigDataYaml: `
prometheus:
  alertmanagerConfigs: null`,
			ExpectedDeleteUserWorkloadMonitoringCM: false,
		},
		{
			name:                                "user-workload-monitoring-config with non-empty config.yaml and empty prometheus with endpoint-monitoring-operator manager",
			UserWorkloadMonitoringConfigCMExist: true,
			Manager:                             endpointMonitoringOperatorMgr,
			UserWorkloadMonitoringConfigDataYaml: `
prometheus: null`,
			ExpectedDeleteUserWorkloadMonitoringCM: false,
		},
		{
			name:                                "user-workload-monitoring-config with non-empty config.yaml and prometheus and alertmanagerConfigs",
			UserWorkloadMonitoringConfigCMExist: true,
			UserWorkloadMonitoringConfigDataYaml: `
prometheus:
  alertmanagerConfigs:
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
			ExpectedDeleteUserWorkloadMonitoringCM: false,
		},
	}

	hubInfo := &operatorconfig.HubInfo{}
	err := yaml.Unmarshal([]byte(hubInfoYAML), &hubInfo)
	if err != nil {
		t.Fatalf("Failed to unmarshal hubInfo: (%v)", err)
	}
	hubInfoObj := newHubInfoSecret([]byte(hubInfoYAML), testNamespace)
	tokenValue := "test-token"
	amAccessSrt := newAMAccessorSecret(testNamespace, tokenValue)

	// Create router CA secret in the user workload monitoring namespace
	routerCASecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hubAmRouterCASecretName,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			"service-ca.crt": []byte("test-ca-crt"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := []runtime.Object{hubInfoObj, amAccessSrt, routerCASecret}
			if tt.UserWorkloadMonitoringConfigCMExist {
				objs = append(objs, newUserWorkloadMonitoringConfigCM(tt.UserWorkloadMonitoringConfigDataYaml, tt.Manager))
			}
			testCreateOrUpdateUserWorkloadMonitoringConfig(t, hubInfo, fake.NewClientBuilder().WithRuntimeObjects(objs...).Build(), tt.ExpectedDeleteUserWorkloadMonitoringCM, tokenValue)
		})
	}
}

func newUserWorkloadMonitoringConfigCM(configDataStr string, mgr string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
			Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
			ManagedFields: []metav1.ManagedFieldsEntry{
				{
					Manager:   mgr,
					Operation: metav1.ManagedFieldsOperationUpdate,
				},
			},
		},
		Data: map[string]string{
			"config.yaml": configDataStr,
		},
	}
}

func testCreateOrUpdateUserWorkloadMonitoringConfig(t *testing.T, hubInfo *operatorconfig.HubInfo, c client.Client, expectedCMDelete bool, tokenValue string) {
	ctx := context.TODO()
	err := createOrUpdateUserWorkloadMonitoringConfig(ctx, c, hubInfo)
	if err != nil {
		t.Fatalf("Failed to create or update the user-workload-monitoring-config configmap: (%v)", err)
	}

	foundUserWorkloadMonitoringConfigMap := &corev1.ConfigMap{}
	err = c.Get(ctx, types.NamespacedName{
		Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
		Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
	}, foundUserWorkloadMonitoringConfigMap)
	if err != nil {
		t.Fatalf("failed to check configmap %s: %v", operatorconfig.OCPUserWorkloadMonitoringConfigMap, err)
	}

	foundUserWorkloadConfigurationYAML, ok := foundUserWorkloadMonitoringConfigMap.Data["config.yaml"]
	if !ok {
		t.Fatalf("configmap: %s doesn't contain key: config.yaml", operatorconfig.OCPUserWorkloadMonitoringConfigMap)
	}
	foundUserWorkloadConfigurationJSON, err := yamltool.YAMLToJSON([]byte(foundUserWorkloadConfigurationYAML))
	if err != nil {
		t.Fatalf("failed to transform YAML to JSON:\n%s\n", foundUserWorkloadConfigurationYAML)
	}

	foundUserWorkloadConfiguration := &cmomanifests.UserWorkloadConfiguration{}
	if err := json.Unmarshal([]byte(foundUserWorkloadConfigurationJSON), foundUserWorkloadConfiguration); err != nil {
		t.Fatalf("failed to marshal the user workload monitoring config: %v:\n%s\n", err, foundUserWorkloadConfigurationJSON)
	}

	if foundUserWorkloadConfiguration.Prometheus == nil {
		t.Fatalf("empty prometheus in UserWorkloadConfiguration: %v", foundUserWorkloadConfiguration)
	}

	if foundUserWorkloadConfiguration.Prometheus.AlertmanagerConfigs == nil {
		t.Fatalf("empty AlertmanagerConfigs in UserWorkloadConfiguration.Prometheus: %v", foundUserWorkloadConfiguration)
	}

	containsOCMAlertmanagerConfig := false
	for _, v := range foundUserWorkloadConfiguration.Prometheus.AlertmanagerConfigs {
		if v.TLSConfig != (cmomanifests.TLSConfig{}) &&
			v.TLSConfig.CA != nil &&
			v.TLSConfig.CA.LocalObjectReference != (corev1.LocalObjectReference{}) &&
			v.TLSConfig.CA.LocalObjectReference.Name == hubAmRouterCASecretName &&
			v.BearerToken != nil &&
			v.BearerToken.LocalObjectReference != (corev1.LocalObjectReference{}) &&
			v.BearerToken.LocalObjectReference.Name == hubAmAccessorSecretName {
			containsOCMAlertmanagerConfig = true
			foundHubAmAccessorSecret := &corev1.Secret{}
			err = c.Get(ctx, types.NamespacedName{
				Name:      v.BearerToken.LocalObjectReference.Name,
				Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
			}, foundHubAmAccessorSecret)
			if err != nil {
				t.Fatalf("failed to check the observability-alertmanager-accessor secret %s: %v", operatorconfig.OCPUserWorkloadMonitoringConfigMap, err)
			}
			foundAmAccessorToken, ok := foundHubAmAccessorSecret.Data[hubAmAccessorSecretKey]
			if !ok {
				t.Fatalf("no key %s found in the observability-alertmanager-accessor secret", hubAmAccessorSecretKey)
			}
			if string(foundAmAccessorToken) != tokenValue {
				t.Fatalf("incorrect token found in the observability-alertmanager-accessor secret, got token: %s, expected value %s", foundAmAccessorToken, tokenValue)
			}
		}
	}

	if containsOCMAlertmanagerConfig == false {
		t.Fatalf("no AlertmanagerConfig for OCM in UserWorkloadConfiguration.Prometheus.AlertmanagerConfigs: %v", foundUserWorkloadConfiguration)
	}

	err = RevertUserWorkloadMonitoringConfig(ctx, c)
	if err != nil {
		t.Fatalf("Failed to revert user-workload-monitoring-config configmap: (%v)", err)
	}

	err = c.Get(ctx, types.NamespacedName{
		Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
		Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
	}, foundUserWorkloadMonitoringConfigMap)
	if expectedCMDelete {
		if err == nil || !errors.IsNotFound(err) {
			t.Fatalf("the configmap %s should be deleted", operatorconfig.OCPUserWorkloadMonitoringConfigMap)
		}
	}

	foundHubAmAccessorSecret := &corev1.Secret{}
	err = c.Get(ctx, types.NamespacedName{
		Name:      hubAmAccessorSecretName,
		Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
	}, foundHubAmAccessorSecret)
	if err != nil {
		t.Fatalf("the secret %s should not be deleted", hubAmAccessorSecretName)
	}

	foundHubAmRouterCASecret := &corev1.Secret{}
	err = c.Get(ctx, types.NamespacedName{
		Name:      hubAmRouterCASecretName,
		Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
	}, foundHubAmRouterCASecret)
	if err != nil {
		t.Fatalf("the secret %s should not be deleted", hubAmRouterCASecretName)
	}

	err = RevertUserWorkloadMonitoringConfig(ctx, c)
	if err != nil {
		t.Fatalf("Run into error when try to revert user-workload-monitoring-config configmap twice: (%v)", err)
	}
}

func TestUserWorkloadMonitoringConfigAlertsDisabled(t *testing.T) {
	testNamespace := operatorconfig.OCPUserWorkloadMonitoringNamespace
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zap.Options{Development: true})))
	ctx := context.TODO()

	hubInfo := &operatorconfig.HubInfo{}
	err := yaml.Unmarshal([]byte(hubInfoYAMLAlertsDisabled), &hubInfo)
	if err != nil {
		t.Fatalf("Failed to unmarshal hubInfo: (%v)", err)
	}
	hubInfoObj := newHubInfoSecret([]byte(hubInfoYAMLAlertsDisabled), testNamespace)
	amAccessSrt := newAMAccessorSecret(testNamespace, "test-token")

	// Create router CA secret in the user workload monitoring namespace
	routerCASecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hubAmRouterCASecretName,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			"service-ca.crt": []byte("test-ca-crt"),
		},
	}

	// Create user-workload-monitoring-config configmap with "manager: endpoint-monitoring-operator"
	uwmConfig := newUserWorkloadMonitoringConfigCM(`
prometheus:
  alertmanagerConfigs:
  - apiVersion: v2
    bearerToken:
      key: token
      name: foo
    pathPrefix: /
    scheme: https
    staticConfigs:
    - test-host.com
    tlsConfig:
      insecureSkipVerify: true`, "endpoint-monitoring-operator")

	objs := []runtime.Object{hubInfoObj, amAccessSrt, routerCASecret, uwmConfig}
	c := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()

	// Test disabling alert forwarding
	err = createOrUpdateUserWorkloadMonitoringConfig(ctx, c, hubInfo)
	if err != nil {
		t.Fatalf("Failed to create or update the user-workload-monitoring-config configmap: (%v)", err)
	}

	// Verify the configmap is updated correctly
	foundUserWorkloadMonitoringConfigMap := &corev1.ConfigMap{}
	err = c.Get(ctx, types.NamespacedName{
		Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
		Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
	}, foundUserWorkloadMonitoringConfigMap)
	if err != nil {
		t.Fatalf("failed to check configmap %s: %v", operatorconfig.OCPUserWorkloadMonitoringConfigMap, err)
	}

	foundUserWorkloadConfigurationYAML, ok := foundUserWorkloadMonitoringConfigMap.Data["config.yaml"]
	if !ok {
		t.Fatalf("configmap: %s doesn't contain key: config.yaml", operatorconfig.OCPUserWorkloadMonitoringConfigMap)
	}

	foundUserWorkloadConfigurationJSON, err := yamltool.YAMLToJSON([]byte(foundUserWorkloadConfigurationYAML))
	if err != nil {
		t.Fatalf("failed to transform YAML to JSON:\n%s\n", foundUserWorkloadConfigurationYAML)
	}

	foundUserWorkloadConfiguration := &cmomanifests.UserWorkloadConfiguration{}
	if err := json.Unmarshal([]byte(foundUserWorkloadConfigurationJSON), foundUserWorkloadConfiguration); err != nil {
		t.Fatalf("failed to marshal the user workload monitoring config: %v:\n%s\n", err, foundUserWorkloadConfigurationJSON)
	}

	if foundUserWorkloadConfiguration.Prometheus == nil {
		t.Fatalf("empty prometheus in UserWorkloadConfiguration: %v", foundUserWorkloadConfiguration)
	}

	if foundUserWorkloadConfiguration.Prometheus.AlertmanagerConfigs != nil && len(foundUserWorkloadConfiguration.Prometheus.AlertmanagerConfigs) > 0 {
		t.Fatalf("AlertmanagerConfigs should be nil or empty when alerts are disabled, got: %v", foundUserWorkloadConfiguration.Prometheus.AlertmanagerConfigs)
	}

	// Test re-enabling alert forwarding
	hubInfo = &operatorconfig.HubInfo{}
	err = yaml.Unmarshal([]byte(hubInfoYAML), &hubInfo)
	if err != nil {
		t.Fatalf("Failed to unmarshal hubInfo: (%v)", err)
	}

	err = createOrUpdateUserWorkloadMonitoringConfig(ctx, c, hubInfo)
	if err != nil {
		t.Fatalf("Failed to create or update the user-workload-monitoring-config configmap: (%v)", err)
	}

	// Verify the configmap is updated correctly
	err = c.Get(ctx, types.NamespacedName{
		Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
		Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
	}, foundUserWorkloadMonitoringConfigMap)
	if err != nil {
		t.Fatalf("failed to check configmap %s: %v", operatorconfig.OCPUserWorkloadMonitoringConfigMap, err)
	}

	foundUserWorkloadConfigurationYAML, ok = foundUserWorkloadMonitoringConfigMap.Data["config.yaml"]
	if !ok {
		t.Fatalf("configmap: %s doesn't contain key: config.yaml", operatorconfig.OCPUserWorkloadMonitoringConfigMap)
	}

	foundUserWorkloadConfigurationJSON, err = yamltool.YAMLToJSON([]byte(foundUserWorkloadConfigurationYAML))
	if err != nil {
		t.Fatalf("failed to transform YAML to JSON:\n%s\n", foundUserWorkloadConfigurationYAML)
	}

	foundUserWorkloadConfiguration = &cmomanifests.UserWorkloadConfiguration{}
	if err := json.Unmarshal([]byte(foundUserWorkloadConfigurationJSON), foundUserWorkloadConfiguration); err != nil {
		t.Fatalf("failed to marshal the user workload monitoring config: %v:\n%s\n", err, foundUserWorkloadConfigurationJSON)
	}

	if foundUserWorkloadConfiguration.Prometheus == nil {
		t.Fatalf("empty prometheus in UserWorkloadConfiguration: %v", foundUserWorkloadConfiguration)
	}

	if foundUserWorkloadConfiguration.Prometheus.AlertmanagerConfigs == nil {
		t.Fatalf("AlertmanagerConfigs should not be nil when alerts are enabled")
	}

	containsOCMAlertmanagerConfig := false
	for _, v := range foundUserWorkloadConfiguration.Prometheus.AlertmanagerConfigs {
		if v.TLSConfig != (cmomanifests.TLSConfig{}) &&
			v.TLSConfig.CA != nil &&
			v.TLSConfig.CA.LocalObjectReference != (corev1.LocalObjectReference{}) &&
			v.TLSConfig.CA.LocalObjectReference.Name == hubAmRouterCASecretName {
			containsOCMAlertmanagerConfig = true
		}
	}

	if !containsOCMAlertmanagerConfig {
		t.Fatalf("AlertmanagerConfigs should contain OCM config when alerts are enabled")
	}
}
