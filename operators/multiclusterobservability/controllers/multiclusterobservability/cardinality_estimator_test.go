// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package multiclusterobservability

import (
	"context"
	"slices"
	"testing"

	"github.com/stolostron/multicluster-observability-operator/estimators/cardinality/pkg/estimator"
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

func TestGenerateCardinalityEstimatorResources(t *testing.T) {
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
		},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(mco, alertmanagerCABundleConfigMap()).Build()
	mcoconfig.CleanUpOperandNames()
	if err := mcoconfig.SetOperandNames(cl); err != nil {
		t.Fatalf("SetOperandNames: %v", err)
	}

	// First reconcile: creates ConfigMap, Service, and Deployment.
	if _, err := GenerateCardinalityEstimatorResources(ctx, cl, s, mco); err != nil {
		t.Fatalf("GenerateCardinalityEstimatorResources failed: %v", err)
	}

	// Verify ConfigMap.
	cm := &corev1.ConfigMap{}
	if err := cl.Get(ctx, types.NamespacedName{
		Name:      mcoconfig.CardinalityEstimatorConfigMapName,
		Namespace: mcoconfig.GetDefaultNamespace(),
	}, cm); err != nil {
		t.Fatalf("failed to get ConfigMap: %v", err)
	}
	if cm.Data[cardinalityEstimatorConfigFileKey] == "" {
		t.Errorf("expected non-empty %s in ConfigMap", cardinalityEstimatorConfigFileKey)
	}
	parsedCfg, err := estimator.ParseConfig([]byte(cm.Data[cardinalityEstimatorConfigFileKey]))
	if err != nil {
		t.Fatalf("ConfigMap YAML failed estimator.ParseConfig validation: %v", err)
	}
	if len(parsedCfg.Streams) != 4 {
		t.Fatalf("expected 4 configured cardinality streams, got %d", len(parsedCfg.Streams))
	}

	// Verify Service.
	svc := &corev1.Service{}
	if err := cl.Get(ctx, types.NamespacedName{
		Name:      mcoconfig.CardinalityEstimatorServiceName,
		Namespace: mcoconfig.GetDefaultNamespace(),
	}, svc); err != nil {
		t.Fatalf("failed to get Service: %v", err)
	}
	if len(svc.Spec.Ports) != 1 || svc.Spec.Ports[0].Port != cardinalityEstimatorPort {
		t.Errorf("unexpected Service ports: %+v", svc.Spec.Ports)
	}

	// Verify Deployment.
	deploy := &appsv1.Deployment{}
	if err := cl.Get(ctx, types.NamespacedName{
		Name:      mcoconfig.GetOperandName(mcoconfig.CardinalityEstimator),
		Namespace: mcoconfig.GetDefaultNamespace(),
	}, deploy); err != nil {
		t.Fatalf("failed to get Deployment: %v", err)
	}
	if deploy.Spec.Replicas == nil || *deploy.Spec.Replicas != 1 {
		t.Errorf("expected 1 replica, got %v", deploy.Spec.Replicas)
	}
	if len(deploy.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(deploy.Spec.Template.Spec.Containers))
	}
	container := deploy.Spec.Template.Spec.Containers[0]
	wantImg := "quay.io/stolostron/cardinality-estimator:test-tag"
	if container.Image != wantImg {
		t.Errorf("container image = %q, want %q", container.Image, wantImg)
	}
	if container.Resources.Requests.Memory().String() != "2Gi" {
		t.Errorf("expected Large memory request 2Gi, got %s", container.Resources.Requests.Memory().String())
	}
	if !slices.Contains(container.Args, "--remote-write.initial-delay=5m") {
		t.Errorf("expected --remote-write.initial-delay=5m in container args, got %v", container.Args)
	}

	// Second reconcile: idempotent update when TShirtSize changes.
	mco.Spec.InstanceSize = mcoconfig.TwoXLarge
	if _, err := GenerateCardinalityEstimatorResources(ctx, cl, s, mco); err != nil {
		t.Fatalf("second GenerateCardinalityEstimatorResources failed: %v", err)
	}
	if err := cl.Get(ctx, types.NamespacedName{
		Name:      mcoconfig.GetOperandName(mcoconfig.CardinalityEstimator),
		Namespace: mcoconfig.GetDefaultNamespace(),
	}, deploy); err != nil {
		t.Fatalf("failed to get updated Deployment: %v", err)
	}
	if deploy.Spec.Template.Spec.Containers[0].Resources.Requests.Memory().String() != "3Gi" {
		t.Errorf("expected 2xlarge memory request 3Gi, got %s", deploy.Spec.Template.Spec.Containers[0].Resources.Requests.Memory().String())
	}

	// Verify newAPISpec always wires CardinalityEstimator into AdditionalWriteEndpoints even when WriteStorage is nil.
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
	if len(eps) != 1 || eps[0].Name != mcoconfig.CardinalityEstimator || eps[0].URL.String() != CardinalityEstimatorWriteURL() {
		t.Errorf("unexpected endpoints: %+v", eps)
	}
}
