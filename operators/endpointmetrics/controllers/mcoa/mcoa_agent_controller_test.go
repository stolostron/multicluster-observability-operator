// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package mcoa

import (
	"context"
	"errors"
	"testing"

	ocinfrav1 "github.com/openshift/api/config/v1"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/observabilityendpoint"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestMCOAAgentReconciler_Reconcile(t *testing.T) {
	t.Parallel()

	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))
	require.NoError(t, ocinfrav1.AddToScheme(s))

	// Register ScrapeConfig for the custom monitoring.rhobs/v1alpha1 API Group
	addRhobsToScheme(t, s)

	namespace := "test-ns"
	clusterID := "test-cluster-id"
	clusterName := "test-cluster-name"
	alertmanagerEndpoint := "https://hub-am.example.com"
	hubClusterID := "hub-id"

	amAccessorSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      observabilityendpoint.AppendHubClusterID(observabilityendpoint.HubAmAccessorSecretName, hubClusterID),
			Namespace: operatorconfig.OCPClusterMonitoringNamespace,
		},
		Data: map[string][]byte{
			"token": []byte("test-token"),
		},
	}

	sourceAmAccessorSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      observabilityendpoint.HubAmAccessorSecretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"token": []byte("test-token"),
		},
	}

	clusterVersion := &ocinfrav1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: "version",
		},
		Spec: ocinfrav1.ClusterVersionSpec{
			ClusterID: ocinfrav1.ClusterID(clusterID),
		},
	}

	tests := []struct {
		name                      string
		req                       ctrl.Request
		alertmanagerEndpoint      string
		hubClusterID              string
		existingObjs              []client.Object
		clientInterceptors        interceptor.Funcs
		expectedMetric            float64
		expectedEvent             bool
		expectedError             bool
		disableUWLAlertForwarding bool
		validate                  func(t *testing.T, c client.Client)
	}{
		{
			name: "CMO ConfigMap missing - Successful create",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
					Namespace: operatorconfig.OCPClusterMonitoringNamespace,
				},
			},
			alertmanagerEndpoint: alertmanagerEndpoint,
			hubClusterID:         hubClusterID,
			existingObjs:         []client.Object{amAccessorSecret, sourceAmAccessorSecret, clusterVersion},
			validate: func(t *testing.T, c client.Client) {
				found := &corev1.ConfigMap{}
				err := c.Get(context.Background(), types.NamespacedName{
					Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
					Namespace: operatorconfig.OCPClusterMonitoringNamespace,
				}, found)
				require.NoError(t, err)
				assert.Contains(t, found.Data[observabilityendpoint.ClusterMonitoringConfigDataKey], "hub-am.example.com")
				assert.Contains(
					t,
					found.Data[observabilityendpoint.ClusterMonitoringConfigDataKey],
					"managed_cluster_name: test-cluster-name",
					"MCOA alert config must include the human-readable managed_cluster_name external label under the successful create path",
				)
			},
		},
		{
			name: "CMO Config Conflict - Metric incremented and event emitted",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
					Namespace: operatorconfig.OCPClusterMonitoringNamespace,
				},
			},
			alertmanagerEndpoint: alertmanagerEndpoint,
			hubClusterID:         hubClusterID,
			existingObjs: []client.Object{
				amAccessorSecret,
				sourceAmAccessorSecret,
				clusterVersion,
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
						Namespace: operatorconfig.OCPClusterMonitoringNamespace,
						ManagedFields: []metav1.ManagedFieldsEntry{
							{
								Manager:    observabilityendpoint.EndpointMonitoringOperatorMgr,
								Operation:  metav1.ManagedFieldsOperationUpdate,
								APIVersion: "v1",
								FieldsType: "FieldsV1",
							},
						},
					},
					Data: map[string]string{
						observabilityendpoint.ClusterMonitoringConfigDataKey: "prometheusK8s:\n  additionalAlertmanagerConfigs:\n  - scheme: https\n    staticConfigs:\n    - old-hub.com",
					},
				},
			},
			expectedMetric: 1.0,
			expectedEvent:  true,
			validate: func(t *testing.T, c client.Client) {
				found := &corev1.ConfigMap{}
				err := c.Get(context.Background(), types.NamespacedName{
					Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
					Namespace: operatorconfig.OCPClusterMonitoringNamespace,
				}, found)
				require.NoError(t, err)
				assert.Contains(t, found.Data[observabilityendpoint.ClusterMonitoringConfigDataKey], "hub-am.example.com")
				assert.Contains(
					t,
					found.Data[observabilityendpoint.ClusterMonitoringConfigDataKey],
					"managed_cluster_name: test-cluster-name",
					"Conflict path reconciliation must re-apply the human-readable managed_cluster_name external label",
				)
			},
		},
		{
			name: "UWL ConfigMap missing - Successful create",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
					Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
				},
			},
			alertmanagerEndpoint: alertmanagerEndpoint,
			hubClusterID:         hubClusterID,
			existingObjs:         []client.Object{amAccessorSecret, sourceAmAccessorSecret, clusterVersion},
			validate: func(t *testing.T, c client.Client) {
				found := &corev1.ConfigMap{}
				err := c.Get(context.Background(), types.NamespacedName{
					Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
					Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
				}, found)
				require.NoError(t, err)
				assert.Contains(t, found.Data[uwlMonitoringConfigDataKey], "hub-am.example.com")
			},
		},
		{
			name: "Ignored resource - No action and no requeue",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "some-other-cm",
					Namespace: "some-other-ns",
				},
			},
			alertmanagerEndpoint: alertmanagerEndpoint,
			hubClusterID:         hubClusterID,
			existingObjs:         []client.Object{},
			validate: func(t *testing.T, c client.Client) {
				found := &corev1.ConfigMap{}
				err := c.Get(context.Background(), types.NamespacedName{
					Name:      "some-other-cm",
					Namespace: "some-other-ns",
				}, found)
				assert.True(t, apierrors.IsNotFound(err))
			},
		},
		{
			name: "AlertmanagerEndpoint empty - Revert path",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
					Namespace: operatorconfig.OCPClusterMonitoringNamespace,
				},
			},
			alertmanagerEndpoint: "",
			hubClusterID:         "hub-id",
			existingObjs: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
						Namespace: operatorconfig.OCPClusterMonitoringNamespace,
						ManagedFields: []metav1.ManagedFieldsEntry{
							{
								Manager:    observabilityendpoint.EndpointMonitoringOperatorMgr,
								Operation:  metav1.ManagedFieldsOperationUpdate,
								APIVersion: "v1",
								FieldsType: "FieldsV1",
							},
						},
					},
					Data: map[string]string{
						observabilityendpoint.ClusterMonitoringConfigDataKey: "prometheusK8s: { additionalAlertmanagerConfigs: [ { scheme: https, tlsConfig: { ca: { name: hub-alertmanager-router-ca-hub-id } }, staticConfigs: [ hub.com ] } ] }",
					},
				},
			},
			validate: func(t *testing.T, c client.Client) {
				found := &corev1.ConfigMap{}
				err := c.Get(context.Background(), types.NamespacedName{
					Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
					Namespace: operatorconfig.OCPClusterMonitoringNamespace,
				}, found)
				if err == nil {
					assert.NotContains(t, found.Data[observabilityendpoint.ClusterMonitoringConfigDataKey], "hub.com")
				}
			},
		},
		{
			name: "CRD deletion event - DeployCRDs failure propagates as error",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "prometheusagents.monitoring.rhobs"},
			},
			alertmanagerEndpoint: alertmanagerEndpoint,
			hubClusterID:         hubClusterID,
			existingObjs:         []client.Object{},
			expectedError:        true,
			clientInterceptors: interceptor.Funcs{
				Get: func(_ context.Context, _ client.WithWatch, _ types.NamespacedName, _ client.Object, _ ...client.GetOption) error {
					return errors.New("injected API error")
				},
			},
		},
		{
			name: "CRD deletion event - immediately re-applies all managed CRDs",
			req: ctrl.Request{
				// CRDs are cluster-scoped: no namespace.
				NamespacedName: types.NamespacedName{Name: "prometheusagents.monitoring.rhobs"},
			},
			alertmanagerEndpoint: alertmanagerEndpoint,
			hubClusterID:         hubClusterID,
			existingObjs:         []client.Object{},
			validate: func(t *testing.T, c client.Client) {
				crd := &unstructured.Unstructured{}
				crd.SetGroupVersionKind(schema.GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"})
				err := c.Get(context.Background(), types.NamespacedName{Name: "prometheusagents.monitoring.rhobs"}, crd)
				require.NoError(t, err)
				assert.True(t, isManagedByUs(crd), "restored CRD must carry the mcoa ownership label")
			},
		},
		{
			name: "UWL ConfigMap - UWL alert forwarding disabled",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
					Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
				},
			},
			alertmanagerEndpoint:      alertmanagerEndpoint,
			hubClusterID:              hubClusterID,
			existingObjs:              []client.Object{amAccessorSecret, sourceAmAccessorSecret, clusterVersion},
			disableUWLAlertForwarding: true,
			validate: func(t *testing.T, c client.Client) {
				found := &corev1.ConfigMap{}
				err := c.Get(context.Background(), types.NamespacedName{
					Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
					Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
				}, found)
				if err == nil {
					assert.NotContains(t, found.Data[uwlMonitoringConfigDataKey], "hub.com")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(s).WithObjects(tt.existingObjs...).WithReturnManagedFields().WithInterceptorFuncs(tt.clientInterceptors).Build()
			recorder := events.NewFakeRecorder(10)

			// Capture initial metric value
			initialMetric := testutil.ToFloat64(cmoConfigConflictsTotal)

			caSecretName := observabilityendpoint.AppendHubClusterID(observabilityendpoint.HubAmRouterCASecretName, tt.hubClusterID)

			r := NewMCOAAgentReconciler(
				c,
				ctrl.Log.WithName("test"),
				s,
				recorder,
				namespace,
				clusterID,
				clusterName,
				tt.alertmanagerEndpoint,
				caSecretName,
				"obs-alertmanager-mtls-cert",
				"observability-alertmanager-accessor",
				!tt.disableUWLAlertForwarding,
			)

			result, err := r.Reconcile(context.Background(), tt.req)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Zero(t, result.RequeueAfter, "reconciler must not schedule a periodic requeue — the CRD watch handles healing")

			if tt.expectedMetric > 0 {
				assert.Equal(t, initialMetric+tt.expectedMetric, testutil.ToFloat64(cmoConfigConflictsTotal))
			}

			if tt.expectedEvent {
				select {
				case event := <-recorder.Events:
					assert.Contains(t, event, "ConfigConflict")
				default:
					t.Errorf("Expected event was not emitted")
				}
			}

			// Run custom explicit validations
			if tt.validate != nil {
				tt.validate(t, c)
			}
		})
	}
}
