// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package multiclusterobservability

import (
	"reflect"
	"testing"
	"time"

	mcoshared "github.com/open-cluster-management/multicluster-observability-operator/api/shared"
	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta2"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFillupStatus(t *testing.T) {

	raw := `
conditions:
- message: Installation is in progress
  reason: Installing
  type: Installing
- message: Observability components are deployed and running
  reason: Ready
  type: Ready
`
	status := mcov1beta2.MultiClusterObservabilityStatus{}
	err := yaml.Unmarshal([]byte(raw), &status)
	if err != nil {
		t.Errorf("Failed to unmarshall MultiClusterObservabilityStatus %v", err)
	}
	newStatus := status.DeepCopy()
	fillupStatus(&newStatus.Conditions)
	for _, condition := range newStatus.Conditions {
		if condition.Status == "" {
			t.Fatal("Failed to fillup the status")
		}
		if condition.LastTransitionTime.IsZero() {
			t.Fatal("Failed to fillup the status")
		}
	}
}

func TestSetStatusCondition(t *testing.T) {
	oneHourBefore := time.Now().Add(-1 * time.Hour)
	oneHourAfter := time.Now().Add(1 * time.Hour)

	tests := []struct {
		name       string
		conditions []mcoshared.Condition
		toAdd      mcoshared.Condition
		expected   []mcoshared.Condition
	}{
		{
			name: "should-add",
			conditions: []mcoshared.Condition{
				{Type: "first"},
				{Type: "third"},
			},
			toAdd: mcoshared.Condition{Type: "second", Status: metav1.ConditionTrue, LastTransitionTime: metav1.Time{Time: oneHourBefore}, Reason: "reason", Message: "message"},
			expected: []mcoshared.Condition{
				{Type: "first"},
				{Type: "third"},
				{Type: "second", Status: metav1.ConditionTrue, LastTransitionTime: metav1.Time{Time: oneHourBefore}, Reason: "reason", Message: "message"},
			},
		},
		{
			name: "use-supplied-time",
			conditions: []mcoshared.Condition{
				{Type: "first"},
				{Type: "second", Status: metav1.ConditionFalse},
				{Type: "third"},
			},
			toAdd: mcoshared.Condition{Type: "second", Status: metav1.ConditionTrue, LastTransitionTime: metav1.Time{Time: oneHourBefore}, Reason: "reason", Message: "message"},
			expected: []mcoshared.Condition{
				{Type: "first"},
				{Type: "second", Status: metav1.ConditionTrue, LastTransitionTime: metav1.Time{Time: oneHourBefore}, Reason: "reason", Message: "message"},
				{Type: "third"},
			},
		},
		{
			name: "update-fields",
			conditions: []mcoshared.Condition{
				{Type: "first"},
				{Type: "second", Status: metav1.ConditionTrue, LastTransitionTime: metav1.Time{Time: oneHourBefore}},
				{Type: "third"},
			},
			toAdd: mcoshared.Condition{Type: "second", Status: metav1.ConditionTrue, LastTransitionTime: metav1.Time{Time: oneHourAfter}, Reason: "reason", Message: "message"},
			expected: []mcoshared.Condition{
				{Type: "first"},
				{Type: "second", Status: metav1.ConditionTrue, LastTransitionTime: metav1.Time{Time: oneHourBefore}, Reason: "reason", Message: "message"},
				{Type: "third"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setStatusCondition(&test.conditions, test.toAdd)
			if !reflect.DeepEqual(test.conditions, test.expected) {
				t.Error(test.conditions)
			}
		})
	}
}

func TestRemoveStatusCondition(t *testing.T) {
	tests := []struct {
		name          string
		conditions    []mcoshared.Condition
		conditionType string
		expected      []mcoshared.Condition
	}{
		{
			name: "present",
			conditions: []mcoshared.Condition{
				{Type: "first"},
				{Type: "second"},
				{Type: "third"},
			},
			conditionType: "second",
			expected: []mcoshared.Condition{
				{Type: "first"},
				{Type: "third"},
			},
		},
		{
			name: "not-present",
			conditions: []mcoshared.Condition{
				{Type: "first"},
				{Type: "second"},
				{Type: "third"},
			},
			conditionType: "fourth",
			expected: []mcoshared.Condition{
				{Type: "first"},
				{Type: "second"},
				{Type: "third"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			removeStatusCondition(&test.conditions, test.conditionType)
			if !reflect.DeepEqual(test.conditions, test.expected) {
				t.Error(test.conditions)
			}
		})
	}
}

func TestFindStatusCondition(t *testing.T) {
	tests := []struct {
		name          string
		conditions    []mcoshared.Condition
		conditionType string
		expected      *mcoshared.Condition
	}{
		{
			name: "not-present",
			conditions: []mcoshared.Condition{
				{Type: "first"},
			},
			conditionType: "second",
			expected:      nil,
		},
		{
			name: "present",
			conditions: []mcoshared.Condition{
				{Type: "first"},
				{Type: "second"},
			},
			conditionType: "second",
			expected:      &mcoshared.Condition{Type: "second"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := findStatusCondition(test.conditions, test.conditionType)
			if !reflect.DeepEqual(actual, test.expected) {
				t.Error(actual)
			}
		})
	}
}
