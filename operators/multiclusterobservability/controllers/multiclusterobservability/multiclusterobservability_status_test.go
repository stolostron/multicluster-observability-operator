// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package multiclusterobservability

import (
	"context"
	"reflect"
	"testing"
	"time"

	mcoshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
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
	addonv1alpha1.AddToScheme(s)
	oav1beta1.AddToScheme(s)

	objs := []runtime.Object{mco, createSecret("test", "test", mcoconfig.GetMCONamespace())}
	cl := fake.NewClientBuilder().
		WithRuntimeObjects(objs...).
		WithStatusSubresource(
			&addonv1alpha1.ManagedClusterAddOn{},
			&mcov1beta2.MultiClusterObservability{},
			&oav1beta1.ObservabilityAddon{},
		).
		Build()

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

func TestUpdateMCOAStatus(t *testing.T) {
	// Register the necessary schemes
	s := scheme.Scheme
	s.AddKnownTypes(mcov1beta2.GroupVersion, &mcov1beta2.MultiClusterObservability{})
	s.AddKnownTypes(apiextensionsv1.SchemeGroupVersion, &apiextensionsv1.CustomResourceDefinition{})

	tests := []struct {
		name           string
		instance       *mcov1beta2.MultiClusterObservability
		existingObjs   []runtime.Object
		expectedStatus *mcoshared.Condition
	}{
		{
			name: "Capabilities not set",
			instance: &mcov1beta2.MultiClusterObservability{
				Spec: mcov1beta2.MultiClusterObservabilitySpec{
					Capabilities: nil,
				},
			},
			expectedStatus: nil,
		},
		{
			name: "Capabilities set but not configured",
			instance: &mcov1beta2.MultiClusterObservability{
				Spec: mcov1beta2.MultiClusterObservabilitySpec{
					Capabilities: &mcov1beta2.CapabilitiesSpec{},
				},
			},
			expectedStatus: nil,
		},
		{
			name: "Capabilities set but CRDs missing",
			instance: &mcov1beta2.MultiClusterObservability{
				Spec: mcov1beta2.MultiClusterObservabilitySpec{
					Capabilities: &mcov1beta2.CapabilitiesSpec{
						Platform: &mcov1beta2.PlatformCapabilitiesSpec{
							Logs: mcov1beta2.PlatformLogsSpec{
								Collection: mcov1beta2.PlatformLogsCollectionSpec{
									Enabled: true,
								},
							},
						},
					},
				},
			},
			expectedStatus: &mcoshared.Condition{
				Type:    reasonMCOADegraded,
				Status:  metav1.ConditionTrue,
				Reason:  reasonMCOADegraded,
				Message: "MultiCluster-Observability-Addon degraded because the following CRDs are not installed on the hub: clusterlogforwarders.observability.openshift.io(v1), instrumentations.opentelemetry.io(v1alpha1), opentelemetrycollectors.opentelemetry.io(v1beta1)",
			},
		},
		{
			name: "Capabilities set and CRDs present",
			instance: &mcov1beta2.MultiClusterObservability{
				Spec: mcov1beta2.MultiClusterObservabilitySpec{
					Capabilities: &mcov1beta2.CapabilitiesSpec{
						Platform: &mcov1beta2.PlatformCapabilitiesSpec{
							Logs: mcov1beta2.PlatformLogsSpec{
								Collection: mcov1beta2.PlatformLogsCollectionSpec{
									Enabled: true,
								},
							},
						},
					},
				},
			},
			existingObjs: []runtime.Object{
				&apiextensionsv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name: "clusterlogforwarders.observability.openshift.io",
					},
					Spec: apiextensionsv1.CustomResourceDefinitionSpec{
						Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
							{
								Name:   "v1",
								Served: true,
							},
						},
					},
				},
			},
			expectedStatus: &mcoshared.Condition{
				Type:    reasonMCOADegraded,
				Status:  metav1.ConditionTrue,
				Reason:  reasonMCOADegraded,
				Message: "MultiCluster-Observability-Addon degraded because the following CRDs are not installed on the hub: instrumentations.opentelemetry.io(v1alpha1), opentelemetrycollectors.opentelemetry.io(v1beta1)",
			},
		},
		{
			name: "When Logs, Metrics, IncidentDetection all are disabled under platform",
			instance: &mcov1beta2.MultiClusterObservability{
				Spec: mcov1beta2.MultiClusterObservabilitySpec{
					Capabilities: &mcov1beta2.CapabilitiesSpec{
						Platform: &mcov1beta2.PlatformCapabilitiesSpec{
							Logs: mcov1beta2.PlatformLogsSpec{
								Collection: mcov1beta2.PlatformLogsCollectionSpec{
									Enabled: false,
								},
							},
							Metrics: mcov1beta2.PlatformMetricsSpec{
								Collection: mcov1beta2.PlatformMetricsCollectionSpec{
									Enabled: false,
								},
							},
							Analytics: mcov1beta2.PlatformAnalyticsSpec{
								IncidentDetection: mcov1beta2.PlatformIncidentDetectionSpec{
									Enabled: false,
								},
							},
						},
					},
				},
			},
			expectedStatus: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(tt.existingObjs...).Build()
			conds := []mcoshared.Condition{}
			updateMCOAStatus(client, &conds, tt.instance)

			if tt.expectedStatus == nil {
				assert.Empty(t, conds)
			} else {
				assert.NotEmpty(t, conds)
				assert.Equal(t, tt.expectedStatus.Type, conds[0].Type)
				assert.Equal(t, tt.expectedStatus.Status, conds[0].Status)
				assert.Equal(t, tt.expectedStatus.Reason, conds[0].Reason)
				assert.Contains(t, conds[0].Message, tt.expectedStatus.Message)
			}
		})
	}
}
