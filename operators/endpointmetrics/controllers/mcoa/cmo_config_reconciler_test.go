// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package mcoa

import (
	"testing"

	cmomanifests "github.com/openshift/cluster-monitoring-operator/pkg/manifests"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/observabilityendpoint"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/yaml"
)

func TestCMOConfigReconciler_detectConflict(t *testing.T) {
	t.Parallel()
	hubInfo := &operatorconfig.HubInfo{
		HubClusterID: "hub-id",
	}

	validCfg := cmomanifests.ClusterMonitoringConfiguration{
		PrometheusK8sConfig: &cmomanifests.PrometheusK8sConfig{
			AlertmanagerConfigs: []cmomanifests.AdditionalAlertmanagerConfig{
				{
					TLSConfig: cmomanifests.TLSConfig{
						CA: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "hub-alertmanager-router-ca-hub-id",
							},
						},
					},
				},
			},
		},
	}
	validYAML, _ := yaml.Marshal(validCfg)

	tests := []struct {
		name     string
		cm       *corev1.ConfigMap
		expected bool
	}{
		{
			name:     "Not managed yet",
			cm:       &corev1.ConfigMap{},
			expected: false,
		},
		{
			name: "Managed but missing data",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					ManagedFields: []metav1.ManagedFieldsEntry{
						{Manager: observabilityendpoint.EndpointMonitoringOperatorMgr},
					},
				},
			},
			expected: true,
		},
		{
			name: "Managed and correct config",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					ManagedFields: []metav1.ManagedFieldsEntry{
						{Manager: observabilityendpoint.EndpointMonitoringOperatorMgr},
					},
				},
				Data: map[string]string{
					observabilityendpoint.ClusterMonitoringConfigDataKey: string(validYAML),
				},
			},
			expected: false,
		},
		{
			name: "Managed and incorrect config (conflict)",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					ManagedFields: []metav1.ManagedFieldsEntry{
						{Manager: observabilityendpoint.EndpointMonitoringOperatorMgr},
					},
				},
				Data: map[string]string{
					observabilityendpoint.ClusterMonitoringConfigDataKey: "prometheusK8s: {}",
				},
			},
			expected: true,
		},
		{
			name: "Managed and corrupted YAML",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					ManagedFields: []metav1.ManagedFieldsEntry{
						{Manager: observabilityendpoint.EndpointMonitoringOperatorMgr},
					},
				},
				Data: map[string]string{
					observabilityendpoint.ClusterMonitoringConfigDataKey: "{invalid: yaml: :}",
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &cmoConfigReconciler{
				Log: ctrl.Log.WithName("test"),
			}
			assert.Equal(t, tt.expected, r.detectConflict(tt.cm, hubInfo))
		})
	}
}
