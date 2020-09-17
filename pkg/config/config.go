// Copyright (c) 2020 Red Hat, Inc.

package config

import (
	"bytes"
	"context"
	"strings"

	ocinfrav1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	ocpClientSet "github.com/openshift/client-go/config/clientset/versioned"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/util"
)

const (
	clusterNameLabelKey      = "cluster"
	obsAPIGateway            = "observatorium-api"
	infrastructureConfigName = "cluster"
	defaultNamespace         = "open-cluster-management-observability"
	defaultTenantName        = "default"
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

	DefaultEnableDownsampling     = true
	DefaultRetentionResolution1h  = "30d"
	DefaultRetentionResolution5m  = "14d"
	DefaultRetentionResolutionRaw = "5d"

	GrafanaImgRepo      = "grafana"
	GrafanaImgTagSuffix = "7.1.3"

	ObservatoriumImgRepo      = "quay.io/observatorium"
	ObservatoriumImgTagSuffix = "master-2020-09-17-d861409"

	EndpointControllerImgTagSuffix = "0.1.0-758599e8bcb0dfa9699a72ab17bd70807af5db12"

	MetricsCollectorImgTagSuffix = "2.1.0-1aa917b69ceb64c5a77b999ffb69529aa6fb069c"

	LeaseControllerImageTagSuffix = "2.1.0-a2899de5ce144e2c0441063e9ee8c4addf3ecb4a"

	DefaultAddonInterval = 60
)

type AnnotationImageInfo struct {
	ImageRepository string
	ImageTagSuffix  string
}

// ObjectStorgeConf is used to Unmarshal from bytes to do validation
type ObjectStorgeConf struct {
	Type   string `yaml:"type"`
	Config Config `yaml:"config"`
}

var (
	log                 = logf.Log.WithName("config")
	monitoringCRName    = ""
	annotationImageInfo = AnnotationImageInfo{}
	tenantUID           = ""
)

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

// GetTenantUID returns tenant uid
func GetTenantUID() string {
	if tenantUID == "" {
		tenantUID = string(uuid.NewUUID())
	}
	return tenantUID
}

// GetObsAPISvc returns observatorium api service
func GetObsAPISvc(instanceName string) string {
	return instanceName + "-observatorium" + "-observatorium-api." + defaultNamespace + ".svc.cluster.local"
}

// GenerateMonitoringCR is used to generate monitoring CR with the default values
// w/ or w/o customized values
func GenerateMonitoringCR(c client.Client,
	mco *mcov1beta1.MultiClusterObservability) (*reconcile.Result, error) {

	if mco.Spec.ImagePullPolicy == "" {
		mco.Spec.ImagePullPolicy = DefaultImgPullPolicy
	}

	if mco.Spec.ImagePullSecret == "" {
		mco.Spec.ImagePullSecret = DefaultImgPullSecret
	}

	if mco.Spec.NodeSelector == nil {
		mco.Spec.NodeSelector = map[string]string{}
	}

	if mco.Spec.StorageConfig == nil {
		mco.Spec.StorageConfig = &mcov1beta1.StorageConfigObject{}
	}

	if mco.Spec.StorageConfig.StatefulSetSize == "" {
		mco.Spec.StorageConfig.StatefulSetSize = DefaultStorageSize
	}

	if mco.Spec.StorageConfig.StatefulSetStorageClass == "" {
		mco.Spec.StorageConfig.StatefulSetStorageClass = DefaultStorageClass
	}

	if mco.Spec.EnableDownSampling != false {
		mco.Spec.EnableDownSampling = DefaultEnableDownsampling
	}

	if mco.Spec.RetentionResolution1h == "" {
		mco.Spec.RetentionResolution1h = DefaultRetentionResolution1h
	}

	if mco.Spec.RetentionResolution5m == "" {
		mco.Spec.RetentionResolution5m = DefaultRetentionResolution5m
	}

	if mco.Spec.RetentionResolutionRaw == "" {
		mco.Spec.RetentionResolutionRaw = DefaultRetentionResolutionRaw
	}

	if mco.Spec.ObservabilityAddonSpec == nil {
		mco.Spec.ObservabilityAddonSpec = &mcov1beta1.ObservabilityAddonSpec{
			EnableMetrics: true,
			Interval:      DefaultAddonInterval,
		}
	}

	if !availabilityConfigIsValid(mco.Spec.AvailabilityConfig) {
		mco.Spec.AvailabilityConfig = mcov1beta1.HAHigh
	}

	found := &mcov1beta1.MultiClusterObservability{}
	err := c.Get(
		context.TODO(),
		types.NamespacedName{
			Name: mco.Name,
		},
		found,
	)
	if err != nil {
		return &reconcile.Result{}, err
	}

	desired, err := yaml.Marshal(mco.Spec)
	if err != nil {
		log.Error(err, "cannot parse the desired MultiClusterObservability values")
	}
	current, err := yaml.Marshal(found.Spec)
	if err != nil {
		log.Error(err, "cannot parse the current MultiClusterObservability values")
	}

	if res := bytes.Compare(desired, current); res != 0 {
		log.Info("Update MultiClusterObservability CR.")
		newObj := found.DeepCopy()
		newObj.Spec = mco.Spec
		err = c.Update(context.TODO(), newObj)
		if err != nil {
			return &reconcile.Result{}, err
		}
	}

	return nil, nil
}

func availabilityConfigIsValid(config mcov1beta1.AvailabilityType) bool {
	switch config {
	case mcov1beta1.HAHigh, mcov1beta1.HABasic:
		return true
	default:
		return false
	}
}
