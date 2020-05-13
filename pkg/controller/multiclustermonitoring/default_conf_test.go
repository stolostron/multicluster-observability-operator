// Copyright (c) 2020 Red Hat, Inc.

package multiclustermonitoring

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	monitoringv1alpha1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/monitoring/v1alpha1"
)

func TestddDefaultConfig(t *testing.T) {
	mcm := &monitoringv1alpha1.MultiClusterMonitoring{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterMonitoring"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "test"},
		Spec:       monitoringv1alpha1.MultiClusterMonitoringSpec{},
	}

	result, err := addDefaultConfig(NewFakeClient(mcm), mcm)
	if result != nil || err != nil {
		t.Errorf("Should return nil for result (%v) and err (%v)", result, err)
	}

	if mcm.Spec.Version != DEFAULT_VERSION {
		t.Errorf("Version (%v) is not the expected (%v)", mcm.Spec.Version, DEFAULT_VERSION)
	}

	if mcm.Spec.ImageRepository != DEFAULT_IMG_REPO {
		t.Errorf("ImageRepository (%v) is not the expected (%v)", mcm.Spec.ImageRepository, DEFAULT_IMG_REPO)
	}

	if string(mcm.Spec.ImagePullPolicy) != string(corev1.PullAlways) {
		t.Errorf("ImagePullPolicy (%v) is not the expected (%v)", mcm.Spec.ImagePullPolicy, corev1.PullAlways)
	}

	if mcm.Spec.ImagePullSecret != DEFAULT_IMG_PULL_SECRET {
		t.Errorf("ImagePullSecret (%v) is not the expected (%v)", mcm.Spec.ImagePullSecret, DEFAULT_IMG_PULL_SECRET)
	}

	if mcm.Spec.NodeSelector == nil {
		t.Errorf("NodeSelector (%v) is not the expected (non-nil)", mcm.Spec.NodeSelector)
	}

	if mcm.Spec.StorageClass != DEFAULT_STORAGE_CLASS {
		t.Errorf("StorageClass (%v) is not the expected (%v)", mcm.Spec.StorageClass, DEFAULT_STORAGE_CLASS)
	}

	if mcm.Spec.Observatorium == nil {
		t.Errorf("Observatorium (%v) is not the expected (non-nil)", mcm.Spec.Observatorium)
	}

	if mcm.Spec.ObjectStorageConfigSpec == nil {
		t.Errorf("ObjectStorageConfigSpec (%v) is not the expected (non-nil)", mcm.Spec.ObjectStorageConfigSpec)
	}

	if mcm.Spec.Grafana == nil {
		t.Errorf("Grafana (%v) is not the expected (non-nil)", mcm.Spec.Grafana)
	}
}
