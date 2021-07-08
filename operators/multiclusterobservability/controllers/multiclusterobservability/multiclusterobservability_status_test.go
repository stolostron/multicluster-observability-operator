// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package multiclusterobservability

import (
	"context"
	"reflect"
	"testing"
	"time"

	mcoshared "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	mcoconfig "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

func TestStartStatusUpdate(t *testing.T) {
	mcoconfig.SetMonitoringCRName("observability")
	// A MultiClusterObservability object with metadata and spec.
	mco := &mcov1beta2.MultiClusterObservability{
		TypeMeta: metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{
			Name: mcoconfig.GetMonitoringCRName(),
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta2.StorageConfig{
				MetricObjectStorage: &mcoshared.PreConfiguredStorage{
					Key:  "test",
					Name: "test",
				},
				StorageClass:            "gp2",
				AlertmanagerStorageSize: "1Gi",
				CompactStorageSize:      "1Gi",
				RuleStorageSize:         "1Gi",
				ReceiveStorageSize:      "1Gi",
				StoreStorageSize:        "1Gi",
			},
			ObservabilityAddonSpec: &mcoshared.ObservabilityAddonSpec{
				EnableMetrics: false,
			},
		},
		Status: mcov1beta2.MultiClusterObservabilityStatus{
			Conditions: []mcoshared.Condition{},
		},
	}

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	mcov1beta2.SchemeBuilder.AddToScheme(s)

	objs := []runtime.Object{mco, createSecret("test", "test", mcoconfig.GetMCONamespace())}
	cl := fake.NewFakeClient(objs...)

	StartStatusUpdate(cl, mco)

	requeueStatusUpdate <- struct{}{}
	time.Sleep(3 * time.Second)

	instance := &mcov1beta2.MultiClusterObservability{}
	_ = cl.Get(context.TODO(), types.NamespacedName{
		Name: mcoconfig.GetMonitoringCRName(),
	}, instance)

	if findStatusCondition(instance.Status.Conditions, "Installing") == nil {
		t.Fatal("failed to update mco status with Installing")
	}
	if findStatusCondition(instance.Status.Conditions, "MetricsDisabled") == nil {
		t.Fatal("failed to update mco status with MetricsDisabled")
	}

	instance.Spec.ObservabilityAddonSpec.EnableMetrics = true
	err := cl.Update(context.TODO(), instance)
	if err != nil {
		t.Fatalf("Failed to update MultiClusterObservability: (%v)", err)
	}
	requeueStatusUpdate <- struct{}{}
	time.Sleep(3 * time.Second)

	instance = &mcov1beta2.MultiClusterObservability{}
	_ = cl.Get(context.TODO(), types.NamespacedName{
		Name: mcoconfig.GetMonitoringCRName(),
	}, instance)

	if findStatusCondition(instance.Status.Conditions, "MetricsDisabled") != nil {
		t.Fatal("failed to update mco status to remove MetricsDisabled")
	}
}
