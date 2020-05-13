// Copyright (c) 2020 Red Hat, Inc.

package multiclustermonitoring

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	monitoringv1alpha1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/monitoring/v1alpha1"
)

const (
	DEFAULT_VERSION         = "latest"
	DEFAULT_IMG_REPO        = "quay.io/open-cluster-management"
	DEFAULT_IMG_PULL_SECRET = "quay-secret"
	DEFAULT_STORAGE_CLASS   = "gp2"
)

func addDefaultConfig(c client.Client, mcm *monitoringv1alpha1.MultiClusterMonitoring) (*reconcile.Result, error) {
	if mcm.Spec.Version == "" {
		mcm.Spec.Version = DEFAULT_VERSION
	}

	if mcm.Spec.ImageRepository == "" {
		mcm.Spec.ImageRepository = DEFAULT_IMG_REPO
	}

	if string(mcm.Spec.ImagePullPolicy) == "" {
		mcm.Spec.ImagePullPolicy = corev1.PullAlways
	}

	if mcm.Spec.ImagePullSecret == "" {
		mcm.Spec.ImagePullSecret = DEFAULT_IMG_PULL_SECRET
	}

	if mcm.Spec.NodeSelector == nil {
		mcm.Spec.NodeSelector = &monitoringv1alpha1.NodeSelector{}
	}

	if mcm.Spec.StorageClass == "" {
		mcm.Spec.StorageClass = DEFAULT_STORAGE_CLASS
	}

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
		mcm.Spec.Grafana = &monitoringv1alpha1.GrafanaSpec{Hostport: 3000}
	}

	log.Info("Add default config to CR")
	err := c.Update(context.TODO(), mcm)
	if err != nil {
		return &reconcile.Result{}, err
	}

	return nil, nil
}
