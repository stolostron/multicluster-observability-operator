// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package dependencies

import (
	"testing"

	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBuildCertManagerOperatorResources(t *testing.T) {
	objs := BuildCertManagerOperatorResources()
	require.Len(t, objs, 3)

	ns, ok := objs[0].(*corev1.Namespace)
	require.True(t, ok, "first object should be a Namespace")
	require.Equal(t, mcoconfig.CertManagerOperatorNamespace, ns.GetName())

	og, ok := objs[1].(*unstructured.Unstructured)
	require.True(t, ok, "second object should be an unstructured OperatorGroup")
	require.Equal(t, "OperatorGroup", og.GetKind())
	require.Equal(t, mcoconfig.CertManagerOperatorPackageName, og.GetName())
	require.Equal(t, mcoconfig.CertManagerOperatorNamespace, og.GetNamespace())

	// No spec.targetNamespaces/spec.selector: global OperatorGroup (AllNamespaces mode).
	_, found, err := unstructured.NestedStringSlice(og.Object, "spec", "targetNamespaces")
	require.NoError(t, err)
	require.False(t, found)

	sub, ok := objs[2].(*unstructured.Unstructured)
	require.True(t, ok, "third object should be an unstructured Subscription")
	require.Equal(t, "Subscription", sub.GetKind())
	require.Equal(t, mcoconfig.CertManagerOperatorPackageName, sub.GetName())
	require.Equal(t, mcoconfig.CertManagerOperatorNamespace, sub.GetNamespace())

	// channel is deliberately left unset so OLM tracks the package's default channel; see the
	// comment in BuildCertManagerOperatorResources for why.
	_, found, err = unstructured.NestedString(sub.Object, "spec", "channel")
	require.NoError(t, err)
	require.False(t, found)

	name, found, err := unstructured.NestedString(sub.Object, "spec", "name")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, mcoconfig.CertManagerOperatorPackageName, name)
}

func TestEnsureCertManagerOperatorInstalled(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	require.NoError(t, EnsureCertManagerOperatorInstalled(t.Context(), fakeClient))
	require.NoError(t, fakeClient.Get(t.Context(), types.NamespacedName{Name: mcoconfig.CertManagerOperatorNamespace}, &corev1.Namespace{}))

	// Calling it again should be a no-op (idempotent), not fail with AlreadyExists.
	require.NoError(t, EnsureCertManagerOperatorInstalled(t.Context(), fakeClient))
}

func TestEnsureCertManagerOperatorInstalled_PropagatesUnexpectedErrors(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	// erroringClient is defined in loki_operator_test.go (same package) and forces every
	// Create call to fail with a non-AlreadyExists error.
	fakeClient := &erroringClient{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}

	err := EnsureCertManagerOperatorInstalled(t.Context(), fakeClient)
	require.Error(t, err)
}
