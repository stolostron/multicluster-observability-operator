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
	vct := newVolumeClaimTemplate("50Gi")
	if vct.Spec.AccessModes[0] != v1.ReadWriteOnce ||
		vct.Spec.Resources.Requests[v1.ResourceStorage] != resource.MustParse(mcoconfig.DefaultStorageSize) {
		t.Errorf("Failed to newVolumeClaimTemplate")
	}
}

func TestNewDefaultObservatorium(t *testing.T) {
	mco := &mcov1beta1.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "test"},
		Spec:       mcov1beta1.MultiClusterObservabilitySpec{},
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
	imgRepo := util.GetAnnotation(mco, mcoconfig.AnnotationKeyImageRepository)
	imgVersion := util.GetAnnotation(mco, mcoconfig.AnnotationKeyImageTagSuffix)

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
		obs.APIQuery.Version != imgVersion {
		t.Errorf("Failed to newDefaultObservatorium")
	}
}

func TestMergeVolumeClaimTemplate(t *testing.T) {
	vct1 := newVolumeClaimTemplate("1Gi")
	vct3 := newVolumeClaimTemplate("3Gi")
	mergeVolumeClaimTemplate(vct1, vct3)
	if vct1.Spec.Resources.Requests[v1.ResourceStorage] != resource.MustParse("3Gi") {
		t.Errorf("Failed to merge %v to %v", vct3, vct1)
	}
}
