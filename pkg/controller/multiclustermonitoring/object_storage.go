// Copyright (c) 2020 Red Hat, Inc.

package multiclustermonitoring

import (
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
