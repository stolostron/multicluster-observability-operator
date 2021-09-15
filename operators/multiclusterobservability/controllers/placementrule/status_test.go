// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"context"
	"testing"
	"time"

	mcov1beta1 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestUpdateAddonStatus(t *testing.T) {
	maddon := &addonv1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.ManagedClusterAddonName,
			Namespace: namespace,
		},
		Status: addonv1alpha1.ManagedClusterAddOnStatus{},
	}
	objs := []runtime.Object{maddon}
	c := fake.NewFakeClient(objs...)

	addonList := &mcov1beta1.ObservabilityAddonList{
		Items: []mcov1beta1.ObservabilityAddon{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      obsAddonName,
					Namespace: namespace,
				},
				Status: mcov1beta1.ObservabilityAddonStatus{
					Conditions: []mcov1beta1.StatusCondition{
						{
							Type:               "Available",
							Status:             metav1.ConditionTrue,
							LastTransitionTime: metav1.NewTime(time.Now()),
							Reason:             "Deployed",
							Message:            "Metrics collector deployed and functional",
						},
					},
				},
			},
		},
	}

	err := updateAddonStatus(c, *addonList)
	if err != nil {
		t.Fatalf("Failed to update status for managedclusteraddon: (%v)", err)
	}

	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      util.ManagedClusterAddonName,
		Namespace: namespace,
	}, maddon)
	if err != nil {
		t.Fatalf("Failed to get managedclusteraddon: (%v)", err)
	}
	if maddon.Status.Conditions == nil || len(maddon.Status.Conditions) != 1 {
		t.Fatalf("Status not updated correctly in managedclusteraddon: (%v)", maddon)
	}
}
