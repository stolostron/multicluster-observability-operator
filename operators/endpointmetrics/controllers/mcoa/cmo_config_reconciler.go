// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package mcoa

import (
	"context"
	"fmt"
	"maps"
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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/yaml"
)

const (
	// uwlMonitoringConfigDataKey is the data key mandated by the user-workload-monitoring ConfigMap schema
	uwlMonitoringConfigDataKey = "config.yaml"

	// labelKeyComponent is the standard metadata label key for Kubernetes component discovery
	labelKeyComponent = "app.kubernetes.io/component"

	// mcoaRawNamePrefix is the unique name prefix assigned to MCOA raw metrics RemoteWrites
	mcoaRawNamePrefix = "mcoa-raw-"

	// Package-level constants for load-bearing component labels
	platformMetricsCollectorRawComponent     = "platform-metrics-collector-raw"
	userWorkloadMetricsCollectorRawComponent = "user-workload-metrics-collector-raw"
	platformMetricsCollectorComponent        = "platform-metrics-collector"
	userWorkloadMetricsCollectorComponent    = "user-workload-metrics-collector"

	// Default bootstrap YAML templates to avoid magic strings
	defaultCMOConfigYAML = "prometheusK8s: {}"
	defaultUWLConfigYAML = "prometheus: {}"
)

var cmoConfigReconcilesTotal = promauto.With(metrics.Registry).NewCounter(
	prometheus.CounterOpts{
		Name: "mcoa_cmo_config_reconciles_total",
		Help: "Total number of reconciles/updates performed on the cluster-monitoring-config ConfigMap.",
	},
)

var uwlConfigReconcilesTotal = promauto.With(metrics.Registry).NewCounter(
	prometheus.CounterOpts{
		Name: "mcoa_uwl_config_reconciles_total",
		Help: "Total number of reconciles/updates performed on the user-workload-monitoring-config ConfigMap.",
	},
)

func (r *MCOAAgentReconciler) ReconcileCMOPlatformConfig(ctx context.Context) error {
	alertmanagerURL := r.HubAlertmanagerURL

	scrapeConfigs, err := r.listScrapeConfigsByComponent(ctx, platformMetricsCollectorRawComponent)
	if err != nil {
		return err
	}

	// Fetch PrometheusAgent dynamically to read remoteWrite config
	agent, err := r.fetchPrometheusAgentByComponent(ctx, platformMetricsCollectorComponent)
	if err != nil {
		return err
	}

	cmoKey := client.ObjectKey{
		Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
		Namespace: operatorconfig.OCPClusterMonitoringNamespace,
	}

	cm, isCreate, err := r.fetchCMOConfigMap(ctx, cmoKey, agent, alertmanagerURL, len(scrapeConfigs))
	if err != nil || cm == nil {
		return err
	}

	configYAML := cm.Data[observabilityendpoint.ClusterMonitoringConfigDataKey]
	parsed := &cmomanifests.ClusterMonitoringConfiguration{}
	if err := yaml.Unmarshal([]byte(configYAML), parsed); err != nil {
		return fmt.Errorf("failed to unmarshal CMO config: %w", err)
	}

	modified := isCreate

	// Ensure PrometheusK8sConfig is initialized if we are deploying configurations
	if (agent != nil || alertmanagerURL != "") && parsed.PrometheusK8sConfig == nil {
		parsed.PrometheusK8sConfig = &cmomanifests.PrometheusK8sConfig{}
	}

	if parsed.PrometheusK8sConfig != nil {
		// 1. Reconcile External Labels
		if r.reconcileExternalLabels(parsed.PrometheusK8sConfig, agent != nil) {
			modified = true
		}

		// 2. Reconcile Alertmanager Configurations
		cleanAm, modifiedAm := r.reconcileAlertmanagerConfigs(parsed.PrometheusK8sConfig.AlertmanagerConfigs, alertmanagerURL, r.EnablePlatformAlertForwarding)
		if modifiedAm {
			parsed.PrometheusK8sConfig.AlertmanagerConfigs = cleanAm
			modified = true

			// Emit event if we had to update/overwrite Alertmanager configs in an existing ConfigMap
			if !isCreate && alertmanagerURL != "" {
				if r.Recorder != nil {
					r.Recorder.Eventf(
						cm, nil, corev1.EventTypeWarning, "ConfigConflict", "ConfigReapply",
						"Detected external mutation overwriting Alertmanager configuration. Re-applying Hub Alertmanager config.",
					)
				}
				r.Log.Info("Detected conflict in Alertmanager configuration, re-applying", "name", cm.Name, "namespace", cm.Namespace)
			}
		}

		// 3. Reconcile Remote Writes
		existingRemoteWrites := parsed.PrometheusK8sConfig.RemoteWrite
		cleanRemoteWrite := r.reconcileRemoteWrites(existingRemoteWrites, scrapeConfigs, agent)
		if !equality.Semantic.DeepEqual(cleanRemoteWrite, existingRemoteWrites) {
			parsed.PrometheusK8sConfig.RemoteWrite = cleanRemoteWrite
			modified = true
		}
	}

	if !modified {
		return nil
	}

	updatedYAML, err := yaml.Marshal(parsed)
	if err != nil {
		return fmt.Errorf("failed to marshal updated CMO config: %w", err)
	}

	if isCreate {
		err := r.createConfigMap(ctx, cm, observabilityendpoint.ClusterMonitoringConfigDataKey, string(updatedYAML))
		if err == nil {
			cmoConfigReconcilesTotal.Inc()
		}
		return err
	}
	err = r.updateConfigMap(ctx, cm, observabilityendpoint.ClusterMonitoringConfigDataKey, string(updatedYAML))
	if err == nil {
		cmoConfigReconcilesTotal.Inc()
	}
	return err
}

func (r *MCOAAgentReconciler) ReconcileCMOUWLConfig(ctx context.Context) error {
	alertmanagerURL := r.HubAlertmanagerURL

	scrapeConfigs, err := r.listScrapeConfigsByComponent(ctx, userWorkloadMetricsCollectorRawComponent)
	if err != nil {
		return err
	}

	// Fetch PrometheusAgent dynamically
	agent, err := r.fetchPrometheusAgentByComponent(ctx, userWorkloadMetricsCollectorComponent)
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
		if (agent == nil && (alertmanagerURL == "" || !r.EnableUWLAlertForwarding)) && len(scrapeConfigs) == 0 {
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
	cleanAm, modifiedAm := r.reconcileAlertmanagerConfigs(existingAM, alertmanagerURL, r.EnableUWLAlertForwarding)
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

	cleanRemoteWrite := r.reconcileRemoteWrites(existingRemoteWrites, scrapeConfigs, agent)

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
		err := r.createConfigMap(ctx, cm, uwlMonitoringConfigDataKey, string(updatedYAML))
		if err == nil {
			uwlConfigReconcilesTotal.Inc()
		}
		return err
	}
	err = r.updateConfigMap(ctx, cm, uwlMonitoringConfigDataKey, string(updatedYAML))
	if err == nil {
		uwlConfigReconcilesTotal.Inc()
	}
	return err
}

func (r *MCOAAgentReconciler) fetchCMOConfigMap(
	ctx context.Context,
	key client.ObjectKey,
	agent *prometheusv1alpha1.PrometheusAgent,
	alertmanagerURL string,
	numScrapeConfigs int,
) (*corev1.ConfigMap, bool, error) {
	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, key, cm)
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, false, fmt.Errorf("failed to get CMO configmap %s/%s: %w", key.Namespace, key.Name, err)
		}
		if agent == nil && alertmanagerURL == "" && numScrapeConfigs == 0 {
			return nil, false, nil
		}
		return &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      key.Name,
				Namespace: key.Namespace,
			},
			Data: map[string]string{
				observabilityendpoint.ClusterMonitoringConfigDataKey: defaultCMOConfigYAML,
			},
		}, true, nil
	}
	return cm, false, nil
}

func (r *MCOAAgentReconciler) reconcileExternalLabels(cfg *cmomanifests.PrometheusK8sConfig, hasAgent bool) bool {
	if cfg == nil {
		return false
	}
	if !hasAgent {
		if cfg.ExternalLabels == nil {
			return false
		}
		modified := false
		if _, exists := cfg.ExternalLabels[operatorconfig.ClusterLabelKeyForAlerts]; exists {
			delete(cfg.ExternalLabels, operatorconfig.ClusterLabelKeyForAlerts)
			modified = true
		}
		if _, exists := cfg.ExternalLabels[operatorconfig.ClusterNameLabelKeyForAlerts]; exists {
			delete(cfg.ExternalLabels, operatorconfig.ClusterNameLabelKeyForAlerts)
			modified = true
		}
		return modified
	}

	modified := false
	if cfg.ExternalLabels == nil {
		cfg.ExternalLabels = make(map[string]string)
	}
	if cfg.ExternalLabels[operatorconfig.ClusterLabelKeyForAlerts] != r.ClusterID {
		cfg.ExternalLabels[operatorconfig.ClusterLabelKeyForAlerts] = r.ClusterID
		modified = true
	}
	if r.ClusterName != "" {
		if cfg.ExternalLabels[operatorconfig.ClusterNameLabelKeyForAlerts] != r.ClusterName {
			cfg.ExternalLabels[operatorconfig.ClusterNameLabelKeyForAlerts] = r.ClusterName
			modified = true
		}
	} else {
		if _, exists := cfg.ExternalLabels[operatorconfig.ClusterNameLabelKeyForAlerts]; exists {
			delete(cfg.ExternalLabels, operatorconfig.ClusterNameLabelKeyForAlerts)
			modified = true
		}
	}
	return modified
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

func (r *MCOAAgentReconciler) newAdditionalAlertmanagerConfig(endpoint string) cmomanifests.AdditionalAlertmanagerConfig {
	config := cmomanifests.AdditionalAlertmanagerConfig{
		Scheme:     "https",
		PathPrefix: "/api/alertmanager/v2/default",
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
		r.Log.Info("failed to parse alertmanager endpoint, falling back to raw value", "endpoint", endpoint, "error", err)
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

func (r *MCOAAgentReconciler) isOwnedAlertmanagerConfig(am cmomanifests.AdditionalAlertmanagerConfig) bool {
	return r.CASecret != "" && am.TLSConfig.CA != nil && am.TLSConfig.CA.Name == r.CASecret
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
	agent *prometheusv1alpha1.PrometheusAgent,
) []cmomanifests.RemoteWriteSpec {
	cleanRemoteWrite := r.filterRemoteWrites(existing)

	if agent != nil && len(scrapeConfigs) > 0 {
		for _, sc := range scrapeConfigs {
			rwSpecsTranspiled, err := remotewrite.Transpile(&sc, agent)
			if err != nil {
				// Log the error at Info level and skip to prevent a single malformed ScrapeConfig
				// from blocking the deployment of other valid scrape configurations on the spoke.
				r.Log.Info("Skipping malformed scrape config during remote write transpilation", "name", sc.Name, "error", err)
				continue
			}

			for i, rwSpec := range rwSpecsTranspiled {
				if rwSpec == nil {
					continue
				}

				suffix := fmt.Sprintf("%d", i)
				if rwSpec.Name != nil {
					suffix = *rwSpec.Name
				}
				name := mcoaRawNamePrefix + sc.Name + "-" + suffix
				rwSpec.Name = &name

				cleanRemoteWrite = append(cleanRemoteWrite, toCMORemoteWrite(rwSpec))
			}
		}
	}
	return cleanRemoteWrite
}

func (r *MCOAAgentReconciler) filterRemoteWrites(rws []cmomanifests.RemoteWriteSpec) []cmomanifests.RemoteWriteSpec {
	clone := slices.Clone(rws)
	return slices.DeleteFunc(clone, func(rw cmomanifests.RemoteWriteSpec) bool {
		return strings.HasPrefix(rw.Name, mcoaRawNamePrefix)
	})
}

func toCMORemoteWrite(rw *monitoringv1.RemoteWriteSpec) cmomanifests.RemoteWriteSpec {
	if rw == nil {
		return cmomanifests.RemoteWriteSpec{}
	}
	spec := cmomanifests.RemoteWriteSpec{
		URL:                 rw.URL,
		WriteRelabelConfigs: rw.WriteRelabelConfigs,
		Headers:             maps.Clone(rw.Headers),
	}

	if rw.Name != nil {
		spec.Name = *rw.Name
	}

	if rw.RemoteTimeout != nil {
		spec.RemoteTimeout = string(*rw.RemoteTimeout)
	}

	if rw.BasicAuth != nil {
		spec.BasicAuth = rw.BasicAuth.DeepCopy()
	}

	if rw.Authorization != nil {
		spec.Authorization = &monitoringv1.SafeAuthorization{
			Type: rw.Authorization.Type,
		}
		if rw.Authorization.Credentials != nil {
			spec.Authorization.Credentials = rw.Authorization.Credentials.DeepCopy()
		}
	}

	if rw.OAuth2 != nil {
		spec.OAuth2 = rw.OAuth2.DeepCopy()
	}

	if rw.TLSConfig != nil {
		// CAFile, CertFile, KeyFile from TLSConfig are intentionally omitted in standard Copy,
		// but since cmomanifests.SafeTLSConfig supports only secret-reference TLS, we dynamically
		// extract both the secret names (directories) and key names (filenames) from these path
		// fields to reconstruct valid secret references dynamically.
		spec.TLSConfig = rw.TLSConfig.SafeTLSConfig.DeepCopy()

		caSecretName, caKeyName := extractSecretAndKey(rw.TLSConfig.CAFile)
		if caSecretName != "" {
			spec.TLSConfig.CA = monitoringv1.SecretOrConfigMap{
				Secret: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: caSecretName,
					},
					Key: caKeyName,
				},
			}
		}

		certSecretName, certKeyName := extractSecretAndKey(rw.TLSConfig.CertFile)
		if certSecretName != "" {
			spec.TLSConfig.Cert = monitoringv1.SecretOrConfigMap{
				Secret: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: certSecretName,
					},
					Key: certKeyName,
				},
			}
		}

		keySecretName, keyName := extractSecretAndKey(rw.TLSConfig.KeyFile)
		if keySecretName != "" {
			spec.TLSConfig.KeySecret = &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: keySecretName,
				},
				Key: keyName,
			}
		}
	}

	if rw.QueueConfig != nil {
		spec.QueueConfig = rw.QueueConfig.DeepCopy()
	}

	if rw.ProxyURL != nil {
		spec.ProxyURL = *rw.ProxyURL
	}

	return spec
}

func extractSecretAndKey(path string) (string, string) {
	if path == "" {
		return "", ""
	}
	parts := strings.Split(path, "/")
	if len(parts) >= 2 {
		secretName := parts[len(parts)-2]
		keyName := parts[len(parts)-1]
		return secretName, keyName
	}
	return "", ""
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

// fetchPrometheusAgentByComponent lists PrometheusAgents by component label and returns
// the first match. Returns (nil, nil) when zero agents exist or the CRD is not yet registered.
func (r *MCOAAgentReconciler) fetchPrometheusAgentByComponent(
	ctx context.Context, component string,
) (*prometheusv1alpha1.PrometheusAgent, error) {
	list := &prometheusv1alpha1.PrometheusAgentList{}
	if err := r.List(ctx, list,
		client.InNamespace(r.Namespace),
		client.MatchingLabels{labelKeyComponent: component},
	); err != nil {
		if meta.IsNoMatchError(err) {
			r.Log.V(1).Info("PrometheusAgent CRD not registered yet, skipping", "component", component, "error", err)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list PrometheusAgents for component %s: %w", component, err)
	}
	if len(list.Items) == 0 {
		r.Log.V(1).Info("No PrometheusAgent found matching component label", "component", component, "namespace", r.Namespace)
		return nil, nil
	}
	if len(list.Items) > 1 {
		r.Log.Info("Multiple PrometheusAgents found matching component label, using the first one",
			"component", component, "count", len(list.Items), "namespace", r.Namespace)
	}
	return &list.Items[0], nil
}
