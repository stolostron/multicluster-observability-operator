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
	"k8s.io/utils/ptr"
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

// TestCMOConfigReconciler_reconcileAlertmanagerConfigs_RenamedCASecret tests that when the
// hub renames the CA secret prefix across upgrades (e.g. obs-alertmanager-mtls-ca → hub-mtls-ca),
// the old configmap entry is still recognized as owned and removed when forwarding is disabled.
func TestCMOConfigReconciler_reconcileAlertmanagerConfigs_RenamedCASecret(t *testing.T) {
	t.Parallel()

	const hubID = "465e377c1ecd4cc29c7"
	// Simulate a hub that renamed the CA secret from "obs-alertmanager-mtls-ca-<hubID>"
	// to "hub-mtls-ca-<hubID>" across an upgrade.
	r := &MCOAAgentReconciler{
		Log:            ctrl.Log.WithName("test"),
		CASecret:       "hub-mtls-ca-" + hubID,   // new prefix after hub rename
		CertSecret:     "hub-mtls-cert-" + hubID, // new prefix after hub rename
		AccessorSecret: "observability-alertmanager-accessor-" + hubID,
	}

	// Existing config was written with the old CA name (before the hub renamed it).
	oldCfg := cmomanifests.AdditionalAlertmanagerConfig{
		Scheme:     "https",
		APIVersion: "v2",
		TLSConfig: cmomanifests.TLSConfig{
			CA: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "obs-alertmanager-mtls-ca-" + hubID,
				},
				Key: "ca.crt",
			},
		},
	}

	// With UWL alert forwarding disabled, the old entry must be removed even though
	// its CA name doesn't exactly match r.CASecret.
	endpoint := "https://observatorium-api.example.com"
	configs, modified := r.reconcileAlertmanagerConfigs([]cmomanifests.AdditionalAlertmanagerConfig{oldCfg}, endpoint, false)
	require.True(t, modified, "should be modified: old entry must be cleaned up")
	assert.Empty(t, configs, "old alertmanager config with renamed CA should be removed")
}

// TestCMOConfigReconciler_reconcileAlertmanagerConfigs_OrderStable verifies that a reorder
// of existing entries by another controller is not treated as a content change.
func TestCMOConfigReconciler_reconcileAlertmanagerConfigs_OrderStable(t *testing.T) {
	t.Parallel()

	r := &MCOAAgentReconciler{
		Log:            ctrl.Log.WithName("test"),
		CASecret:       "obs-alertmanager-mtls-ca-hub1",
		CertSecret:     "obs-alertmanager-mtls-cert-hub1",
		AccessorSecret: "observability-alertmanager-accessor-hub1",
	}

	endpoint := "https://our-am.example.com"

	// Produce the canonical entry for our hub so the test doesn't hard-code field values.
	canonical, _ := r.reconcileAlertmanagerConfigs(nil, endpoint, true)
	require.Len(t, canonical, 1)
	ourEntry := canonical[0]

	externalEntry := cmomanifests.AdditionalAlertmanagerConfig{
		Scheme:        "http",
		APIVersion:    "v2",
		StaticConfigs: []string{"external-am.example.com"},
	}

	// Another controller stored them in reversed order.
	existing := []cmomanifests.AdditionalAlertmanagerConfig{externalEntry, ourEntry}

	// Forwarding still enabled: content is unchanged, only order differs.
	_, modified := r.reconcileAlertmanagerConfigs(existing, endpoint, true)
	assert.False(t, modified, "reorder by another controller must not trigger an update")
}

func TestCMOConfigReconciler_reconcileRemoteWrites(t *testing.T) {
	t.Parallel()

	r := &MCOAAgentReconciler{
		Log:        ctrl.Log.WithName("test"),
		CASecret:   "test-ca-secret",
		CertSecret: "test-cert-secret",
	}

	agent := &prometheusv1alpha1.PrometheusAgent{
		Spec: prometheusv1alpha1.PrometheusAgentSpec{
			CommonPrometheusFields: monitoringv1.CommonPrometheusFields{
				RemoteWrite: []monitoringv1.RemoteWriteSpec{
					{
						Name: ptr.To("acm-observability"),
						URL:  "https://hub-am.example.com",
						TLSConfig: &monitoringv1.TLSConfig{
							SafeTLSConfig: monitoringv1.SafeTLSConfig{
								CA: monitoringv1.SecretOrConfigMap{
									Secret: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "test-ca-secret",
										},
										Key: "ca.crt",
									},
								},
								Cert: monitoringv1.SecretOrConfigMap{
									Secret: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "test-cert-secret",
										},
										Key: "tls.crt",
									},
								},
								KeySecret: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "test-cert-secret",
									},
									Key: "tls.key",
								},
							},
						},
					},
				},
			},
		},
	}

	// Case 1: Empty scrape configs returns empty/clean remote write list
	configs := r.reconcileRemoteWrites(nil, nil, agent)
	assert.Empty(t, configs)

	// Case 2: Agent empty returns empty/clean remote write list even if scrape configs are present
	sc := newRawScrapeConfig("test-sc", "test-ns", platformMetricsCollectorRawComponent)
	configs = r.reconcileRemoteWrites(nil, []prometheusv1alpha1.ScrapeConfig{*sc}, nil)
	assert.Empty(t, configs)

	// Case 3: Endpoint and ScrapeConfigs present -> transpiles correctly
	configs = r.reconcileRemoteWrites(nil, []prometheusv1alpha1.ScrapeConfig{*sc}, agent)
	require.Len(t, configs, 1)

	rw := configs[0]
	assert.Equal(t, "https://hub-am.example.com", rw.URL)

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

	configs = r.reconcileRemoteWrites(nil, []prometheusv1alpha1.ScrapeConfig{*scComplex}, agent)
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

	agentPlatform := newTestPrometheusAgent("test-agent-platform", namespace, platformMetricsCollectorComponent, "https://hub-am.example.com", "", "")
	agentUWL := newTestPrometheusAgent("test-agent-uwl", namespace, userWorkloadMetricsCollectorComponent, "https://hub-am.example.com", "", "")

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

	clPlatform := fake.NewClientBuilder().WithScheme(s).WithObjects(scPlatform, cmPlatform, agentPlatform).Build()

	rPlatform := &MCOAAgentReconciler{
		Client:                        clPlatform,
		Log:                           ctrl.Log.WithName("test-controller"),
		Namespace:                     namespace,
		HubAlertmanagerURL:            "https://hub-am.example.com",
		CASecret:                      "test-ca-secret",
		CertSecret:                    "test-cert-secret",
		EnablePlatformAlertForwarding: true,
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
	assert.Contains(t, platformYAML, "https://hub-am.example.com")
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

	clUWL := fake.NewClientBuilder().WithScheme(s).WithObjects(scUWL, cmUWL, agentUWL).Build()

	rUWL := &MCOAAgentReconciler{
		Client:                   clUWL,
		Log:                      ctrl.Log.WithName("test-controller"),
		Namespace:                namespace,
		HubAlertmanagerURL:       "https://hub-am.example.com",
		CASecret:                 "test-ca-secret",
		CertSecret:               "test-cert-secret",
		EnableUWLAlertForwarding: true,
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
	assert.Contains(t, uwlYAML, "https://hub-am.example.com")
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

	agent := newTestPrometheusAgent("test-agent", namespace, platformMetricsCollectorComponent, "https://new-hub.com", "", "")

	clPlatform := fake.NewClientBuilder().WithScheme(s).WithObjects(cmPlatform, agent).Build()

	rPlatform := &MCOAAgentReconciler{
		Client:                        clPlatform,
		Log:                           ctrl.Log.WithName("test-controller"),
		Namespace:                     namespace,
		ClusterID:                     "new-cluster-id",
		ClusterName:                   "new-cluster-name",
		HubAlertmanagerURL:            "https://new-hub.com",
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

func TestToCMORemoteWrite_ReconstructsTLSSecrets(t *testing.T) {
	rw := &monitoringv1.RemoteWriteSpec{
		URL: "https://hub.example.com",
		TLSConfig: &monitoringv1.TLSConfig{
			SafeTLSConfig: monitoringv1.SafeTLSConfig{
				CA:   monitoringv1.SecretOrConfigMap{},
				Cert: monitoringv1.SecretOrConfigMap{},
			},
			CAFile:   "/etc/prometheus/secrets/obs-alertmanager-mtls-ca-465e377c1ecd4cc29c7/custom-ca.pem",
			CertFile: "/etc/prometheus/secrets/obs-alertmanager-mtls-cert-465e377c1ecd4cc29c7/custom-cert.pem",
			KeyFile:  "/etc/prometheus/secrets/obs-alertmanager-mtls-cert-465e377c1ecd4cc29c7/custom-key.pem",
		},
	}

	cmoRW := toCMORemoteWrite(rw)

	assert.NotNil(t, cmoRW.TLSConfig)
	assert.NotNil(t, cmoRW.TLSConfig.CA.Secret)
	assert.Equal(t, "obs-alertmanager-mtls-ca-465e377c1ecd4cc29c7", cmoRW.TLSConfig.CA.Secret.Name)
	assert.Equal(t, "custom-ca.pem", cmoRW.TLSConfig.CA.Secret.Key)

	assert.NotNil(t, cmoRW.TLSConfig.Cert.Secret)
	assert.Equal(t, "obs-alertmanager-mtls-cert-465e377c1ecd4cc29c7", cmoRW.TLSConfig.Cert.Secret.Name)
	assert.Equal(t, "custom-cert.pem", cmoRW.TLSConfig.Cert.Secret.Key)

	assert.NotNil(t, cmoRW.TLSConfig.KeySecret)
	assert.Equal(t, "obs-alertmanager-mtls-cert-465e377c1ecd4cc29c7", cmoRW.TLSConfig.KeySecret.Name)
	assert.Equal(t, "custom-key.pem", cmoRW.TLSConfig.KeySecret.Key)
}

func TestCMOConfigReconciler_reconcileRemoteWrites_Sorting(t *testing.T) {
	t.Parallel()

	r := &MCOAAgentReconciler{
		Log:        ctrl.Log.WithName("test"),
		CASecret:   "test-ca-secret",
		CertSecret: "test-cert-secret",
	}

	agent := &prometheusv1alpha1.PrometheusAgent{
		Spec: prometheusv1alpha1.PrometheusAgentSpec{
			CommonPrometheusFields: monitoringv1.CommonPrometheusFields{
				RemoteWrite: []monitoringv1.RemoteWriteSpec{
					{
						Name: ptr.To("acm-observability"),
						URL:  "https://hub-am.example.com",
					},
				},
			},
		},
	}

	// Create scrape configs in non-sorted name order: "z-sc", "a-sc", "m-sc"
	scZ := newRawScrapeConfig("z-sc", "test-ns", platformMetricsCollectorRawComponent)
	scA := newRawScrapeConfig("a-sc", "test-ns", platformMetricsCollectorRawComponent)
	scM := newRawScrapeConfig("m-sc", "test-ns", platformMetricsCollectorRawComponent)

	// Case 1: passing them as [scZ, scA, scM]
	configs1 := r.reconcileRemoteWrites(nil, []prometheusv1alpha1.ScrapeConfig{*scZ, *scA, *scM}, agent)
	require.Len(t, configs1, 3)

	// Since they are sorted by Name ("a-sc", "m-sc", "z-sc"), the resulting RemoteWrites must be in that exact name order
	assert.Contains(t, configs1[0].Name, "a-sc")
	assert.Contains(t, configs1[1].Name, "m-sc")
	assert.Contains(t, configs1[2].Name, "z-sc")

	// Case 2: passing them in a different order [scA, scM, scZ] should produce the EXACT same ordered slice
	configs2 := r.reconcileRemoteWrites(nil, []prometheusv1alpha1.ScrapeConfig{*scA, *scM, *scZ}, agent)
	require.Len(t, configs2, 3)
	assert.Equal(t, configs1, configs2)
}

func TestCMOConfigReconciler_reconcileExternalLabels(t *testing.T) {
	t.Parallel()

	r := &MCOAAgentReconciler{
		Log:         ctrl.Log.WithName("test"),
		ClusterID:   "my-cluster-id",
		ClusterName: "my-cluster-name",
	}

	// Case 1: neither agent present nor forwarding enabled -> remove external labels
	cfg1 := &cmomanifests.PrometheusK8sConfig{
		ExternalLabels: map[string]string{
			operatorconfig.ClusterLabelKeyForAlerts:     "old-id",
			operatorconfig.ClusterNameLabelKeyForAlerts: "old-name",
		},
	}
	modified := r.reconcileExternalLabels(cfg1, false)
	assert.True(t, modified)
	assert.Empty(t, cfg1.ExternalLabels)

	// Case 2: forwarding enabled with NO agent (retainLabels = true) -> preserve and reconcile external labels
	cfg2 := &cmomanifests.PrometheusK8sConfig{
		ExternalLabels: map[string]string{
			operatorconfig.ClusterLabelKeyForAlerts:     "old-id",
			operatorconfig.ClusterNameLabelKeyForAlerts: "old-name",
		},
	}
	modified = r.reconcileExternalLabels(cfg2, true)
	assert.True(t, modified)
	assert.Equal(t, "my-cluster-id", cfg2.ExternalLabels[operatorconfig.ClusterLabelKeyForAlerts])
	assert.Equal(t, "my-cluster-name", cfg2.ExternalLabels[operatorconfig.ClusterNameLabelKeyForAlerts])

	// Case 3: forwarding enabled with NO agent, labels already correct -> no modification
	cfg3 := &cmomanifests.PrometheusK8sConfig{
		ExternalLabels: map[string]string{
			operatorconfig.ClusterLabelKeyForAlerts:     "my-cluster-id",
			operatorconfig.ClusterNameLabelKeyForAlerts: "my-cluster-name",
		},
	}
	modified = r.reconcileExternalLabels(cfg3, true)
	assert.False(t, modified)
}

func TestCMOConfigReconciler_reconcileExternalLabels_DisabledForwarding(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))
	addRhobsToScheme(t, s)

	ctx := context.Background()
	namespace := "test-namespace"

	// Existing config map containing old externalLabels
	oldCfg := cmomanifests.ClusterMonitoringConfiguration{
		PrometheusK8sConfig: &cmomanifests.PrometheusK8sConfig{
			ExternalLabels: map[string]string{
				operatorconfig.ClusterLabelKeyForAlerts:     "my-cluster-id",
				operatorconfig.ClusterNameLabelKeyForAlerts: "my-cluster-name",
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

	// Platform alert forwarding disabled (EnablePlatformAlertForwarding: false)
	rPlatform := &MCOAAgentReconciler{
		Client:                        clPlatform,
		Log:                           ctrl.Log.WithName("test-controller"),
		Namespace:                     namespace,
		ClusterID:                     "my-cluster-id",
		ClusterName:                   "my-cluster-name",
		HubAlertmanagerURL:            "https://new-hub.com",
		CASecret:                      "test-ca-secret",
		CertSecret:                    "test-cert-secret",
		EnablePlatformAlertForwarding: false,
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
	// Since alert forwarding was disabled, external labels must be removed!
	assert.NotContains(t, platformYAML, "my-cluster-id")
	assert.NotContains(t, platformYAML, "my-cluster-name")
}
