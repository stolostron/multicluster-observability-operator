// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package dependencies

import (
	"context"
	"testing"

	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBuildLokiOperatorResources(t *testing.T) {
	objs := BuildLokiOperatorResources()
	require.Len(t, objs, 3)

	ns, ok := objs[0].(*corev1.Namespace)
	require.True(t, ok, "first object should be a Namespace")
	require.Equal(t, mcoconfig.LokiOperatorNamespace, ns.GetName())

	og, ok := objs[1].(*unstructured.Unstructured)
	require.True(t, ok, "second object should be an unstructured OperatorGroup")
	require.Equal(t, "OperatorGroup", og.GetKind())
	require.Equal(t, mcoconfig.LokiOperatorPackageName, og.GetName())
	require.Equal(t, mcoconfig.LokiOperatorNamespace, og.GetNamespace())

	sub, ok := objs[2].(*unstructured.Unstructured)
	require.True(t, ok, "third object should be an unstructured Subscription")
	require.Equal(t, "Subscription", sub.GetKind())
	require.Equal(t, mcoconfig.LokiOperatorPackageName, sub.GetName())
	require.Equal(t, mcoconfig.LokiOperatorNamespace, sub.GetNamespace())

	channel, found, err := unstructured.NestedString(sub.Object, "spec", "channel")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, mcoconfig.LokiOperatorChannel, channel)
}

func TestEnsureLokiOperatorInstalled(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	require.NoError(t, EnsureLokiOperatorInstalled(t.Context(), fakeClient))
	require.NoError(t, fakeClient.Get(t.Context(), types.NamespacedName{Name: mcoconfig.LokiOperatorNamespace}, &corev1.Namespace{}))

	// Calling it again should be a no-op (idempotent), not fail with AlreadyExists.
	require.NoError(t, EnsureLokiOperatorInstalled(t.Context(), fakeClient))
}

func TestEnsureLokiOperatorInstalled_PropagatesUnexpectedErrors(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	fakeClient := &erroringClient{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}

	err := EnsureLokiOperatorInstalled(t.Context(), fakeClient)
	require.Error(t, err)
}

// erroringClient wraps a client.Client and forces every Create call to fail with a
// non-AlreadyExists error, so we can assert that EnsureLokiOperatorInstalled propagates it.
type erroringClient struct {
	client.Client
}

func (e *erroringClient) Create(_ context.Context, obj client.Object, _ ...client.CreateOption) error {
	return apierrors.NewBadRequest("boom for " + obj.GetObjectKind().GroupVersionKind().Kind)
}
