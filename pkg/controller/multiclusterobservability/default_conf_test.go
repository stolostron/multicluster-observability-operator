// Copyright (c) 2020 Red Hat, Inc.

package multiclusterobservability

import (
	"testing"

	observatoriumv1alpha1 "github.com/observatorium/deployments/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
)

func NewFakeClient(mco *mcov1beta1.MultiClusterObservability,
	obs *observatoriumv1alpha1.Observatorium) client.Client {
	s := scheme.Scheme
	s.AddKnownTypes(mcov1beta1.SchemeGroupVersion, mco)
	s.AddKnownTypes(observatoriumv1alpha1.GroupVersion, obs)
	objs := []runtime.Object{mco, obs}
	return fake.NewFakeClientWithScheme(s, objs...)
}

func TestGenerateMonitoringEmptyCR(t *testing.T) {
	mco := &mcov1beta1.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "test"},
		Spec:       mcov1beta1.MultiClusterObservabilitySpec{},
	}

	result, err := GenerateMonitoringCR(NewFakeClient(mco, &observatoriumv1alpha1.Observatorium{}), mco)
	if result != nil || err != nil {
		t.Errorf("Should return nil for result (%v) and err (%v)", result, err)
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
}

func TestGenerateMonitoringCustomizedCR(t *testing.T) {
	mco := &mcov1beta1.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "test"},
		Spec:       mcov1beta1.MultiClusterObservabilitySpec{},
	}
	fakeClient := NewFakeClient(mco, &observatoriumv1alpha1.Observatorium{})
	result, err := GenerateMonitoringCR(fakeClient, mco)
	if result != nil || err != nil {
		t.Fatalf("Should return nil for result (%v) and err (%v)", result, err)
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

}
