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
	defaultVersion       = "latest"
	defaultImgRepo       = "quay.io/open-cluster-management"
	defaultImgPullSecret = "quay-secret"
	defaultStorageClass  = "gp2"
)

func UpdateMonitoringCR(
	c client.Client,
	mcm *monitoringv1alpha1.MultiClusterMonitoring) (*reconcile.Result, error) {

	if mcm.Spec.Version == "" {
		mcm.Spec.Version = defaultVersion
	}

	if mcm.Spec.ImageRepository == "" {
		mcm.Spec.ImageRepository = defaultImgRepo
	}

	if string(mcm.Spec.ImagePullPolicy) == "" {
		mcm.Spec.ImagePullPolicy = corev1.PullAlways
	}

	if mcm.Spec.ImagePullSecret == "" {
		mcm.Spec.ImagePullSecret = defaultImgPullSecret
	}

	if mcm.Spec.NodeSelector == nil {
		mcm.Spec.NodeSelector = &monitoringv1alpha1.NodeSelector{}
	}

	if mcm.Spec.StorageClass == "" {
		mcm.Spec.StorageClass = defaultStorageClass
	}

	if mcm.Spec.Observatorium == nil {
		log.Info("Add default object storage configuration")
		mcm.Spec.Observatorium = newDefaultObservatoriumSpec()
	} else {
		result, err := updateObservatoriumSpec(c, mcm)
		if result != nil {
			return result, err
		}
	}

	if mcm.Spec.ObjectStorageConfigSpec == nil {
		log.Info("Add default observatorium spec")
		mcm.Spec.ObjectStorageConfigSpec = newDefaultObjectStorageConfigSpec()
	} else {
		result, err := updateObjStorageConfig(c, mcm)
		if result != nil {
			return result, err
		}
	}

	if mcm.Spec.Grafana == nil {
		log.Info("Add default grafana config")
		mcm.Spec.Grafana = newGrafanaConfigSpec()
	}

	log.Info("Add default config to CR")
	err := c.Update(context.TODO(), mcm)
	if err != nil {
		return &reconcile.Result{}, err
	}

	return nil, nil
}
