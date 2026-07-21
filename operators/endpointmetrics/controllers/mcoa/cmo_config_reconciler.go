// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package mcoa

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"

	cmomanifests "github.com/openshift/cluster-monitoring-operator/pkg/manifests"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	prometheusv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/observabilityendpoint"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/remotewrite"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/yaml"
)

const (
	// uwlMonitoringConfigDataKey is the data key mandated by the user-workload-monitoring ConfigMap schema
	uwlMonitoringConfigDataKey = "config.yaml"

	// labelKeyComponent is the standard metadata label key for Kubernetes component discovery
	labelKeyComponent = "app.kubernetes.io/component"

	// Package-level constants for load-bearing component labels
	platformMetricsCollectorRawComponent     = "platform-metrics-collector-raw"
	userWorkloadMetricsCollectorRawComponent = "user-workload-metrics-collector-raw"

	// Default bootstrap YAML templates to avoid magic strings
	defaultCMOConfigYAML = "prometheusK8s: {}"
	defaultUWLConfigYAML = "prometheus: {}"
)

var cmoConfigConflictsTotal = promauto.With(metrics.Registry).NewCounter(
	prometheus.CounterOpts{
		Name: "mcoa_cmo_config_conflicts_total",
		Help: "Total number of conflicts detected in the cluster-monitoring-config ConfigMap.",
	},
)

func (r *MCOAAgentReconciler) isOwnedAlertmanagerConfig(am cmomanifests.AdditionalAlertmanagerConfig) bool {
	return r.CASecret != "" && am.TLSConfig.CA != nil && am.TLSConfig.CA.Name == r.CASecret
}

func (r *MCOAAgentReconciler) newAdditionalAlertmanagerConfig(endpoint string) cmomanifests.AdditionalAlertmanagerConfig {
	config := cmomanifests.AdditionalAlertmanagerConfig{
		Scheme:     "https",
		PathPrefix: "/",
		APIVersion: "v2",
		TLSConfig: cmomanifests.TLSConfig{
			CA: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: r.CASecret,
				},
				Key: "ca.crt",
			},
			Cert: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: r.CertSecret,
				},
				Key: "tls.crt",
			},
			Key: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: r.CertSecret,
				},
				Key: "tls.key",
			},
			InsecureSkipVerify: false,
		},
		BearerToken: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: r.AccessorSecret,
			},
			Key: "token",
		},
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		r.Log.Info("failed to parse alertmanager endpoint, falling back to raw value", "endpoint", endpoint, "error", err.Error())
		config.StaticConfigs = []string{endpoint}
		return config
	}

	if u.Host == "" {
		config.StaticConfigs = []string{endpoint}
		return config
	}

	if u.Path != "" {
		config.PathPrefix = u.Path
	}
	config.StaticConfigs = []string{u.Host}
	return config
}

func (r *MCOAAgentReconciler) filterRemoteWrites(rws []cmomanifests.RemoteWriteSpec) []cmomanifests.RemoteWriteSpec {
	clone := slices.Clone(rws)
	return slices.DeleteFunc(clone, func(rw cmomanifests.RemoteWriteSpec) bool {
		return r.CASecret != "" && rw.TLSConfig != nil && rw.TLSConfig.CA.Secret != nil && rw.TLSConfig.CA.Secret.Name == r.CASecret
	})
}

func (r *MCOAAgentReconciler) reconcileAlertmanagerConfigs(
	existing []cmomanifests.AdditionalAlertmanagerConfig,
	endpoint string,
	enable bool,
) ([]cmomanifests.AdditionalAlertmanagerConfig, bool) {
	cleaned := slices.DeleteFunc(slices.Clone(existing), r.isOwnedAlertmanagerConfig)
	if endpoint != "" && enable {
		cleaned = append(cleaned, r.newAdditionalAlertmanagerConfig(endpoint))
	}
	return cleaned, !equality.Semantic.DeepEqual(cleaned, existing)
}

func (r *MCOAAgentReconciler) buildRemoteWriteSpec(rwURL string, writeRelabelConfigs []monitoringv1.RelabelConfig) cmomanifests.RemoteWriteSpec {
	return cmomanifests.RemoteWriteSpec{
		URL: rwURL,
		TLSConfig: &monitoringv1.SafeTLSConfig{
			CA: monitoringv1.SecretOrConfigMap{
				Secret: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: r.CASecret,
					},
					Key: "ca.crt",
				},
			},
			Cert: monitoringv1.SecretOrConfigMap{
				Secret: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: r.CertSecret,
					},
					Key: "tls.crt",
				},
			},
			KeySecret: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: r.CertSecret,
				},
				Key: "tls.key",
			},
			InsecureSkipVerify: ptr.To(false),
		},
		WriteRelabelConfigs: writeRelabelConfigs,
	}
}

func (r *MCOAAgentReconciler) detectConflict(cm *corev1.ConfigMap) bool {
	configYAML, ok := observabilityendpoint.HasClusterMonitoringConfigData(cm)
	if !ok {
		return false // No data is a clean slate, not an external mutation conflict
	}

	parsed := &cmomanifests.ClusterMonitoringConfiguration{}
	if err := yaml.Unmarshal([]byte(configYAML), parsed); err != nil {
		return true // Corrupted YAML is a conflict
	}

	if parsed.PrometheusK8sConfig == nil || len(parsed.PrometheusK8sConfig.AlertmanagerConfigs) == 0 {
		return false // No existing alertmanager configuration is a clean slate / legitimate first-write
	}

	return !slices.ContainsFunc(parsed.PrometheusK8sConfig.AlertmanagerConfigs, r.isOwnedAlertmanagerConfig)
}

func (r *MCOAAgentReconciler) buildRemoteWriteURL(endpoint string) (string, error) {
	if !strings.HasPrefix(endpoint, "https://") && !strings.HasPrefix(endpoint, "http://") {
		endpoint = "https://" + endpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("failed to parse endpoint %q for remote write URL: %w", endpoint, err)
	}
	if !strings.HasSuffix(u.Path, operatorconfig.ObservatoriumAPIRemoteWritePath) {
		u = u.JoinPath(operatorconfig.ObservatoriumAPIRemoteWritePath)
	}
	return u.String(), nil
}

func (r *MCOAAgentReconciler) listScrapeConfigsByComponent(ctx context.Context, component string) ([]prometheusv1alpha1.ScrapeConfig, error) {
	if r.Namespace == "" {
		return nil, nil
	}
	scrapeConfigs := &prometheusv1alpha1.ScrapeConfigList{}
	err := r.List(ctx, scrapeConfigs, client.InNamespace(r.Namespace), client.MatchingLabels{
		labelKeyComponent: component,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list raw scrape configs for component %s: %w", component, err)
	}
	return scrapeConfigs.Items, nil
}

func (r *MCOAAgentReconciler) reconcileRemoteWrites(
	existing []cmomanifests.RemoteWriteSpec,
	scrapeConfigs []prometheusv1alpha1.ScrapeConfig,
	endpoint string,
) ([]cmomanifests.RemoteWriteSpec, error) {
	cleanRemoteWrite := r.filterRemoteWrites(existing)

	if endpoint != "" && len(scrapeConfigs) > 0 {
		rwURL, err := r.buildRemoteWriteURL(endpoint)
		if err != nil {
			return nil, err
		}

		for _, sc := range scrapeConfigs {
			rwSpecTranspiled, err := remotewrite.Transpile(&sc, nil)
			if err != nil {
				// Log the error at Info level and skip to prevent a single malformed ScrapeConfig
				// from blocking the deployment of other valid scrape configurations on the spoke.
				r.Log.Info("Skipping malformed scrape config during remote write transpilation", "name", sc.Name, "error", err.Error())
				continue
			}
			if rwSpecTranspiled == nil {
				continue
			}

			rwSpec := r.buildRemoteWriteSpec(rwURL, rwSpecTranspiled.WriteRelabelConfigs)
			cleanRemoteWrite = append(cleanRemoteWrite, rwSpec)
		}
	}
	return cleanRemoteWrite, nil
}

func (r *MCOAAgentReconciler) createConfigMap(ctx context.Context, cm *corev1.ConfigMap, dataKey, updatedYAML string) error {
	cm = cm.DeepCopy()
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	cm.Data[dataKey] = updatedYAML
	if err := r.Create(ctx, cm); err != nil {
		return fmt.Errorf("failed to create ConfigMap %s/%s: %w", cm.Namespace, cm.Name, err)
	}
	return nil
}

func (r *MCOAAgentReconciler) updateConfigMap(ctx context.Context, cm *corev1.ConfigMap, dataKey, updatedYAML string) error {
	cm = cm.DeepCopy()
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	cm.Data[dataKey] = updatedYAML
	if err := r.Update(ctx, cm); err != nil {
		return fmt.Errorf("failed to update ConfigMap %s/%s: %w", cm.Namespace, cm.Name, err)
	}
	return nil
}

func (r *MCOAAgentReconciler) ReconcileCMOPlatformConfig(ctx context.Context) error {
	endpoint, _ := r.AlertConfig()

	scrapeConfigs, err := r.listScrapeConfigsByComponent(ctx, platformMetricsCollectorRawComponent)
	if err != nil {
		return err
	}

	cmoKey := client.ObjectKey{
		Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
		Namespace: operatorconfig.OCPClusterMonitoringNamespace,
	}

	isCreate := false
	cm := &corev1.ConfigMap{}
	err = r.Get(ctx, cmoKey, cm)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get CMO configmap %s/%s: %w", cmoKey.Namespace, cmoKey.Name, err)
		}
		if endpoint == "" && len(scrapeConfigs) == 0 {
			return nil
		}
		isCreate = true
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
				Namespace: operatorconfig.OCPClusterMonitoringNamespace,
			},
			Data: map[string]string{
				observabilityendpoint.ClusterMonitoringConfigDataKey: defaultCMOConfigYAML,
			},
		}
	}

	configYAML := cm.Data[observabilityendpoint.ClusterMonitoringConfigDataKey]
	parsed := &cmomanifests.ClusterMonitoringConfiguration{}
	if err := yaml.Unmarshal([]byte(configYAML), parsed); err != nil {
		return fmt.Errorf("failed to unmarshal CMO config: %w", err)
	}

	// detectConflict is telemetry-only; reconciliation proceeds regardless.
	if !isCreate && endpoint != "" {
		if r.detectConflict(cm) {
			cmoConfigConflictsTotal.Inc()
			r.Recorder.Eventf(
				cm, nil, corev1.EventTypeWarning, "ConfigConflict", "ConfigReapply",
				"Detected external mutation overwriting Alertmanager configuration. Re-applying Hub Alertmanager config.",
			)
			r.Log.Info("Detected conflict in CMO ConfigMap, re-applying configuration", "name", cm.Name, "namespace", cm.Namespace)
		}
	}

	modified := isCreate

	// Inject external labels and Alertmanager config
	if endpoint != "" {
		if parsed.PrometheusK8sConfig == nil {
			parsed.PrometheusK8sConfig = &cmomanifests.PrometheusK8sConfig{}
		}

		if parsed.PrometheusK8sConfig.ExternalLabels == nil {
			parsed.PrometheusK8sConfig.ExternalLabels = make(map[string]string)
		}
		if parsed.PrometheusK8sConfig.ExternalLabels[operatorconfig.ClusterLabelKeyForAlerts] != r.ClusterID {
			parsed.PrometheusK8sConfig.ExternalLabels[operatorconfig.ClusterLabelKeyForAlerts] = r.ClusterID
			modified = true
		}
		if r.ClusterName != "" {
			if parsed.PrometheusK8sConfig.ExternalLabels[operatorconfig.ClusterNameLabelKeyForAlerts] != r.ClusterName {
				parsed.PrometheusK8sConfig.ExternalLabels[operatorconfig.ClusterNameLabelKeyForAlerts] = r.ClusterName
				modified = true
			}
		} else {
			if _, exists := parsed.PrometheusK8sConfig.ExternalLabels[operatorconfig.ClusterNameLabelKeyForAlerts]; exists {
				delete(parsed.PrometheusK8sConfig.ExternalLabels, operatorconfig.ClusterNameLabelKeyForAlerts)
				modified = true
			}
		}

		cleanAm, modifiedAm := r.reconcileAlertmanagerConfigs(parsed.PrometheusK8sConfig.AlertmanagerConfigs, endpoint, true)
		if modifiedAm {
			parsed.PrometheusK8sConfig.AlertmanagerConfigs = cleanAm
			modified = true
		}
	} else if parsed.PrometheusK8sConfig != nil {
		if parsed.PrometheusK8sConfig.ExternalLabels != nil {
			if _, exists := parsed.PrometheusK8sConfig.ExternalLabels[operatorconfig.ClusterLabelKeyForAlerts]; exists {
				delete(parsed.PrometheusK8sConfig.ExternalLabels, operatorconfig.ClusterLabelKeyForAlerts)
				modified = true
			}
			if _, exists := parsed.PrometheusK8sConfig.ExternalLabels[operatorconfig.ClusterNameLabelKeyForAlerts]; exists {
				delete(parsed.PrometheusK8sConfig.ExternalLabels, operatorconfig.ClusterNameLabelKeyForAlerts)
				modified = true
			}
		}

		cleanAm, modifiedAm := r.reconcileAlertmanagerConfigs(parsed.PrometheusK8sConfig.AlertmanagerConfigs, "", false)
		if modifiedAm {
			parsed.PrometheusK8sConfig.AlertmanagerConfigs = cleanAm
			modified = true
		}
	}

	// Reconcile raw metrics RemoteWrite
	var existingRemoteWrites []cmomanifests.RemoteWriteSpec
	if parsed.PrometheusK8sConfig != nil {
		existingRemoteWrites = parsed.PrometheusK8sConfig.RemoteWrite
	}

	cleanRemoteWrite, err := r.reconcileRemoteWrites(existingRemoteWrites, scrapeConfigs, endpoint)
	if err != nil {
		return err
	}

	if !equality.Semantic.DeepEqual(cleanRemoteWrite, existingRemoteWrites) {
		if parsed.PrometheusK8sConfig == nil {
			parsed.PrometheusK8sConfig = &cmomanifests.PrometheusK8sConfig{}
		}
		parsed.PrometheusK8sConfig.RemoteWrite = cleanRemoteWrite
		modified = true
	}

	if !modified {
		return nil
	}

	updatedYAML, err := yaml.Marshal(parsed)
	if err != nil {
		return fmt.Errorf("failed to marshal updated CMO config: %w", err)
	}

	if isCreate {
		if err := r.createConfigMap(ctx, cm, observabilityendpoint.ClusterMonitoringConfigDataKey, string(updatedYAML)); err != nil {
			return err
		}
	} else {
		if err := r.updateConfigMap(ctx, cm, observabilityendpoint.ClusterMonitoringConfigDataKey, string(updatedYAML)); err != nil {
			return err
		}
	}

	r.Log.Info("Successfully reconciled cluster-monitoring-config")
	return nil
}

func (r *MCOAAgentReconciler) ReconcileCMOUWLConfig(ctx context.Context) error {
	endpoint, enableUWL := r.AlertConfig()

	scrapeConfigs, err := r.listScrapeConfigsByComponent(ctx, userWorkloadMetricsCollectorRawComponent)
	if err != nil {
		return err
	}

	isCreate := false
	cm := &corev1.ConfigMap{}
	err = r.Get(ctx, client.ObjectKey{
		Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
		Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
	}, cm)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get UWL configmap: %w", err)
		}
		if (endpoint == "" || !enableUWL) && len(scrapeConfigs) == 0 {
			return nil
		}
		isCreate = true
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
				Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
			},
			Data: map[string]string{
				uwlMonitoringConfigDataKey: defaultUWLConfigYAML,
			},
		}
	}

	configYAML := cm.Data[uwlMonitoringConfigDataKey]
	parsed := &cmomanifests.UserWorkloadConfiguration{}
	if err := yaml.Unmarshal([]byte(configYAML), parsed); err != nil {
		return fmt.Errorf("failed to unmarshal UWL config: %w", err)
	}

	modified := isCreate

	// Reconcile UWL Alertmanager forwarding
	var existingAM []cmomanifests.AdditionalAlertmanagerConfig
	if parsed.Prometheus != nil {
		existingAM = parsed.Prometheus.AlertmanagerConfigs
	}
	cleanAm, modifiedAm := r.reconcileAlertmanagerConfigs(existingAM, endpoint, enableUWL)
	if modifiedAm {
		if parsed.Prometheus == nil {
			parsed.Prometheus = &cmomanifests.PrometheusRestrictedConfig{}
		}
		parsed.Prometheus.AlertmanagerConfigs = cleanAm
		modified = true
	}

	// Reconcile raw metrics RemoteWrite
	var existingRemoteWrites []cmomanifests.RemoteWriteSpec
	if parsed.Prometheus != nil {
		existingRemoteWrites = parsed.Prometheus.RemoteWrite
	}

	cleanRemoteWrite, err := r.reconcileRemoteWrites(existingRemoteWrites, scrapeConfigs, endpoint)
	if err != nil {
		return err
	}

	if !equality.Semantic.DeepEqual(cleanRemoteWrite, existingRemoteWrites) {
		if parsed.Prometheus == nil {
			parsed.Prometheus = &cmomanifests.PrometheusRestrictedConfig{}
		}
		parsed.Prometheus.RemoteWrite = cleanRemoteWrite
		modified = true
	}

	if !modified {
		return nil
	}

	updatedYAML, err := yaml.Marshal(parsed)
	if err != nil {
		return fmt.Errorf("failed to marshal updated UWL config: %w", err)
	}

	if isCreate {
		if err := r.createConfigMap(ctx, cm, uwlMonitoringConfigDataKey, string(updatedYAML)); err != nil {
			return err
		}
	} else {
		if err := r.updateConfigMap(ctx, cm, uwlMonitoringConfigDataKey, string(updatedYAML)); err != nil {
			return err
		}
	}

	r.Log.Info("Successfully reconciled user-workload-monitoring-config")
	return nil
}
