package multiclustermonitoring

import (
	"context"
	"fmt"
	"time"

	monitoringv1alpha1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/monitoring/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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

func checkObjStorageConfig(c client.Client, mcm *monitoringv1alpha1.MultiClusterMonitoring) (*reconcile.Result, error) {
	// Check valid object storage type
	if mcm.Spec.ObjectStorageConfigSpec != nil {
		objStorageType := mcm.Spec.ObjectStorageConfigSpec.Type
		if objStorageType != "minio" && objStorageType != "s3" {
			return &reconcile.Result{}, fmt.Errorf("Invalid object storage type, support s3 and minio only")
		}
		return nil, nil
	}

	log.Info("Add default object storage configuration")
	mcm.Spec.ObjectStorageConfigSpec = &monitoringv1alpha1.ObjectStorageConfigSpec{}
	mcm.Spec.ObjectStorageConfigSpec.Type = DEFAULT_OBJ_STORAGE_TYPE
	mcm.Spec.ObjectStorageConfigSpec.Config.Bucket = DEFAULT_OBJ_STORAGE_BUCKET
	mcm.Spec.ObjectStorageConfigSpec.Config.Endpoint = DEFAULT_OBJ_STORAGE_ENDPOINT
	mcm.Spec.ObjectStorageConfigSpec.Config.Insecure = DEFAULT_OBJ_STORAGE_INSECURE
	mcm.Spec.ObjectStorageConfigSpec.Config.AccessKey = DEFAULT_OBJ_STORAGE_ACCESSKEY
	mcm.Spec.ObjectStorageConfigSpec.Config.SecretKey = DEFAULT_OBJ_STORAGE_SECRETKEY
	mcm.Spec.ObjectStorageConfigSpec.Config.Storage = DEFAULT_OBJ_STORAGE_STORAGE

	err := c.Update(context.TODO(), mcm)
	if err != nil {
		if errors.IsConflict(err) {
			// Error from object being modified is normal behavior and should not be treated like an error
			log.Info("Failed to update object storage configuration", "Reason", "Object has been modified")
			return &reconcile.Result{RequeueAfter: time.Second}, nil
		}

		log.Error(err, fmt.Sprintf("Failed to update %s/%s object storage configuration ", mcm.Namespace, mcm.Name))
		return &reconcile.Result{}, err
	}
	return nil, nil
}
