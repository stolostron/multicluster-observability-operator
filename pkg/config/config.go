// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

import (
	"context"
	"os"
	"strings"

	ocinfrav1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	ocpClientSet "github.com/openshift/client-go/config/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	clusterNameLabelKey      = "cluster"
	obsAPIGateway            = "observatorium-api"
	infrastructureConfigName = "cluster"
	defaultNamespace         = "open-cluster-management-observability"
	defaultTenantName        = "default"
	placementRuleName        = "observability"

	AnnotationKeyImageRepository          = "mco-imageRepository"
	AnnotationKeyImageTagSuffix           = "mco-imageTagSuffix"
	AnnotationMCOPause                    = "mco-pause"
	AnnotationMCOWithoutResourcesRequests = "mco-thanos-without-resources-requests"
	AnnotationSkipCreation                = "skip-creation-if-exist"

	DefaultImgRepository   = "quay.io/open-cluster-management"
	DefaultDSImgRepository = "quay.io:443/acm-d"
	DefaultImgTagSuffix    = "latest"

	ImageManifestConfigMapName = "mch-image-manifest-"

	ComponentVersion = "COMPONENT_VERSION"

	ServerCerts  = "observability-server-certs"
	GrafanaCerts = "observability-grafana-certs"

	AlertRuleDefaultConfigMapName = "thanos-ruler-default-rules"
	AlertRuleDefaultFileKey       = "default_rules.yaml"
	AlertRuleCustomConfigMapName  = "thanos-ruler-custom-rules"
	AlertRuleCustomFileKey        = "custom_rules.yaml"
	AlertmanagerURL               = "http://alertmanager:9093"
	AlertmanagerConfigName        = "alertmanager-config"

	AllowlistConfigMapName       = "observability-metrics-allowlist"
	AllowlistCustomConfigMapName = "observability-metrics-custom-allowlist"
)

const (
	ObservatoriumImgRepo           = "quay.io/observatorium"
	ObservatoriumAPIImgName        = "observatorium"
	ObservatoriumAPIImgTag         = "master-2020-09-18-v0.1.1-148-gb105421"
	ObservatoriumOperatorImgName   = "observatorium_operator"
	ThanosReceiveControllerImgName = "thanos-receive-controller"
	//ThanosReceiveControllerKey is used to get from mch-image-manifest.xxx configmap
	ThanosReceiveControllerKey    = "thanos_receive_controller"
	ThanosReceiveControllerImgTag = "master-2021-02-08-36d6090"

	ThanosImgName = "thanos"
	ThanosImgTag  = "2.3.0-SNAPSHOT-2021-03-19-13-50-28"

	MemcachedImgRepo = "docker.io"
	MemcachedImgName = "memcached"
	MemcachedImgTag  = "1.6.3-alpine"

	MemcachedExporterImgRepo = "prom"
	MemcachedExporterImgName = "memcached-exporter"
	MemcachedExporterKey     = "memcached_exporter"
	MemcachedExporterImgTag  = "v0.8.0"

	GrafanaImgRepo            = "grafana"
	GrafanaImgName            = "grafana"
	GrafanaImgTagSuffix       = "7.4.2"
	GrafanaDashboardLoaderKey = "grafana_dashboard_loader"

	AlertManagerImgRepo           = "quay.io/openshift"
	AlertManagerKey               = "prometheus-alertmanager"
	ConfigmapReloaderImgRepo      = "quay.io/openshift"
	ConfigmapReloaderImgName      = "origin-configmap-reloader"
	ConfigmapReloaderImgTagSuffix = "4.5.0"
	ConfigmapReloaderKey          = "prometheus-config-reloader"

	EndpointControllerImgTagSuffix = "2.2.0-6a5ea47fc39d51fb4fade6157843f2977442996e"
	EndpointControllerImgName      = "endpoint-monitoring-operator"
	EndpointControllerKey          = "endpoint_monitoring_operator"

	MetricsCollectorImgTagSuffix = "2.2.0-ff79e6ec8783756b942a77f08b3ab763dfd2dc15"
	MetricsCollectorImgName      = "metrics-collector"
	MetricsCollectorKey          = "metrics_collector"

	LeaseControllerImageTagSuffix = "2.1.0-a2899de5ce144e2c0441063e9ee8c4addf3ecb4a"
	LeaseControllerImageName      = "klusterlet-addon-lease-controller"
	LeaseControllerKey            = "klusterlet_addon_lease_controller"

	RbacQueryProxyKey = "rbac_query_proxy"
)

const (
	ObservatoriumAPICPURequets    = "20m"
	ObservatoriumAPIMemoryRequets = "128Mi"

	ThanosQueryFrontendCPURequets    = "100m"
	ThanosQueryFrontendMemoryRequets = "256Mi"

	ThanosQueryCPURequets    = "300m"
	ThanosQueryMemoryRequets = "1Gi"

	ThanosCompactCPURequets    = "100m"
	ThanosCompactMemoryRequets = "512Mi"

	ObservatoriumReceiveControllerCPURequets    = "4m"
	ObservatoriumReceiveControllerMemoryRequets = "32Mi"

	ThanosReceiveCPURequets    = "300m"
	ThanosReceiveMemoryRequets = "512Mi"

	ThanosRuleCPURequets            = "50m"
	ThanosRuleMemoryRequets         = "512Mi"
	ThanosRuleReloaderCPURequets    = "4m"
	ThanosRuleReloaderMemoryRequets = "25Mi"

	ThanosCahcedCPURequets            = "45m"
	ThanosCahcedMemoryRequets         = "128Mi"
	ThanosCahcedExporterCPURequets    = "5m"
	ThanosCahcedExporterMemoryRequets = "50Mi"

	ThanosStoreCPURequets    = "100m"
	ThanosStoreMemoryRequets = "1Gi"
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

// ReadImageManifestConfigMap reads configmap with the name is mch-image-manifest-xxx
func ReadImageManifestConfigMap(c client.Client) (bool, error) {
	//Only need to read if imageManifests is empty
	if len(imageManifests) != 0 {
		return false, nil
	}

	imageCMName := ImageManifestConfigMapName
	componentVersion, found := os.LookupEnv(ComponentVersion)
	if found {
		imageCMName = ImageManifestConfigMapName + componentVersion
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
			if errors.IsNotFound(err) {
				log.Info("Cannot get image manifest configmap", "configmap name", imageCMName)
			} else {
				log.Error(err, "Failed to read mch-image-manifest configmap")
				return false, err
			}

		}
	}
	return true, nil
}

// GetImageManifests...
func GetImageManifests() map[string]string {
	return imageManifests
}

// SetImageManifests sets imageManifests
func SetImageManifests(images map[string]string) {
	imageManifests = images
}

// ReplaceImage is used to replace the image with specified annotation or imagemanifest configmap
func ReplaceImage(annotations map[string]string, imageRepo, componentName string) (bool, string) {
	if annotations != nil {
		annotationImageRepo, _ := annotations[AnnotationKeyImageRepository]
		if annotationImageRepo == "" {
			annotationImageRepo = DefaultImgRepository
		}
		// This is for test only. e.g.:
		// if there is "mco-metrics_collector-image" defined in annotation, use it for testing
		componentImage, hasComponentImage := annotations["mco-"+componentName+"-image"]
		tagSuffix, hasTagSuffix := annotations[AnnotationKeyImageTagSuffix]
		sameOrg := strings.Contains(imageRepo, DefaultImgRepository)

		if hasComponentImage {
			return true, componentImage
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

// WithoutResourcesRequests returns true if the multiclusterobservability instance has annotation:
// mco-thanos-without-resources-requests: "true"
// This is just for test purpose: the KinD cluster does not have enough resources for the requests.
// We won't expose this annotation to the customer.
func WithoutResourcesRequests(annotations map[string]string) bool {
	if annotations == nil {
		return false
	}

	if annotations[AnnotationMCOWithoutResourcesRequests] != "" &&
		strings.EqualFold(annotations[AnnotationMCOWithoutResourcesRequests], "true") {
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
	return instanceName + "-observatorium-api." + defaultNamespace + ".svc.cluster.local"
}

// SetCustomRuleConfigMap set true if there is custom rule configmap
func SetCustomRuleConfigMap(hasConfigMap bool) {
	hasCustomRuleConfigMap = hasConfigMap
}

// HasCustomRuleConfigMap returns true if there is custom rule configmap
func HasCustomRuleConfigMap() bool {
	return hasCustomRuleConfigMap
}
