// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project.
package observabilityendpoint

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	oashared "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	oav1beta1 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	operatorconfig "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/config"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
)

func getAllowlistCM() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconfig.AllowlistConfigMapName,
			Namespace: namespace,
		},
		Data: map[string]string{
			metricsConfigMapKey: `
names:
  - a
  - b
matches:
  - c
rules:
  - record: f
    expr: g
`},
	}
}

func init() {
	s := scheme.Scheme
	addonv1alpha1.AddToScheme(s)
	oav1beta1.AddToScheme(s)

	namespace = testNamespace
	hubNamespace = testHubNamspace
}

func TestMetricsCollector(t *testing.T) {
	hubInfo := &operatorconfig.HubInfo{
		ClusterName:              "test-cluster",
		ObservatoriumAPIEndpoint: "http://test-endpoint",
	}
	allowlistCM := getAllowlistCM()
	obsAddon := oashared.ObservabilityAddonSpec{
		EnableMetrics: true,
		Interval:      60,
	}

	ctx := context.TODO()
	c := fake.NewFakeClient(allowlistCM)
	// Default deployment with instance count 1
	_, err := updateMetricsCollector(ctx, c, obsAddon, *hubInfo, testClusterID, "", 1, false)
	if err != nil {
		t.Fatalf("Failed to create metrics collector deployment: (%v)", err)
	}
	// Update deployment to reduce instance count to zero
	_, err = updateMetricsCollector(ctx, c, obsAddon, *hubInfo, testClusterID, "", 0, false)
	if err != nil {
		t.Fatalf("Failed to create metrics collector deployment: (%v)", err)
	}

	_, err = updateMetricsCollector(ctx, c, obsAddon, *hubInfo, testClusterID+"-update", "SNO", 1, false)
	if err != nil {
		t.Fatalf("Failed to create metrics collector deployment: (%v)", err)
	}

	_, err = updateMetricsCollector(ctx, c, obsAddon, *hubInfo, testClusterID+"-update", "SNO", 1, true)
	if err != nil {
		t.Fatalf("Failed to update metrics collector deployment: (%v)", err)
	}

	err = deleteMetricsCollector(ctx, c)
	if err != nil {
		t.Fatalf("Failed to delete metrics collector deployment: (%v)", err)
	}
}
