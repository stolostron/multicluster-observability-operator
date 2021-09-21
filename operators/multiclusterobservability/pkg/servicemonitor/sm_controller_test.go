// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package servicemonitor

import (
	"testing"

	"github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRewriteLabels(t *testing.T) {
	sm := &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: ocpMonitoringNamespace,
		},
		Spec: promv1.ServiceMonitorSpec{
			Endpoints: []promv1.Endpoint{
				{
					Path: "test",
				},
			},
		},
	}
	updated := rewriteLabels(sm, "")
	if len(updated.Spec.NamespaceSelector.MatchNames) == 0 || updated.Spec.NamespaceSelector.MatchNames[0] != config.GetDefaultNamespace() {
		t.Errorf("Wrong NamespaceSelector: %v", updated.Spec.NamespaceSelector)
	}
	if len(updated.Spec.Endpoints[0].MetricRelabelConfigs) != 1 {
		t.Errorf("Wrong MetricRelabelConfigs: %v", updated.Spec.Endpoints[0].MetricRelabelConfigs)
	}
}
