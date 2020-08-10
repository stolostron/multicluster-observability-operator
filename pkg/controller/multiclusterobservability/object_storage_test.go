// Copyright (c) 2020 Red Hat, Inc.

package multiclusterobservability

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
)

func TestGenerateObjectStorageSecret(t *testing.T) {
	mco := &mcov1beta1.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "test"},
		Spec:       mcov1beta1.MultiClusterObservabilitySpec{},
	}

	s := scheme.Scheme
	s.AddKnownTypes(mcov1beta1.SchemeGroupVersion, mco)
	objs := []runtime.Object{mco}
	c := fake.NewFakeClientWithScheme(s, objs...)
	err := GenerateObjectStorageSecret(c, mco)
	if err != nil {
		t.Errorf("Should return nil, err: %v", err)
	}

	err = GenerateObjectStorageSecret(c, mco)
	if err != nil {
		t.Errorf("Secret already exists, should return nil, err: %v", err)
	}
}
