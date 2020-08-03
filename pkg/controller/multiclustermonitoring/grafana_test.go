// Copyright (c) 2020 Red Hat, Inc.

package multiclustermonitoring

import (
	"testing"

	monitoringv1alpha1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/monitoring/v1alpha1"
)

func TestUpdateGrafanaSpec(t *testing.T) {
	mco := &monitoringv1alpha1.MultiClusterObservability{
		Spec: monitoringv1alpha1.MultiClusterMonitoringSpec{
			Grafana: &monitoringv1alpha1.GrafanaSpec{
				Hostport: defaultHostport,
			},
		},
	}

	updateGrafanaConfig(mco)

	if mco.Spec.Grafana.Replicas != 1 {
		t.Errorf("Replicas (%v) is not the expected (%v)", mco.Spec.Grafana.Replicas, defaultReplicas)
	}
}
