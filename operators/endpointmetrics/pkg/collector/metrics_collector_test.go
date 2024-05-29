// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package collector_test

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubectl/pkg/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/go-logr/logr"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/collector"
	oashared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
)

const (
	metricsCollectorName    = "metrics-collector-deployment"
	namespace               = "testNamespace"
	uwlMetricsCollectorName = "uwl-metrics-collector-deployment"
	uwlNamespace            = "openshift-user-workload-monitoring"
	uwlSts                  = "prometheus-user-workload"
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

func newUwlPrometheus() *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uwlSts,
			Namespace: uwlNamespace,
		},
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

	// namespace = testNamespace
	// hubNamespace = testHubNamspace
}

func checkAnnotationsAndProxySettings(ctx context.Context, c client.Client, deploymentName string, t *testing.T) {
	deployment := &appsv1.Deployment{}
	err := c.Get(ctx, types.NamespacedName{Name: deploymentName,
		Namespace: namespace}, deployment)
	if err != nil {
		t.Fatalf("Failed to query deployment: %v, err: (%v)", deploymentName, err)
	}

	v, ok := deployment.Spec.Template.Annotations[operatorconfig.WorkloadPartitioningPodAnnotationKey]
	if !ok || v != operatorconfig.WorkloadPodExpectedValueJSON {
		t.Fatalf("Failed to find annotation %v: %v on the pod spec of deployment: %v",
			operatorconfig.WorkloadPartitioningPodAnnotationKey,
			operatorconfig.WorkloadPodExpectedValueJSON,
			deploymentName,
		)
	}

	env := deployment.Spec.Template.Spec.Containers[0].Env
	envChecks := map[string]string{
		"HTTP_PROXY":            "http://foo.com",
		"HTTPS_PROXY":           "https://foo.com",
		"NO_PROXY":              "bar.com",
		"HTTPS_PROXY_CA_BUNDLE": "custom-ca.crt",
	}
	foundEnv := map[string]bool{}
	for _, e := range env {
		if v, ok := envChecks[e.Name]; ok {
			if e.Value != v {
				t.Fatalf("Env var %v is not set correctly: expected %v, got %v", e.Name, v, e.Value)
			}
			foundEnv[e.Name] = true
		}
	}
	for k := range envChecks {
		if !foundEnv[k] {
			t.Fatalf("Env var %v is not present in env", k)
		}
	}
}

func TestMetricsCollector(t *testing.T) {
	hubInfo := &operatorconfig.HubInfo{
		ClusterName:              "test-cluster",
		ObservatoriumAPIEndpoint: "http://test-endpoint",
	}

	objs := []runtime.Object{getAllowlistCM(),
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "extension-apiserver-authentication",
				Namespace: "kube-system",
			},
			Data: map[string]string{
				"client-ca-file": "test",
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "endpoint-observability-operator",
				Namespace: namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "endpoint-observability-operator",
								Env: []corev1.EnvVar{
									{
										Name:  "HTTP_PROXY",
										Value: "http://foo.com",
									},
									{
										Name:  "HTTPS_PROXY",
										Value: "https://foo.com",
									},
									{
										Name:  "NO_PROXY",
										Value: "bar.com",
									},
									{
										Name:  "HTTPS_PROXY_CA_BUNDLE",
										Value: "custom-ca.crt",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	promv1.AddToScheme(scheme.Scheme)
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(objs...).Build()

	baseMetricsCollector := &collector.MetricsCollector{
		Client: c,
		ClusterInfo: collector.ClusterInfo{
			ClusterID: "test-cluster",
		},
		HubInfo:   hubInfo,
		Log:       logr.Logger{},
		Namespace: namespace,
		ObsAddonSpec: &oashared.ObservabilityAddonSpec{
			EnableMetrics: true,
			Interval:      60,
		},
		ServiceAccountName: "test-sa",
	}

	metricsCollector := cloneMetricsCollector(baseMetricsCollector)
	if err := metricsCollector.Update(context.Background(), ctrl.Request{}); err != nil {
		t.Fatalf("Failed to update metrics collector: %v", err)
	}

	checkAnnotationsAndProxySettings(context.Background(), c, metricsCollectorName, t)

	// Replicas should be 0 when metrics is disabled and is not hub collector
	metricsCollector = cloneMetricsCollector(baseMetricsCollector)
	metricsCollector.ObsAddonSpec.EnableMetrics = false
	metricsCollector.ClusterInfo.IsHubMetricsCollector = false
	if err := metricsCollector.Update(context.Background(), ctrl.Request{}); err != nil {
		t.Fatalf("Failed to update metrics collector: %v", err)
	}

	deployment := getMetricsCollectorDeployment(t, context.Background(), c, metricsCollectorName)
	if *deployment.Spec.Replicas != 0 {
		t.Fatalf("Replicas should be 0 when metrics is disabled and is not hub collector")
	}

	// Hub metrics collector should have 1 replica even if metrics is disabled
	metricsCollector = cloneMetricsCollector(baseMetricsCollector)
	metricsCollector.ObsAddonSpec.EnableMetrics = false
	metricsCollector.ClusterInfo.IsHubMetricsCollector = true
	if err := metricsCollector.Update(context.Background(), ctrl.Request{}); err != nil {
		t.Fatalf("Failed to update metrics collector: %v", err)
	}

	deployment = getMetricsCollectorDeployment(t, context.Background(), c, metricsCollectorName)
	if *deployment.Spec.Replicas != 1 {
		t.Fatalf("Hub metrics collector should have 1 replica even if metrics is disabled")
	}

	// Should force reload if certs are updated
	certs := []string{
		"observability-controller-open-cluster-management.io-observability-signer-client-cert",
		"observability-managed-cluster-certs",
		"observability-server-ca-certs",
	}
	metricsCollector = cloneMetricsCollector(baseMetricsCollector)
	for _, cert := range certs {
		// Set running replicas to 1
		deployment = getMetricsCollectorDeployment(t, context.Background(), c, metricsCollectorName)
		deployment.Status.ReadyReplicas = 1
		if err := c.Update(context.Background(), deployment); err != nil {
			t.Fatalf("Failed to update deployment: %v", err)
		}

		// Update with cert request
		if err := metricsCollector.Update(context.Background(), ctrl.Request{types.NamespacedName{Name: cert}}); err != nil {
			t.Fatalf("Failed to update metrics collector: %v", err)
		}
		deployment = getMetricsCollectorDeployment(t, context.Background(), c, metricsCollectorName)
		if _, ok := deployment.Spec.Template.ObjectMeta.Labels["cert/time-restarted"]; !ok {
			t.Fatalf("Should force reload if certs are updated. Label not found: %v", deployment.Spec.Template.ObjectMeta.Labels)
		}

		// Reset the label
		deployment.Spec.Template.ObjectMeta.Labels["cert/time-restarted"] = ""
		if err := c.Update(context.Background(), deployment); err != nil {
			t.Fatalf("Failed to update deployment: %v", err)
		}
	}

	// Should not create a uwl metrics collector if no custom allowlist is present
	metricsCollector = cloneMetricsCollector(baseMetricsCollector)
	metricsCollector.ObsAddonSpec.EnableMetrics = true
	if err := metricsCollector.Update(context.Background(), ctrl.Request{}); err != nil {
		t.Fatalf("Failed to update metrics collector: %v", err)
	}

	deployment = &appsv1.Deployment{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: uwlMetricsCollectorName, Namespace: namespace}, deployment); err != nil {
		if !errors.IsNotFound(err) {
			t.Fatalf("Failed to get deployment %s/%s: %v", namespace, uwlMetricsCollectorName, err)
		}
	} else {
		t.Fatalf("Should not create a uwl metrics collector if no custom allowlist is present")
	}

	// It should deploy uwl metrics collector if a custom allowlist is present and uwl prometheus is present
	// c = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(append(objs, getCustomAllowlistCM(), newUwlPrometheus())...).Build()
	c = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(append(objs, newUwlPrometheus())...).Build()
	baseMetricsCollector.Client = c

	metricsCollector = cloneMetricsCollector(baseMetricsCollector)
	metricsCollector.ObsAddonSpec.EnableMetrics = true
	if err := metricsCollector.Update(context.Background(), ctrl.Request{}); err != nil {
		t.Fatalf("Failed to update metrics collector: %v", err)
	}

	_ = getMetricsCollectorDeployment(t, context.Background(), c, uwlMetricsCollectorName)
	// fmt.Println(deployment.Spec.Template.Spec.Containers[0].Args)

	// Should delete uwl metrics collector if uwl prometheus is removed
	if err := c.Delete(context.Background(), newUwlPrometheus()); err != nil {
		t.Fatalf("Failed to delete custom allowlist configmap: %v", err)
	}
	if err := metricsCollector.Update(context.Background(), ctrl.Request{}); err != nil {
		t.Fatalf("Failed to update metrics collector: %v", err)
	}

	deployment = &appsv1.Deployment{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: uwlMetricsCollectorName, Namespace: namespace}, deployment); err != nil {
		if !errors.IsNotFound(err) {
			t.Fatalf("Failed to get deployment %s/%s: %v", namespace, uwlMetricsCollectorName, err)
		}
	} else {
		t.Fatalf("Should not create a uwl metrics collector if no custom allowlist is present")
	}

	// err = deleteMetricsCollector(ctx, c, uwlMetricsCollectorName)
	// if err != nil {
	// 	t.Fatalf("Failed to delete uwl metrics collector deployment: (%v)", err)
	// }
}

func getMetricsCollectorDeployment(t *testing.T, ctx context.Context, c client.Client, name string) *appsv1.Deployment {
	deployment := &appsv1.Deployment{}
	err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, deployment)
	if err != nil {
		t.Fatalf("Failed to get deployment %s/%s: %v", namespace, name, err)
	}
	return deployment
}

func cloneMetricsCollector(original *collector.MetricsCollector) *collector.MetricsCollector {
	ret := *original
	hubInfo := *original.HubInfo
	ret.HubInfo = &hubInfo
	addon := *original.ObsAddonSpec
	ret.ObsAddonSpec = &addon
	return &ret
}
