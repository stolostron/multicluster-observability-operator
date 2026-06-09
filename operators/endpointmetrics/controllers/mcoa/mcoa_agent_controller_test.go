// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package mcoa

import (
	"context"
	"strings"
	"testing"

	ocinfrav1 "github.com/openshift/api/config/v1"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/observabilityendpoint"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestMCOAAgentReconciler_Reconcile(t *testing.T) {
	t.Parallel()

	s := scheme.Scheme
	_ = ocinfrav1.AddToScheme(s)

	namespace := "test-ns"
	clusterID := "test-cluster-id"
	hubInfo := &operatorconfig.HubInfo{
		AlertmanagerEndpoint: "https://hub-am.example.com",
		HubClusterID:         "hub-id",
	}

	amAccessorSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      observabilityendpoint.AppendHubClusterID(observabilityendpoint.HubAmAccessorSecretName, hubInfo),
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
		name           string
		req            ctrl.Request
		hubInfo        *operatorconfig.HubInfo
		existingObjs   []client.Object
		expectedMetric float64
		expectedEvent  bool
		expectedError  bool
	}{
		{
			name: "CMO ConfigMap missing - Successful create",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
					Namespace: operatorconfig.OCPClusterMonitoringNamespace,
				},
			},
			hubInfo:      hubInfo,
			existingObjs: []client.Object{amAccessorSecret, sourceAmAccessorSecret, clusterVersion},
		},
		{
			name: "CMO Config Conflict - Metric incremented and event emitted",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
					Namespace: operatorconfig.OCPClusterMonitoringNamespace,
				},
			},
			hubInfo: hubInfo,
			existingObjs: []client.Object{
				amAccessorSecret,
				sourceAmAccessorSecret,
				clusterVersion,
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
						Namespace: operatorconfig.OCPClusterMonitoringNamespace,
						ManagedFields: []metav1.ManagedFieldsEntry{
							{Manager: observabilityendpoint.EndpointMonitoringOperatorMgr},
						},
					},
					Data: map[string]string{
						observabilityendpoint.ClusterMonitoringConfigDataKey: "prometheusK8s: {}",
					},
				},
			},
			expectedMetric: 1.0,
			expectedEvent:  true,
		},
		{
			name: "UWL ConfigMap missing - Successful create",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
					Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
				},
			},
			hubInfo:      hubInfo,
			existingObjs: []client.Object{amAccessorSecret, sourceAmAccessorSecret, clusterVersion},
		},
		{
			name: "Ignored resource - No action",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "some-other-cm",
					Namespace: "some-other-ns",
				},
			},
			hubInfo:      hubInfo,
			existingObjs: []client.Object{},
		},
		{
			name: "AlertmanagerEndpoint empty - Revert path",

			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
					Namespace: operatorconfig.OCPClusterMonitoringNamespace,
				},
			},
			hubInfo: &operatorconfig.HubInfo{AlertmanagerEndpoint: "", HubClusterID: "hub-id"},
			existingObjs: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      operatorconfig.OCPClusterMonitoringConfigMapName,
						Namespace: operatorconfig.OCPClusterMonitoringNamespace,
						ManagedFields: []metav1.ManagedFieldsEntry{
							{Manager: observabilityendpoint.EndpointMonitoringOperatorMgr},
						},
					},
					Data: map[string]string{
						observabilityendpoint.ClusterMonitoringConfigDataKey: "prometheusK8s: { additionalAlertmanagerConfigs: [ { scheme: https, tlsConfig: { ca: { name: hub-alertmanager-router-ca-hub-id } }, staticConfigs: [ hub.com ] } ] }",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(s).WithObjects(tt.existingObjs...).Build()
			recorder := record.NewFakeRecorder(10)

			// Capture initial metric value
			initialMetric := testutil.ToFloat64(cmoConfigConflictsTotal)

			r := NewMCOAAgentReconciler(
				c,
				ctrl.Log.WithName("test"),
				s,
				recorder,
				namespace,
				clusterID,
				tt.hubInfo,
				"hub-alertmanager-router-ca",
				"obs-alertmanager-mtls-cert",
				"observability-alertmanager-accessor",
			)

			_, err := r.Reconcile(context.Background(), tt.req)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

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

			// Verify object creation/update for CMO/UWL
			if !tt.expectedError {
				found := &corev1.ConfigMap{}
				err := c.Get(context.Background(), tt.req.NamespacedName, found)
				if tt.hubInfo.AlertmanagerEndpoint != "" {
					if tt.req.Name == operatorconfig.OCPClusterMonitoringConfigMapName || tt.req.Name == operatorconfig.OCPUserWorkloadMonitoringConfigMap {
						require.NoError(t, err)
						// staticConfigs contains the host only
						host := strings.TrimPrefix(tt.hubInfo.AlertmanagerEndpoint, "https://")
						assert.Contains(t, found.Data[observabilityendpoint.ClusterMonitoringConfigDataKey], host)
					}
				} else if tt.req.Name == operatorconfig.OCPClusterMonitoringConfigMapName {
					// Revert path
					if err == nil {
						assert.NotContains(t, found.Data[observabilityendpoint.ClusterMonitoringConfigDataKey], "hub.com")
					}
				}
			}
		})
	}
}
