// Copyright (c) 2020 Red Hat, Inc.

package multiclustermonitoring

import (
	"context"
	"fmt"
	"reflect"

	"github.com/imdario/mergo"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	monitoringv1alpha1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/monitoring/v1alpha1"
)

const (
	defaultObjStorageType      = "minio"
	defaultObjStorageBucket    = "thanos"
	defaultObjStorageEndpoint  = "minio:9000"
	defaultObjStorageInsecure  = true
	defaultObjStorageAccesskey = "minio"
	defaultObjStorageSecretkey = "minio123"
	defaultObjStorageStorage   = "1Gi"
)

func newDefaultObjectStorageConfigSpec() *monitoringv1alpha1.ObjectStorageConfigSpec {
	spec := &monitoringv1alpha1.ObjectStorageConfigSpec{}
	spec.Type = defaultObjStorageType
	spec.Config.Bucket = defaultObjStorageBucket
	spec.Config.Endpoint = defaultObjStorageEndpoint
	spec.Config.Insecure = defaultObjStorageInsecure
	spec.Config.AccessKey = defaultObjStorageAccesskey
	spec.Config.SecretKey = defaultObjStorageSecretkey
	spec.Config.Storage = defaultObjStorageStorage

	return spec
}

func updateObjStorageConfig(
	c client.Client,
	mcm *monitoringv1alpha1.MultiClusterMonitoring) (*reconcile.Result, error) {

	// Check valid object storage type
	if mcm.Spec.ObjectStorageConfigSpec != nil {
		objStorageType := mcm.Spec.ObjectStorageConfigSpec.Type
		if objStorageType != "minio" && objStorageType != "s3" {
			return &reconcile.Result{}, fmt.Errorf("Invalid object storage type, support s3 and minio only")
		}

		found := &monitoringv1alpha1.MultiClusterMonitoring{}
		err := c.Get(context.TODO(), types.NamespacedName{Name: mcm.Name, Namespace: mcm.Namespace}, found)
		if err != nil && errors.IsNotFound(err) {
			return &reconcile.Result{}, nil
		} else if err != nil {
			return &reconcile.Result{}, err
		}

		oldSpec := found.Spec.ObjectStorageConfigSpec
		newSpec := mcm.Spec.ObjectStorageConfigSpec
		if !reflect.DeepEqual(oldSpec, newSpec) {
			// merge newSpec to oldSpec
			if err := mergo.Merge(&oldSpec, newSpec, mergo.WithOverride); err != nil {
				return &reconcile.Result{}, err
			}
			newObj := found.DeepCopy()
			newObj.Spec.ObjectStorageConfigSpec = oldSpec
			err = c.Update(context.TODO(), newObj)
			if err != nil {
				return &reconcile.Result{}, err
			}
		}

		return nil, nil
	}

	return nil, nil
}
