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

	monitoringv1alpha1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/monitoring/v1alpha1"
)

func NewFakeClient(mco *monitoringv1alpha1.MultiClusterObservability,
	obs *observatoriumv1alpha1.Observatorium) client.Client {
	s := scheme.Scheme
	s.AddKnownTypes(monitoringv1alpha1.SchemeGroupVersion, mco)
	s.AddKnownTypes(observatoriumv1alpha1.GroupVersion, obs)
	objs := []runtime.Object{mco, obs}
	return fake.NewFakeClientWithScheme(s, objs...)
}

func TestGenerateMonitoringEmptyCR(t *testing.T) {
	mco := &monitoringv1alpha1.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "test"},
		Spec:       monitoringv1alpha1.MultiClusterMonitoringSpec{},
	}

	result, err := GenerateMonitoringCR(NewFakeClient(mco, &observatoriumv1alpha1.Observatorium{}), mco)
	if result != nil || err != nil {
		t.Errorf("Should return nil for result (%v) and err (%v)", result, err)
	}

	if mco.Spec.Version != defaultVersion {
		t.Errorf("Version (%v) is not the expected (%v)", mco.Spec.Version, defaultVersion)
	}

	if mco.Spec.ImageRepository != defaultImgRepo {
		t.Errorf("ImageRepository (%v) is not the expected (%v)", mco.Spec.ImageRepository, defaultImgRepo)
	}

	if string(mco.Spec.ImagePullPolicy) != string(corev1.PullAlways) {
		t.Errorf("ImagePullPolicy (%v) is not the expected (%v)", mco.Spec.ImagePullPolicy, corev1.PullAlways)
	}

	if mco.Spec.ImagePullSecret != defaultImgPullSecret {
		t.Errorf("ImagePullSecret (%v) is not the expected (%v)", mco.Spec.ImagePullSecret, defaultImgPullSecret)
	}

	if mco.Spec.NodeSelector == nil {
		t.Errorf("NodeSelector (%v) is not the expected (non-nil)", mco.Spec.NodeSelector)
	}

	if mco.Spec.StorageClass != defaultStorageClass {
		t.Errorf("StorageClass (%v) is not the expected (%v)", mco.Spec.StorageClass, defaultStorageClass)
	}

	if mco.Spec.Observatorium == nil {
		t.Errorf("Observatorium (%v) is not the expected (non-nil)", mco.Spec.Observatorium)
	}

	if mco.Spec.ObjectStorageConfigSpec == nil {
		t.Errorf("ObjectStorageConfigSpec (%v) is not the expected (non-nil)", mco.Spec.ObjectStorageConfigSpec)
	}

	if mco.Spec.Grafana == nil {
		t.Errorf("Grafana (%v) is not the expected (non-nil)", mco.Spec.Grafana)
	}
}

func TestGenerateMonitoringCustomizedCR(t *testing.T) {
	retention := "20d"
	mco := &monitoringv1alpha1.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "test"},
		Spec: monitoringv1alpha1.MultiClusterMonitoringSpec{
			Observatorium: &observatoriumv1alpha1.ObservatoriumSpec{
				Compact: observatoriumv1alpha1.CompactSpec{
					RetentionResolutionRaw: retention,
				},
			},
		},
	}
	fakeClient := NewFakeClient(mco, &observatoriumv1alpha1.Observatorium{})
	result, err := GenerateMonitoringCR(fakeClient, mco)
	if result != nil || err != nil {
		t.Fatalf("Should return nil for result (%v) and err (%v)", result, err)
	}

	if mco.Spec.Version != defaultVersion {
		t.Errorf("Version (%v) is not the expected (%v)", mco.Spec.Version, defaultVersion)
	}

	if mco.Spec.ImageRepository != defaultImgRepo {
		t.Errorf("ImageRepository (%v) is not the expected (%v)",
			mco.Spec.ImageRepository, defaultImgRepo)
	}

	if string(mco.Spec.ImagePullPolicy) != string(corev1.PullAlways) {
		t.Errorf("ImagePullPolicy (%v) is not the expected (%v)",
			mco.Spec.ImagePullPolicy, corev1.PullAlways)
	}

	if mco.Spec.ImagePullSecret != defaultImgPullSecret {
		t.Errorf("ImagePullSecret (%v) is not the expected (%v)",
			mco.Spec.ImagePullSecret, defaultImgPullSecret)
	}

	if mco.Spec.NodeSelector == nil {
		t.Errorf("NodeSelector (%v) is not the expected (non-nil)", mco.Spec.NodeSelector)
	}

	if mco.Spec.StorageClass != defaultStorageClass {
		t.Errorf("StorageClass (%v) is not the expected (%v)",
			mco.Spec.StorageClass, defaultStorageClass)
	}

	if mco.Spec.Observatorium == nil {
		t.Errorf("Observatorium (%v) is not the expected (non-nil)", mco.Spec.Observatorium)
	} else {
		if mco.Spec.Observatorium.Compact.RetentionResolutionRaw != retention {
			t.Errorf("RetentionResolutionRaw (%v) is not the expected (%v)",
				mco.Spec.Observatorium.Compact.RetentionResolutionRaw, retention)
		}
	}

	if mco.Spec.ObjectStorageConfigSpec == nil {
		t.Errorf("ObjectStorageConfigSpec (%v) is not the expected (non-nil)",
			mco.Spec.ObjectStorageConfigSpec)
	}

	if mco.Spec.Grafana == nil {
		t.Errorf("Grafana (%v) is not the expected (non-nil)", mco.Spec.Grafana)
	}

}
