// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package status

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/status"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
)

func init() {
	os.Setenv("UNIT_TEST", "true")
	s := scheme.Scheme
	_ = oav1beta1.AddToScheme(s)
}

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

			s, err := New(log.NewLogfmtLogger(os.Stdout), false, tc.isUwl)
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
