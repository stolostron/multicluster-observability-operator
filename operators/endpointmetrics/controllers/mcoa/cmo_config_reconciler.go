// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package mcoa

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	cmomanifests "github.com/openshift/cluster-monitoring-operator/pkg/manifests"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/observabilityendpoint"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/yaml"
)

var (
	cmoConfigConflictsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "mcoa_cmo_config_conflicts_total",
			Help: "Total number of conflicts detected in the cluster-monitoring-config ConfigMap.",
		},
	)
	registerMetricsOnce sync.Once
)

func registerMetrics() {
	registerMetricsOnce.Do(func() {
		metrics.Registry.MustRegister(cmoConfigConflictsTotal)
	})
}

// cmoConfigReconciler handles the reconciliation of the Cluster Monitoring Operator configuration.
type cmoConfigReconciler struct {
	client.Client
	Log            logr.Logger
	Recorder       record.EventRecorder
	Namespace      string
	ClusterID      string
	HubInfo        *operatorconfig.HubInfo
	CASecret       string
	CertSecret     string
	AccessorSecret string
}

func (r *cmoConfigReconciler) reconcile(ctx context.Context, req client.ObjectKey) error {
	// If Alertmanager forwarding is disabled, we ensure our config is removed.
	if r.HubInfo.AlertmanagerEndpoint == "" {
		r.Log.Info("Alertmanager endpoint is empty, ensuring Hub configuration is removed")
		return observabilityendpoint.RevertClusterMonitoringConfig(ctx, r.Client, r.HubInfo)
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
		if r.detectConflict(cm, r.HubInfo) {
			cmoConfigConflictsTotal.Inc()
			r.Recorder.Event(cm, corev1.EventTypeWarning, "ConfigConflict", "Detected external mutation overwriting Alertmanager configuration. Re-applying Hub Alertmanager config.")
			r.Log.Info("Detected conflict in CMO ConfigMap, re-applying configuration", "name", cm.Name, "namespace", cm.Namespace)
		}
	}

	_, err = observabilityendpoint.CreateOrUpdateCMOConfig(
		ctx,
		r.Client,
		r.ClusterID,
		r.HubInfo,
		r.CASecret,
		r.CertSecret,
		r.AccessorSecret,
		"", // Pass empty namespace to skip legacy revert-marker logic
	)

	return err
}

func (r *cmoConfigReconciler) reconcileUWLConfig(ctx context.Context) error {
	if err := observabilityendpoint.CreateOrUpdateUserWorkloadMonitoringConfig(
		ctx,
		r.Client,
		r.HubInfo,
		r.CASecret,
		r.CertSecret,
		r.AccessorSecret,
	); err != nil {
		return fmt.Errorf("failed to create or update user workload monitoring config: %w", err)
	}
	return nil
}

func (r *cmoConfigReconciler) detectConflict(cm *corev1.ConfigMap, hubInfo *operatorconfig.HubInfo) bool {
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
		if observabilityendpoint.IsManaged(amc, hubInfo, r.CASecret) {
			foundManaged = true
			break
		}
	}

	return !foundManaged
}
