// Copyright (c) 2020 Red Hat, Inc.

package multiclustermonitoring

import (
	"context"

	monitoringv1alpha1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/monitoring/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func addDefaultConfig(c client.Client, mcm *monitoringv1alpha1.MultiClusterMonitoring) (*reconcile.Result, error) {
	if mcm.Spec.Observatorium == nil {
		log.Info("Add default object storage configuration")
		mcm.Spec.Observatorium = newDefaultObservatoriumSpec()
	}

	if mcm.Spec.ObjectStorageConfigSpec == nil {
		log.Info("Add default observatorium spec")
		mcm.Spec.ObjectStorageConfigSpec = newDefaultObjectStorageConfigSpec()
	}

	if mcm.Spec.Grafana == nil {
		log.Info("Add default grafana config")
		mcm.Spec.Grafana = &monitoringv1alpha1.GrafanaSpec{}
	}

	log.Info("Add default config to CR")
	err := c.Update(context.TODO(), mcm)
	if err != nil {
		return &reconcile.Result{}, err
	}

	return nil, nil
}
