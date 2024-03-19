// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package observabilityendpoint

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	oashared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
)

func getAllowlistCM() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconfig.AllowlistConfigMapName,
			Namespace: namespace,
		},
		Data: map[string]string{
			operatorconfig.MetricsConfigMapKey: `
names:
  - a
  - b
matches:
  - __name__="c"
recording_rules:
  - record: f
    expr: g
collect_rules:
  - name: h
    selector:
      matchExpressions:
        - key: clusterType
          operator: NotIn
          values: ["SNO"]
    rules:
      - collect: j
        expr: k
        for: 1m
        names:
          - c
        matches:
          - __name__="a"
`,
			operatorconfig.UwlMetricsConfigMapKey: `
names:
  - uwl_a
  - uwl_b
`},
	}
}

func getCustomAllowlistCM() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconfig.AllowlistCustomConfigMapName,
			Namespace: "default",
		},
		Data: map[string]string{
			operatorconfig.UwlMetricsConfigMapKey: `
names:
  - custom_c
matches:
  - __name__=test
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

func checkAnnotationsAndProxySettings(
	ctx context.Context,
	c client.Client,
	deploymentName string,
	t *testing.T) {

	deployment := &appsv1.Deployment{}
	err := c.Get(ctx, types.NamespacedName{Name: deploymentName,
		Namespace: namespace}, deployment)
	if err != nil {
		t.Fatalf("Failed to query deployment: %v, err: (%v)", deploymentName, err)
	}

	annotations := deployment.Spec.Template.Annotations
	v, found := annotations[operatorconfig.WorkloadPartitioningPodAnnotationKey]
	if !found || v != operatorconfig.WorkloadPodExpectedValueJSON {
		t.Fatalf("Failed to find annotation %v: %v on the pod spec of deployment: %v",
			operatorconfig.WorkloadPartitioningPodAnnotationKey,
			operatorconfig.WorkloadPodExpectedValueJSON,
			deploymentName,
		)
	}

	env := deployment.Spec.Template.Spec.Containers[0].Env
	foundHTTPProxy := false
	foundHTTPSProxy := false
	foundNOProxy := false
	foundCABundle := false
	for _, e := range env {
		if e.Name == "HTTP_PROXY" {
			foundHTTPProxy = true
			if e.Value != "http://foo.com" {
				t.Fatalf("HTTP_PROXY is not set correctly: expected %s, got %s", "http://foo.com", e.Value)
			}
		} else if e.Name == "HTTPS_PROXY" {
			foundHTTPSProxy = true
			if e.Value != "https://foo.com" {
				t.Fatalf("HTTPS_PROXY is not set correctly: expected %s, got %s", "https://foo.com", e.Value)
			}
		} else if e.Name == "NO_PROXY" {
			foundNOProxy = true
			if e.Value != "bar.com" {
				t.Fatalf("NO_PROXY is not set correctly: expected %s, got %s", "bar.com", e.Value)
			}
		} else if e.Name == "HTTPS_PROXY_CA_BUNDLE" {
			foundCABundle = true
			if e.Value != "custom-ca.crt" {
				t.Fatalf("HTTPS_PROXY_CA_BUNDLE is not set correctly: expected %s, got %s", "custom-ca.crt", e.Value)
			}
		}
	}
	if !foundHTTPProxy {
		t.Fatalf("HTTP_PROXY is not present in env")
	}
	if !foundHTTPSProxy {
		t.Fatalf("HTTPS_PROXY is not present in env")
	}
	if !foundNOProxy {
		t.Fatalf("NO_PROXY is not present in env")
	}
	if !foundCABundle {
		t.Fatalf("HTTPS_PROXY_CA_BUNDLE is not present in env")
	}
}

func TestMetricsCollector(t *testing.T) {
	hubInfo := &operatorconfig.HubInfo{
		ClusterName:              "test-cluster",
		ObservatoriumAPIEndpoint: "http://test-endpoint",
	}
	obsAddon := oashared.ObservabilityAddonSpec{
		EnableMetrics: true,
		Interval:      60,
	}

	ctx := context.TODO()
	objs := []runtime.Object{getAllowlistCM(), getCustomAllowlistCM(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "extension-apiserver-authentication",
			Namespace: "kube-system",
		},
		Data: map[string]string{
			"client-ca-file": "test",
		},
	}}
	promv1.AddToScheme(scheme.Scheme)
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(objs...).Build()

	list, uwlList, err := getMetricsAllowlist(ctx, c, "")
	if err != nil {
		t.Fatalf("Failed to get allowlist: (%v)", err)
	}
	// Default deployment with instance count 1
	params := CollectorParams{
		isUWL:        false,
		clusterID:    testClusterID,
		clusterType:  defaultClusterType,
		obsAddonSpec: obsAddon,
		hubInfo:      *hubInfo,
		allowlist:    list,
		replicaCount: 1,
		httpProxy:    "http://foo.com",
		httpsProxy:   "https://foo.com",
		noProxy:      "bar.com",
		CABundle:     "custom-ca.crt",
	}

	_, err = updateMetricsCollector(ctx, c, params, false)
	if err != nil {
		t.Fatalf("Failed to create metrics collector deployment: (%v)", err)
	}
	checkAnnotationsAndProxySettings(ctx, c, metricsCollectorName, t)

	// Update deployment to reduce instance count to zero
	params.replicaCount = 0
	_, err = updateMetricsCollector(ctx, c, params, false)
	if err != nil {
		t.Fatalf("Failed to create metrics collector deployment: (%v)", err)
	}
	checkAnnotationsAndProxySettings(ctx, c, metricsCollectorName, t)

	params.replicaCount = 1
	params.clusterID = testClusterID + "-update"
	params.clusterType = snoClusterType
	_, err = updateMetricsCollector(ctx, c, params, false)
	if err != nil {
		t.Fatalf("Failed to create metrics collector deployment: (%v)", err)
	}
	checkAnnotationsAndProxySettings(ctx, c, metricsCollectorName, t)

	_, err = updateMetricsCollector(ctx, c, params, true)
	if err != nil {
		t.Fatalf("Failed to update metrics collector deployment: (%v)", err)
	}
	checkAnnotationsAndProxySettings(ctx, c, metricsCollectorName, t)

	params.isUWL = true
	params.allowlist = uwlList
	_, err = updateMetricsCollector(ctx, c, params, true)
	if err != nil {
		t.Fatalf("Failed to create uwl metrics collector deployment: (%v)", err)
	}
	checkAnnotationsAndProxySettings(ctx, c, uwlMetricsCollectorName, t)

	err = deleteMetricsCollector(ctx, c, metricsCollectorName)
	if err != nil {
		t.Fatalf("Failed to delete metrics collector deployment: (%v)", err)
	}

	err = deleteMetricsCollector(ctx, c, uwlMetricsCollectorName)
	if err != nil {
		t.Fatalf("Failed to delete uwl metrics collector deployment: (%v)", err)
	}
}
