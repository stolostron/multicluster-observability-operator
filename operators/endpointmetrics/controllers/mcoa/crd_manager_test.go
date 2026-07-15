// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package mcoa

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestIsManagedCRDName(t *testing.T) {
	t.Parallel()

	expected := []string{
		"podmonitors.monitoring.rhobs",
		"probes.monitoring.rhobs",
		"prometheusagents.monitoring.rhobs",
		"prometheuses.monitoring.rhobs",
		"prometheusrules.monitoring.rhobs",
		"scrapeconfigs.monitoring.rhobs",
		"servicemonitors.monitoring.rhobs",
	}
	actual := GetManagedCRDNames()
	slices.Sort(expected)
	slices.Sort(actual)
	assert.Equal(t, expected, actual, "GetManagedCRDNames must return exactly the expected set of managed CRDs")

	for _, name := range actual {
		assert.True(t, isManagedCRDName(name), "expected %q to be a managed CRD name", name)
	}

	notManaged := []string{"", "podmonitors.monitoring.coreos.com", "unknown", "prometheusagents"}
	for _, name := range notManaged {
		assert.False(t, isManagedCRDName(name), "expected %q not to be a managed CRD name", name)
	}
}

func TestDeployAndCleanUpCRDs(t *testing.T) {
	scheme := runtime.NewScheme()
	expectedCRDs := GetManagedCRDNames()

	crdGVK := schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Version: "v1",
		Kind:    "CustomResourceDefinition",
	}

	t.Run("Deploy CRDs when they do not exist", func(t *testing.T) {
		t.Parallel()
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		ctx := context.Background()

		err := DeployCRDs(ctx, cl)
		assert.NoError(t, err)

		for _, crdName := range expectedCRDs {
			crd := &unstructured.Unstructured{}
			crd.SetGroupVersionKind(crdGVK)
			err := cl.Get(ctx, types.NamespacedName{Name: crdName}, crd)
			assert.NoError(t, err)
			assert.True(t, isManagedByUs(crd))
		}
	})

	t.Run("Deploy CRDs when they already exist but managed by someone else", func(t *testing.T) {
		t.Parallel()
		// Populate client with a CRD having an outside label
		existingCRD := &unstructured.Unstructured{}
		existingCRD.SetGroupVersionKind(crdGVK)
		existingCRD.SetName("podmonitors.monitoring.rhobs")
		existingCRD.SetLabels(map[string]string{ManagedByLabelKey: "something-else"})

		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingCRD).Build()
		ctx := context.Background()

		err := DeployCRDs(ctx, cl)
		assert.NoError(t, err)

		// Check that the label remains unchanged (we skipped it)
		crd := &unstructured.Unstructured{}
		crd.SetGroupVersionKind(crdGVK)
		err = cl.Get(ctx, types.NamespacedName{Name: "podmonitors.monitoring.rhobs"}, crd)
		assert.NoError(t, err)
		assert.Equal(t, "something-else", crd.GetLabels()[ManagedByLabelKey])
	})

	t.Run("Upgrade existing managed CRD on subsequent deployments", func(t *testing.T) {
		t.Parallel()
		// Populate client with a CRD we manage
		existingCRD := &unstructured.Unstructured{}
		existingCRD.SetGroupVersionKind(crdGVK)
		existingCRD.SetName("probes.monitoring.rhobs")
		existingCRD.SetLabels(map[string]string{ManagedByLabelKey: ManagedByLabelValue})
		existingCRD.SetAnnotations(map[string]string{"test-drift": "drifted"})

		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingCRD).Build()
		ctx := context.Background()

		err := DeployCRDs(ctx, cl)
		assert.NoError(t, err)

		// Check that the CRD is successfully applied and preserved our ownership
		crd := &unstructured.Unstructured{}
		crd.SetGroupVersionKind(crdGVK)
		err = cl.Get(ctx, types.NamespacedName{Name: "probes.monitoring.rhobs"}, crd)
		assert.NoError(t, err)
		assert.True(t, isManagedByUs(crd))
	})

	t.Run("Cleanup deletes only the CRDs managed by us", func(t *testing.T) {
		t.Parallel()
		// Populate fake client: one managed, one non-managed
		managedCRD := &unstructured.Unstructured{}
		managedCRD.SetGroupVersionKind(crdGVK)
		managedCRD.SetName("probes.monitoring.rhobs")
		managedCRD.SetLabels(map[string]string{ManagedByLabelKey: ManagedByLabelValue})

		otherCRD := &unstructured.Unstructured{}
		otherCRD.SetGroupVersionKind(crdGVK)
		otherCRD.SetName("podmonitors.monitoring.rhobs")
		otherCRD.SetLabels(map[string]string{ManagedByLabelKey: "other-operator"})

		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(managedCRD, otherCRD).Build()
		ctx := context.Background()

		err := CleanUpCRDs(ctx, cl)
		assert.NoError(t, err)

		// Managed CRD should be deleted
		crd := &unstructured.Unstructured{}
		crd.SetGroupVersionKind(crdGVK)
		err = cl.Get(ctx, types.NamespacedName{Name: "probes.monitoring.rhobs"}, crd)
		assert.Error(t, err)

		// Other CRD should still exist
		err = cl.Get(ctx, types.NamespacedName{Name: "podmonitors.monitoring.rhobs"}, crd)
		assert.NoError(t, err)
	})

	t.Run("DeployCRDs handles Get error gracefully", func(t *testing.T) {
		t.Parallel()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, clientww client.WithWatch, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
				return fmt.Errorf("injected get error")
			},
		}).Build()

		err := DeployCRDs(context.Background(), cl)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to check existence of CRD")
	})

	t.Run("DeployCRDs handles Apply error on initial create", func(t *testing.T) {
		t.Parallel()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
			Apply: func(ctx context.Context, clientww client.WithWatch, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
				return fmt.Errorf("injected create apply error")
			},
		}).Build()

		err := DeployCRDs(context.Background(), cl)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to server-side apply CRD")
	})

	t.Run("DeployCRDs handles Apply error on managed upgrade", func(t *testing.T) {
		t.Parallel()
		existingCRD := &unstructured.Unstructured{}
		existingCRD.SetGroupVersionKind(crdGVK)
		existingCRD.SetName("probes.monitoring.rhobs")
		existingCRD.SetLabels(map[string]string{ManagedByLabelKey: ManagedByLabelValue})

		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingCRD).WithInterceptorFuncs(interceptor.Funcs{
			Apply: func(ctx context.Context, clientww client.WithWatch, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
				type hasName interface{ GetName() string }
				if u, ok := obj.(hasName); ok && u.GetName() == "probes.monitoring.rhobs" {
					return fmt.Errorf("injected upgrade apply error")
				}
				return clientww.Apply(ctx, obj, opts...)
			},
		}).Build()

		err := DeployCRDs(context.Background(), cl)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update managed CRD")
	})

	t.Run("CleanUpCRDs handles Get error", func(t *testing.T) {
		t.Parallel()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, clientww client.WithWatch, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
				return fmt.Errorf("injected cleanup get error")
			},
		}).Build()

		err := CleanUpCRDs(context.Background(), cl)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to fetch CRD")
	})

	t.Run("CleanUpCRDs handles Delete error", func(t *testing.T) {
		t.Parallel()
		managedCRD := &unstructured.Unstructured{}
		managedCRD.SetGroupVersionKind(crdGVK)
		managedCRD.SetName("probes.monitoring.rhobs")
		managedCRD.SetLabels(map[string]string{ManagedByLabelKey: ManagedByLabelValue})

		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(managedCRD).WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(ctx context.Context, clientww client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				return fmt.Errorf("injected cleanup delete error")
			},
		}).Build()

		err := CleanUpCRDs(context.Background(), cl)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete managed CRD")
	})
}
