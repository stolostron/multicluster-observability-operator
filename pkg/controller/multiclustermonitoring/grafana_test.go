// Copyright (c) 2020 Red Hat, Inc.

package multiclustermonitoring

import (
	"testing"

	monitoringv1alpha1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/monitoring/v1alpha1"
)

func TestUpdateGrafanaSpec(t *testing.T) {
	mcm := &monitoringv1alpha1.MultiClusterMonitoring{
		Spec: monitoringv1alpha1.MultiClusterMonitoringSpec{
			Grafana: &monitoringv1alpha1.GrafanaSpec{
				Hostport: defaultHostport,
			},
		},
	}

	updateGrafanaConfig(mcm)

	if mcm.Spec.Grafana.Replicas != 1 {
		t.Errorf("Replicas (%v) is not the expected (%v)", mcm.Spec.Grafana.Replicas, defaultReplicas)
	}
}
