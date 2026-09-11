// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package mcoa

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, clusterv1alpha1.AddToScheme(s))
	return s
}

func newCOOSubscription(name, namespace string, labels map[string]string) *unstructured.Unstructured {
	sub := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "operators.coreos.com/v1alpha1",
			"kind":       "Subscription",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]any{
				"name": cooSubscriptionName,
			},
		},
	}
	if labels != nil {
		sub.SetLabels(labels)
	}
	return sub
}

func getClaimValue(t *testing.T, c client.Client, name string) string {
	t.Helper()
	claim := &clusterv1alpha1.ClusterClaim{}
	err := c.Get(context.Background(), client.ObjectKey{Name: name}, claim)
	require.NoError(t, err)
	return claim.Spec.Value
}

func TestWriteCOOStatus(t *testing.T) {
	tests := []struct {
		name              string
		olmAvailable      bool
		objects           []client.Object
		expectedInstalled string
		expectedManagedBy string
		expectNoClaims    bool
	}{
		{
			name:           "OLM not available: no-op",
			olmAvailable:   false,
			expectNoClaims: true,
		},
		{
			name:              "no subscriptions: COO not installed",
			olmAvailable:      true,
			expectedInstalled: "false",
			expectedManagedBy: "none",
		},
		{
			name:         "COO installed by external party",
			olmAvailable: true,
			objects: []client.Object{
				newCOOSubscription("my-coo", "openshift-operators", nil),
			},
			expectedInstalled: "true",
			expectedManagedBy: "external",
		},
		{
			name:         "COO installed by MCOA",
			olmAvailable: true,
			objects: []client.Object{
				newCOOSubscription("coo", "openshift-operators", map[string]string{
					mcoaReleaseLabel: mcoaReleaseName,
				}),
			},
			expectedInstalled: "true",
			expectedManagedBy: "mcoa",
		},
		{
			name:         "unrelated subscription: COO not installed",
			olmAvailable: true,
			objects: []client.Object{
				func() *unstructured.Unstructured {
					sub := &unstructured.Unstructured{
						Object: map[string]any{
							"apiVersion": "operators.coreos.com/v1alpha1",
							"kind":       "Subscription",
							"metadata": map[string]any{
								"name":      "some-other-operator",
								"namespace": "openshift-operators",
							},
							"spec": map[string]any{
								"name": "some-other-operator",
							},
						},
					}
					return sub
				}(),
			},
			expectedInstalled: "false",
			expectedManagedBy: "none",
		},
		{
			name:         "updates existing claims",
			olmAvailable: true,
			objects: []client.Object{
				&clusterv1alpha1.ClusterClaim{
					ObjectMeta: metav1.ObjectMeta{Name: CooInstalledClaimName},
					Spec:       clusterv1alpha1.ClusterClaimSpec{Value: "true"},
				},
				&clusterv1alpha1.ClusterClaim{
					ObjectMeta: metav1.ObjectMeta{Name: CooManagedByClaimName},
					Spec:       clusterv1alpha1.ClusterClaimSpec{Value: "mcoa"},
				},
			},
			expectedInstalled: "false",
			expectedManagedBy: "none",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestScheme(t)
			c := fake.NewClientBuilder().WithScheme(s).WithObjects(tc.objects...).Build()

			r := &MCOAAgentReconciler{
				Client:       c,
				Log:          ctrl.Log.WithName("test"),
				OLMAvailable: tc.olmAvailable,
			}

			err := r.WriteCOOStatus(context.Background())
			require.NoError(t, err)

			if tc.expectNoClaims {
				claim := &clusterv1alpha1.ClusterClaim{}
				assert.Error(t, c.Get(context.Background(), client.ObjectKey{Name: CooInstalledClaimName}, claim))
				return
			}

			assert.Equal(t, tc.expectedInstalled, getClaimValue(t, c, CooInstalledClaimName))
			assert.Equal(t, tc.expectedManagedBy, getClaimValue(t, c, CooManagedByClaimName))
		})
	}
}

func TestGetCOOSubscriptionStatus(t *testing.T) {
	tests := []struct {
		name              string
		objects           []client.Object
		expectedInstalled string
		expectedManagedBy string
	}{
		{
			name:              "no subscriptions",
			expectedInstalled: "false",
			expectedManagedBy: "",
		},
		{
			name: "COO subscription without MCOA label",
			objects: []client.Object{
				newCOOSubscription("coo", "openshift-operators", nil),
			},
			expectedInstalled: "true",
			expectedManagedBy: "external",
		},
		{
			name: "COO subscription with MCOA label",
			objects: []client.Object{
				newCOOSubscription("coo", "openshift-operators", map[string]string{
					mcoaReleaseLabel: mcoaReleaseName,
				}),
			},
			expectedInstalled: "true",
			expectedManagedBy: "mcoa",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestScheme(t)
			c := fake.NewClientBuilder().WithScheme(s).WithObjects(tc.objects...).Build()

			installed, managedBy, err := getCOOSubscriptionStatus(context.Background(), c)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedInstalled, installed)
			assert.Equal(t, tc.expectedManagedBy, managedBy)
		})
	}
}
