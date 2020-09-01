// Copyright (c) 2020 Red Hat, Inc.

package multiclusterobservability

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	mcoconfig "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/util"
)

func TestNewVolumeClaimTemplate(t *testing.T) {
	vct := newVolumeClaimTemplate("10Gi", "test")
	if vct.Spec.AccessModes[0] != v1.ReadWriteOnce ||
		vct.Spec.Resources.Requests[v1.ResourceStorage] != resource.MustParse(mcoconfig.DefaultStorageSize) {
		t.Errorf("Failed to newVolumeClaimTemplate")
	}
}

func TestNewDefaultObservatoriumSpec(t *testing.T) {
	storageClassName := "gp2"
	statefulSetSize := "1Gi"
	mco := &mcov1beta1.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "test"},
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

	obs := newDefaultObservatoriumSpec(mco)
	if obs.Query.Image != defaultThanosImage ||
		obs.Query.Version != defaultThanosVersion {
		t.Errorf("Failed to newDefaultObservatorium")
	}

	mco.Annotations = map[string]string{
		mcoconfig.AnnotationKeyImageRepository: "repo",
		mcoconfig.AnnotationKeyImageTagSuffix:  "tag",
	}
	imgRepo := util.GetAnnotation(mco.GetAnnotations(), mcoconfig.AnnotationKeyImageRepository)
	imgVersion := util.GetAnnotation(mco.GetAnnotations(), mcoconfig.AnnotationKeyImageTagSuffix)
	receiversStorage := obs.Receivers.VolumeClaimTemplate.Spec.Resources.Requests["storage"]
	ruleStorage := obs.Rule.VolumeClaimTemplate.Spec.Resources.Requests["storage"]
	storeStorage := obs.Store.VolumeClaimTemplate.Spec.Resources.Requests["storage"]
	compactStorage := obs.Compact.VolumeClaimTemplate.Spec.Resources.Requests["storage"]
	obs = newDefaultObservatoriumSpec(mco)
	APIImg := imgRepo + "/observatorium:" + imgVersion
	queryCacheImg := imgRepo + "/cortex:" + imgVersion
	controllerImg := imgRepo + "/thanos-receive-controller:" + imgVersion
	thanosImg := imgRepo + "/" + thanosImgName + ":" + imgVersion
	if obs.API.Image != APIImg ||
		obs.API.Version != imgVersion ||
		obs.QueryCache.Image != queryCacheImg ||
		obs.QueryCache.Version != imgVersion ||
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
		obs.APIQuery.Image != thanosImg ||
		obs.APIQuery.Version != imgVersion ||
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
