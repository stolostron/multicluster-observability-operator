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

func TestCheckObjStorageConfig(t *testing.T) {
	mcm := &monitoringv1alpha1.MultiClusterMonitoring{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterMonitoring"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "test"},
		Spec: monitoringv1alpha1.MultiClusterMonitoringSpec{
			ObjectStorageConfigSpec: &monitoringv1alpha1.ObjectStorageConfigSpec{
				Type: "minio",
				Config: monitoringv1alpha1.ObjectStorageConfig{
					Bucket:    "test",
					Endpoint:  "test",
					Insecure:  false,
					AccessKey: "test",
					SecretKey: "test",
					Storage:   "test",
				},
			},
		},
	}

	result, err := checkObjStorageConfig(NewFakeClient(mcm), mcm)
	if result != nil || err != nil {
		t.Errorf("should return nil for result (%v) and err (%v)", result, err)
	}

	mcm.Spec.ObjectStorageConfigSpec.Type = "invalid"
	result, err = checkObjStorageConfig(NewFakeClient(mcm), mcm)
	if result == nil || err == nil {
		t.Errorf("failed to check valid object storage type: result: (%v) err: (%v)", result, err)
	}

	mcm.Spec.ObjectStorageConfigSpec.Type = "minio"
	result, err = checkObjStorageConfig(NewFakeClient(mcm), mcm)
	if result != nil || err != nil {
		t.Errorf("minio should be a valid type")
	}

	mcm.Spec.ObjectStorageConfigSpec.Type = "s3"
	result, err = checkObjStorageConfig(NewFakeClient(mcm), mcm)
	if result != nil || err != nil {
		t.Errorf("s3 should be a valid type")
	}

	checkObjStorageConfig(NewFakeClient(mcm), mcm)
	if mcm.Spec.ObjectStorageConfigSpec.Config.Bucket != "test" {
		t.Errorf("bucket (%v) is not the expected (test)", mcm.Spec.ObjectStorageConfigSpec.Config.Bucket)
	}

	mcm.Spec.ObjectStorageConfigSpec = nil
	checkObjStorageConfig(NewFakeClient(mcm), mcm)
	if mcm.Spec.ObjectStorageConfigSpec.Config.Endpoint != "minio:9000" {
		t.Errorf("endpoint (%v) is not the expected (minio:9000)", mcm.Spec.ObjectStorageConfigSpec.Config.Endpoint)
	}
}
