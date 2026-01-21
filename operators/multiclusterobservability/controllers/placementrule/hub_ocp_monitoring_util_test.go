// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"context"
	"testing"

	ghodssyaml "github.com/ghodss/yaml"
	cmomanifests "github.com/openshift/cluster-monitoring-operator/pkg/manifests"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	goyaml "sigs.k8s.io/yaml"
)

func TestRevertHubClusterMonitoringConfig(t *testing.T) {
	// Initialize scheme
	s := scheme.Scheme
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add corev1 scheme: (%v)", err)
	}

	// Test case 1: Happy path - ConfigMap exists and is managed by endpoint-monitoring-operator
	t.Run("Revert ConfigMap when managed by endpoint-monitoring-operator", func(t *testing.T) {
		// Mock HubInfo Secret
		hubInfo := &operatorconfig.HubInfo{
			HubClusterID: "test-cluster-id",
		}
		hubInfoBytes, _ := goyaml.Marshal(hubInfo)
		hubInfoSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operatorconfig.HubInfoSecretName,
				Namespace: "open-cluster-management-observability", // Default namespace from config
			},
			Data: map[string][]byte{
				operatorconfig.HubInfoSecretKey: hubInfoBytes,
			},
		}

		// Mock Cluster Monitoring ConfigMap with managed fields
		initialConfig := cmomanifests.ClusterMonitoringConfiguration{
			PrometheusK8sConfig: &cmomanifests.PrometheusK8sConfig{
				ExternalLabels: map[string]string{
					operatorconfig.ClusterLabelKeyForAlerts: "test-cluster-id",
					"other-label":                           "value",
				},
				AlertmanagerConfigs: []cmomanifests.AdditionalAlertmanagerConfig{
					{
						TLSConfig: cmomanifests.TLSConfig{
							CA: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: hubAmRouterCASecretName + "-" + hubInfo.HubClusterID,
								},
							},
						},
					},
					{
						TLSConfig: cmomanifests.TLSConfig{
							CA: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "other-secret",
								},
							},
						},
					},
				},
			},
		}
		initialConfigBytes, _ := ghodssyaml.Marshal(initialConfig)
		initialConfigYAMLBytes, _ := ghodssyaml.JSONToYAML(initialConfigBytes)

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterMonitoringConfigName,
				Namespace: promNamespace,
				ManagedFields: []metav1.ManagedFieldsEntry{
					{
						Manager: endpointMonitoringOperatorMgr,
					},
				},
			},
			Data: map[string]string{
				clusterMonitoringConfigDataKey: string(initialConfigYAMLBytes),
			},
		}

		// Create fake client
		client := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(hubInfoSecret, cm).Build()

		// Call RevertHubClusterMonitoringConfig
		err := RevertHubClusterMonitoringConfig(context.TODO(), client)
		assert.NoError(t, err)

		// Verify the ConfigMap update
		updatedCM := &corev1.ConfigMap{}
		err = client.Get(context.TODO(), types.NamespacedName{Name: clusterMonitoringConfigName, Namespace: promNamespace}, updatedCM)
		assert.NoError(t, err)

		updatedConfig := &cmomanifests.ClusterMonitoringConfiguration{}
		ghodssyaml.Unmarshal([]byte(updatedCM.Data[clusterMonitoringConfigDataKey]), updatedConfig)

		// Check externalLabels: should have "other-label" but not "cluster_id"
		assert.NotContains(t, updatedConfig.PrometheusK8sConfig.ExternalLabels, operatorconfig.ClusterLabelKeyForAlerts)
		assert.Contains(t, updatedConfig.PrometheusK8sConfig.ExternalLabels, "other-label")

		// Check AlertmanagerConfigs: should have only "other-secret" config
		assert.Len(t, updatedConfig.PrometheusK8sConfig.AlertmanagerConfigs, 1)
		assert.Equal(t, "other-secret", updatedConfig.PrometheusK8sConfig.AlertmanagerConfigs[0].TLSConfig.CA.LocalObjectReference.Name)
	})

	// Test case 2: ConfigMap not managed by us - should not touch
	t.Run("Do not revert ConfigMap when not managed by known managers", func(t *testing.T) {
		// Mock HubInfo Secret (needed for init, though unused in logic if not managed)
		hubInfoSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operatorconfig.HubInfoSecretName,
				Namespace: "open-cluster-management-observability",
			},
			Data: map[string][]byte{
				operatorconfig.HubInfoSecretKey: []byte("cluster-name: test"),
			},
		}

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterMonitoringConfigName,
				Namespace: promNamespace,
				ManagedFields: []metav1.ManagedFieldsEntry{
					{
						Manager: "some-other-manager",
					},
				},
			},
			Data: map[string]string{
				clusterMonitoringConfigDataKey: "some: data",
			},
		}

		client := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(hubInfoSecret, cm).Build()

		err := RevertHubClusterMonitoringConfig(context.TODO(), client)
		assert.NoError(t, err)

		// Verify ConfigMap is unchanged
		updatedCM := &corev1.ConfigMap{}
		client.Get(context.TODO(), types.NamespacedName{Name: clusterMonitoringConfigName, Namespace: promNamespace}, updatedCM)
		assert.Equal(t, "some: data", updatedCM.Data[clusterMonitoringConfigDataKey])
	})

	// Test case 3: Secret not found - should return nil (no error)
	t.Run("Return nil when HubInfo secret is missing", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(s).Build()
		err := RevertHubClusterMonitoringConfig(context.TODO(), client)
		assert.NoError(t, err)
	})
}
