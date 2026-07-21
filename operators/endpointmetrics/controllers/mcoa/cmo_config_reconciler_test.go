// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package mcoa

import (
	"context"
	"strings"
	"testing"

	cmomanifests "github.com/openshift/cluster-monitoring-operator/pkg/manifests"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	prometheusv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
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

func TestCMOConfigReconciler_reconcileAlertmanagerConfigs(t *testing.T) {
	t.Parallel()

	r := &MCOAAgentReconciler{
		Log:            ctrl.Log.WithName("test"),
		CASecret:       "obs-alertmanager-mtls-ca-465e377c1ecd4cc29c7",
		CertSecret:     "obs-alertmanager-mtls-cert-465e377c1ecd4cc29c7",
		AccessorSecret: "observability-alertmanager-accessor-465e377c1ecd4cc29c7",
	}

	endpoint := "https://observatorium-api-open-cluster-management-observability.apps.sno-4xlarge-422-7wgdx.dev07.red-chesterfield.com"
	configs, modified := r.reconcileAlertmanagerConfigs(nil, endpoint, true)
	require.True(t, modified)
	require.Len(t, configs, 1)

	am := configs[0]
	assert.Equal(t, "v2", am.APIVersion)
	assert.Equal(t, "/api/alertmanager/v2/default", am.PathPrefix)
	assert.Equal(t, "https", am.Scheme)
	assert.Equal(t, []string{"observatorium-api-open-cluster-management-observability.apps.sno-4xlarge-422-7wgdx.dev07.red-chesterfield.com"}, am.StaticConfigs)

	// Assert TLS Config
	assert.False(t, am.TLSConfig.InsecureSkipVerify)
	assert.Equal(t, "ca.crt", am.TLSConfig.CA.Key)
	assert.Equal(t, "obs-alertmanager-mtls-ca-465e377c1ecd4cc29c7", am.TLSConfig.CA.Name)
	assert.Equal(t, "tls.crt", am.TLSConfig.Cert.Key)
	assert.Equal(t, "obs-alertmanager-mtls-cert-465e377c1ecd4cc29c7", am.TLSConfig.Cert.Name)
	assert.Equal(t, "tls.key", am.TLSConfig.Key.Key)
	assert.Equal(t, "obs-alertmanager-mtls-cert-465e377c1ecd4cc29c7", am.TLSConfig.Key.Name)

	// Assert Bearer Token
	assert.Equal(t, "token", am.BearerToken.Key)
	assert.Equal(t, "observability-alertmanager-accessor-465e377c1ecd4cc29c7", am.BearerToken.Name)
}

func TestCMOConfigReconciler_reconcileRemoteWrites(t *testing.T) {
	t.Parallel()

	r := &MCOAAgentReconciler{
		Log:        ctrl.Log.WithName("test"),
		CASecret:   "test-ca-secret",
		CertSecret: "test-cert-secret",
	}

	// Case 1: Empty scrape configs returns empty/clean remote write list
	configs, err := r.reconcileRemoteWrites(nil, nil, "https://hub-am.example.com")
	require.NoError(t, err)
	assert.Empty(t, configs)

	// Case 2: Endpoint empty returns empty/clean remote write list even if scrape configs are present
	sc := newRawScrapeConfig("test-sc", "test-ns", platformMetricsCollectorRawComponent)
	configs, err = r.reconcileRemoteWrites(nil, []prometheusv1alpha1.ScrapeConfig{*sc}, "")
	require.NoError(t, err)
	assert.Empty(t, configs)

	// Case 3: Endpoint and ScrapeConfigs present -> transpiles correctly
	configs, err = r.reconcileRemoteWrites(nil, []prometheusv1alpha1.ScrapeConfig{*sc}, "https://hub-am.example.com")
	require.NoError(t, err)
	require.Len(t, configs, 1)

	rw := configs[0]
	assert.Equal(t, "https://hub-am.example.com/api/metrics/v1/default/api/v1/receive", rw.URL)

	// Assert TLS Config
	require.NotNil(t, rw.TLSConfig)
	assert.Equal(t, "ca.crt", rw.TLSConfig.CA.Secret.Key)
	assert.Equal(t, "test-ca-secret", rw.TLSConfig.CA.Secret.Name)
	assert.Equal(t, "tls.crt", rw.TLSConfig.Cert.Secret.Key)
	assert.Equal(t, "test-cert-secret", rw.TLSConfig.Cert.Secret.Name)
	assert.Equal(t, "tls.key", rw.TLSConfig.KeySecret.Key)
	assert.Equal(t, "test-cert-secret", rw.TLSConfig.KeySecret.Name)

	// Assert Relabel Configs
	require.NotEmpty(t, rw.WriteRelabelConfigs)
	assert.Equal(t, "__name__", string(rw.WriteRelabelConfigs[0].SourceLabels[0]))

	// Case 4: Endpoint and ScrapeConfig with custom metricRelabelings (Job/Instance normalization) -> transpiles and appends correctly
	scComplex := newRawScrapeConfig("test-sc-complex", "test-ns", platformMetricsCollectorRawComponent)
	scComplex.Spec.MetricRelabelConfigs = []monitoringv1.RelabelConfig{
		{
			Action:       "replace",
			SourceLabels: []monitoringv1.LabelName{"exported_job"},
			TargetLabel:  "job",
		},
		{
			Action:       "replace",
			SourceLabels: []monitoringv1.LabelName{"exported_instance"},
			TargetLabel:  "instance",
		},
		{
			Action: "labeldrop",
			Regex:  "exported_job|exported_instance",
		},
	}

	configs, err = r.reconcileRemoteWrites(nil, []prometheusv1alpha1.ScrapeConfig{*scComplex}, "https://hub-am.example.com")
	require.NoError(t, err)
	require.Len(t, configs, 1)

	rwComplex := configs[0]
	// Assert the transpiled relabel configs contain our custom ones appended at the end of standard ones
	require.NotEmpty(t, rwComplex.WriteRelabelConfigs)

	totalRelabels := len(rwComplex.WriteRelabelConfigs)
	assert.Greater(t, totalRelabels, 3)

	// Last 3 relabels should match the custom MetricRelabelConfigs exactly:
	lastRelabel := rwComplex.WriteRelabelConfigs[totalRelabels-1]
	assert.Equal(t, "labeldrop", strings.ToLower(lastRelabel.Action))
	assert.Equal(t, "exported_job|exported_instance", lastRelabel.Regex)

	prevRelabel := rwComplex.WriteRelabelConfigs[totalRelabels-2]
	assert.Equal(t, "replace", strings.ToLower(prevRelabel.Action))
	assert.Equal(t, "instance", prevRelabel.TargetLabel)
	assert.Equal(t, "exported_instance", string(prevRelabel.SourceLabels[0]))

	firstCustomRelabel := rwComplex.WriteRelabelConfigs[totalRelabels-3]
	assert.Equal(t, "replace", strings.ToLower(firstCustomRelabel.Action))
	assert.Equal(t, "job", firstCustomRelabel.TargetLabel)
	assert.Equal(t, "exported_job", string(firstCustomRelabel.SourceLabels[0]))
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
		HubEndpoint: "https://hub-am.example.com",
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
		HubEndpoint: "https://hub-am.example.com",
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
		Client:                        clPlatform,
		Log:                           ctrl.Log.WithName("test-controller"),
		Namespace:                     namespace,
		ClusterID:                     "new-cluster-id",
		ClusterName:                   "new-cluster-name",
		HubEndpoint:                   "https://new-hub.com",
		CASecret:                      "test-ca-secret",
		CertSecret:                    "test-cert-secret",
		EnablePlatformAlertForwarding: true,
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
