// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package status

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/status"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestUpdateStatus(t *testing.T) {
	testCases := map[string]struct {
		reason            status.Reason
		message           string
		isUwl             bool
		initialConditions []oav1beta1.StatusCondition
		expectedCondition oav1beta1.StatusCondition
	}{
		"new status should be appended": {
			reason:            status.ForwardSuccessful,
			message:           "Forwarding metrics successful",
			initialConditions: []oav1beta1.StatusCondition{},
			expectedCondition: oav1beta1.StatusCondition{
				Type:               string(status.MetricsCollector),
				Status:             metav1.ConditionTrue,
				Reason:             string(status.ForwardSuccessful),
				Message:            "Forwarding metrics successful",
				LastTransitionTime: metav1.NewTime(time.Now()),
			},
		},
		"existing status should be updated": {
			reason:  status.ForwardFailed,
			message: "Forwarding metrics failed",
			initialConditions: []oav1beta1.StatusCondition{
				{
					Type:               string(status.MetricsCollector),
					Status:             metav1.ConditionTrue,
					Reason:             string(status.ForwardSuccessful),
					Message:            "Forwarding metrics successful",
					LastTransitionTime: metav1.NewTime(time.Now().Add(-3 * time.Minute)),
				},
			},
			expectedCondition: oav1beta1.StatusCondition{
				Type:               string(status.MetricsCollector),
				Status:             metav1.ConditionTrue,
				Reason:             string(status.ForwardFailed),
				Message:            "Forwarding metrics failed",
				LastTransitionTime: metav1.NewTime(time.Now()),
			},
		},
		"same status should not be updated": {
			reason:  status.ForwardSuccessful,
			message: "Forwarding metrics successful",
			initialConditions: []oav1beta1.StatusCondition{
				{
					Type:               string(status.MetricsCollector),
					Status:             metav1.ConditionTrue,
					Reason:             string(status.ForwardSuccessful),
					Message:            "Forwarding metrics successful",
					LastTransitionTime: metav1.NewTime(time.Now().Add(-3 * time.Minute)),
				},
			},
			expectedCondition: oav1beta1.StatusCondition{
				Type:               string(status.MetricsCollector),
				Status:             metav1.ConditionTrue,
				Reason:             string(status.ForwardSuccessful),
				Message:            "Forwarding metrics successful",
				LastTransitionTime: metav1.NewTime(time.Now().Add(-3 * time.Minute)),
			},
		},
		"updateFailed to forward transition should not be allowed": {
			reason:  status.ForwardSuccessful,
			message: "Forwarding metrics successful",
			initialConditions: []oav1beta1.StatusCondition{
				{
					Type:               string(status.MetricsCollector),
					Status:             metav1.ConditionTrue,
					Reason:             string(status.UpdateFailed),
					Message:            "Update failed",
					LastTransitionTime: metav1.NewTime(time.Now().Add(-3 * time.Minute)),
				},
			},
			expectedCondition: oav1beta1.StatusCondition{
				Type:               string(status.MetricsCollector),
				Status:             metav1.ConditionTrue,
				Reason:             string(status.UpdateFailed),
				Message:            "Update failed",
				LastTransitionTime: metav1.NewTime(time.Now().Add(-3 * time.Minute)),
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			addon := &oav1beta1.ObservabilityAddon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      addonName,
					Namespace: addonNamespace,
				},
				Status: oav1beta1.ObservabilityAddonStatus{
					Conditions: tc.initialConditions,
				},
			}

			sc := scheme.Scheme
			if err := oav1beta1.AddToScheme(sc); err != nil {
				t.Fatal("failed to add observabilityaddon into scheme")
			}
			kubeClient := fake.NewClientBuilder().
				WithScheme(sc).
				WithStatusSubresource(&oav1beta1.ObservabilityAddon{}).
				Build()

			s, err := New(kubeClient, slog.New(slog.NewTextHandler(os.Stdout, nil)), false, tc.isUwl)
			if err != nil {
				t.Fatalf("Failed to create new Status struct: (%v)", err)
			}

			if err := s.statusClient.Create(context.Background(), addon); err != nil {
				t.Fatalf("Failed to create observabilityAddon: (%v)", err)
			}

			s.UpdateStatus(context.Background(), tc.reason, tc.message)

			foundAddon := &oav1beta1.ObservabilityAddon{}
			if err := s.statusClient.Get(context.Background(), types.NamespacedName{Name: addonName, Namespace: addonNamespace}, foundAddon); err != nil {
				t.Fatalf("Failed to get observabilityAddon: (%v)", err)
			}

			if len(foundAddon.Status.Conditions) == 0 {
				t.Fatalf("No conditions found in observabilityAddon")
			}

			condition := foundAddon.Status.Conditions[0]
			assert.Equal(t, tc.expectedCondition.Type, condition.Type)
			assert.Equal(t, tc.expectedCondition.Status, condition.Status)
			assert.Equal(t, tc.expectedCondition.Reason, condition.Reason)
			assert.Equal(t, tc.expectedCondition.Message, condition.Message)
			assert.InEpsilon(t, tc.expectedCondition.LastTransitionTime.Unix(), condition.LastTransitionTime.Unix(), 1)
		})
	}
}
