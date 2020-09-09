// Copyright (c) 2020 Red Hat, Inc.

package config

import (
	"context"
	"strings"

	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/util"
	ocinfrav1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	ocpClientSet "github.com/openshift/client-go/config/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	clusterNameLabelKey      = "cluster"
	obsAPIGateway            = "observatorium-api"
	infrastructureConfigName = "cluster"
	defaultNamespace         = "open-cluster-management-observability"
	defaultTenantName        = "prod"
	placementRuleName        = "observability"

	AnnotationKeyImageRepository = "mco-imageRepository"
	AnnotationKeyImageTagSuffix  = "mco-imageTagSuffix"
	AnnotationMCOPause           = "mco-pause"

	DefaultImgPullPolicy   = corev1.PullAlways
	DefaultImgPullSecret   = "multiclusterhub-operator-pull-secret"
	DefaultImgRepository   = "quay.io/open-cluster-management"
	DefaultDSImgRepository = "quay.io:443/acm-d"
	DefaultImgTagSuffix    = "latest"
	DefaultStorageClass    = "gp2"
	DefaultStorageSize     = "10Gi"

	DefaultEnableDownSampling     = false
	DefaultRetentionResolution1h  = "30d"
	DefaultRetentionResolution5m  = "14d"
	DefaultRetentionResolutionRaw = "5d"

	GrafanaImgRepo      = "grafana"
	GrafanaImgTagSuffix = "7.1.3"

	ObservatoriumImgRepo      = "quay.io/observatorium"
	ObservatoriumImgTagSuffix = "master-2020-09-02-52bf608"

	EndpointControllerImgTagSuffix = "0.1.0-9dddad57ace8425ff06ee6a4a9143e1066c03dda"

	MetricsCollectorImgTagSuffix = "2.1.0-1aa917b69ceb64c5a77b999ffb69529aa6fb069c"

	DefaultAddonInterval = 60
)

type AnnotationImageInfo struct {
	ImageRepository string
	ImageTagSuffix  string
}

var log = logf.Log.WithName("config")

var monitoringCRName = ""

var annotationImageInfo = AnnotationImageInfo{}

// GetClusterNameLabelKey returns the key for the injected label
func GetClusterNameLabelKey() string {
	return clusterNameLabelKey
}

func IsNeededReplacement(annotations map[string]string, imageRepo string) bool {
	if annotations != nil {
		annotationImageRepo, hasImage := annotations[AnnotationKeyImageRepository]
		_, hasTagSuffix := annotations[AnnotationKeyImageTagSuffix]
		sameOrg := strings.Contains(imageRepo, DefaultImgRepository)
		isFromDS := strings.Contains(annotationImageRepo, DefaultDSImgRepository)
		if isFromDS {
			return hasImage && hasTagSuffix
		}
		return hasImage && hasTagSuffix && sameOrg
	}
	return false
}

// GetDefaultTenantName returns the default tenant name
func GetDefaultTenantName() string {
	return defaultTenantName
}

// GetObsAPIUrl is used to get the URL for observartium api gateway
func GetObsAPIUrl(client client.Client, namespace string) (string, error) {
	found := &routev1.Route{}

	err := client.Get(context.TODO(), types.NamespacedName{Name: obsAPIGateway, Namespace: namespace}, found)
	if err != nil {
		return "", err
	}
	return found.Spec.Host, nil
}

func GetDefaultNamespace() string {
	return defaultNamespace
}

// GetAnnotationImageInfo returns the configured image repo and tag
func GetAnnotationImageInfo() AnnotationImageInfo {
	return annotationImageInfo
}

// SetAnnotationImageInfo set the configured image repo and tag
func SetAnnotationImageInfo(annotations map[string]string) AnnotationImageInfo {
	imgRepo := util.GetAnnotation(annotations, AnnotationKeyImageRepository)
	imgVersion := util.GetAnnotation(annotations, AnnotationKeyImageTagSuffix)
	if imgVersion == "" {
		imgVersion = DefaultImgTagSuffix
	}
	annotationImageInfo = AnnotationImageInfo{
		ImageRepository: imgRepo,
		ImageTagSuffix:  imgVersion,
	}
	return annotationImageInfo
}

// GetMonitoringCRName returns monitoring cr name
func GetMonitoringCRName() string {
	return monitoringCRName
}

// SetMonitoringCRName sets the cr name
func SetMonitoringCRName(crName string) {
	monitoringCRName = crName
}

func infrastructureConfigNameNsN() types.NamespacedName {
	return types.NamespacedName{
		Name: infrastructureConfigName,
	}
}

// GetKubeAPIServerAddress is used to get the api server url
func GetKubeAPIServerAddress(client client.Client) (string, error) {
	infraConfig := &ocinfrav1.Infrastructure{}
	if err := client.Get(context.TODO(), infrastructureConfigNameNsN(), infraConfig); err != nil {
		return "", err
	}

	return infraConfig.Status.APIServerURL, nil
}

// GetClusterID is used to get the cluster uid
func GetClusterID(ocpClient ocpClientSet.Interface) (string, error) {
	clusterVersion, err := ocpClient.ConfigV1().ClusterVersions().Get("version", v1.GetOptions{})
	if err != nil {
		log.Error(err, "Failed to get clusterVersion")
		return "", err
	}

	return string(clusterVersion.Spec.ClusterID), nil
}

// GetPlacementRuleName is used to get placementRuleName
func GetPlacementRuleName() string {
	return placementRuleName
}

// IsPaused returns true if the multiclusterobservability instance is labeled as paused, and false otherwise
func IsPaused(annotations map[string]string) bool {
	if annotations == nil {
		return false
	}

	if annotations[AnnotationMCOPause] != "" &&
		strings.EqualFold(annotations[AnnotationMCOPause], "true") {
		return true
	}

	return false
}
