// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package mcoa

import (
	"context"
	"testing"

	cmomanifests "github.com/openshift/cluster-monitoring-operator/pkg/manifests"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/observabilityendpoint"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"
)

func TestCMOConfigReconciler_detectConflict(t *testing.T) {
	t.Parallel()

	validCfg := cmomanifests.ClusterMonitoringConfiguration{
		PrometheusK8sConfig: &cmomanifests.PrometheusK8sConfig{
			AlertmanagerConfigs: []cmomanifests.AdditionalAlertmanagerConfig{
				{
					TLSConfig: cmomanifests.TLSConfig{
						CA: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "hub-alertmanager-router-ca-hub-id",
							},
						},
					},
				},
			},
		},
	}
	validYAML, _ := yaml.Marshal(validCfg)

	tests := []struct {
		name     string
		cm       *corev1.ConfigMap
		expected bool
	}{
		{
			name:     "Empty ConfigMap (not a conflict on clean slate)",
			cm:       &corev1.ConfigMap{},
			expected: false,
		},
		{
			name: "Missing data (not a conflict on clean slate)",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					ManagedFields: []metav1.ManagedFieldsEntry{
						{Manager: observabilityendpoint.EndpointMonitoringOperatorMgr},
					},
				},
			},
			expected: false,
		},
		{
			name: "Correct config",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					ManagedFields: []metav1.ManagedFieldsEntry{
						{Manager: observabilityendpoint.EndpointMonitoringOperatorMgr},
					},
				},
				Data: map[string]string{
					observabilityendpoint.ClusterMonitoringConfigDataKey: string(validYAML),
				},
			},
			expected: false,
		},
		{
			name: "Conflict config (conflict)",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					ManagedFields: []metav1.ManagedFieldsEntry{
						{Manager: observabilityendpoint.EndpointMonitoringOperatorMgr},
					},
				},
				Data: map[string]string{
					observabilityendpoint.ClusterMonitoringConfigDataKey: "prometheusK8s:\n  additionalAlertmanagerConfigs:\n  - scheme: https\n    staticConfigs:\n    - old-hub.com",
				},
			},
			expected: true,
		},
		{
			name: "Corrupted YAML (conflict)",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					ManagedFields: []metav1.ManagedFieldsEntry{
						{Manager: observabilityendpoint.EndpointMonitoringOperatorMgr},
					},
				},
				Data: map[string]string{
					observabilityendpoint.ClusterMonitoringConfigDataKey: "{invalid: yaml: :}",
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := &MCOAAgentReconciler{
				Log:      ctrl.Log.WithName("test"),
				CASecret: "hub-alertmanager-router-ca-hub-id", // Match the validCfg Name exactly
			}
			assert.Equal(t, tt.expected, r.detectConflict(tt.cm))
		})
	}
}

func TestCMOConfigReconciler_reconcileRawMetrics(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))

	// Register ScrapeConfig for the custom monitoring.rhobs/v1alpha1 API Group
	addRhobsToScheme(t, s)

	ctx := context.Background()
	namespace := "test-namespace"

	// 1. Reconcile Platform (CMO) raw metrics RemoteWrite
	scPlatform := newRawScrapeConfig("test-raw-scrape-platform", namespace, platformMetricsCollectorRawComponent)

	cmPlatform := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
			Namespace: operatorconfig.OCPClusterMonitoringNamespace,
		},
		Data: map[string]string{
			observabilityendpoint.ClusterMonitoringConfigDataKey: defaultCMOConfigYAML,
		},
	}

	clPlatform := fake.NewClientBuilder().WithScheme(s).WithObjects(scPlatform, cmPlatform).Build()

	rPlatform := &MCOAAgentReconciler{
		Client:      clPlatform,
		Log:         ctrl.Log.WithName("test-controller"),
		Namespace:   namespace,
		hubEndpoint: "https://hub-am.example.com",
		CASecret:    "test-ca-secret",
		CertSecret:  "test-cert-secret",
	}

	err := rPlatform.ReconcileCMOPlatformConfig(ctx)
	require.NoError(t, err)

	updatedCMPlatform := &corev1.ConfigMap{}
	err = clPlatform.Get(ctx, client.ObjectKey{
		Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
		Namespace: operatorconfig.OCPClusterMonitoringNamespace,
	}, updatedCMPlatform)
	require.NoError(t, err)

	platformYAML := updatedCMPlatform.Data[observabilityendpoint.ClusterMonitoringConfigDataKey]
	assert.Contains(t, platformYAML, "https://hub-am.example.com/api/metrics/v1/default/api/v1/receive")
	assert.Contains(t, platformYAML, "test-ca-secret")
	assert.Contains(t, platformYAML, "test-cert-secret")
	assert.Contains(t, platformYAML, "up")

	// 2. Reconcile User Workload (UWL) raw metrics RemoteWrite
	scUWL := newRawScrapeConfig("test-raw-scrape-uwl", namespace, userWorkloadMetricsCollectorRawComponent)

	cmUWL := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
			Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
		},
		Data: map[string]string{
			uwlMonitoringConfigDataKey: defaultUWLConfigYAML,
		},
	}

	clUWL := fake.NewClientBuilder().WithScheme(s).WithObjects(scUWL, cmUWL).Build()

	rUWL := &MCOAAgentReconciler{
		Client:      clUWL,
		Log:         ctrl.Log.WithName("test-controller"),
		Namespace:   namespace,
		hubEndpoint: "https://hub-am.example.com",
		CASecret:    "test-ca-secret",
		CertSecret:  "test-cert-secret",
	}

	err = rUWL.ReconcileCMOUWLConfig(ctx)
	require.NoError(t, err)

	updatedCMUWL := &corev1.ConfigMap{}
	err = clUWL.Get(ctx, client.ObjectKey{
		Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
		Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
	}, updatedCMUWL)
	require.NoError(t, err)

	uwlYAML := updatedCMUWL.Data[uwlMonitoringConfigDataKey]
	assert.Contains(t, uwlYAML, "https://hub-am.example.com/api/metrics/v1/default/api/v1/receive")
	assert.Contains(t, uwlYAML, "test-ca-secret")
	assert.Contains(t, uwlYAML, "test-cert-secret")
	assert.Contains(t, uwlYAML, "up")
}

func TestCMOConfigReconciler_reconcileConfigMutation(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))

	// Register ScrapeConfig for the custom monitoring.rhobs/v1alpha1 API Group
	addRhobsToScheme(t, s)

	ctx := context.Background()
	namespace := "test-namespace"

	// Outdated config map containing old alertmanager configs and old externalLabels
	oldCfg := cmomanifests.ClusterMonitoringConfiguration{
		PrometheusK8sConfig: &cmomanifests.PrometheusK8sConfig{
			ExternalLabels: map[string]string{
				operatorconfig.ClusterLabelKeyForAlerts:     "old-cluster-id",
				operatorconfig.ClusterNameLabelKeyForAlerts: "old-cluster-name",
			},
			AlertmanagerConfigs: []cmomanifests.AdditionalAlertmanagerConfig{
				{
					Scheme:     "https",
					PathPrefix: "/api/alerts/v1/default",
					TLSConfig: cmomanifests.TLSConfig{
						CA: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "test-ca-secret",
							},
							Key: "ca.crt",
						},
					},
					StaticConfigs: []string{"old-hub.com"},
				},
			},
		},
	}
	oldYAML, err := yaml.Marshal(oldCfg)
	require.NoError(t, err)

	cmPlatform := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
			Namespace: operatorconfig.OCPClusterMonitoringNamespace,
		},
		Data: map[string]string{
			observabilityendpoint.ClusterMonitoringConfigDataKey: string(oldYAML),
		},
	}

	clPlatform := fake.NewClientBuilder().WithScheme(s).WithObjects(cmPlatform).Build()

	rPlatform := &MCOAAgentReconciler{
		Client:      clPlatform,
		Log:         ctrl.Log.WithName("test-controller"),
		Namespace:   namespace,
		ClusterID:   "new-cluster-id",
		ClusterName: "new-cluster-name",
		hubEndpoint: "https://new-hub.com",
		CASecret:    "test-ca-secret",
		CertSecret:  "test-cert-secret",
	}

	err = rPlatform.ReconcileCMOPlatformConfig(ctx)
	require.NoError(t, err)

	updatedCMPlatform := &corev1.ConfigMap{}
	err = clPlatform.Get(ctx, client.ObjectKey{
		Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
		Namespace: operatorconfig.OCPClusterMonitoringNamespace,
	}, updatedCMPlatform)
	require.NoError(t, err)

	platformYAML := updatedCMPlatform.Data[observabilityendpoint.ClusterMonitoringConfigDataKey]
	// Assert the config was mutated and upgraded successfully!
	assert.Contains(t, platformYAML, "new-cluster-id")
	assert.Contains(t, platformYAML, "new-cluster-name")
	assert.Contains(t, platformYAML, "new-hub.com")
	assert.NotContains(t, platformYAML, "old-cluster-id")
	assert.NotContains(t, platformYAML, "old-cluster-name")
	assert.NotContains(t, platformYAML, "old-hub.com")
}
