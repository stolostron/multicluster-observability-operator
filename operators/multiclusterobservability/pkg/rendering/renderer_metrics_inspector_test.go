// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rendering

import (
	"os"
	"path"
	"slices"
	"testing"
	"time"

	ocinfrav1 "github.com/openshift/api/config/v1"
	"github.com/stolostron/multicluster-observability-operator/metrics-inspector/pkg/estimator"
	mcoshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/rendering/templates"
	templatesutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering/templates"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/util/tlstesting"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestMetricsInspectorRendering(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir: %v", err)
	}
	templatesPath := path.Join(path.Dir(path.Dir(wd)), "manifests")
	t.Setenv(templatesutil.TemplatesPathEnvVar, templatesPath)
	templates.ResetTemplates()
	t.Cleanup(templates.ResetTemplates)

	ns := mcoconfig.GetDefaultNamespace()
	objSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "thanos-object-storage",
			Namespace: ns,
		},
		Data: map[string][]byte{
			"thanos.yaml": []byte("type: S3\nconfig:\n  bucket: test-bucket\n  endpoint: s3.amazonaws.com\n  http_config:\n    tls_config:\n      ca_file: /etc/minio/certs/ca.crt\n"),
		},
	}
	tlsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "thanos-objstore-tls",
			Namespace: ns,
		},
		Data: map[string][]byte{
			"ca.crt": []byte("fake-ca"),
		},
	}

	kubeClient := tlstesting.NewFakeTLSClientBuilder().
		WithScheme(corev1.AddToScheme).
		WithScheme(ocinfrav1.AddToScheme).
		WithObjects(objSecret, tlsSecret).
		Build(t)

	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{
			Name: "observability",
			Annotations: map[string]string{
				mcoconfig.AnnotationKeyImageRepository: "quay.io/stolostron",
				mcoconfig.AnnotationKeyImageTagSuffix:  "test-tag",
			},
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			InstanceSize: mcoconfig.Large,
			StorageConfig: &mcov1beta2.StorageConfig{
				MetricObjectStorage: &mcoshared.PreConfiguredStorage{
					Name:                     "thanos-object-storage",
					Key:                      "thanos.yaml",
					TLSSecretName:            "thanos-objstore-tls",
					ServiceAccountProjection: true,
				},
			},
			AdvancedConfig: &mcov1beta2.AdvancedConfig{
				Store: &mcov1beta2.StoreSpec{
					ServiceAccountAnnotations: map[string]string{
						"eks.amazonaws.com/role-arn": "arn:aws:iam::123456789012:role/thanos-s3-role",
					},
				},
				RetentionConfig: &mcov1beta2.RetentionConfig{
					RetentionResolutionRaw: "14d",
					RetentionResolution5m:  "90d",
					RetentionResolution1h:  "180d",
				},
			},
		},
	}

	if MetricsInspectorEnabled(mco) {
		t.Fatalf("expected MetricsInspectorEnabled to be false by default")
	}

	// Enable both MCOA platform metrics and the metrics inspector capability.
	mco.Spec.Capabilities = &mcov1beta2.CapabilitiesSpec{
		Platform: &mcov1beta2.PlatformCapabilitiesSpec{
			Metrics: mcov1beta2.PlatformMetricsSpec{
				Default: mcov1beta2.PlatformMetricsDefaultSpec{
					Enabled: true,
				},
				Inspector: mcov1beta2.MetricsInspectorSpec{
					Enabled: true,
				},
			},
		},
	}
	if !MetricsInspectorEnabled(mco) {
		t.Fatalf("expected MetricsInspectorEnabled to be true when enabled in CR")
	}

	renderer := NewMCORenderer(mco, kubeClient, nil)
	namespace, labels := renderer.NamespaceAndLabels()
	resources, err := renderer.MetricsInspectorResources(t.Context(), namespace, labels)
	if err != nil {
		t.Fatalf("MetricsInspectorResources failed: %v", err)
	}

	// With MCOA enabled, all 11 resources in manifests/base/metrics-inspector must be rendered:
	// 1 Deployment, 1 Service, 1 ServiceAccount, 1 ClusterRole, 1 ClusterRoleBinding,
	// 1 ServiceMonitor, 1 ScrapeConfig, and 4 ConfigMaps (config + 3 cardinality dashboards).
	if len(resources) != 11 {
		t.Fatalf("expected 11 rendered metrics-inspector resources, got %d", len(resources))
	}

	findResource := func(kind, name string) *unstructured.Unstructured {
		for _, r := range resources {
			if r.GetKind() == kind && r.GetName() == name {
				return r
			}
		}
		return nil
	}

	// 1. Validate ConfigMap (observability-metrics-inspector-config) and estimator YAML.
	cmUnstructured := findResource("ConfigMap", mcoconfig.MetricsInspectorConfigMapName)
	if cmUnstructured == nil {
		t.Fatalf("expected ConfigMap %q to be rendered", mcoconfig.MetricsInspectorConfigMapName)
	}
	cm := &corev1.ConfigMap{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(cmUnstructured.Object, cm); err != nil {
		t.Fatalf("FromUnstructured ConfigMap: %v", err)
	}
	parsedCfg, err := estimator.ParseConfig([]byte(cm.Data["config.yaml"]))
	if err != nil {
		t.Fatalf("ConfigMap YAML failed estimator.ParseConfig validation: %v", err)
	}
	if len(parsedCfg.Streams) != 4 {
		t.Fatalf("expected 4 configured cardinality streams, got %d", len(parsedCfg.Streams))
	}
	for i, s := range parsedCfg.Streams {
		if s.Interval != 15*time.Minute {
			t.Errorf("stream[%d] Interval = %s, want 15m", i, s.Interval)
		}
	}

	// 2. Validate the 3 cardinality Grafana dashboards and ScrapeConfig.
	for _, dashName := range []string{
		"grafana-dashboard-acm-metrics-cardinality-overview",
		"grafana-dashboard-acm-metrics-cardinality-cluster",
		"grafana-dashboard-acm-metrics-cardinality-name",
	} {
		if findResource("ConfigMap", dashName) == nil {
			t.Errorf("expected dashboard ConfigMap %q to be rendered", dashName)
		}
	}
	if findResource("ScrapeConfig", "platform-metrics-cardinality") == nil {
		t.Errorf("expected ScrapeConfig platform-metrics-cardinality to be rendered when MCOA is enabled")
	}

	// 3. Validate ServiceAccount annotations, ClusterRole, ClusterRoleBinding, Service, and ServiceMonitor.
	saUnstructured := findResource("ServiceAccount", mcoconfig.MetricsInspectorServiceName)
	if saUnstructured == nil {
		t.Fatalf("expected ServiceAccount %q to be rendered", mcoconfig.MetricsInspectorServiceName)
	}
	if saUnstructured.GetAnnotations()["eks.amazonaws.com/role-arn"] != "arn:aws:iam::123456789012:role/thanos-s3-role" {
		t.Errorf("unexpected ServiceAccount annotations: %+v", saUnstructured.GetAnnotations())
	}
	if findResource("ClusterRole", mcoconfig.MetricsInspectorServiceName) == nil {
		t.Errorf("expected ClusterRole %q to be rendered", mcoconfig.MetricsInspectorServiceName)
	}
	if findResource("ClusterRoleBinding", mcoconfig.MetricsInspectorServiceName) == nil {
		t.Errorf("expected ClusterRoleBinding %q to be rendered", mcoconfig.MetricsInspectorServiceName)
	}
	svcUnstructured := findResource("Service", mcoconfig.MetricsInspectorServiceName)
	if svcUnstructured == nil {
		t.Fatalf("expected Service %q to be rendered", mcoconfig.MetricsInspectorServiceName)
	}
	if svcUnstructured.GetAnnotations()["service.beta.openshift.io/serving-cert-secret-name"] != "observability-metrics-inspector-metrics-certs" {
		t.Errorf("expected OpenShift serving-cert annotation on Service, got %+v", svcUnstructured.GetAnnotations())
	}
	if findResource("ServiceMonitor", mcoconfig.MetricsInspectorServiceName) == nil {
		t.Errorf("expected ServiceMonitor %q to be rendered", mcoconfig.MetricsInspectorServiceName)
	}

	// 4. Validate Deployment (metrics-inspector container + kube-rbac-proxy sidecar).
	depUnstructured := findResource("Deployment", mcoconfig.MetricsInspectorServiceName)
	if depUnstructured == nil {
		t.Fatalf("expected Deployment %q to be rendered", mcoconfig.MetricsInspectorServiceName)
	}
	dep := &appsv1.Deployment{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(depUnstructured.Object, dep); err != nil {
		t.Fatalf("FromUnstructured Deployment: %v", err)
	}
	if len(dep.Spec.Template.Spec.Containers) != 2 {
		t.Fatalf("expected 2 containers (metrics-inspector and kube-rbac-proxy), got %d", len(dep.Spec.Template.Spec.Containers))
	}
	inspectorContainer := dep.Spec.Template.Spec.Containers[0]
	rbacProxyContainer := dep.Spec.Template.Spec.Containers[1]
	if inspectorContainer.Image != "quay.io/stolostron/metrics-inspector:test-tag" {
		t.Errorf("inspectorContainer.Image = %q", inspectorContainer.Image)
	}
	if inspectorContainer.Resources.Requests.Memory().String() != "2Gi" {
		t.Errorf("expected Large memory request 2Gi, got %s", inspectorContainer.Resources.Requests.Memory().String())
	}
	for _, wantArg := range []string{
		"--config.file=/etc/metrics-inspector/config.yaml",
		"--estimator.interval=15m",
		"--estimator.max-items-per-interval=15000000",
		"--web.listen-address=:8490",
		"--metrics.listen-address=127.0.0.1:8080",
		"--tls.server.cert-file=/var/metrics-inspector/server-certs/tls.crt",
		"--tls.server.key-file=/var/metrics-inspector/server-certs/tls.key",
		"--tls.client-ca-file=/var/metrics-inspector/client-ca/ca.crt",
		"--remote-write.interval=60s",
		"--remote-write.initial-delay=6m",
		"--objstore.config-file=/etc/metrics-inspector/objstore/thanos.yaml",
		"--bucket-inspect.interval=15m",
		"--retention.resolution-raw=14d",
		"--retention.resolution-5m=90d",
		"--retention.resolution-1h=180d",
		"--tls-min-version=VersionTLS12",
	} {
		if !slices.Contains(inspectorContainer.Args, wantArg) {
			t.Errorf("expected %q in inspectorContainer.Args, got %v", wantArg, inspectorContainer.Args)
		}
	}
	if !slices.ContainsFunc(inspectorContainer.Args, func(arg string) bool {
		return len(arg) > len("--tls-cipher-suites=") && arg[:len("--tls-cipher-suites=")] == "--tls-cipher-suites="
	}) {
		t.Errorf("expected --tls-cipher-suites=<ciphers> in inspectorContainer.Args, got %v", inspectorContainer.Args)
	}
	if rbacProxyContainer.Name != "kube-rbac-proxy" {
		t.Errorf("second container Name = %q, want kube-rbac-proxy", rbacProxyContainer.Name)
	}
	initialHash := dep.Spec.Template.Annotations[metricsInspectorConfigHashKey]
	if initialHash == "" {
		t.Fatalf("expected non-empty %s annotation on Deployment PodTemplate", metricsInspectorConfigHashKey)
	}

	// Verify that modifying the observability-metrics-inspector-config ConfigMap data changes config-hash.
	customCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcoconfig.MetricsInspectorConfigMapName,
			Namespace: ns,
		},
		Data: map[string]string{
			"config.yaml": "streams:\n  - interval: 15m\n    group_by: [cluster]\n",
		},
	}
	if err := kubeClient.Create(t.Context(), customCM); err != nil {
		t.Fatalf("failed to create custom ConfigMap in fake client: %v", err)
	}
	updatedResources, err := renderer.MetricsInspectorResources(t.Context(), namespace, labels)
	if err != nil {
		t.Fatalf("MetricsInspectorResources after ConfigMap update failed: %v", err)
	}
	for _, r := range updatedResources {
		if r.GetKind() == "Deployment" && r.GetName() == mcoconfig.MetricsInspectorServiceName {
			updatedDep := &appsv1.Deployment{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(r.Object, updatedDep); err != nil {
				t.Fatalf("FromUnstructured updatedDep: %v", err)
			}
			newHash := updatedDep.Spec.Template.Annotations[metricsInspectorConfigHashKey]
			if newHash == initialHash {
				t.Errorf("expected %s to change when ConfigMap data changes, still %q", metricsInspectorConfigHashKey, newHash)
			}
		}
	}

	// 5. In legacy collector mode (MCOA disabled), ScrapeConfig is omitted during render
	// but included in MetricsInspectorResourcesForRemoval.
	mco.Spec.Capabilities.Platform.Metrics.Default.Enabled = false
	legacyResources, err := renderer.MetricsInspectorResources(t.Context(), namespace, labels)
	if err != nil {
		t.Fatalf("MetricsInspectorResources (legacy mode) failed: %v", err)
	}
	if len(legacyResources) != 10 {
		t.Fatalf("expected 10 resources in legacy collector mode (no ScrapeConfig), got %d", len(legacyResources))
	}
	removalResources, err := renderer.MetricsInspectorResourcesForRemoval(t.Context(), namespace, labels)
	if err != nil {
		t.Fatalf("MetricsInspectorResourcesForRemoval failed: %v", err)
	}
	if len(removalResources) != 11 {
		t.Fatalf("expected 11 resources in MetricsInspectorResourcesForRemoval, got %d", len(removalResources))
	}
}

func TestResolveMetricsInspectorMaxItems(t *testing.T) {
	cases := []struct {
		size mcov1beta2.TShirtSize
		want int
	}{
		{size: "", want: 500000},
		{size: mcoconfig.Default, want: 500000},
		{size: mcoconfig.Minimal, want: 1500000},
		{size: mcoconfig.Small, want: 1500000},
		{size: mcoconfig.Medium, want: 7500000},
		{size: mcoconfig.Large, want: 15000000},
		{size: mcoconfig.XLarge, want: 30000000},
		{size: mcoconfig.TwoXLarge, want: 75000000},
		{size: mcoconfig.FourXLarge, want: 150000000},
	}
	for _, tc := range cases {
		if got := resolveMetricsInspectorMaxItems(tc.size); got != tc.want {
			t.Errorf("resolveMetricsInspectorMaxItems(%q) = %d, want %d", tc.size, got, tc.want)
		}
	}
}
