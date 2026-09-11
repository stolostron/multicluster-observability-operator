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
		name           string
		olmAvailable   bool
		objects        []client.Object
		expectedStatus string
		expectNoClaim  bool
	}{
		{
			name:          "OLM not available: no-op",
			olmAvailable:  false,
			expectNoClaim: true,
		},
		{
			name:           "no subscriptions: not installed",
			olmAvailable:   true,
			expectedStatus: CooStatusNotInstalled,
		},
		{
			name:         "COO installed by external party",
			olmAvailable: true,
			objects: []client.Object{
				newCOOSubscription("my-coo", "openshift-operators", nil),
			},
			expectedStatus: CooStatusExternal,
		},
		{
			name:         "COO installed by MCOA",
			olmAvailable: true,
			objects: []client.Object{
				newCOOSubscription("coo", "openshift-operators", map[string]string{
					mcoaReleaseLabel: mcoaReleaseName,
				}),
			},
			expectedStatus: CooStatusMCOA,
		},
		{
			name:         "unrelated subscription: not installed",
			olmAvailable: true,
			objects: []client.Object{
				func() *unstructured.Unstructured {
					return &unstructured.Unstructured{
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
				}(),
			},
			expectedStatus: CooStatusNotInstalled,
		},
		{
			name:         "updates existing claim",
			olmAvailable: true,
			objects: []client.Object{
				&clusterv1alpha1.ClusterClaim{
					ObjectMeta: metav1.ObjectMeta{Name: CooStatusClaimName},
					Spec:       clusterv1alpha1.ClusterClaimSpec{Value: CooStatusMCOA},
				},
			},
			expectedStatus: CooStatusNotInstalled,
		},
		{
			name:         "multiple subscriptions with one external: external wins",
			olmAvailable: true,
			objects: []client.Object{
				newCOOSubscription("coo-mcoa", "openshift-operators", map[string]string{
					mcoaReleaseLabel: mcoaReleaseName,
				}),
				newCOOSubscription("coo-admin", "other-namespace", nil),
			},
			expectedStatus: CooStatusExternal,
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

			if tc.expectNoClaim {
				claim := &clusterv1alpha1.ClusterClaim{}
				assert.Error(t, c.Get(context.Background(), client.ObjectKey{Name: CooStatusClaimName}, claim))
				return
			}

			assert.Equal(t, tc.expectedStatus, getClaimValue(t, c, CooStatusClaimName))
		})
	}
}

func TestGetCOOSubscriptionStatus(t *testing.T) {
	tests := []struct {
		name           string
		objects        []client.Object
		expectedStatus string
	}{
		{
			name:           "no subscriptions",
			expectedStatus: CooStatusNotInstalled,
		},
		{
			name: "COO subscription without MCOA label",
			objects: []client.Object{
				newCOOSubscription("coo", "openshift-operators", nil),
			},
			expectedStatus: CooStatusExternal,
		},
		{
			name: "COO subscription with MCOA label",
			objects: []client.Object{
				newCOOSubscription("coo", "openshift-operators", map[string]string{
					mcoaReleaseLabel: mcoaReleaseName,
				}),
			},
			expectedStatus: CooStatusMCOA,
		},
		{
			name: "multiple COO subscriptions all MCOA",
			objects: []client.Object{
				newCOOSubscription("coo-1", "ns-1", map[string]string{mcoaReleaseLabel: mcoaReleaseName}),
				newCOOSubscription("coo-2", "ns-2", map[string]string{mcoaReleaseLabel: mcoaReleaseName}),
			},
			expectedStatus: CooStatusMCOA,
		},
		{
			name: "multiple COO subscriptions one external",
			objects: []client.Object{
				newCOOSubscription("coo-mcoa", "ns-1", map[string]string{mcoaReleaseLabel: mcoaReleaseName}),
				newCOOSubscription("coo-admin", "ns-2", nil),
			},
			expectedStatus: CooStatusExternal,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestScheme(t)
			c := fake.NewClientBuilder().WithScheme(s).WithObjects(tc.objects...).Build()

			status, err := getCOOSubscriptionStatus(context.Background(), c)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedStatus, status)
		})
	}
}
