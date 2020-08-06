// Copyright (c) 2020 Red Hat, Inc.

package multiclusterobservability

import (
	"bytes"
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	mcoconfig "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
)

const (
	defaultImagePullPolicy = corev1.PullAlways
	defaultImagePullSecret = "multiclusterhub-operator-pull-secret"
	defaultImageRepository = "quay.io/open-cluster-management"
	defaultImageTagSuffix  = ""
	defaultStorageClass    = "gp2"
	defaultStorageSize     = "50Gi"
)

// GenerateMonitoringCR is used to generate monitoring CR with the default values
// w/ or w/o customized values
func GenerateMonitoringCR(c client.Client,
	mco *mcov1beta1.MultiClusterObservability) (*reconcile.Result, error) {

	if mco.Annotations == nil {
		mco.Annotations = map[string]string{
			mcoconfig.AnnotationKeyImageRepository: defaultImageRepository,
			mcoconfig.AnnotationKeyImageTagSuffix:  defaultImageTagSuffix,
		}
	} else {
		if _, ok := mco.Annotations[mcoconfig.AnnotationKeyImageRepository]; !ok {
			mco.Annotations[mcoconfig.AnnotationKeyImageRepository] = defaultImageRepository
		}
		if _, ok := mco.Annotations[mcoconfig.AnnotationKeyImageTagSuffix]; !ok {
			mco.Annotations[mcoconfig.AnnotationKeyImageTagSuffix] = defaultImageTagSuffix
		}
	}

	if mco.Spec.ImagePullPolicy == "" {
		mco.Spec.ImagePullPolicy = defaultImagePullPolicy
	}

	if mco.Spec.ImagePullSecret == "" {
		mco.Spec.ImagePullSecret = defaultImagePullSecret
	}

	if mco.Spec.NodeSelector == nil {
		mco.Spec.NodeSelector = map[string]string{}
	}

	if mco.Spec.StorageClass == "" {
		mco.Spec.StorageClass = defaultStorageClass
	}

	if mco.Spec.ObjectStorageConfig == nil {
		err := GenerateObjectStorageSecret(c, mco)
		if err != nil {
			return &reconcile.Result{}, err
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
