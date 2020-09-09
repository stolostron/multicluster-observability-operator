// Copyright (c) 2020 Red Hat, Inc.

package multiclusterobservability

import (
	"bytes"
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	mcoconfig "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
)

// GenerateMonitoringCR is used to generate monitoring CR with the default values
// w/ or w/o customized values
func GenerateMonitoringCR(c client.Client,
	mco *mcov1beta1.MultiClusterObservability) (*reconcile.Result, error) {

	if mco.Spec.ImagePullPolicy == "" {
		mco.Spec.ImagePullPolicy = mcoconfig.DefaultImgPullPolicy
	}

	if mco.Spec.ImagePullSecret == "" {
		mco.Spec.ImagePullSecret = mcoconfig.DefaultImgPullSecret
	}

	if mco.Spec.NodeSelector == nil {
		mco.Spec.NodeSelector = map[string]string{}
	}

	if mco.Spec.StorageConfig == nil {
		mco.Spec.StorageConfig = &mcov1beta1.StorageConfigObject{}
	}

	if mco.Spec.StorageConfig.StatefulSetSize == "" {
		mco.Spec.StorageConfig.StatefulSetSize = mcoconfig.DefaultStorageSize
	}

	if mco.Spec.StorageConfig.StatefulSetStorageClass == "" {
		mco.Spec.StorageConfig.StatefulSetStorageClass = mcoconfig.DefaultStorageClass
	}

	if mco.Spec.EnableDownSampling == "" {
		mco.Spec.EnableDownSampling = mcoconfig.DefaultEnableDownSampling
	}

	if mco.Spec.RetentionResolution1h == "" {
		mco.Spec.RetentionResolution1h = mcoconfig.DefaultRetentionResolution1h
	}

	if mco.Spec.RetentionResolution5m == "" {
		mco.Spec.RetentionResolution5m = mcoconfig.DefaultRetentionResolution5m
	}

	if mco.Spec.RetentionResolutionRaw == "" {
		mco.Spec.RetentionResolutionRaw = mcoconfig.DefaultRetentionResolutionRaw
	}

	if mco.Spec.ObservabilityAddonSpec == nil {
		mco.Spec.ObservabilityAddonSpec = &mcov1beta1.ObservabilityAddonSpec{
			EnableMetrics: true,
			Interval:      mcoconfig.DefaultAddonInterval,
		}
	}

	found := &mcov1beta1.MultiClusterObservability{}
	err := c.Get(
		context.TODO(),
		types.NamespacedName{
			Name: mco.Name,
		},
		found,
	)
	if err != nil {
		return &reconcile.Result{}, err
	}

	desired, err := yaml.Marshal(mco.Spec)
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
		newObj.Spec = mco.Spec
		err = c.Update(context.TODO(), newObj)
		if err != nil {
			return &reconcile.Result{}, err
		}
	}

	return nil, nil
}
