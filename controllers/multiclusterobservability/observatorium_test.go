// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package multiclusterobservability

import (
	"bytes"
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"

	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta2"
	mcoconfig "github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
	observatoriumv1alpha1 "github.com/open-cluster-management/observatorium-operator/api/v1alpha1"
)

var (
	storageClassName = ""
)

func TestNewVolumeClaimTemplate(t *testing.T) {
	vct := newVolumeClaimTemplate("10Gi", "test")
	if vct.Spec.AccessModes[0] != v1.ReadWriteOnce ||
		vct.Spec.Resources.Requests[v1.ResourceStorage] != resource.MustParse("10Gi") {
		t.Errorf("Failed to newVolumeClaimTemplate")
	}
}

func TestNewDefaultObservatoriumSpec(t *testing.T) {
	statefulSetSize := "1Gi"
	mco := &mcov1beta2.MultiClusterObservability{
		TypeMeta: metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			Annotations: map[string]string{
				mcoconfig.AnnotationKeyImageRepository: "quay.io:443/acm-d",
				mcoconfig.AnnotationKeyImageTagSuffix:  "tag",
			},
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta2.StorageConfigObject{
				MetricObjectStorage: &mcov1beta2.PreConfiguredStorage{
					Key:  "key",
					Name: "name",
				},
				StorageClass:            storageClassName,
				AlertmanagerStorageSize: "1Gi",
				CompactStorageSize:      "1Gi",
				RuleStorageSize:         "1Gi",
				ReceiveStorageSize:      "1Gi",
				StoreStorageSize:        "1Gi",
			},
			RetentionConfig: &mcov1beta2.RetentionConfig{
				RetentionResolutionRaw: "1h",
				RetentionResolution5m:  "1h",
				RetentionResolution1h:  "1h",
			},
		},
	}

	obs := newDefaultObservatoriumSpec(mco, storageClassName)

	receiversStorage := obs.Receivers.VolumeClaimTemplate.Spec.Resources.Requests["storage"]
	ruleStorage := obs.Rule.VolumeClaimTemplate.Spec.Resources.Requests["storage"]
	storeStorage := obs.Store.VolumeClaimTemplate.Spec.Resources.Requests["storage"]
	compactStorage := obs.Compact.VolumeClaimTemplate.Spec.Resources.Requests["storage"]
	obs = newDefaultObservatoriumSpec(mco, storageClassName)
	if *obs.Receivers.VolumeClaimTemplate.Spec.StorageClassName != storageClassName ||
		*obs.Rule.VolumeClaimTemplate.Spec.StorageClassName != storageClassName ||
		*obs.Store.VolumeClaimTemplate.Spec.StorageClassName != storageClassName ||
		*obs.Compact.VolumeClaimTemplate.Spec.StorageClassName != storageClassName ||
		receiversStorage.String() != statefulSetSize ||
		ruleStorage.String() != statefulSetSize ||
		storeStorage.String() != statefulSetSize ||
		compactStorage.String() != statefulSetSize ||
		obs.ObjectStorageConfig.Thanos.Key != "key" ||
		obs.ObjectStorageConfig.Thanos.Name != "name" {
		t.Errorf("Failed to newDefaultObservatorium")
	}
}

func TestMergeVolumeClaimTemplate(t *testing.T) {
	vct1 := newVolumeClaimTemplate("1Gi", "test")
	vct3 := newVolumeClaimTemplate("3Gi", "test")
	mergeVolumeClaimTemplate(vct1, vct3)
	if vct1.Spec.Resources.Requests[v1.ResourceStorage] != resource.MustParse("3Gi") {
		t.Errorf("Failed to merge %v to %v", vct3, vct1)
	}
}

func TestNoUpdateObservatoriumCR(t *testing.T) {
	var (
		name      = "monitoring"
		namespace = mcoconfig.GetDefaultNamespace()
	)

	// A MultiClusterObservability object with metadata and spec.
	mco := &mcov1beta2.MultiClusterObservability{
		TypeMeta: metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				mcoconfig.AnnotationKeyImageTagSuffix: "tag",
			},
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta2.StorageConfigObject{
				MetricObjectStorage: &mcov1beta2.PreConfiguredStorage{
					Key:  "test",
					Name: "test",
				},
				StorageClass:            storageClassName,
				AlertmanagerStorageSize: "1Gi",
				CompactStorageSize:      "1Gi",
				RuleStorageSize:         "1Gi",
				ReceiveStorageSize:      "1Gi",
				StoreStorageSize:        "1Gi",
			},
			RetentionConfig: &mcov1beta2.RetentionConfig{
				RetentionResolutionRaw: "1h",
				RetentionResolution5m:  "1h",
				RetentionResolution1h:  "1h",
			},
		},
	}
	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	mcov1beta2.SchemeBuilder.AddToScheme(s)
	observatoriumv1alpha1.AddToScheme(s)

	objs := []runtime.Object{mco}
	// Create a fake client to mock API calls.
	cl := fake.NewFakeClient(objs...)

	GenerateObservatoriumCR(cl, s, mco)

	// Check if this Observatorium CR already exists
	observatoriumCRFound := &observatoriumv1alpha1.Observatorium{}
	cl.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
		observatoriumCRFound,
	)

	oldSpec := observatoriumCRFound.Spec
	newSpec := newDefaultObservatoriumSpec(mco, storageClassName)
	oldSpecBytes, _ := yaml.Marshal(oldSpec)
	newSpecBytes, _ := yaml.Marshal(newSpec)

	if res := bytes.Compare(newSpecBytes, oldSpecBytes); res != 0 {
		t.Errorf("%v should be equal to %v", string(oldSpecBytes), string(newSpecBytes))
	}

	_, err := GenerateObservatoriumCR(cl, s, mco)
	if err != nil {
		t.Errorf("Failed to update observatorium due to %v", err)
	}
}
