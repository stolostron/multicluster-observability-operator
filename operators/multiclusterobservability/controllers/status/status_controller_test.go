// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package status

import (
	"fmt"
	"testing"
	"time"

	mcoshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func TestFillupStatus(t *testing.T) {
	raw := fmt.Sprintf(`
conditions:
- message: Installation is in progress
  reason: %s
  type: %s
- message: Observability components are deployed and running
  reason: %s
  type: %s
`, ConditionTypeInstalling, ConditionTypeInstalling, ConditionTypeReady, ConditionTypeReady)
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
	now := metav1.Now()
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
			toAdd: mcoshared.Condition{Type: "second", Status: metav1.ConditionTrue, Reason: "reason", Message: "message"},
			expected: []mcoshared.Condition{
				{Type: "first"},
				{Type: "third"},
				{Type: "second", Status: metav1.ConditionTrue, Reason: "reason", Message: "message"},
			},
		},
		{
			name: "use-supplied-time",
			conditions: []mcoshared.Condition{
				{Type: ConditionTypeReady, Status: metav1.ConditionFalse, LastTransitionTime: metav1.NewTime(now.Add(-1 * time.Hour))},
			},
			toAdd: mcoshared.Condition{Type: ConditionTypeReady, Status: metav1.ConditionTrue, LastTransitionTime: now},
			expected: []mcoshared.Condition{
				{Type: ConditionTypeReady, Status: metav1.ConditionTrue, LastTransitionTime: now},
			},
		},
		{
			name: "update-fields-not-time",
			conditions: []mcoshared.Condition{
				{Type: ConditionTypeReady, Status: metav1.ConditionTrue, Reason: "Old", Message: "Old", LastTransitionTime: now},
			},
			toAdd: mcoshared.Condition{Type: ConditionTypeReady, Status: metav1.ConditionTrue, Reason: "New", Message: "New"},
			expected: []mcoshared.Condition{
				{Type: ConditionTypeReady, Status: metav1.ConditionTrue, Reason: "New", Message: "New", LastTransitionTime: now},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setStatusCondition(&test.conditions, test.toAdd)
			found := FindStatusCondition(test.conditions, test.toAdd.Type)
			assert.NotNil(t, found)
			assert.Equal(t, test.toAdd.Status, found.Status)
			assert.Equal(t, test.toAdd.Reason, found.Reason)
			assert.Equal(t, test.toAdd.Message, found.Message)
			if !test.toAdd.LastTransitionTime.IsZero() {
				assert.Equal(t, test.toAdd.LastTransitionTime.Unix(), found.LastTransitionTime.Unix())
			}
		})
	}

	t.Run("nil-conditions", func(t *testing.T) {
		var conds []mcoshared.Condition
		setStatusCondition(&conds, mcoshared.Condition{Type: "Test"})
		assert.Len(t, conds, 1)
		assert.Equal(t, "Test", conds[0].Type)
	})
}

func TestRemoveStatusCondition(t *testing.T) {
	conds := []mcoshared.Condition{
		{Type: "A"},
		{Type: "B"},
	}
	RemoveStatusCondition(&conds, "A")
	assert.Len(t, conds, 1)
	assert.Equal(t, "B", conds[0].Type)

	RemoveStatusCondition(&conds, "C")
	assert.Len(t, conds, 1)

	t.Run("nil-conditions", func(t *testing.T) {
		var conds []mcoshared.Condition
		RemoveStatusCondition(&conds, "A")
		assert.Nil(t, conds)
	})
}

func TestSortConditions(t *testing.T) {
	conds := []mcoshared.Condition{
		{Type: "Z"},
		{Type: "A"},
		{Type: "M"},
	}
	sortConditions(conds)
	assert.Equal(t, "A", conds[0].Type)
	assert.Equal(t, "M", conds[1].Type)
	assert.Equal(t, "Z", conds[2].Type)
}

func TestFindStatusCondition(t *testing.T) {
	conds := []mcoshared.Condition{
		{Type: ConditionTypeReady, Status: metav1.ConditionTrue},
	}
	found := FindStatusCondition(conds, ConditionTypeReady)
	assert.NotNil(t, found)
	assert.Equal(t, metav1.ConditionTrue, found.Status)

	found = FindStatusCondition(conds, "NotPresent")
	assert.Nil(t, found)
}

func TestGetConditionChanges(t *testing.T) {
	tests := []struct {
		name     string
		oldConds []mcoshared.Condition
		newConds []mcoshared.Condition
		expected []string
	}{
		{
			name:     "no-changes",
			oldConds: []mcoshared.Condition{{Type: "A", Status: "True", Reason: "R1", Message: "M1"}},
			newConds: []mcoshared.Condition{{Type: "A", Status: "True", Reason: "R1", Message: "M1"}},
			expected: nil,
		},
		{
			name:     "added-condition",
			oldConds: []mcoshared.Condition{{Type: "A", Status: "True"}},
			newConds: []mcoshared.Condition{{Type: "A", Status: "True"}, {Type: "B", Status: "False", Reason: "R2"}},
			expected: []string{"Added: B (Status: False, Reason: R2)"},
		},
		{
			name:     "removed-condition",
			oldConds: []mcoshared.Condition{{Type: "A", Status: "True"}, {Type: "B", Status: "False"}},
			newConds: []mcoshared.Condition{{Type: "A", Status: "True"}},
			expected: []string{"Removed: B"},
		},
		{
			name:     "modified-status",
			oldConds: []mcoshared.Condition{{Type: "A", Status: "True"}},
			newConds: []mcoshared.Condition{{Type: "A", Status: "False"}},
			expected: []string{"Modified: A (Status: True->False, Reason: ->)"},
		},
		{
			name:     "modified-reason",
			oldConds: []mcoshared.Condition{{Type: "A", Status: "True", Reason: "OldReason"}},
			newConds: []mcoshared.Condition{{Type: "A", Status: "True", Reason: "NewReason"}},
			expected: []string{"Modified: A (Status: True->True, Reason: OldReason->NewReason)"},
		},
		{
			name:     "modified-message",
			oldConds: []mcoshared.Condition{{Type: "A", Status: "True", Message: "OldMsg"}},
			newConds: []mcoshared.Condition{{Type: "A", Status: "True", Message: "NewMsg"}},
			expected: []string{"Modified: A (Status: True->True, Reason: -> [Message updated])"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changes := getConditionChanges(tt.oldConds, tt.newConds)
			assert.ElementsMatch(t, tt.expected, changes)
		})
	}
}

func TestReconcileStatus(t *testing.T) {
	mcoconfig.SetMonitoringCRName("observability")
	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{
			Name: mcoconfig.GetMonitoringCRName(),
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta2.StorageConfig{
				MetricObjectStorage: &mcoshared.PreConfiguredStorage{
					Key:  "test",
					Name: "test",
				},
			},
			ObservabilityAddonSpec: &mcoshared.ObservabilityAddonSpec{
				EnableMetrics: false,
			},
		},
	}

	s := runtime.NewScheme()
	scheme.AddToScheme(s)
	mcov1beta2.SchemeBuilder.AddToScheme(s)
	addonv1alpha1.AddToScheme(s)
	oav1beta1.AddToScheme(s)

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(mco).
		WithStatusSubresource(&mcov1beta2.MultiClusterObservability{}).
		Build()

	r := &StatusReconciler{
		Client: cl,
		Log:    logf.Log,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: mcoconfig.GetMonitoringCRName(),
		},
	}

	// 1. Initial reconcile - should fail because storage secret is missing
	_, err := r.Reconcile(t.Context(), req)
	assert.NoError(t, err)

	instance := &mcov1beta2.MultiClusterObservability{}
	_ = cl.Get(t.Context(), req.NamespacedName, instance)
	assert.NotNil(t, FindStatusCondition(instance.Status.Conditions, ConditionTypeFailed))
	assert.Equal(t, ReasonObjectStorageNotFound, FindStatusCondition(instance.Status.Conditions, ConditionTypeFailed).Reason)

	// 2. Add metrics disabled check
	assert.NotNil(t, FindStatusCondition(instance.Status.Conditions, ConditionTypeMetricsDisabled))

	// 3. Idempotency check - reconcile again, status should not change
	_, err = r.Reconcile(t.Context(), req)
	assert.NoError(t, err)

	instance2 := &mcov1beta2.MultiClusterObservability{}
	_ = cl.Get(t.Context(), req.NamespacedName, instance2)
	assert.Equal(t, instance.ResourceVersion, instance2.ResourceVersion)
	assert.Equal(t, instance.Status.Conditions, instance2.Status.Conditions)
}

func TestCheckDeployStatus(t *testing.T) {
	s := runtime.NewScheme()
	scheme.AddToScheme(s)
	appsv1.AddToScheme(s)

	tests := []struct {
		name           string
		existingObjs   []runtime.Object
		expectedStatus *mcoshared.Condition
	}{
		{
			name: "deployment-missing",
			expectedStatus: &mcoshared.Condition{
				Reason: ReasonDeploymentNotFound,
			},
		},
		{
			name: "deployment-not-ready",
			existingObjs: func() []runtime.Object {
				var objs []runtime.Object
				for _, name := range config.GetExpectedDeploymentNames() {
					objs = append(objs, &appsv1.Deployment{
						ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: mcoconfig.GetDefaultNamespace()},
						Status:     appsv1.DeploymentStatus{Replicas: 1, ReadyReplicas: 0},
					})
				}
				return objs
			}(),
			expectedStatus: &mcoshared.Condition{
				Reason: ReasonDeploymentNotReady,
			},
		},
		{
			name: "all-deployments-ready",
			existingObjs: func() []runtime.Object {
				var objs []runtime.Object
				for _, name := range config.GetExpectedDeploymentNames() {
					objs = append(objs, &appsv1.Deployment{
						ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: mcoconfig.GetDefaultNamespace()},
						Status:     appsv1.DeploymentStatus{Replicas: 1, ReadyReplicas: 1},
					})
				}
				return objs
			}(),
			expectedStatus: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(tt.existingObjs...).Build()
			cond := checkDeployStatus(t.Context(), cl)
			if tt.expectedStatus == nil {
				assert.Nil(t, cond)
			} else {
				assert.NotNil(t, cond)
				assert.Equal(t, tt.expectedStatus.Reason, cond.Reason)
			}
		})
	}
}

func TestCheckStatefulSetStatus(t *testing.T) {
	s := runtime.NewScheme()
	scheme.AddToScheme(s)
	appsv1.AddToScheme(s)

	tests := []struct {
		name           string
		existingObjs   []runtime.Object
		expectedStatus *mcoshared.Condition
	}{
		{
			name: "statefulset-missing",
			expectedStatus: &mcoshared.Condition{
				Reason: ReasonStatefulSetNotFound,
			},
		},
		{
			name: "statefulset-not-ready",
			existingObjs: func() []runtime.Object {
				var objs []runtime.Object
				for _, name := range config.GetExpectedStatefulSetNames() {
					objs = append(objs, &appsv1.StatefulSet{
						ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: mcoconfig.GetDefaultNamespace()},
						Status:     appsv1.StatefulSetStatus{Replicas: 1, ReadyReplicas: 0},
					})
				}
				return objs
			}(),
			expectedStatus: &mcoshared.Condition{
				Reason: ReasonStatefulSetNotReady,
			},
		},
		{
			name: "all-statefulsets-ready",
			existingObjs: func() []runtime.Object {
				var objs []runtime.Object
				for _, name := range config.GetExpectedStatefulSetNames() {
					objs = append(objs, &appsv1.StatefulSet{
						ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: mcoconfig.GetDefaultNamespace()},
						Status:     appsv1.StatefulSetStatus{Replicas: 1, ReadyReplicas: 1},
					})
				}
				return objs
			}(),
			expectedStatus: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(tt.existingObjs...).Build()
			cond := checkStatefulSetStatus(t.Context(), cl)
			if tt.expectedStatus == nil {
				assert.Nil(t, cond)
			} else {
				assert.NotNil(t, cond)
				assert.Equal(t, tt.expectedStatus.Reason, cond.Reason)
			}
		})
	}
}

func TestCheckObjStorageStatus(t *testing.T) {
	s := runtime.NewScheme()
	scheme.AddToScheme(s)
	corev1.AddToScheme(s)

	mco := &mcov1beta2.MultiClusterObservability{
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta2.StorageConfig{
				MetricObjectStorage: &mcoshared.PreConfiguredStorage{
					Key:  "test",
					Name: "test",
				},
			},
		},
	}

	tests := []struct {
		name           string
		existingObjs   []runtime.Object
		expectedStatus *mcoshared.Condition
	}{
		{
			name: "secret-missing",
			expectedStatus: &mcoshared.Condition{
				Reason: ReasonObjectStorageNotFound,
			},
		},
		{
			name: "secret-key-missing",
			existingObjs: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: mcoconfig.GetDefaultNamespace()},
					Data:       map[string][]byte{"other": []byte("data")},
				},
			},
			expectedStatus: &mcoshared.Condition{
				Reason: ReasonObjectStorageInvalid,
			},
		},
		{
			name: "secret-invalid-conf",
			existingObjs: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: mcoconfig.GetDefaultNamespace()},
					Data:       map[string][]byte{"test": []byte("invalid-yaml")},
				},
			},
			expectedStatus: &mcoshared.Condition{
				Reason: ReasonObjectStorageInvalid,
			},
		},
		{
			name: "valid-conf",
			existingObjs: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: mcoconfig.GetDefaultNamespace()},
					Data:       map[string][]byte{"test": []byte("type: s3\nconfig:\n  bucket: test-bucket\n  endpoint: s3.amazonaws.com\n")},
				},
			},
			expectedStatus: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(tt.existingObjs...).Build()
			cond := checkObjStorageStatus(t.Context(), cl, mco)
			if tt.expectedStatus == nil {
				assert.Nil(t, cond)
			} else {
				assert.NotNil(t, cond)
				assert.Equal(t, tt.expectedStatus.Reason, cond.Reason)
			}
		})
	}
}

func TestUpdateMCOAStatus(t *testing.T) {
	s := runtime.NewScheme()
	scheme.AddToScheme(s)
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
			name: "Platform metrics enabled but CRDs missing",
			instance: &mcov1beta2.MultiClusterObservability{
				Spec: mcov1beta2.MultiClusterObservabilitySpec{
					Capabilities: &mcov1beta2.CapabilitiesSpec{
						Platform: &mcov1beta2.PlatformCapabilitiesSpec{
							Metrics: mcov1beta2.PlatformMetricsSpec{
								Default: mcov1beta2.PlatformMetricsDefaultSpec{
									Enabled: true,
								},
							},
						},
					},
				},
			},
			expectedStatus: &mcoshared.Condition{
				Type:   ConditionTypeMCOADegraded,
				Status: metav1.ConditionTrue,
			},
		},
		{
			name: "Platform metrics enabled and CRDs present",
			instance: &mcov1beta2.MultiClusterObservability{
				Spec: mcov1beta2.MultiClusterObservabilitySpec{
					Capabilities: &mcov1beta2.CapabilitiesSpec{
						Platform: &mcov1beta2.PlatformCapabilitiesSpec{
							Metrics: mcov1beta2.PlatformMetricsSpec{
								Default: mcov1beta2.PlatformMetricsDefaultSpec{
									Enabled: true,
								},
							},
						},
					},
				},
			},
			existingObjs: []runtime.Object{
				&apiextensionsv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "clusterlogforwarders.observability.openshift.io"},
					Spec: apiextensionsv1.CustomResourceDefinitionSpec{
						Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
							{Name: "v1", Served: true},
						},
					},
				},
				&apiextensionsv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "opentelemetrycollectors.opentelemetry.io"},
					Spec: apiextensionsv1.CustomResourceDefinitionSpec{
						Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
							{Name: "v1beta1", Served: true},
						},
					},
				},
				&apiextensionsv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "instrumentations.opentelemetry.io"},
					Spec: apiextensionsv1.CustomResourceDefinitionSpec{
						Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
							{Name: "v1alpha1", Served: true},
						},
					},
				},
				&apiextensionsv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "prometheusagents.monitoring.rhobs"},
					Spec: apiextensionsv1.CustomResourceDefinitionSpec{
						Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
							{Name: "v1alpha1", Served: true},
						},
					},
				},
				&apiextensionsv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "prometheusrules.monitoring.rhobs"},
					Spec: apiextensionsv1.CustomResourceDefinitionSpec{
						Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
							{Name: "v1", Served: true},
						},
					},
				},
				&apiextensionsv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "scrapeconfigs.monitoring.rhobs"},
					Spec: apiextensionsv1.CustomResourceDefinitionSpec{
						Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
							{Name: "v1alpha1", Served: true},
						},
					},
				},
			},
			expectedStatus: nil,
		},
		{
			name: "Capabilities enabled but none selected",
			instance: &mcov1beta2.MultiClusterObservability{
				Spec: mcov1beta2.MultiClusterObservabilitySpec{
					Capabilities: &mcov1beta2.CapabilitiesSpec{
						Platform: &mcov1beta2.PlatformCapabilitiesSpec{},
					},
				},
			},
			expectedStatus: nil,
		},
		{
			name: "UserWorkloads enabled but CRD missing",
			instance: &mcov1beta2.MultiClusterObservability{
				Spec: mcov1beta2.MultiClusterObservabilitySpec{
					Capabilities: &mcov1beta2.CapabilitiesSpec{
						UserWorkloads: &mcov1beta2.UserWorkloadCapabilitiesSpec{
							Logs: mcov1beta2.UserWorkloadLogsSpec{
								Collection: mcov1beta2.UserWorkloadLogsCollectionSpec{
									ClusterLogForwarder: mcov1beta2.ClusterLogForwarderSpec{
										Enabled: true,
									},
								},
							},
						},
					},
				},
			},
			expectedStatus: &mcoshared.Condition{
				Type:    ConditionTypeMCOADegraded,
				Status:  metav1.ConditionTrue,
				Reason:  ReasonCRDMissing,
				Message: "MultiCluster-Observability-Addon degraded because the following CRDs are not installed on the hub: clusterlogforwarders.observability.openshift.io(v1), instrumentations.opentelemetry.io(v1alpha1), opentelemetrycollectors.opentelemetry.io(v1beta1), prometheusagents.monitoring.rhobs(v1alpha1), prometheusrules.monitoring.rhobs(v1), scrapeconfigs.monitoring.rhobs(v1alpha1)",
			},
		},
		{
			name: "CRD present but version unserved",
			instance: &mcov1beta2.MultiClusterObservability{
				Spec: mcov1beta2.MultiClusterObservabilitySpec{
					Capabilities: &mcov1beta2.CapabilitiesSpec{
						Platform: &mcov1beta2.PlatformCapabilitiesSpec{
							Metrics: mcov1beta2.PlatformMetricsSpec{
								Default: mcov1beta2.PlatformMetricsDefaultSpec{
									Enabled: true,
								},
							},
						},
					},
				},
			},
			existingObjs: []runtime.Object{
				&apiextensionsv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "clusterlogforwarders.observability.openshift.io"},
					Spec: apiextensionsv1.CustomResourceDefinitionSpec{
						Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
							{Name: "v1", Served: false}, // Version present but NOT served
						},
					},
				},
			},
			expectedStatus: &mcoshared.Condition{
				Type:    ConditionTypeMCOADegraded,
				Status:  metav1.ConditionTrue,
				Reason:  ReasonCRDMissing,
				Message: "MultiCluster-Observability-Addon degraded because the following CRDs are not installed on the hub: clusterlogforwarders.observability.openshift.io(v1), instrumentations.opentelemetry.io(v1alpha1), opentelemetrycollectors.opentelemetry.io(v1beta1), prometheusagents.monitoring.rhobs(v1alpha1), prometheusrules.monitoring.rhobs(v1), scrapeconfigs.monitoring.rhobs(v1alpha1)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(tt.existingObjs...).Build()
			r := &StatusReconciler{Client: cl}
			conds := []mcoshared.Condition{}
			r.updateMCOAStatus(t.Context(), tt.instance, &conds)

			if tt.expectedStatus == nil {
				assert.Empty(t, conds)
			} else {
				found := FindStatusCondition(conds, tt.expectedStatus.Type)
				assert.NotNil(t, found)
				assert.Equal(t, tt.expectedStatus.Status, found.Status)
				if tt.expectedStatus.Reason != "" {
					assert.Equal(t, tt.expectedStatus.Reason, found.Reason)
				}
				if tt.expectedStatus.Message != "" {
					assert.Equal(t, tt.expectedStatus.Message, found.Message)
				}
			}
		})
	}
}
