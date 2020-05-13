// Copyright (c) 2020 Red Hat, Inc.

package multiclustermonitoring

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	monitoringv1alpha1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/monitoring/v1alpha1"
)

func NewFakeClient(mcm *monitoringv1alpha1.MultiClusterMonitoring) client.Client {
	s := scheme.Scheme
	s.AddKnownTypes(monitoringv1alpha1.SchemeGroupVersion, mcm)
	objs := []runtime.Object{mcm}
	return fake.NewFakeClient(objs...)
}

func TestNewDefaultObjectStorageConfigSpec(t *testing.T) {
	spec := newDefaultObjectStorageConfigSpec()

	if spec.Type != DEFAULT_OBJ_STORAGE_TYPE {
		t.Errorf("Type (%v) is not the expected (%v)", spec.Type, DEFAULT_OBJ_STORAGE_TYPE)
	}

	if spec.Config.Bucket != DEFAULT_OBJ_STORAGE_BUCKET {
		t.Errorf("Bucket (%v) is not the expected (%v)", spec.Config.Bucket, DEFAULT_OBJ_STORAGE_BUCKET)
	}

	if spec.Config.Endpoint != DEFAULT_OBJ_STORAGE_ENDPOINT {
		t.Errorf("Endpoint (%v) is not the expected (%v)", spec.Config.Endpoint, DEFAULT_OBJ_STORAGE_ENDPOINT)
	}

	if spec.Config.Insecure != DEFAULT_OBJ_STORAGE_INSECURE {
		t.Errorf("Insecure (%v) is not the expected (%v)", spec.Config.Insecure, DEFAULT_OBJ_STORAGE_INSECURE)
	}

	if spec.Config.AccessKey != DEFAULT_OBJ_STORAGE_ACCESSKEY {
		t.Errorf("AccessKey (%v) is not the expected (%v)", spec.Config.AccessKey, DEFAULT_OBJ_STORAGE_ACCESSKEY)
	}

	if spec.Config.SecretKey != DEFAULT_OBJ_STORAGE_SECRETKEY {
		t.Errorf("SecretKey (%v) is not the expected (%v)", spec.Config.SecretKey, DEFAULT_OBJ_STORAGE_SECRETKEY)
	}

	if spec.Config.Storage != DEFAULT_OBJ_STORAGE_STORAGE {
		t.Errorf("Storage (%v) is not the expected (%v)", spec.Config.Storage, DEFAULT_OBJ_STORAGE_STORAGE)
	}

}

func TestCheckObjStorageConfig(t *testing.T) {
	mcm := &monitoringv1alpha1.MultiClusterMonitoring{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterMonitoring"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "test"},
		Spec: monitoringv1alpha1.MultiClusterMonitoringSpec{
			ObjectStorageConfigSpec: newDefaultObjectStorageConfigSpec(),
		},
	}

	result, err := updateObjStorageConfig(NewFakeClient(mcm), mcm)
	if result != nil || err != nil {
		t.Errorf("Should return nil for result (%v) and err (%v)", result, err)
	}

	mcm.Spec.ObjectStorageConfigSpec.Type = "invalid"
	result, err = updateObjStorageConfig(NewFakeClient(mcm), mcm)
	if result == nil || err == nil {
		t.Errorf("Failed to check valid object storage type: result: (%v) err: (%v)", result, err)
	}

	mcm.Spec.ObjectStorageConfigSpec.Type = DEFAULT_OBJ_STORAGE_TYPE
	result, err = updateObjStorageConfig(NewFakeClient(mcm), mcm)
	if result != nil || err != nil {
		t.Errorf("(%v) should be a valid type", DEFAULT_OBJ_STORAGE_TYPE)
	}

	mcm.Spec.ObjectStorageConfigSpec.Type = "s3"
	result, err = updateObjStorageConfig(NewFakeClient(mcm), mcm)
	if result != nil || err != nil {
		t.Errorf("(s3) should be a valid type")
	}

	updateObjStorageConfig(NewFakeClient(mcm), mcm)
	if mcm.Spec.ObjectStorageConfigSpec.Config.Bucket != DEFAULT_OBJ_STORAGE_BUCKET {
		t.Errorf("Bucket (%v) is not the expected (%v)", mcm.Spec.ObjectStorageConfigSpec.Config.Bucket, DEFAULT_OBJ_STORAGE_BUCKET)
	}

	mcm.Spec.ObjectStorageConfigSpec = newDefaultObjectStorageConfigSpec()
	if mcm.Spec.ObjectStorageConfigSpec.Config.Endpoint != DEFAULT_OBJ_STORAGE_ENDPOINT {
		t.Errorf("Endpoint (%v) is not the expected (%v)", mcm.Spec.ObjectStorageConfigSpec.Config.Endpoint, DEFAULT_OBJ_STORAGE_ENDPOINT)
	}
}
