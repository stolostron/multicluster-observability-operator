// Copyright (c) 2020 Red Hat, Inc.

package config

import (
	"bytes"
	"context"
	"os"
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
	AnnotationSkipCreation       = "skip-creation-if-exist"

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

	DefaultAddonInterval = 60

	ImageManifestConfigmapName = "mch-image-manifest-"

	ComponentVersion = "COMPONENT_VERSION"

	ServerCerts  = "observability-server-certs"
	GrafanaCerts = "observability-grafana-certs"

	AlertRuleDefaultConfigMapName = "thanos-ruler-default-rules"
	AlertRuleDefaultFileKey       = "default_rules.yaml"
	AlertRuleCustomConfigMapName  = "thanos-ruler-custom-rules"
	AlertRuleCustomFileKey        = "custom_rules.yaml"
	AlertmanagerURL               = "http://alertmanager:9093"
	AlertmanagerConfigName        = "alertmanager-config"

	WhitelistConfigMapName       = "observability-metrics-whitelist"
	WhitelistCustomConfigMapName = "observability-metrics-custom-whitelist"
)

const (
	ObservatoriumImgRepo           = "quay.io/observatorium"
	ObservatoriumAPIImgName        = "observatorium"
	ObservatoriumAPIImgTag         = "latest"
	ObservatoriumOperatorImgName   = "observatorium_operator"
	ObservatoriumOperatorImgTag    = "master-2020-10-09-ecc37fc"
	ThanosReceiveControllerImgName = "thanos-receive-controller"
	//ThanosReceiveControllerKey is used to get from mch-image-manifest.xxx configmap
	ThanosReceiveControllerKey    = "thanos_receive_controller"
	ThanosReceiveControllerImgTag = "master-2020-06-17-a9d9169"

	ThanosImgRepo = "quay.io/thanos"
	ThanosImgName = "thanos"
	ThanosImgTag  = "master-2020-08-12-70f89d83"

	MemcachedImgRepo = "docker.io"
	MemcachedImgName = "memcached"
	MemcachedImgTag  = "1.6.3-alpine"

	MemcachedExporterImgRepo = "prom"
	MemcachedExporterImgName = "memcached-exporter"
	MemcachedExporterKey     = "memcached_exporter"
	MemcachedExporterImgTag  = "v0.6.0"

	GrafanaImgRepo            = "grafana"
	GrafanaImgName            = "grafana"
	GrafanaImgTagSuffix       = "6.5.3"
	GrafanaDashboardLoaderKey = "grafana_dashboard_loader"

	AlertManagerImgRepo      = "quay.io/openshift"
	AlertManagerKey          = "prometheus-alertmanager"
	ConfigmapReloaderImgRepo = "quay.io/openshift"
	ConfigmapReloaderKey     = "prometheus-config-reloader"

	EndpointControllerImgTagSuffix = "0.1.0-758599e8bcb0dfa9699a72ab17bd70807af5db12"
	EndpointControllerImgName      = "endpoint-monitoring-operator"
	EndpointControllerKey          = "endpoint_monitoring_operator"

	MetricsCollectorImgTagSuffix = "2.1.0-1aa917b69ceb64c5a77b999ffb69529aa6fb069c"
	MetricsCollectorImgName      = "metrics-collector"
	MetricsCollectorKey          = "metrics_collector"

	LeaseControllerImageTagSuffix = "2.1.0-a2899de5ce144e2c0441063e9ee8c4addf3ecb4a"
	LeaseControllerImageName      = "klusterlet-addon-lease-controller"
	LeaseControllerKey            = "klusterlet_addon_lease_controller"

	RbacQueryProxyKey = "rbac_query_proxy"
)

// ObjectStorgeConf is used to Unmarshal from bytes to do validation
type ObjectStorgeConf struct {
	Type   string `yaml:"type"`
	Config Config `yaml:"config"`
}

var (
	log                         = logf.Log.WithName("config")
	monitoringCRName            = ""
	tenantUID                   = ""
	imageManifests              = map[string]string{}
	hasCustomRuleConfigMap      = false
	hasCustomAlertmanagerConfig = false
)

// GetClusterNameLabelKey returns the key for the injected label
func GetClusterNameLabelKey() string {
	return clusterNameLabelKey
}

// ReplaceImage is used to replace the image with specified annotation or imagemanifest configmap
func ReplaceImage(annotations map[string]string, imageRepo, componentName string) (bool, string) {
	if annotations != nil {
		annotationImageRepo, _ := annotations[AnnotationKeyImageRepository]
		// This is for test only. e.g.:
		// if there is "mco-metrics_collector-tag" defined in annotation, use it for testing
		componentTagSuffix, hasComponentTagSuffix := annotations["mco-"+componentName+"-tag"]
		tagSuffix, hasTagSuffix := annotations[AnnotationKeyImageTagSuffix]
		sameOrg := strings.Contains(imageRepo, DefaultImgRepository)

		if hasComponentTagSuffix {
			repoSlice := strings.Split(imageRepo, "/")
			imageName := strings.Split(repoSlice[len(repoSlice)-1], ":")[0]
			image := annotationImageRepo + "/" + imageName + ":" + componentTagSuffix
			return true, image
		} else if hasTagSuffix && sameOrg {
			repoSlice := strings.Split(imageRepo, "/")
			imageName := strings.Split(repoSlice[len(repoSlice)-1], ":")[0]
			image := annotationImageRepo + "/" + imageName + ":" + tagSuffix
			return true, image
		} else if !hasTagSuffix {
			image, found := imageManifests[componentName]
			if found {
				return true, image
			}
			return false, ""
		}
		return false, ""
	} else {
		image, found := imageManifests[componentName]
		if found {
			return true, image
		}
		return false, ""
	}
}

// GetDefaultTenantName returns the default tenant name
func GetDefaultTenantName() string {
	return defaultTenantName
}

func SetImageManifests(images map[string]string) {
	imageManifests = images
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
	clusterVersion, err := ocpClient.ConfigV1().ClusterVersions().Get(context.TODO(), "version", v1.GetOptions{})
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
	// set mco-imageRepository if needed
	if util.GetAnnotation(mco.Annotations, AnnotationKeyImageRepository) == "" {
		// set the default image repo
		if mco.Annotations == nil {
			mco.Annotations = map[string]string{}
		}
		mco.Annotations[AnnotationKeyImageRepository] = DefaultImgRepository
		imageCMName := ImageManifestConfigmapName + "2.1.0"
		componentVersion, found := os.LookupEnv(ComponentVersion)
		if found {
			imageCMName = ImageManifestConfigmapName + componentVersion
		}

		podNamespace, found := os.LookupEnv("POD_NAMESPACE")
		if found {
			//Get image manifest configmap
			imageCM := &corev1.ConfigMap{}
			err := c.Get(
				context.TODO(),
				types.NamespacedName{
					Name:      imageCMName,
					Namespace: podNamespace,
				},
				imageCM)
			if err == nil {
				imageManifests = imageCM.Data
			} else {
				log.Info("Cannot get image manifest configmap", "configmap name", imageCMName)
			}
			if len(imageManifests) != 0 {
				for _, value := range imageManifests {
					if strings.Contains(value, DefaultDSImgRepository) {
						mco.Annotations[AnnotationKeyImageRepository] = DefaultDSImgRepository
						break
					}
				}
			}
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

	needUpdate := false
	newObj := found.DeepCopy()
	//set default annotation
	if util.GetAnnotation(found.GetAnnotations(), AnnotationKeyImageRepository) !=
		util.GetAnnotation(mco.Annotations, AnnotationKeyImageRepository) {
		if newObj.Annotations == nil {
			newObj.Annotations = map[string]string{}
		}
		newObj.Annotations[AnnotationKeyImageRepository] =
			util.GetAnnotation(mco.Annotations, AnnotationKeyImageRepository)
		needUpdate = true
	}

	if res := bytes.Compare(desired, current); res != 0 {
		newObj.Spec = mco.Spec
		needUpdate = true
	}

	if needUpdate {
		log.Info("Update MultiClusterObservability CR.")
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

// SetCustomRuleConfigMap set true if there is custom rule configmap
func SetCustomRuleConfigMap(hasConfigMap bool) {
	hasCustomRuleConfigMap = hasConfigMap
}

// HasCustomRuleConfigMap returns true if there is custom rule configmap
func HasCustomRuleConfigMap() bool {
	return hasCustomRuleConfigMap
}
