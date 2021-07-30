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

	mcoshared "github.com/open-cluster-management/multicluster-observability-operator/api/shared"
	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta2"
	mcoconfig "github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
	observatoriumv1alpha1 "github.com/open-cluster-management/observatorium-operator/api/v1alpha1"
)

var (
	storageClassName = "gp2"
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
			StorageConfig: &mcov1beta2.StorageConfig{
				MetricObjectStorage: &mcoshared.PreConfiguredStorage{
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
		},
	}

	obs := newDefaultObservatoriumSpec(mco, storageClassName)

	receiversStorage := obs.Thanos.Receivers.VolumeClaimTemplate.Spec.Resources.Requests["storage"]
	ruleStorage := obs.Thanos.Rule.VolumeClaimTemplate.Spec.Resources.Requests["storage"]
	storeStorage := obs.Thanos.Store.VolumeClaimTemplate.Spec.Resources.Requests["storage"]
	compactStorage := obs.Thanos.Compact.VolumeClaimTemplate.Spec.Resources.Requests["storage"]
	obs = newDefaultObservatoriumSpec(mco, storageClassName)
	if *obs.Thanos.Receivers.VolumeClaimTemplate.Spec.StorageClassName != storageClassName ||
		*obs.Thanos.Rule.VolumeClaimTemplate.Spec.StorageClassName != storageClassName ||
		*obs.Thanos.Store.VolumeClaimTemplate.Spec.StorageClassName != storageClassName ||
		*obs.Thanos.Compact.VolumeClaimTemplate.Spec.StorageClassName != storageClassName ||
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
		namespace = mcoconfig.GetDefaultNamespace()
	)

	// A MultiClusterObservability object with metadata and spec.
	mco := &mcov1beta2.MultiClusterObservability{
		TypeMeta: metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{
			Name: mcoconfig.GetDefaultCRName(),
			Annotations: map[string]string{
				mcoconfig.AnnotationKeyImageTagSuffix: "tag",
			},
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta2.StorageConfig{
				MetricObjectStorage: &mcoshared.PreConfiguredStorage{
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
		},
	}
	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	mcov1beta2.SchemeBuilder.AddToScheme(s)
	observatoriumv1alpha1.AddToScheme(s)

	sc := createStorageClass("gp2", true, true)
	objs := []runtime.Object{mco, sc}
	// Create a fake client to mock API calls.
	cl := fake.NewFakeClient(objs...)

	GenerateObservatoriumCR(cl, s, mco)

	// Check if this Observatorium CR already exists
	observatoriumCRFound := &observatoriumv1alpha1.Observatorium{}
	cl.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      mcoconfig.GetDefaultCRName(),
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

func TestRollbackStorageResizeForObservatorium(t *testing.T) {
	cases := []struct {
		name         string
		newSpec      *observatoriumv1alpha1.ObservatoriumSpec
		oldSpec      *observatoriumv1alpha1.ObservatoriumSpec
		expectedSpec *observatoriumv1alpha1.ObservatoriumSpec
	}{
		{
			name: "rollback with the same size",
			newSpec: &observatoriumv1alpha1.ObservatoriumSpec{
				Thanos: observatoriumv1alpha1.ThanosSpec{
					Compact: observatoriumv1alpha1.CompactSpec{
						VolumeClaimTemplate: newVolumeClaimTemplate("1Gi", "foo2"),
					},
					Rule: observatoriumv1alpha1.RuleSpec{
						VolumeClaimTemplate: newVolumeClaimTemplate("2Gi", "bar2"),
					},
					Receivers: observatoriumv1alpha1.ReceiversSpec{
						VolumeClaimTemplate: newVolumeClaimTemplate("3Gi", "baz2"),
					},
					Store: observatoriumv1alpha1.StoreSpec{
						VolumeClaimTemplate: newVolumeClaimTemplate("4Gi", "qux2"),
					},
				},
			},
			oldSpec: &observatoriumv1alpha1.ObservatoriumSpec{
				Thanos: observatoriumv1alpha1.ThanosSpec{
					Compact: observatoriumv1alpha1.CompactSpec{
						VolumeClaimTemplate: newVolumeClaimTemplate("1Gi", "foo1"),
					},
					Rule: observatoriumv1alpha1.RuleSpec{
						VolumeClaimTemplate: newVolumeClaimTemplate("2Gi", "bar1"),
					},
					Receivers: observatoriumv1alpha1.ReceiversSpec{
						VolumeClaimTemplate: newVolumeClaimTemplate("3Gi", "baz1"),
					},
					Store: observatoriumv1alpha1.StoreSpec{
						VolumeClaimTemplate: newVolumeClaimTemplate("4Gi", "qux1"),
					},
				},
			},
			expectedSpec: &observatoriumv1alpha1.ObservatoriumSpec{
				Thanos: observatoriumv1alpha1.ThanosSpec{
					Compact: observatoriumv1alpha1.CompactSpec{
						VolumeClaimTemplate: newVolumeClaimTemplate("1Gi", "foo2"),
					},
					Rule: observatoriumv1alpha1.RuleSpec{
						VolumeClaimTemplate: newVolumeClaimTemplate("2Gi", "bar2"),
					},
					Receivers: observatoriumv1alpha1.ReceiversSpec{
						VolumeClaimTemplate: newVolumeClaimTemplate("3Gi", "baz2"),
					},
					Store: observatoriumv1alpha1.StoreSpec{
						VolumeClaimTemplate: newVolumeClaimTemplate("4Gi", "qux2"),
					},
				},
			},
		},
		{
			name: "rollback with the different sizes",
			newSpec: &observatoriumv1alpha1.ObservatoriumSpec{
				Thanos: observatoriumv1alpha1.ThanosSpec{
					Compact: observatoriumv1alpha1.CompactSpec{
						VolumeClaimTemplate: newVolumeClaimTemplate("10Gi", "foo2"),
					},
					Rule: observatoriumv1alpha1.RuleSpec{
						VolumeClaimTemplate: newVolumeClaimTemplate("20Gi", "bar2"),
					},
					Receivers: observatoriumv1alpha1.ReceiversSpec{
						VolumeClaimTemplate: newVolumeClaimTemplate("30Gi", "baz2"),
					},
					Store: observatoriumv1alpha1.StoreSpec{
						VolumeClaimTemplate: newVolumeClaimTemplate("40Gi", "qux2"),
					},
				},
			},
			oldSpec: &observatoriumv1alpha1.ObservatoriumSpec{
				Thanos: observatoriumv1alpha1.ThanosSpec{
					Compact: observatoriumv1alpha1.CompactSpec{
						VolumeClaimTemplate: newVolumeClaimTemplate("1Gi", "foo1"),
					},
					Rule: observatoriumv1alpha1.RuleSpec{
						VolumeClaimTemplate: newVolumeClaimTemplate("2Gi", "bar1"),
					},
					Receivers: observatoriumv1alpha1.ReceiversSpec{
						VolumeClaimTemplate: newVolumeClaimTemplate("3Gi", "baz1"),
					},
					Store: observatoriumv1alpha1.StoreSpec{
						VolumeClaimTemplate: newVolumeClaimTemplate("4Gi", "qux1"),
					},
				},
			},
			expectedSpec: &observatoriumv1alpha1.ObservatoriumSpec{
				Thanos: observatoriumv1alpha1.ThanosSpec{
					Compact: observatoriumv1alpha1.CompactSpec{
						VolumeClaimTemplate: newVolumeClaimTemplate("1Gi", "foo2"),
					},
					Rule: observatoriumv1alpha1.RuleSpec{
						VolumeClaimTemplate: newVolumeClaimTemplate("2Gi", "bar2"),
					},
					Receivers: observatoriumv1alpha1.ReceiversSpec{
						VolumeClaimTemplate: newVolumeClaimTemplate("3Gi", "baz2"),
					},
					Store: observatoriumv1alpha1.StoreSpec{
						VolumeClaimTemplate: newVolumeClaimTemplate("4Gi", "qux2"),
					},
				},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rollbackStorageResizeForObservatorium(c.oldSpec, c.newSpec)
			expectedSpecBytes, err := yaml.Marshal(c.expectedSpec)
			if err != nil {
				t.Errorf("failed to marshal observatorium CR Spec: %v", err)
			}
			newSpecBytes, err := yaml.Marshal(c.newSpec)
			if err != nil {
				t.Errorf("failed to marshal observatorium CR Spec: %v", err)
			}
			if !bytes.Equal(newSpecBytes, expectedSpecBytes) {
				t.Errorf("case (%v) different ObservatoriumSpec, got:\n%s\n want:\n%s\n", c.name, newSpecBytes, expectedSpecBytes)
			}
		})
	}
}
