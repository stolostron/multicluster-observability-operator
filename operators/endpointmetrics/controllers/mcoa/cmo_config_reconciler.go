// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package mcoa

import (
	"context"
	"fmt"

	cmomanifests "github.com/openshift/cluster-monitoring-operator/pkg/manifests"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/observabilityendpoint"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/yaml"
)

var cmoConfigConflictsTotal = promauto.With(metrics.Registry).NewCounter(
	prometheus.CounterOpts{
		Name: "mcoa_cmo_config_conflicts_total",
		Help: "Total number of conflicts detected in the cluster-monitoring-config ConfigMap.",
	},
)

func (r *MCOAAgentReconciler) reconcileCMO(ctx context.Context, req client.ObjectKey) error {
	// If Alertmanager forwarding is disabled, we ensure our config is removed.
	if r.AlertmanagerEndpoint == "" {
		r.Log.Info("Alertmanager endpoint is empty, ensuring Hub configuration is removed")
		return observabilityendpoint.RevertClusterMonitoringConfig(ctx, r.Client, r.CASecret)
	}

	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, req, cm)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		cm = nil
	}

	// We trigger a repair if the config is missing or corrupted.
	if cm != nil {
		if r.detectConflict(cm) {
			cmoConfigConflictsTotal.Inc()
			r.Recorder.Eventf(
				cm, nil, corev1.EventTypeWarning, "ConfigConflict", "ConfigReapply",
				"Detected external mutation overwriting Alertmanager configuration. Re-applying Hub Alertmanager config.",
			)
			r.Log.Info("Detected conflict in CMO ConfigMap, re-applying configuration", "name", cm.Name, "namespace", cm.Namespace)
		}
	}

	_, err = observabilityendpoint.CreateOrUpdateCMOConfig(
		ctx,
		r.Client,
		r.ClusterID,
		r.AlertmanagerEndpoint,
		r.CASecret,
		r.CertSecret,
		r.AccessorSecret,
		"", // Pass empty namespace since MCOA handles cleanup dynamically in the reconciler and bypasses legacy persistent configmap markers.
	)

	return err
}

func (r *MCOAAgentReconciler) reconcileUWLConfig(ctx context.Context) error {
	if err := observabilityendpoint.CreateOrUpdateUserWorkloadMonitoringConfig(
		ctx,
		r.Client,
		r.AlertmanagerEndpoint,
		!r.EnableUWLAlertForwarding, // uwmAlertingDisabled
		r.CASecret,
		r.CertSecret,
		r.AccessorSecret,
	); err != nil {
		return fmt.Errorf("failed to create or update user workload monitoring config: %w", err)
	}
	return nil
}

func (r *MCOAAgentReconciler) detectConflict(cm *corev1.ConfigMap) bool {
	// If ACM didn't create/update this config, we don't treat it as a conflict yet.
	if !observabilityendpoint.InManagedFields(cm) {
		return false
	}

	configYAML, ok := observabilityendpoint.HasClusterMonitoringConfigData(cm)
	if !ok {
		return true
	}

	parsed := &cmomanifests.ClusterMonitoringConfiguration{}
	if err := yaml.Unmarshal([]byte(configYAML), parsed); err != nil {
		// Corrupted YAML is a conflict that needs repair.
		return true
	}

	if parsed.PrometheusK8sConfig == nil || parsed.PrometheusK8sConfig.AlertmanagerConfigs == nil {
		return true
	}

	foundManaged := false
	for _, amc := range parsed.PrometheusK8sConfig.AlertmanagerConfigs {
		if observabilityendpoint.IsManaged(amc, r.CASecret) {
			foundManaged = true
			break
		}
	}

	return !foundManaged
}
