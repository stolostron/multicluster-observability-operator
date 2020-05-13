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
	DEFAULT_OBJ_STORAGE_TYPE      = "minio"
	DEFAULT_OBJ_STORAGE_BUCKET    = "thanos"
	DEFAULT_OBJ_STORAGE_ENDPOINT  = "minio:9000"
	DEFAULT_OBJ_STORAGE_INSECURE  = true
	DEFAULT_OBJ_STORAGE_ACCESSKEY = "minio"
	DEFAULT_OBJ_STORAGE_SECRETKEY = "minio123"
	DEFAULT_OBJ_STORAGE_STORAGE   = "1Gi"
)

func newDefaultObjectStorageConfigSpec() *monitoringv1alpha1.ObjectStorageConfigSpec {
	spec := &monitoringv1alpha1.ObjectStorageConfigSpec{}
	spec.Type = DEFAULT_OBJ_STORAGE_TYPE
	spec.Config.Bucket = DEFAULT_OBJ_STORAGE_BUCKET
	spec.Config.Endpoint = DEFAULT_OBJ_STORAGE_ENDPOINT
	spec.Config.Insecure = DEFAULT_OBJ_STORAGE_INSECURE
	spec.Config.AccessKey = DEFAULT_OBJ_STORAGE_ACCESSKEY
	spec.Config.SecretKey = DEFAULT_OBJ_STORAGE_SECRETKEY
	spec.Config.Storage = DEFAULT_OBJ_STORAGE_STORAGE

	return spec
}

func updateObjStorageConfig(c client.Client, mcm *monitoringv1alpha1.MultiClusterMonitoring) (*reconcile.Result, error) {
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
