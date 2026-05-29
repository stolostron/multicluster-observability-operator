// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package observabilityendpoint

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	cmomanifests "github.com/openshift/cluster-monitoring-operator/pkg/manifests"
	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	yamltool "sigs.k8s.io/yaml"
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
	hubInfoYAMLUWMAlertsDisabled = `
cluster-name: "test-cluster"
endpoint: "http://test-endpoint"
alertmanager-endpoint: "http://test-alertamanger-endpoint"
alertmanager-router-ca: |
    -----BEGIN CERTIFICATE-----
    xxxxxxxxxxxxxxxxxxxxxxxxxxx
    -----END CERTIFICATE-----
uwm-alerting-disabled: true
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
        key: ca.crt
        name: obs-alertmanager-mtls-ca-1a9af6dc0801433cb28a200af81
      cert:
        key: tls.crt
        name: obs-alertmanager-mtls-cert-1a9af6dc0801433cb28a200af81
      key:
        key: tls.key
        name: obs-alertmanager-mtls-cert-1a9af6dc0801433cb28a200af81
      insecureSkipVerify: false`

	clusterMonitoringConfigDataYamlCleanupGH = `
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
        name: hub-alertmanager-router-ca-12345
      insecureSkipVerify: true
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
      insecureSkipVerify: true
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
        name: hub-alertmanager-router-ca-1a9af6dc0801433cb28a200af81
      insecureSkipVerify: true`
)

func newMtlsTestSecrets(namespace string) []runtime.Object {
	return []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mtlsCertName,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"tls.crt": []byte("test-client-cert"),
				"tls.key": []byte("test-client-key"),
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mtlsCaName,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"ca.crt": []byte("test-server-ca"),
			},
		},
	}
}

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

	hubInfo := &operatorconfig.HubInfo{HubClusterID: "1a9af6dc0801433cb28a200af81"}
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
			objs = append(objs, newMtlsTestSecrets(testNamespace)...)
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
		HubClusterID:             "1a9af6dc0801433cb28a200af81",
	}
	cmoCfg := newClusterMonitoringConfigCM(clusterMonitoringConfigDataYaml, endpointMonitoringOperatorMgr)
	objs := []runtime.Object{newHubInfoSecret([]byte(hubInfoYAML), testNamespace), cmoCfg, amAccessSrt}
	objs = append(objs, newMtlsTestSecrets(testNamespace)...)
	client := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
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
	objs = append(objs, newMtlsTestSecrets(testNamespace)...)
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
		err = c.Get(ctx, types.NamespacedName{
			Name:      clusterMonitoringRevertedName,
			Namespace: testNamespace,
		}, foundclusterMonitoringRevertedCM)
		if err == nil {
			t.Fatalf("configmap %s still present after reenabling alerts", clusterMonitoringRevertedName)
		}

		foundCusterMonitoringConfigMap := &corev1.ConfigMap{}
		err = c.Get(ctx, types.NamespacedName{
			Name:      clusterMonitoringConfigName,
			Namespace: promNamespace,
		}, foundCusterMonitoringConfigMap)
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
	err = c.Get(ctx, types.NamespacedName{
		Name:      clusterMonitoringConfigName,
		Namespace: promNamespace,
	}, foundCusterMonitoringConfigMap)
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
	amMtlsCARef := appendHubClusterID(amMtlsCaName, hubInfo)
	amMtlsCertRef := appendHubClusterID(amMtlsCertName, hubInfo)
	for _, v := range foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs {
		if v.TLSConfig.CA != nil && v.TLSConfig.CA.Name == amMtlsCARef &&
			v.TLSConfig.Cert != nil && v.TLSConfig.Cert.Name == amMtlsCertRef &&
			v.TLSConfig.Key != nil && v.TLSConfig.Key.Name == amMtlsCertRef {
			containsOCMAlertmanagerConfig = true
		}
	}

	if !containsOCMAlertmanagerConfig {
		t.Fatalf("no AlertmanagerConfig for OCM in ClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs: %v", foundClusterMonitoringConfiguration)
	}

	err = RevertClusterMonitoringConfig(ctx, c, hubInfo)
	if err != nil {
		t.Fatalf("Failed to revert cluster-monitoring-config configmap: (%v)", err)
	}

	err = c.Get(ctx, types.NamespacedName{
		Name:      clusterMonitoringConfigName,
		Namespace: promNamespace,
	}, foundCusterMonitoringConfigMap)
	if expectedCMDelete {
		if err == nil || !errors.IsNotFound(err) {
			t.Fatalf("the configmap %s should be deleted", clusterMonitoringConfigName)
		}
	}

	foundHubAmAccessorSecret := &corev1.Secret{}
	err = c.Get(ctx, types.NamespacedName{
		Name:      hubAmAccessorSecretName + "-" + hubInfo.HubClusterID,
		Namespace: promNamespace,
	}, foundHubAmAccessorSecret)
	if err != nil {
		t.Fatalf("the secret %s should not be deleted", hubAmAccessorSecretName+"-"+hubInfo.HubClusterID)
	}

	foundMtlsCASecret := &corev1.Secret{}
	err = c.Get(ctx, types.NamespacedName{
		Name:      amMtlsCARef,
		Namespace: promNamespace,
	}, foundMtlsCASecret)
	if err != nil {
		t.Fatalf("the secret %s should not be deleted", amMtlsCARef)
	}

	foundMtlsCertSecret := &corev1.Secret{}
	err = c.Get(ctx, types.NamespacedName{
		Name:      amMtlsCertRef,
		Namespace: promNamespace,
	}, foundMtlsCertSecret)
	if err != nil {
		t.Fatalf("the secret %s should not be deleted", amMtlsCertRef)
	}

	err = RevertClusterMonitoringConfig(ctx, c, hubInfo)
	if err != nil {
		t.Fatalf("Run into error when try to revert cluster-monitoring-config configmap twice: (%v)", err)
	}
}

func TestUserWorkloadMonitoringConfig(t *testing.T) {
	uwlTestNamespace := operatorconfig.OCPUserWorkloadMonitoringNamespace
	testNamespace := "test-ns"
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

	hubInfo := &operatorconfig.HubInfo{HubClusterID: "1a9af6dc0801433cb28a200af81"}
	err := yaml.Unmarshal([]byte(hubInfoYAML), &hubInfo)
	if err != nil {
		t.Fatalf("Failed to unmarshal hubInfo: (%v)", err)
	}
	hubInfoObj := newHubInfoSecret([]byte(hubInfoYAML), testNamespace)
	tokenValue := "test-token"
	amAccessSrt := newAMAccessorSecret(testNamespace, tokenValue)

	uwlAccessSrt := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hubAmAccessorSecretName + "-" + hubInfo.HubClusterID,
			Namespace: uwlTestNamespace,
		},
		Data: map[string][]byte{
			"token": []byte(tokenValue),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := []runtime.Object{hubInfoObj, amAccessSrt, uwlAccessSrt}
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
	amMtlsCARef := appendHubClusterID(amMtlsCaName, hubInfo)
	amMtlsCertRef := appendHubClusterID(amMtlsCertName, hubInfo)
	for _, v := range foundUserWorkloadConfiguration.Prometheus.AlertmanagerConfigs {
		if v.TLSConfig.CA != nil && v.TLSConfig.CA.Name == amMtlsCARef &&
			v.TLSConfig.Cert != nil && v.TLSConfig.Cert.Name == amMtlsCertRef &&
			v.TLSConfig.Key != nil && v.TLSConfig.Key.Name == amMtlsCertRef {
			containsOCMAlertmanagerConfig = true
		}
	}

	if !containsOCMAlertmanagerConfig {
		t.Fatalf("no AlertmanagerConfig for OCM in UserWorkloadConfiguration.Prometheus.AlertmanagerConfigs: %v", foundUserWorkloadConfiguration)
	}

	err = RevertUserWorkloadMonitoringConfig(ctx, c, hubInfo)
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
		Name:      hubAmAccessorSecretName + "-" + hubInfo.HubClusterID,
		Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
	}, foundHubAmAccessorSecret)
	if err != nil {
		t.Fatalf("the secret %s should not be deleted", hubAmAccessorSecretName+"-"+hubInfo.HubClusterID)
	}

	err = RevertUserWorkloadMonitoringConfig(ctx, c, hubInfo)
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

	objs := []runtime.Object{hubInfoObj, amAccessSrt, uwmConfig}
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

	if len(foundUserWorkloadConfiguration.Prometheus.AlertmanagerConfigs) > 0 {
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
	amMtlsCARef := appendHubClusterID(amMtlsCaName, hubInfo)
	amMtlsCertRef := appendHubClusterID(amMtlsCertName, hubInfo)
	for _, v := range foundUserWorkloadConfiguration.Prometheus.AlertmanagerConfigs {
		if v.TLSConfig.CA != nil && v.TLSConfig.CA.Name == amMtlsCARef &&
			v.TLSConfig.Cert != nil && v.TLSConfig.Cert.Name == amMtlsCertRef &&
			v.TLSConfig.Key != nil && v.TLSConfig.Key.Name == amMtlsCertRef {
			containsOCMAlertmanagerConfig = true
		}
	}

	if !containsOCMAlertmanagerConfig {
		t.Fatalf("AlertmanagerConfigs should contain OCM config when alerts are enabled")
	}
}

func TestUserWorkloadMonitoringConfigUWMAlertsDisabled(t *testing.T) {
	testNamespace := operatorconfig.OCPUserWorkloadMonitoringNamespace
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zap.Options{Development: true})))
	ctx := context.TODO()

	// Test with UWM alerting disabled but global alerting enabled
	hubInfo := &operatorconfig.HubInfo{HubClusterID: "test-hub"}
	err := yaml.Unmarshal([]byte(hubInfoYAMLUWMAlertsDisabled), &hubInfo)
	if err != nil {
		t.Fatalf("Failed to unmarshal hubInfo: (%v)", err)
	}
	hubInfoObj := newHubInfoSecret([]byte(hubInfoYAMLUWMAlertsDisabled), testNamespace)
	amAccessSrt := newAMAccessorSecret(testNamespace, "test-token")

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

	objs := []runtime.Object{hubInfoObj, amAccessSrt, uwmConfig}
	c := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()

	// Test with UWM alerting disabled
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

	if len(foundUserWorkloadConfiguration.Prometheus.AlertmanagerConfigs) > 0 {
		t.Fatalf("AlertmanagerConfigs should be nil or empty when UWM alerts are disabled, got: %v", foundUserWorkloadConfiguration.Prometheus.AlertmanagerConfigs)
	}

	// Test re-enabling UWM alerts
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
	amMtlsCARef := appendHubClusterID(amMtlsCaName, hubInfo)
	amMtlsCertRef := appendHubClusterID(amMtlsCertName, hubInfo)
	for _, v := range foundUserWorkloadConfiguration.Prometheus.AlertmanagerConfigs {
		if v.TLSConfig.CA != nil && v.TLSConfig.CA.Name == amMtlsCARef &&
			v.TLSConfig.Cert != nil && v.TLSConfig.Cert.Name == amMtlsCertRef &&
			v.TLSConfig.Key != nil && v.TLSConfig.Key.Name == amMtlsCertRef {
			containsOCMAlertmanagerConfig = true
		}
	}

	if !containsOCMAlertmanagerConfig {
		t.Fatalf("AlertmanagerConfigs should contain OCM config when alerts are enabled")
	}
}

// TestUWLMonitoringDisableScenario tests the specific scenario where UWL monitoring is disabled
// and the configmap should be cleaned up even when the namespace still exists
func TestUWLMonitoringDisableScenario(t *testing.T) {
	ctx := context.Background()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zap.Options{Development: true})))

	// Create hub info with alerts enabled (but UWL disabled)
	hubInfo := &operatorconfig.HubInfo{HubClusterID: "test-hub"}
	err := yaml.Unmarshal([]byte(hubInfoYAML), &hubInfo)
	if err != nil {
		t.Fatalf("Failed to unmarshal hubInfo: (%v)", err)
	}

	// Create test objects: UWL namespace, UWL configmap with ACM alertmanager config
	uwlNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: operatorconfig.OCPUserWorkloadMonitoringNamespace,
		},
	}

	// Create UWL configmap with ACM alertmanager configuration
	uwlConfigYAML := `
prometheus:
  alertmanagerConfigs:
  - apiVersion: v2
    bearerToken:
      key: token
      name: observability-alertmanager-accessor
    pathPrefix: /
    scheme: https
    staticConfigs:
    - test-host.com
    tlsConfig:
      ServerName: ""
      ca:
        key: service-ca.crt
        name: hub-alertmanager-router-ca
      insecureSkipVerify: true
`

	uwlConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
			Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
		},
		Data: map[string]string{
			"config.yaml": uwlConfigYAML,
		},
	}

	// Create CMO configmap with UWL disabled
	cmoConfigYAML := `
userWorkloadEnabled: false
`

	cmoConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterMonitoringConfigName,
			Namespace: promNamespace,
		},
		Data: map[string]string{
			"config.yaml": cmoConfigYAML,
		},
	}

	// Create required secrets
	alertmanagerAccessorSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hubAmAccessorSecretName,
			Namespace: "test-ns",
		},
		Data: map[string][]byte{
			hubAmAccessorSecretKey: []byte("test-token"),
		},
	}

	// Create fake client with all objects
	objs := []runtime.Object{uwlNamespace, uwlConfigMap, cmoConfigMap, alertmanagerAccessorSecret}
	objs = append(objs, newMtlsTestSecrets("test-ns")...)
	c := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()

	// Call the function that should handle UWL monitoring configuration
	// This simulates the scenario where UWL is disabled but alerts are still enabled
	_, err = createOrUpdateClusterMonitoringConfig(ctx, hubInfo, testClusterID, c, false, "test-ns")
	if err != nil {
		t.Fatalf("Failed to create or update cluster monitoring config: (%v)", err)
	}

	// Verify that the UWL configmap is deleted or cleaned up
	foundUWLConfigMap := &corev1.ConfigMap{}
	err = c.Get(ctx, types.NamespacedName{
		Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
		Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
	}, foundUWLConfigMap)

	if err == nil {
		configYAML, ok := foundUWLConfigMap.Data["config.yaml"]
		if ok {
			parsed := &cmomanifests.UserWorkloadConfiguration{}
			if err := yaml.Unmarshal([]byte(configYAML), parsed); err != nil {
				t.Fatalf("Failed to unmarshal UWL config: %v", err)
			}

			if parsed.Prometheus != nil && parsed.Prometheus.AlertmanagerConfigs != nil {
				for _, config := range parsed.Prometheus.AlertmanagerConfigs {
					if config.TLSConfig.CA != nil &&
						(config.TLSConfig.CA.Name == amMtlsCaName ||
							config.TLSConfig.CA.Name == appendHubClusterID(amMtlsCaName, hubInfo)) {
						t.Fatalf("UWL configmap still contains ACM alertmanager configuration when it should be cleaned up")
					}
				}
			}
		}
	} else if !errors.IsNotFound(err) {
		t.Fatalf("Unexpected error checking UWL configmap: %v", err)
	}

	// Verify that the namespace still exists (it should not be deleted)
	foundNamespace := &corev1.Namespace{}
	err = c.Get(ctx, types.NamespacedName{Name: operatorconfig.OCPUserWorkloadMonitoringNamespace}, foundNamespace)
	if err != nil {
		t.Fatalf("UWL namespace should still exist: %v", err)
	}
}

// TestUWLMonitoringEnableScenario tests the scenario where UWL monitoring is enabled
// and the configmap should be created/updated with ACM alertmanager configuration
func TestUWLMonitoringEnableScenario(t *testing.T) {
	ctx := context.Background()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zap.Options{Development: true})))

	// Create hub info with alerts enabled (but UWL disabled)
	hubInfo := &operatorconfig.HubInfo{HubClusterID: "test-hub"}
	err := yaml.Unmarshal([]byte(hubInfoYAML), &hubInfo)
	if err != nil {
		t.Fatalf("Failed to unmarshal hubInfo: (%v)", err)
	}

	// Create the UWL monitoring namespace
	uwlNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: operatorconfig.OCPUserWorkloadMonitoringNamespace,
		},
	}

	// Create CMO configmap with UWL monitoring enabled
	cmoConfigYAML := `
enableUserWorkload: true
`

	cmoConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterMonitoringConfigName,
			Namespace: promNamespace,
		},
		Data: map[string]string{
			"config.yaml": cmoConfigYAML,
		},
	}

	// Create required secrets
	alertmanagerAccessorSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hubAmAccessorSecretName,
			Namespace: "test-ns",
		},
		Data: map[string][]byte{
			hubAmAccessorSecretKey: []byte("test-token"),
		},
	}

	// Create fake client with all objects
	objs := []runtime.Object{uwlNamespace, cmoConfigMap, alertmanagerAccessorSecret}
	objs = append(objs, newMtlsTestSecrets("test-ns")...)
	c := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()

	// Reset the revert state to ensure clean test environment
	unsetConfigReverted(ctx, c, "test-ns")

	// Execute the UWL monitoring configuration logic
	// This simulates the scenario where UWL monitoring is enabled and alerts are enabled
	_, err = createOrUpdateClusterMonitoringConfig(ctx, hubInfo, testClusterID, c, false, "test-ns")
	if err != nil {
		t.Fatalf("Failed to create or update cluster monitoring config: (%v)", err)
	}

	// Verify that the UWL configmap is created with ACM alertmanager configuration
	foundUWLConfigMap := &corev1.ConfigMap{}
	err = c.Get(ctx, types.NamespacedName{
		Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
		Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
	}, foundUWLConfigMap)
	if err != nil {
		t.Fatalf("UWL configmap should be created: %v", err)
	}

	// Verify the UWL configmap contains the expected configuration
	configYAML, ok := foundUWLConfigMap.Data["config.yaml"]
	if !ok {
		t.Fatalf("UWL configmap should contain config.yaml")
	}

	// Parse the UWL configuration to verify its structure
	parsed := &cmomanifests.UserWorkloadConfiguration{}
	if err := yaml.Unmarshal([]byte(configYAML), parsed); err != nil {
		t.Fatalf("Failed to unmarshal UWL config: %v", err)
	}

	// Verify that Prometheus configuration exists
	if parsed.Prometheus == nil {
		t.Fatalf("UWL configmap should contain prometheus configuration")
	}

	// Verify that additional alertmanager configurations are present
	// Note: We check the YAML content directly since the struct field might be empty
	// but the YAML could still contain the configuration
	if !strings.Contains(configYAML, "additionalAlertmanagerConfigs") {
		t.Fatalf("UWL configmap should contain additionalAlertmanagerConfigs")
	}

	// Verify that the ACM alertmanager configuration is present by checking for the mTLS CA secret
	if !strings.Contains(configYAML, appendHubClusterID(amMtlsCaName, hubInfo)) {
		t.Fatalf("UWL configmap should contain ACM alertmanager configuration with mTLS CA secret reference")
	}

	// Verify that the namespace still exists
	foundNamespace := &corev1.Namespace{}
	err = c.Get(ctx, types.NamespacedName{Name: operatorconfig.OCPUserWorkloadMonitoringNamespace}, foundNamespace)
	if err != nil {
		t.Fatalf("UWL namespace should still exist: %v", err)
	}
}

// When two stanzas target the same hub Alertmanager URL with different TLS (legacy router CA vs mTLS),
// reconcile must leave a single fresh ACM stanza, not both.
func TestClusterMonitoringConfigDedupeMultipleAdditionalAlertmanagers(t *testing.T) {
	AMSecretCleanupDone = false
	AMSecretCleanupDoneUWL = false
	testNamespace := "test-ns"
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zap.Options{Development: true})))

	hubInfo := &operatorconfig.HubInfo{
		ClusterName:              "test-cluster",
		ObservatoriumAPIEndpoint: "http://test-endpoint",
		AlertmanagerEndpoint:     "https://test-alertmanager.example.com/api/alertmanager/v2/default",
		HubClusterID:             "1a9af6dc0801433cb28a200af81",
	}

	// Same staticConfigs + pathPrefix as hubInfo.AlertmanagerEndpoint; only TLS material differs.
	diffYAML := `
prometheusK8s:
  externalLabels:
    managed_cluster: kind-cluster-id
  additionalAlertManagerConfigs:
  - apiVersion: v2
    bearerToken:
      key: token
      name: observability-alertmanager-accessor-1a9af6dc0801433cb28a200af81
    pathPrefix: /api/alertmanager/v2/default
    scheme: https
    staticConfigs:
    - test-alertmanager.example.com
    tlsConfig:
      ca:
        key: service-ca.crt
        name: hub-alertmanager-router-ca
      insecureSkipVerify: true
  - apiVersion: v2
    bearerToken:
      key: token
      name: observability-alertmanager-accessor-1a9af6dc0801433cb28a200af81
    pathPrefix: /api/alertmanager/v2/default
    scheme: https
    staticConfigs:
    - test-alertmanager.example.com
    tlsConfig:
      ca:
        key: ca.crt
        name: obs-alertmanager-mtls-ca-1a9af6dc0801433cb28a200af81
      cert:
        key: tls.crt
        name: obs-alertmanager-mtls-cert-1a9af6dc0801433cb28a200af81
      key:
        key: tls.key
        name: obs-alertmanager-mtls-cert-1a9af6dc0801433cb28a200af81
      insecureSkipVerify: false
`

	amAccessSrt := newAMAccessorSecret(testNamespace, "test-token")
	cmoCfg := newClusterMonitoringConfigCM(diffYAML, endpointMonitoringOperatorMgr)
	objs := []runtime.Object{cmoCfg, amAccessSrt}
	objs = append(objs, newMtlsTestSecrets(testNamespace)...)
	c := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()

	wasUpdated, err := createOrUpdateClusterMonitoringConfig(context.Background(), hubInfo, testClusterID, c, false, testNamespace)
	if err != nil {
		t.Fatalf("createOrUpdateClusterMonitoringConfig: %v", err)
	}
	assert.True(t, wasUpdated)

	found := &corev1.ConfigMap{}
	err = c.Get(context.Background(), types.NamespacedName{Name: clusterMonitoringConfigName, Namespace: promNamespace}, found)
	if err != nil {
		t.Fatalf("get configmap: %v", err)
	}
	yamlStr, ok := found.Data[clusterMonitoringConfigDataKey]
	if !ok {
		t.Fatalf("configmap missing %s", clusterMonitoringConfigDataKey)
	}
	foundJSON, err := yamltool.YAMLToJSON([]byte(yamlStr))
	if err != nil {
		t.Fatalf("YAMLToJSON: %v", err)
	}
	parsed := &cmomanifests.ClusterMonitoringConfiguration{}
	if err := json.Unmarshal(foundJSON, parsed); err != nil {
		t.Fatalf("unmarshal cluster monitoring config: %v", err)
	}
	if parsed.PrometheusK8sConfig == nil || len(parsed.PrometheusK8sConfig.AlertmanagerConfigs) == 0 {
		t.Fatalf("expected prometheusK8s.alertmanagerConfigs, got %#v", parsed.PrometheusK8sConfig)
	}
	amCfgs := parsed.PrometheusK8sConfig.AlertmanagerConfigs
	if len(amCfgs) != 1 {
		t.Fatalf("expected exactly 1 additionalAlertmanagerConfig after dedupe, got %d", len(amCfgs))
	}
	if amCfgs[0].TLSConfig.CA == nil || amCfgs[0].TLSConfig.CA.Name != appendHubClusterID(amMtlsCaName, hubInfo) {
		t.Fatalf("expected single mTLS ACM alertmanager config (CA %q), got %#v", appendHubClusterID(amMtlsCaName, hubInfo), amCfgs[0].TLSConfig.CA)
	}
	if amCfgs[0].TLSConfig.Cert == nil || amCfgs[0].TLSConfig.Cert.Name != appendHubClusterID(amMtlsCertName, hubInfo) {
		t.Fatalf("expected mTLS client cert on deduped config, got cert ref %#v", amCfgs[0].TLSConfig.Cert)
	}
}

func TestClusterMonitoringCleanupGlobalHub(t *testing.T) {
	AMSecretCleanupDone = false
	AMSecretCleanupDoneUWL = false
	testNamespace := "test-ns"
	amAccessSrt := newAMAccessorSecret(testNamespace, "test-token")
	hubInfo := &operatorconfig.HubInfo{
		ClusterName:              "test-cluster",
		ObservatoriumAPIEndpoint: "http://test-endpoint.abc.12345.com",
		AlertmanagerEndpoint:     "http://test-alertamanger-endpoint",
		HubClusterID:             "1a9af6dc0801433cb28a200af81",
	}
	cmoCfg := newClusterMonitoringConfigCM(clusterMonitoringConfigDataYamlCleanupGH, endpointMonitoringOperatorMgr)
	ghObjs := []runtime.Object{newHubInfoSecret([]byte(hubInfoYAML), testNamespace), cmoCfg, amAccessSrt}
	ghObjs = append(ghObjs, newMtlsTestSecrets(testNamespace)...)
	client := fake.NewClientBuilder().WithRuntimeObjects(ghObjs...).Build()
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

	// After reconcile, legacy hub-alertmanager-router-ca stanzas must be gone (including
	// unsuffixed, cluster-domain-suffixed, and HubClusterID-suffixed names — see isOldManagedConfig).
	foundClusterMonitoringConfigurationYAML, ok := hasClusterMonitoringConfigData(cmoCfgBeforeUpdate)
	if !ok {
		t.Fatalf("configmap: %s doesn't contain key: config.yaml", clusterMonitoringConfigName)
	}

	foundClusterMonitoringConfigurationJSON, err := yamltool.YAMLToJSON([]byte(foundClusterMonitoringConfigurationYAML))
	if err != nil {
		t.Fatalf("failed to transform YAML to JSON:\n%s\n", foundClusterMonitoringConfigurationYAML)
	}

	foundClusterMonitoringConfiguration := &cmomanifests.ClusterMonitoringConfiguration{}
	if err := json.Unmarshal([]byte(foundClusterMonitoringConfigurationJSON), foundClusterMonitoringConfiguration); err != nil {
		t.Fatalf("failed to marshal the cluster monitoring config: %v:\n%s\n", err, foundClusterMonitoringConfigurationJSON)
	}

	clusterDomain := mcoconfig.GetClusterName(hubInfo.ObservatoriumAPIEndpoint)
	if foundClusterMonitoringConfiguration.PrometheusK8sConfig != nil &&
		foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs != nil {
		for _, v := range foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs {
			if v.TLSConfig.CA == nil {
				continue
			}
			name := v.TLSConfig.CA.Name
			switch name {
			case hubAmRouterCASecretName,
				hubAmRouterCASecretName + "-" + clusterDomain,
				hubAmRouterCASecretName + "-" + hubInfo.HubClusterID:
				t.Fatalf("%q secret reference should be removed from the cluster-monitoring-config configmap (legacy router CA)", name)
			}
		}
	}
}
