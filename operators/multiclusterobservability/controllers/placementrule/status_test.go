// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"context"
	"slices"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcov1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestUpdateAddonStatus(t *testing.T) {
	testCases := map[string]struct {
		currentObsAddonConditions      []mcov1beta1.StatusCondition
		currentClusterAddonConditions  []metav1.Condition
		expectedClusterAddonConditions []metav1.Condition
		isUpdated                      bool
	}{
		"updated addon conditions should be applied": {
			currentObsAddonConditions: []mcov1beta1.StatusCondition{
				{
					Type:               "Available",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.NewTime(time.Now()),
					Reason:             "Deployed",
					Message:            "It is deployed",
				},
			},
			currentClusterAddonConditions: []metav1.Condition{},
			expectedClusterAddonConditions: []metav1.Condition{
				{
					Type:               "Available",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.NewTime(time.Now()),
					Reason:             "Deployed",
					Message:            "It is deployed",
				},
			},
			isUpdated: true,
		},
		"same conditions should not be updated": {
			currentObsAddonConditions: []mcov1beta1.StatusCondition{
				{
					Type:               "Available",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.NewTime(time.Unix(1e9, 0)),
					Reason:             "Deployed",
					Message:            "It is deployed",
				},
			},
			currentClusterAddonConditions: []metav1.Condition{
				{
					Type:               "Available",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.NewTime(time.Unix(1e9, 0)),
					Reason:             "Deployed",
					Message:            "It is deployed",
				},
			},
			expectedClusterAddonConditions: []metav1.Condition{
				{
					Type:               "Available",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.NewTime(time.Unix(1e9, 0)),
					Reason:             "Deployed",
					Message:            "It is deployed",
				},
			},
			isUpdated: false,
		},
	}

	sortConditionsFunc := func(a, b metav1.Condition) int {
		if a.Type < b.Type {
			return -1
		}
		if a.Type > b.Type {
			return 1
		}
		return 0
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			addonv1alpha1.AddToScheme(scheme)
			mcov1beta1.AddToScheme(scheme)
			mcov1beta2.AddToScheme(scheme)

			clusterAddon := &addonv1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.ManagedClusterAddonName,
					Namespace: namespace,
				},
				Status: addonv1alpha1.ManagedClusterAddOnStatus{
					Conditions: tc.currentClusterAddonConditions,
				},
			}

			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(clusterAddon).
				WithStatusSubresource(
					&addonv1alpha1.ManagedClusterAddOn{},
					&mcov1beta2.MultiClusterObservability{},
					&mcov1beta1.ObservabilityAddon{},
				).
				Build()

			addonList := mcov1beta1.ObservabilityAddonList{
				Items: []mcov1beta1.ObservabilityAddon{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      obsAddonName,
							Namespace: namespace,
						},
						Status: mcov1beta1.ObservabilityAddonStatus{
							Conditions: tc.currentObsAddonConditions,
						},
					},
				},
			}

			foundClusterAddon := &addonv1alpha1.ManagedClusterAddOn{}
			if err := c.Get(context.Background(), types.NamespacedName{
				Name:      util.ManagedClusterAddonName,
				Namespace: namespace,
			}, foundClusterAddon); err != nil {
				t.Fatalf("Failed to get managedclusteraddon: (%v)", err)
			}
			initVersion := foundClusterAddon.ResourceVersion

			err := updateAddonStatus(context.Background(), c, addonList)
			if err != nil {
				t.Fatalf("Failed to update status for managedclusteraddon: (%v)", err)
			}

			if err := c.Get(context.Background(), types.NamespacedName{
				Name:      util.ManagedClusterAddonName,
				Namespace: namespace,
			}, foundClusterAddon); err != nil {
				t.Fatalf("Failed to get managedclusteraddon: (%v)", err)
			}

			if tc.isUpdated {
				assert.NotEqual(t, initVersion, foundClusterAddon.ResourceVersion)
			} else {
				assert.Equal(t, initVersion, foundClusterAddon.ResourceVersion)
			}

			slices.SortFunc(foundClusterAddon.Status.Conditions, sortConditionsFunc)
			slices.SortFunc(tc.expectedClusterAddonConditions, sortConditionsFunc)
			assert.Equal(t, len(tc.expectedClusterAddonConditions), len(foundClusterAddon.Status.Conditions))
			for i := range tc.expectedClusterAddonConditions {
				assert.Equal(t, tc.expectedClusterAddonConditions[i].Type, foundClusterAddon.Status.Conditions[i].Type)
				assert.Equal(t, tc.expectedClusterAddonConditions[i].Status, foundClusterAddon.Status.Conditions[i].Status)
				assert.Equal(t, tc.expectedClusterAddonConditions[i].Reason, foundClusterAddon.Status.Conditions[i].Reason)
				assert.Equal(t, tc.expectedClusterAddonConditions[i].Message, foundClusterAddon.Status.Conditions[i].Message)
				assert.InEpsilon(t, tc.expectedClusterAddonConditions[i].LastTransitionTime.Unix(), foundClusterAddon.Status.Conditions[i].LastTransitionTime.Unix(), 1)
			}
		})
	}
}
