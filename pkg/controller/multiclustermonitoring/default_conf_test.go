// Copyright (c) 2020 Red Hat, Inc.

package multiclustermonitoring

import (
	"testing"

	observatoriumv1alpha1 "github.com/observatorium/configuration/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	monitoringv1alpha1 "github.com/open-cluster-management/multicluster-observability-operator/pkg/apis/monitoring/v1alpha1"
)

func NewFakeClient(mcm *monitoringv1alpha1.MultiClusterMonitoring,
	obs *observatoriumv1alpha1.Observatorium) client.Client {
	s := scheme.Scheme
	s.AddKnownTypes(monitoringv1alpha1.SchemeGroupVersion, mcm)
	s.AddKnownTypes(observatoriumv1alpha1.GroupVersion, obs)
	objs := []runtime.Object{mcm, obs}
	return fake.NewFakeClientWithScheme(s, objs...)
}

func TestGenerateMonitoringEmptyCR(t *testing.T) {
	mcm := &monitoringv1alpha1.MultiClusterMonitoring{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterMonitoring"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "test"},
		Spec:       monitoringv1alpha1.MultiClusterMonitoringSpec{},
	}

	result, err := GenerateMonitoringCR(NewFakeClient(mcm, &observatoriumv1alpha1.Observatorium{}), mcm)
	if result != nil || err != nil {
		t.Errorf("Should return nil for result (%v) and err (%v)", result, err)
	}

	if mcm.Spec.Version != defaultVersion {
		t.Errorf("Version (%v) is not the expected (%v)", mcm.Spec.Version, defaultVersion)
	}

	if mcm.Spec.ImageRepository != defaultImgRepo {
		t.Errorf("ImageRepository (%v) is not the expected (%v)", mcm.Spec.ImageRepository, defaultImgRepo)
	}

	if string(mcm.Spec.ImagePullPolicy) != string(corev1.PullAlways) {
		t.Errorf("ImagePullPolicy (%v) is not the expected (%v)", mcm.Spec.ImagePullPolicy, corev1.PullAlways)
	}

	if mcm.Spec.ImagePullSecret != defaultImgPullSecret {
		t.Errorf("ImagePullSecret (%v) is not the expected (%v)", mcm.Spec.ImagePullSecret, defaultImgPullSecret)
	}

	if mcm.Spec.NodeSelector == nil {
		t.Errorf("NodeSelector (%v) is not the expected (non-nil)", mcm.Spec.NodeSelector)
	}

	if mcm.Spec.StorageClass != defaultStorageClass {
		t.Errorf("StorageClass (%v) is not the expected (%v)", mcm.Spec.StorageClass, defaultStorageClass)
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

func TestGenerateMonitoringCustomizedCR(t *testing.T) {
	retention := "20d"
	mcm := &monitoringv1alpha1.MultiClusterMonitoring{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterMonitoring"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "test"},
		Spec: monitoringv1alpha1.MultiClusterMonitoringSpec{
			Observatorium: &observatoriumv1alpha1.ObservatoriumSpec{
				Compact: observatoriumv1alpha1.CompactSpec{
					RetentionResolutionRaw: retention,
				},
			},
		},
	}
	fakeClient := NewFakeClient(mcm, &observatoriumv1alpha1.Observatorium{})
	result, err := GenerateMonitoringCR(fakeClient, mcm)
	if result != nil || err != nil {
		t.Fatalf("Should return nil for result (%v) and err (%v)", result, err)
	}

	if mcm.Spec.Version != defaultVersion {
		t.Errorf("Version (%v) is not the expected (%v)", mcm.Spec.Version, defaultVersion)
	}

	if mcm.Spec.ImageRepository != defaultImgRepo {
		t.Errorf("ImageRepository (%v) is not the expected (%v)",
			mcm.Spec.ImageRepository, defaultImgRepo)
	}

	if string(mcm.Spec.ImagePullPolicy) != string(corev1.PullAlways) {
		t.Errorf("ImagePullPolicy (%v) is not the expected (%v)",
			mcm.Spec.ImagePullPolicy, corev1.PullAlways)
	}

	if mcm.Spec.ImagePullSecret != defaultImgPullSecret {
		t.Errorf("ImagePullSecret (%v) is not the expected (%v)",
			mcm.Spec.ImagePullSecret, defaultImgPullSecret)
	}

	if mcm.Spec.NodeSelector == nil {
		t.Errorf("NodeSelector (%v) is not the expected (non-nil)", mcm.Spec.NodeSelector)
	}

	if mcm.Spec.StorageClass != defaultStorageClass {
		t.Errorf("StorageClass (%v) is not the expected (%v)",
			mcm.Spec.StorageClass, defaultStorageClass)
	}

	if mcm.Spec.Observatorium == nil {
		t.Errorf("Observatorium (%v) is not the expected (non-nil)", mcm.Spec.Observatorium)
	} else {
		if mcm.Spec.Observatorium.Compact.RetentionResolutionRaw != retention {
			t.Errorf("RetentionResolutionRaw (%v) is not the expected (%v)",
				mcm.Spec.Observatorium.Compact.RetentionResolutionRaw, retention)
		}
	}

	if mcm.Spec.ObjectStorageConfigSpec == nil {
		t.Errorf("ObjectStorageConfigSpec (%v) is not the expected (non-nil)",
			mcm.Spec.ObjectStorageConfigSpec)
	}

	if mcm.Spec.Grafana == nil {
		t.Errorf("Grafana (%v) is not the expected (non-nil)", mcm.Spec.Grafana)
	}

}
