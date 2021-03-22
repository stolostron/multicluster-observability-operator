// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package multiclusterobservability

import (
	"reflect"
	"testing"
	"time"

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
		conditions []mcov1beta2.Condition
		toAdd      mcov1beta2.Condition
		expected   []mcov1beta2.Condition
	}{
		{
			name: "should-add",
			conditions: []mcov1beta2.Condition{
				{Type: "first"},
				{Type: "third"},
			},
			toAdd: mcov1beta2.Condition{Type: "second", Status: metav1.ConditionTrue, LastTransitionTime: metav1.Time{Time: oneHourBefore}, Reason: "reason", Message: "message"},
			expected: []mcov1beta2.Condition{
				{Type: "first"},
				{Type: "third"},
				{Type: "second", Status: metav1.ConditionTrue, LastTransitionTime: metav1.Time{Time: oneHourBefore}, Reason: "reason", Message: "message"},
			},
		},
		{
			name: "use-supplied-time",
			conditions: []mcov1beta2.Condition{
				{Type: "first"},
				{Type: "second", Status: metav1.ConditionFalse},
				{Type: "third"},
			},
			toAdd: mcov1beta2.Condition{Type: "second", Status: metav1.ConditionTrue, LastTransitionTime: metav1.Time{Time: oneHourBefore}, Reason: "reason", Message: "message"},
			expected: []mcov1beta2.Condition{
				{Type: "first"},
				{Type: "second", Status: metav1.ConditionTrue, LastTransitionTime: metav1.Time{Time: oneHourBefore}, Reason: "reason", Message: "message"},
				{Type: "third"},
			},
		},
		{
			name: "update-fields",
			conditions: []mcov1beta2.Condition{
				{Type: "first"},
				{Type: "second", Status: metav1.ConditionTrue, LastTransitionTime: metav1.Time{Time: oneHourBefore}},
				{Type: "third"},
			},
			toAdd: mcov1beta2.Condition{Type: "second", Status: metav1.ConditionTrue, LastTransitionTime: metav1.Time{Time: oneHourAfter}, Reason: "reason", Message: "message"},
			expected: []mcov1beta2.Condition{
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
		conditions    []mcov1beta2.Condition
		conditionType string
		expected      []mcov1beta2.Condition
	}{
		{
			name: "present",
			conditions: []mcov1beta2.Condition{
				{Type: "first"},
				{Type: "second"},
				{Type: "third"},
			},
			conditionType: "second",
			expected: []mcov1beta2.Condition{
				{Type: "first"},
				{Type: "third"},
			},
		},
		{
			name: "not-present",
			conditions: []mcov1beta2.Condition{
				{Type: "first"},
				{Type: "second"},
				{Type: "third"},
			},
			conditionType: "fourth",
			expected: []mcov1beta2.Condition{
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
		conditions    []mcov1beta2.Condition
		conditionType string
		expected      *mcov1beta2.Condition
	}{
		{
			name: "not-present",
			conditions: []mcov1beta2.Condition{
				{Type: "first"},
			},
			conditionType: "second",
			expected:      nil,
		},
		{
			name: "present",
			conditions: []mcov1beta2.Condition{
				{Type: "first"},
				{Type: "second"},
			},
			conditionType: "second",
			expected:      &mcov1beta2.Condition{Type: "second"},
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
