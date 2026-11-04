// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package multiclusterobservability

import (
	"context"
	"slices"
	"testing"
	"time"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stolostron/multicluster-observability-operator/metrics-inspector/pkg/estimator"
	mcoshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	mcoutil "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	observatoriumv1alpha1 "github.com/stolostron/observatorium-operator/api/v1alpha1"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGenerateMetricsInspectorResources(t *testing.T) {
	ctx := context.Background()
	s := runtime.NewScheme()
	if err := scheme.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	if err := mcov1beta2.SchemeBuilder.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme mco: %v", err)
	}
	if err := observatoriumv1alpha1.SchemeBuilder.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme observatorium: %v", err)
	}
	if err := monitoringv1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme monitoringv1: %v", err)
	}

	objSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "thanos-object-storage",
			Namespace: mcoconfig.GetDefaultNamespace(),
		},
		Data: map[string][]byte{
			"thanos.yaml": []byte("type: S3\nconfig:\n  bucket: test-bucket\n  endpoint: s3.amazonaws.com\n  http_config:\n    tls_config:\n      ca_file: /etc/minio/certs/ca.crt\n"),
		},
	}
	tlsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "thanos-objstore-tls",
			Namespace: mcoconfig.GetDefaultNamespace(),
		},
		Data: map[string][]byte{
			"ca.crt": []byte("fake-ca"),
		},
	}

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

	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(mco, objSecret, tlsSecret, alertmanagerCABundleConfigMap()).Build()
	mcoconfig.CleanUpOperandNames()
	if err := mcoconfig.SetOperandNames(cl); err != nil {
		t.Fatalf("SetOperandNames: %v", err)
	}

	// First reconcile: creates ConfigMap, Service, and Deployment.
	if _, err := GenerateMetricsInspectorResources(ctx, cl, s, mco); err != nil {
		t.Fatalf("GenerateMetricsInspectorResources failed: %v", err)
	}

	// Verify ConfigMap.
	cm := &corev1.ConfigMap{}
	if err := cl.Get(ctx, types.NamespacedName{
		Name:      mcoconfig.MetricsInspectorConfigMapName,
		Namespace: mcoconfig.GetDefaultNamespace(),
	}, cm); err != nil {
		t.Fatalf("failed to get ConfigMap: %v", err)
	}
	if cm.Data[metricsInspectorConfigFileKey] == "" {
		t.Errorf("expected non-empty %s in ConfigMap", metricsInspectorConfigFileKey)
	}
	parsedCfg, err := estimator.ParseConfig([]byte(cm.Data[metricsInspectorConfigFileKey]))
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

	// Verify ServiceAccount.
	sa := &corev1.ServiceAccount{}
	if err := cl.Get(ctx, types.NamespacedName{
		Name:      mcoconfig.MetricsInspectorServiceName,
		Namespace: mcoconfig.GetDefaultNamespace(),
	}, sa); err != nil {
		t.Fatalf("failed to get ServiceAccount: %v", err)
	}
	if sa.Annotations["eks.amazonaws.com/role-arn"] != "arn:aws:iam::123456789012:role/thanos-s3-role" {
		t.Errorf("unexpected ServiceAccount annotations: %+v", sa.Annotations)
	}

	// Verify Service.
	svc := &corev1.Service{}
	if err := cl.Get(ctx, types.NamespacedName{
		Name:      mcoconfig.MetricsInspectorServiceName,
		Namespace: mcoconfig.GetDefaultNamespace(),
	}, svc); err != nil {
		t.Fatalf("failed to get Service: %v", err)
	}
	if len(svc.Spec.Ports) != 1 || svc.Spec.Ports[0].Port != metricsInspectorPort {
		t.Errorf("unexpected Service ports: %+v", svc.Spec.Ports)
	}

	// Verify ServiceMonitor.
	sm := &monitoringv1.ServiceMonitor{}
	if err := cl.Get(ctx, types.NamespacedName{
		Name:      mcoconfig.MetricsInspectorServiceName,
		Namespace: mcoconfig.GetDefaultNamespace(),
	}, sm); err != nil {
		t.Fatalf("failed to get ServiceMonitor: %v", err)
	}
	if len(sm.Spec.Endpoints) != 1 || sm.Spec.Endpoints[0].Port != "http" || sm.Spec.Endpoints[0].Path != "/metrics" {
		t.Errorf("unexpected ServiceMonitor endpoints: %+v", sm.Spec.Endpoints)
	}

	// Verify Deployment.
	deploy := &appsv1.Deployment{}
	if err := cl.Get(ctx, types.NamespacedName{
		Name:      mcoconfig.GetOperandName(mcoconfig.MetricsInspector),
		Namespace: mcoconfig.GetDefaultNamespace(),
	}, deploy); err != nil {
		t.Fatalf("failed to get Deployment: %v", err)
	}
	if deploy.Spec.Replicas == nil || *deploy.Spec.Replicas != 1 {
		t.Errorf("expected 1 replica, got %v", deploy.Spec.Replicas)
	}
	if deploy.Spec.Template.Spec.ServiceAccountName != mcoconfig.MetricsInspectorServiceName {
		t.Errorf("ServiceAccountName = %q, want %q", deploy.Spec.Template.Spec.ServiceAccountName, mcoconfig.MetricsInspectorServiceName)
	}
	if len(deploy.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(deploy.Spec.Template.Spec.Containers))
	}
	container := deploy.Spec.Template.Spec.Containers[0]
	wantImg := "quay.io/stolostron/metrics-inspector:test-tag"
	if container.Image != wantImg {
		t.Errorf("container image = %q, want %q", container.Image, wantImg)
	}
	if container.Resources.Requests.Memory().String() != "2Gi" {
		t.Errorf("expected Large memory request 2Gi, got %s", container.Resources.Requests.Memory().String())
	}
	if !slices.Contains(container.Args, "--estimator.interval=15m") {
		t.Errorf("expected --estimator.interval=15m in container args, got %v", container.Args)
	}
	if !slices.Contains(container.Args, "--remote-write.initial-delay=5m") {
		t.Errorf("expected --remote-write.initial-delay=5m in container args, got %v", container.Args)
	}
	for _, wantArg := range []string{
		"--objstore.config-file=/etc/metrics-inspector/objstore/thanos.yaml",
		"--bucket-inspect.interval=15m",
		"--retention.resolution-raw=14d",
		"--retention.resolution-5m=90d",
		"--retention.resolution-1h=180d",
	} {
		if !slices.Contains(container.Args, wantArg) {
			t.Errorf("expected %q in container args, got %v", wantArg, container.Args)
		}
	}
	if len(container.VolumeMounts) != 4 {
		t.Fatalf("expected 4 volume mounts (config, objstore-secret, objstore-tls-secret, bound-sa-token), got %+v", container.VolumeMounts)
	}
	if container.VolumeMounts[2].MountPath != "/etc/minio/certs" {
		t.Errorf("expected derived TLS mount path /etc/minio/certs, got %q", container.VolumeMounts[2].MountPath)
	}
	if container.VolumeMounts[3].Name != "bound-sa-token" || container.VolumeMounts[3].MountPath != metricsInspectorSATokenMountPath {
		t.Errorf("unexpected projected SA token volume mount: %+v", container.VolumeMounts[3])
	}
	initialHash := deploy.Spec.Template.Annotations[metricsInspectorConfigHashKey]
	if initialHash == "" {
		t.Fatalf("expected non-empty %s annotation", metricsInspectorConfigHashKey)
	}

	// Verify Secret rotation updates config hash annotation on Deployment.
	objSecret.Data["thanos.yaml"] = []byte("type: S3\nconfig:\n  bucket: rotated-bucket\n  endpoint: s3.amazonaws.com\n")
	if err := cl.Update(ctx, objSecret); err != nil {
		t.Fatalf("Update objSecret failed: %v", err)
	}

	// Second reconcile: idempotent update when TShirtSize and Secret data change.
	mco.Spec.InstanceSize = mcoconfig.TwoXLarge
	if _, err := GenerateMetricsInspectorResources(ctx, cl, s, mco); err != nil {
		t.Fatalf("second GenerateMetricsInspectorResources failed: %v", err)
	}
	if err := cl.Get(ctx, types.NamespacedName{
		Name:      mcoconfig.GetOperandName(mcoconfig.MetricsInspector),
		Namespace: mcoconfig.GetDefaultNamespace(),
	}, deploy); err != nil {
		t.Fatalf("failed to get updated Deployment: %v", err)
	}
	if deploy.Spec.Template.Spec.Containers[0].Resources.Requests.Memory().String() != "3Gi" {
		t.Errorf("expected 2xlarge memory request 3Gi, got %s", deploy.Spec.Template.Spec.Containers[0].Resources.Requests.Memory().String())
	}
	rotatedHash := deploy.Spec.Template.Annotations[metricsInspectorConfigHashKey]
	if rotatedHash == initialHash {
		t.Errorf("expected config hash annotation to change after Secret rotation, stayed %q", rotatedHash)
	}

	// Verify newAPISpec always wires MetricsInspector into AdditionalWriteEndpoints even when WriteStorage is nil.
	mcoNoWriteStorage := newBaseMCO()
	mcoNoWriteStorage.Spec.StorageConfig.WriteStorage = nil
	apiSpec, err := newAPISpec(cl, mcoNoWriteStorage, observatoriumv1alpha1.TLSProfileSpec{})
	if err != nil {
		t.Fatalf("newAPISpec failed: %v", err)
	}
	if apiSpec.AdditionalWriteEndpoints == nil || apiSpec.AdditionalWriteEndpoints.EndpointsConfigSecret != endpointsConfigName {
		t.Fatalf("expected AdditionalWriteEndpoints to be set, got %+v", apiSpec.AdditionalWriteEndpoints)
	}
	epSecret := &corev1.Secret{}
	if err := cl.Get(ctx, types.NamespacedName{
		Name:      endpointsConfigName,
		Namespace: mcoconfig.GetDefaultNamespace(),
	}, epSecret); err != nil {
		t.Fatalf("failed to get endpoints secret: %v", err)
	}
	var eps []mcoutil.RemoteWriteEndpointWithSecret
	if err := yaml.Unmarshal(epSecret.Data[endpointsKey], &eps); err != nil {
		t.Fatalf("failed to unmarshal endpoints.yaml: %v", err)
	}
	if len(eps) != 1 || eps[0].Name != mcoconfig.MetricsInspector || eps[0].URL.String() != MetricsInspectorWriteURL() {
		t.Errorf("unexpected endpoints: %+v", eps)
	}
}
