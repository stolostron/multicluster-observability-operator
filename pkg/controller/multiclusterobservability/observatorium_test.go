// Copyright (c) 2020 Red Hat, Inc.

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

	observatoriumv1alpha1 "github.com/observatorium/operator/api/v1alpha1"
	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	mcoconfig "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/util"
)

var (
	storageClassName = ""
)

func TestNewVolumeClaimTemplate(t *testing.T) {
	vct := newVolumeClaimTemplate("10Gi", "test")
	if vct.Spec.AccessModes[0] != v1.ReadWriteOnce ||
		vct.Spec.Resources.Requests[v1.ResourceStorage] != resource.MustParse(mcoconfig.DefaultStorageSize) {
		t.Errorf("Failed to newVolumeClaimTemplate")
	}
}

func TestNewDefaultObservatoriumSpec(t *testing.T) {
	statefulSetSize := "1Gi"
	mco := &mcov1beta1.MultiClusterObservability{
		TypeMeta: metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
			Name:      "test",
			Annotations: map[string]string{
				mcoconfig.AnnotationKeyImageRepository: "quay.io:443/acm-d",
				mcoconfig.AnnotationKeyImageTagSuffix:  "tag",
			},
		},
		Spec: mcov1beta1.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta1.StorageConfigObject{
				MetricObjectStorage: &mcov1beta1.PreConfiguredStorage{
					Key:  "key",
					Name: "name",
				},
				StatefulSetSize:         statefulSetSize,
				StatefulSetStorageClass: storageClassName,
			},
		},
	}

	obs := newDefaultObservatoriumSpec(mco, storageClassName)

	imgRepo := util.GetAnnotation(mco.GetAnnotations(), mcoconfig.AnnotationKeyImageRepository)
	imgVersion := util.GetAnnotation(mco.GetAnnotations(), mcoconfig.AnnotationKeyImageTagSuffix)
	receiversStorage := obs.Receivers.VolumeClaimTemplate.Spec.Resources.Requests["storage"]
	ruleStorage := obs.Rule.VolumeClaimTemplate.Spec.Resources.Requests["storage"]
	storeStorage := obs.Store.VolumeClaimTemplate.Spec.Resources.Requests["storage"]
	compactStorage := obs.Compact.VolumeClaimTemplate.Spec.Resources.Requests["storage"]
	obs = newDefaultObservatoriumSpec(mco, storageClassName)
	APIImg := imgRepo + "/observatorium:" + imgVersion
	controllerImg := imgRepo + "/thanos-receive-controller:" + imgVersion
	thanosImg := imgRepo + "/" + mcoconfig.ThanosImgName + ":" + imgVersion
	if obs.API.Image != APIImg ||
		obs.API.Version != imgVersion ||
		obs.ThanosReceiveController.Image != controllerImg ||
		obs.ThanosReceiveController.Version != imgVersion ||
		obs.Query.Image != thanosImg ||
		obs.Query.Version != imgVersion ||
		obs.Receivers.Image != thanosImg ||
		obs.Receivers.Version != imgVersion ||
		*obs.Receivers.VolumeClaimTemplate.Spec.StorageClassName != storageClassName ||
		*obs.Rule.VolumeClaimTemplate.Spec.StorageClassName != storageClassName ||
		*obs.Store.VolumeClaimTemplate.Spec.StorageClassName != storageClassName ||
		*obs.Compact.VolumeClaimTemplate.Spec.StorageClassName != storageClassName ||
		receiversStorage.String() != statefulSetSize ||
		ruleStorage.String() != statefulSetSize ||
		storeStorage.String() != statefulSetSize ||
		compactStorage.String() != statefulSetSize ||
		obs.Rule.Image != thanosImg ||
		obs.Rule.Version != imgVersion ||
		obs.Store.Image != thanosImg ||
		obs.Store.Version != imgVersion ||
		obs.Compact.Image != thanosImg ||
		obs.Compact.Version != imgVersion ||
		obs.Store.Cache.Image != imgRepo+"/memcached:"+imgVersion ||
		obs.Store.Cache.Version != imgVersion ||
		obs.Store.Cache.ExporterImage != imgRepo+"/memcached-exporter:"+imgVersion ||
		obs.Store.Cache.ExporterVersion != imgVersion ||
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
	mco := &mcov1beta1.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec:       mcov1beta1.MultiClusterObservabilitySpec{},
	}
	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	mcov1beta1.SchemeBuilder.AddToScheme(s)
	observatoriumv1alpha1.AddToScheme(s)

	objs := []runtime.Object{mco}
	// Create a fake client to mock API calls.
	cl := fake.NewFakeClient(objs...)

	if _, err := mcoconfig.GenerateMonitoringCR(cl, mco); err != nil {
		t.Errorf("Failed to generate monitoring CR: %v", err)
	}

	GenerateObservatoriumCR(cl, s, mco)

	// Check if this Observatorium CR already exists
	observatoriumCRFound := &observatoriumv1alpha1.Observatorium{}
	cl.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      name + obsPartoOfName,
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
}
