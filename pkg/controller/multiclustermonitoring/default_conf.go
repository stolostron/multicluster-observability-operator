// Copyright (c) 2020 Red Hat, Inc.

package multiclustermonitoring

import (
	"bytes"
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	monitoringv1alpha1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/monitoring/v1alpha1"
)

const (
	defaultVersion       = "latest"
	defaultImgRepo       = "quay.io/open-cluster-management"
	defaultImgPullSecret = "multiclusterhub-operator-pull-secret"
	defaultStorageClass  = "gp2"
)

// GenerateMonitoringCR is used to generate monitoring CR with the default values
// w/ or w/o customized values
func GenerateMonitoringCR(c client.Client,
	mcm *monitoringv1alpha1.MultiClusterObservability) (*reconcile.Result, error) {

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
		mcm.Spec.NodeSelector = map[string]string{}
	}

	if mcm.Spec.StorageClass == "" {
		mcm.Spec.StorageClass = defaultStorageClass
	}

	if mcm.Spec.Observatorium == nil {
		log.Info("Add default observatorium spec")
		mcm.Spec.Observatorium = newDefaultObservatoriumSpec()
	} else {
		result, err := updateObservatoriumSpec(c, mcm)
		if result != nil {
			return result, err
		}
	}

	if mcm.Spec.ObjectStorageConfigSpec == nil {
		log.Info("Add default object storage configuration")
		mcm.Spec.ObjectStorageConfigSpec = newDefaultObjectStorageConfigSpec()
	}

	if mcm.Spec.Grafana == nil {
		log.Info("Add default grafana config")
		mcm.Spec.Grafana = newGrafanaConfigSpec()
	} else {
		updateGrafanaConfig(mcm)
	}

	found := &monitoringv1alpha1.MultiClusterObservability{}
	err := c.Get(
		context.TODO(),
		types.NamespacedName{
			Name: mcm.Name,
		},
		found,
	)
	if err != nil {
		return &reconcile.Result{}, err
	}

	desired, err := yaml.Marshal(mcm.Spec)
	if err != nil {
		log.Error(err, "cannot parse the desired MultiClusterObservability values")
	}
	current, err := yaml.Marshal(found.Spec)
	if err != nil {
		log.Error(err, "cannot parse the current MultiClusterObservability values")
	}

	if res := bytes.Compare(desired, current); res != 0 {
		log.Info("Update MultiClusterObservability CR.")
		newObj := found.DeepCopy()
		newObj.Spec = mcm.Spec
		err = c.Update(context.TODO(), newObj)
		if err != nil {
			return &reconcile.Result{}, err
		}
	}

	return nil, nil
}
