// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package config

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	imagev1client "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"

	ocinfrav1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	ocpClientSet "github.com/openshift/client-go/config/clientset/versioned"
	obsv1alpha1 "github.com/stolostron/observatorium-operator/api/v1alpha1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	observabilityv1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
)

const (
	crLabelKey                        = "observability.open-cluster-management.io/name"
	clusterNameLabelKey               = "cluster"
	obsAPIGateway                     = "observatorium-api"
	infrastructureConfigName          = "cluster"
	defaultMCONamespace               = "open-cluster-management"
	defaultNamespace                  = "open-cluster-management-observability"
	defaultTenantName                 = "default"
	defaultCRName                     = "observability"
	operandNamePrefix                 = "observability-"
	OpenshiftIngressOperatorNamespace = "openshift-ingress-operator"
	OpenshiftIngressNamespace         = "openshift-ingress"
	OpenshiftIngressOperatorCRName    = "default"
	OpenshiftIngressDefaultCertName   = "router-certs-default"
	OpenshiftIngressRouteCAName       = "router-ca"

	AnnotationKeyImageRepository = "mco-imageRepository"
	AnnotationKeyImageTagSuffix  = "mco-imageTagSuffix"
	AnnotationMCOPause           = "mco-pause"
	AnnotationCertDuration       = "mco-cert-duration"
	AnnotationDisableMCOAlerting = "mco-disable-alerting"

	MCHUpdatedRequestName               = "mch-updated-request"
	MCOUpdatedRequestName               = "mco-updated-request"
	ClusterManagementAddOnUpdateName    = "clustermgmtaddon-updated-request"
	AddonDeploymentConfigUpdateName     = "addondc-updated-request"
	MulticloudConsoleRouteName          = "multicloud-console"
	ImageManifestConfigMapNamePrefix    = "mch-image-manifest-"
	OCMManifestConfigMapTypeLabelKey    = "ocm-configmap-type"
	OCMManifestConfigMapTypeLabelValue  = "image-manifest"
	OCMManifestConfigMapVersionLabelKey = "ocm-release-version"

	ComponentVersion = "COMPONENT_VERSION"

	ServerCACerts    = "observability-server-ca-certs"
	ClientCACerts    = "observability-client-ca-certs"
	ServerCerts      = "observability-server-certs"
	ServerCertCN     = "observability-server-certificate"
	GrafanaCerts     = "observability-grafana-certs"
	GrafanaCN        = "grafana"
	ManagedClusterOU = "acm"

	GrafanaRouteName         = "grafana"
	GrafanaServiceName       = "grafana"
	GrafanaOauthClientName   = "grafana-proxy-client" // #nosec G101 -- Not a hardcoded credential.
	GrafanaOauthClientSecret = "grafana-proxy-client" // #nosec G101 -- Not a hardcoded credential.

	AlertmanagerAccessorSAName     = "observability-alertmanager-accessor"
	AlertmanagerAccessorSecretName = "observability-alertmanager-accessor" // #nosec G101 -- Not a hardcoded credential.
	AlertmanagerServiceName        = "alertmanager"
	AlertmanagerRouteName          = "alertmanager"
	AlertmanagerRouteBYOCAName     = "alertmanager-byo-ca"
	AlertmanagerRouteBYOCERTName   = "alertmanager-byo-cert"

	AlertRuleDefaultConfigMapName = "thanos-ruler-default-rules"
	AlertRuleDefaultFileKey       = "default_rules.yaml"
	AlertRuleCustomConfigMapName  = "thanos-ruler-custom-rules"
	AlertRuleCustomFileKey        = "custom_rules.yaml"
	AlertmanagerURL               = "http://alertmanager:9093"
	AlertmanagerConfigName        = "alertmanager-config"

	AlertmanagersDefaultConfigMapName     = "thanos-ruler-config"
	AlertmanagersDefaultConfigFileKey     = "config.yaml"
	AlertmanagersDefaultCaBundleMountPath = "/etc/thanos/configmaps/alertmanager-ca-bundle"
	AlertmanagersDefaultCaBundleName      = "alertmanager-ca-bundle"
	AlertmanagersDefaultCaBundleKey       = "service-ca.crt"

	AllowlistCustomConfigMapName              = "observability-metrics-custom-allowlist"
	ManagedClusterLabelAllowListConfigMapName = "observability-managed-cluster-label-allowlist"

	ProxyServiceName      = "rbac-query-proxy"
	ProxyRouteName        = "rbac-query-proxy"
	ProxyRouteBYOCAName   = "proxy-byo-ca"
	ProxyRouteBYOCERTName = "proxy-byo-cert"

	ValidatingWebhookConfigurationName = "multicluster-observability-operator"
	WebhookServiceName                 = "multicluster-observability-webhook-service"
	BackupLabelName                    = "cluster.open-cluster-management.io/backup"
	BackupLabelValue                   = ""
	OpenShiftClusterMonitoringlabel    = "openshift.io/cluster-monitoring"
)

const (
	DefaultImgRepository = "quay.io/stolostron"
	DefaultImgTagSuffix  = "2.4.0-SNAPSHOT-2021-09-23-07-02-14"

	ObservatoriumImgRepo           = "quay.io/observatorium"
	ObservatoriumAPIImgName        = "observatorium"
	ObservatoriumOperatorImgName   = "observatorium-operator"
	ObservatoriumOperatorImgKey    = "observatorium_operator"
	ThanosReceiveControllerImgName = "thanos-receive-controller"
	// ThanosReceiveControllerKey is used to get from mch-image-manifest.xxx configmap.
	ThanosReceiveControllerKey    = "thanos_receive_controller"
	ThanosReceiveControllerImgTag = "master-2022-04-01-b58820f"
	ThanosImgName                 = "thanos"

	MemcachedImgRepo = "quay.io/ocm-observability"
	MemcachedImgName = "memcached"
	MemcachedImgTag  = "1.6.3-alpine"

	MemcachedExporterImgRepo = "quay.io/prometheus"
	MemcachedExporterImgName = "memcached-exporter"
	MemcachedExporterKey     = "memcached_exporter"
	MemcachedExporterImgTag  = "v0.9.0"

	GrafanaImgKey               = "grafana"
	GrafanaDashboardLoaderName  = "grafana-dashboard-loader" // #nosec G101 -- Not a hardcoded credential.
	GrafanaDashboardLoaderKey   = "grafana_dashboard_loader" // #nosec G101 -- Not a hardcoded credential.
	GrafanaCustomDashboardLabel = "grafana-custom-dashboard"

	AlertManagerImgName           = "prometheus-alertmanager"
	AlertManagerImgKey            = "prometheus_alertmanager"
	ConfigmapReloaderImgRepo      = "quay.io/openshift"
	ConfigmapReloaderImgName      = "origin-configmap-reloader"
	ConfigmapReloaderImgTagSuffix = "4.8.0"
	ConfigmapReloaderKey          = "configmap_reloader"
	KubeRBACProxyKey              = "kube_rbac_proxy"
	KubeRBACProxyImgName          = "kube-rbac-proxy"

	EndpointControllerImgName = "endpoint-monitoring-operator"
	EndpointControllerKey     = "endpoint_monitoring_operator"

	MultiClusterObservabilityAddonImgRepo      = "quay.io/rhobs"
	MultiClusterObservabilityAddonImgName      = "multicluster-observability-addon"
	MultiClusterObservabilityAddonImgTagSuffix = "v0.0.1"
	MultiClusterObservabilityAddonImgKey       = "multicluster_observability_addon"

	RBACQueryProxyImgName = "rbac-query-proxy"
	RBACQueryProxyKey     = "rbac_query_proxy"

	ObservatoriumAPI               = "observatorium-api"
	ThanosCompact                  = "thanos-compact"
	ThanosQuery                    = "thanos-query"
	ThanosQueryFrontend            = "thanos-query-frontend"
	ThanosQueryFrontendMemcached   = "thanos-query-frontend-memcached"
	ThanosRule                     = "thanos-rule"
	ThanosReceive                  = "thanos-receive-default"
	ThanosStoreMemcached           = "thanos-store-memcached"
	ThanosStoreShard               = "thanos-store-shard"
	MemcachedExporter              = "memcached-exporter"
	Grafana                        = "grafana"
	RBACQueryProxy                 = "rbac-query-proxy"
	Alertmanager                   = "alertmanager"
	ThanosReceiveController        = "thanos-receive-controller"
	ObservatoriumOperator          = "observatorium-operator"
	MetricsCollector               = "metrics-collector"
	Observatorium                  = "observatorium"
	MultiClusterObservabilityAddon = "multicluster-observability-addon"

	RetentionResolutionRaw = "365d"
	RetentionResolution5m  = "365d"
	RetentionResolution1h  = "365d"
	RetentionInLocal       = "24h"
	DeleteDelay            = "48h"
	BlockDuration          = "2h"

	DefaultImagePullPolicy = "IfNotPresent"
	DefaultImagePullSecret = "multiclusterhub-operator-pull-secret"
)

const (
	MCORsName = "multiclusterobservabilities"
)

const (
	IngressControllerCRD           = "ingresscontrollers.operator.openshift.io"
	MCHCrdName                     = "multiclusterhubs.operator.open-cluster-management.io"
	MCOCrdName                     = "multiclusterobservabilities.observability.open-cluster-management.io"
	StorageVersionMigrationCrdName = "storageversionmigrations.migration.k8s.io"
	MCGHCrdName                    = "multiclusterglobalhubs.operator.open-cluster-management.io"
)

const (
	ResourceTypeConfigMap = "ConfigMap"
	ResourceTypeSecret    = "Secret"
)

const (
	HubEndpointOperatorName    = "endpoint-observability-operator"
	HubMetricsCollectorName    = "metrics-collector-deployment"
	HubUwlMetricsCollectorName = "uwl-metrics-collector-deployment"
	HubUwlMetricsCollectorNs   = "openshift-user-workload-monitoring"
	HubEndpointSaName          = "endpoint-observability-operator-sa"
)

const schemeHttps = "https"

const (
	OauthProxyImageStreamName      = "oauth-proxy"
	OauthProxyImageStreamNamespace = "openshift"
)

const (
	ClusterLogForwarderCRDName    = "clusterlogforwarders.observability.openshift.io"
	OpenTelemetryCollectorCRDName = "opentelemetrycollectors.opentelemetry.io"
	InstrumentationCRDName        = "instrumentations.opentelemetry.io"
	PrometheusAgentCRDName        = "prometheusagents.monitoring.coreos.com"
)

var (
	mcoaSupportedCRDs = map[string]string{
		ClusterLogForwarderCRDName:    "v1",
		OpenTelemetryCollectorCRDName: "v1beta1",
		InstrumentationCRDName:        "v1alpha1",
		PrometheusAgentCRDName:        "v1alpha1",
	}
)

// ObjectStorgeConf is used to Unmarshal from bytes to do validation.
type ObjectStorgeConf struct {
	Type   string `yaml:"type"`
	Config Config `yaml:"config"`
}

var (
	log                        = logf.Log.WithName("config")
	monitoringCRName           = ""
	tenantUID                  = ""
	imageManifests             = map[string]string{}
	imageManifestConfigMapName = ""
	hasCustomRuleConfigMap     = false
	certDuration               = time.Hour * 24 * 365
	isAlertingDisabled         = false

	// use this map to store the operand name.
	operandNames = map[string]string{}

	MemoryLimitMB   = int32(1024)
	ConnectionLimit = int32(1024)
	MaxItemSize     = "1m"

	BackupResourceMap = map[string]string{
		AllowlistCustomConfigMapName:              ResourceTypeConfigMap,
		AlertRuleCustomConfigMapName:              ResourceTypeConfigMap,
		ManagedClusterLabelAllowListConfigMapName: ResourceTypeConfigMap,

		AlertmanagerConfigName:       ResourceTypeSecret,
		AlertmanagerRouteBYOCAName:   ResourceTypeSecret,
		AlertmanagerRouteBYOCERTName: ResourceTypeSecret,
		ProxyRouteBYOCAName:          ResourceTypeSecret,
		ProxyRouteBYOCERTName:        ResourceTypeSecret,
		DefaultImagePullSecret:       ResourceTypeSecret,
	}

	multicloudConsoleRouteHost = ""
	imageManifestCache         sync.Map
)

// GetCrLabelKey returns the key for the CR label injected into the resources created by the operator.
func GetCrLabelKey() string {
	return crLabelKey
}

// GetClusterNameLabelKey returns the key for the injected label.
func GetClusterNameLabelKey() string {
	return clusterNameLabelKey
}

func GetImageManifestConfigMapName() string {
	return imageManifestConfigMapName
}

// ReadImageManifestConfigMap reads configmap with the label ocm-configmap-type=image-manifest.
func ReadImageManifestConfigMap(c client.Client, version string) (map[string]string, bool, error) {
	mcoNamespace := GetMCONamespace()
	// List image manifest configmap with label ocm-configmap-type=image-manifest and ocm-release-version
	matchLabels := map[string]string{
		OCMManifestConfigMapTypeLabelKey:    OCMManifestConfigMapTypeLabelValue,
		OCMManifestConfigMapVersionLabelKey: version,
	}
	listOpts := []client.ListOption{
		client.InNamespace(mcoNamespace),
		client.MatchingLabels(matchLabels),
	}

	imageCMList := &corev1.ConfigMapList{}
	err := c.List(context.TODO(), imageCMList, listOpts...)
	if err != nil {
		return nil, false, fmt.Errorf("failed to list mch-image-manifest configmaps: %w", err)
	}

	if len(imageCMList.Items) != 1 {
		// there should be only one matched image manifest configmap found
		return nil, false, nil
	}

	imageManifests = imageCMList.Items[0].Data
	log.V(1).Info("Coleen test the length of mch-image-manifest configmap", "imageManifests", len(imageManifests))
	return imageManifests, true, nil
}

// GetImageManifests...
func GetImageManifests() map[string]string {
	return imageManifests
}

// SetImageManifests sets imageManifests.
func SetImageManifests(images map[string]string) {
	imageManifests = images
}

// ReplaceImage is used to replace the image with specified annotation or imagemanifest configmap.
func ReplaceImage(annotations map[string]string, imageRepo, componentName string) (bool, string) {
	if annotations != nil {
		annotationImageRepo := annotations[AnnotationKeyImageRepository]
		if annotationImageRepo == "" {
			annotationImageRepo = DefaultImgRepository
		}
		// This is for test only. e.g.:
		// if there is "mco-metrics_collector-image" defined in annotation, use it for testing
		componentImage, hasComponentImage := annotations["mco-"+componentName+"-image"]
		tagSuffix, hasTagSuffix := annotations[AnnotationKeyImageTagSuffix]
		sameOrg := strings.Contains(imageRepo, DefaultImgRepository)

		if hasComponentImage {
			log.V(1).Info("image replacement: custom image found", "componentImage", componentImage)
			return true, componentImage
		} else if hasTagSuffix && sameOrg {
			repoSlice := strings.Split(imageRepo, "/")
			imageName := strings.Split(repoSlice[len(repoSlice)-1], ":")[0]
			image := annotationImageRepo + "/" + imageName + ":" + tagSuffix
			log.V(1).Info("image replacement: has tag suffix", "componentName", componentName, "imageRepo", imageRepo, "image", image)
			return true, image
		} else if !hasTagSuffix {
			image, found := imageManifests[componentName]
			log.V(1).Info("image replacement", "componentName", componentName, "image", image)
			if found {
				return true, image
			}
			return false, ""
		}
		return false, ""
	} else {
		image, found := imageManifests[componentName]
		log.V(1).Info("image replacement", "componentName", componentName, "image", image)
		if found {
			return true, image
		}
		return false, ""
	}
}

// GetDefaultTenantName returns the default tenant name.
func GetDefaultTenantName() string {
	return defaultTenantName
}

// GetObsAPIRouteHost is used to Route's host for Observatorium API. This doesn't take into consideration
// the `advanced.customObservabilityHubURL` configuration.
func GetObsAPIRouteHost(ctx context.Context, client client.Client, namespace string) (string, error) {
	return GetRouteHost(client, obsAPIGateway, namespace)
}

// GetObsAPIExternalURL is used to get the frontend URL that should be used to reach the Observatorium API instance.
// This takes into consideration the `advanced.customObservabilityHubURL` configuration.
func GetObsAPIExternalURL(ctx context.Context, client client.Client, namespace string) (*url.URL, error) {
	mco := &observabilityv1beta2.MultiClusterObservability{}
	err := client.Get(ctx,
		types.NamespacedName{
			Name: GetMonitoringCRName(),
		}, mco)
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}
	advancedConfig := mco.Spec.AdvancedConfig
	if advancedConfig != nil && advancedConfig.CustomObservabilityHubURL != "" {
		hubObsUrl := advancedConfig.CustomObservabilityHubURL
		obsURL, err := hubObsUrl.URL()
		if err != nil {
			return nil, err
		}
		return obsURL, nil
	}
	routeHost, err := GetRouteHost(client, obsAPIGateway, namespace)
	if err != nil {
		return nil, err
	}
	return url.Parse(fmt.Sprintf("%s://%s", schemeHttps, routeHost))
}

func GetRouteHost(client client.Client, name string, namespace string) (string, error) {
	found := &routev1.Route{}

	err := client.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		// if the router is not created yet, fallback to get host
		// from the domain of ingresscontroller
		domain, err := getDomainForIngressController(
			client,
			OpenshiftIngressOperatorCRName,
			OpenshiftIngressOperatorNamespace,
		)
		if err != nil {
			return "", nil
		}
		return name + "-" + namespace + "." + domain, nil
	} else if err != nil {
		return "", err
	}
	return found.Spec.Host, nil
}

func GetMCONamespace() string {
	podNamespace, found := os.LookupEnv("POD_NAMESPACE")
	if !found {
		podNamespace = defaultMCONamespace
	}
	return podNamespace
}

// GetAlertmanagerURL is used to get the URL for alertmanager.
func GetAlertmanagerURL(ctx context.Context, client client.Client, namespace string) (*url.URL, error) {
	mco := &observabilityv1beta2.MultiClusterObservability{}
	err := client.Get(ctx,
		types.NamespacedName{
			Name: GetMonitoringCRName(),
		}, mco)
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}
	advancedConfig := mco.Spec.AdvancedConfig
	if advancedConfig != nil && advancedConfig.CustomAlertmanagerHubURL != "" {
		err := advancedConfig.CustomAlertmanagerHubURL.Validate()
		if err != nil {
			return nil, err
		}
		return advancedConfig.CustomAlertmanagerHubURL.URL()
	}

	found := &routev1.Route{}
	err = client.Get(ctx, types.NamespacedName{Name: AlertmanagerRouteName, Namespace: namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		// if the alertmanager router is not created yet, fallback to get host from the domain of ingresscontroller
		domain, err := getDomainForIngressController(
			client,
			OpenshiftIngressOperatorCRName,
			OpenshiftIngressOperatorNamespace,
		)
		if err != nil {
			return nil, err
		}
		return url.Parse(fmt.Sprintf("%s://%s-%s.%s", schemeHttps, AlertmanagerRouteName, namespace, domain))
	} else if err != nil {
		return nil, err
	}
	return url.Parse(fmt.Sprintf("%s://%s", schemeHttps, found.Spec.Host))
}

// getDomainForIngressController get the domain for the given ingresscontroller instance.
func getDomainForIngressController(client client.Client, name, namespace string) (string, error) {
	ingressOperatorInstance := &operatorv1.IngressController{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, ingressOperatorInstance)
	if err != nil {
		return "", err
	}
	domain := ingressOperatorInstance.Status.Domain
	if domain == "" {
		return "", fmt.Errorf("no domain found in the ingressOperator: %s/%s.", namespace, name)
	}
	return domain, nil
}

// GetAlertmanagerRouterCA is used to get the CA of openshift Route.
func GetAlertmanagerRouterCA(client client.Client) (string, error) {
	amRouteBYOCaSrt := &corev1.Secret{}
	amRouteBYOCertSrt := &corev1.Secret{}
	err1 := client.Get(
		context.TODO(),
		types.NamespacedName{Name: AlertmanagerRouteBYOCAName, Namespace: GetDefaultNamespace()},
		amRouteBYOCaSrt,
	)
	err2 := client.Get(
		context.TODO(),
		types.NamespacedName{Name: AlertmanagerRouteBYOCERTName, Namespace: GetDefaultNamespace()},
		amRouteBYOCertSrt,
	)
	if err1 == nil && err2 == nil {
		return string(amRouteBYOCaSrt.Data["tls.crt"]), nil
	}

	ingressOperator := &operatorv1.IngressController{}
	err := client.Get(
		context.TODO(),
		types.NamespacedName{Name: OpenshiftIngressOperatorCRName, Namespace: OpenshiftIngressOperatorNamespace},
		ingressOperator,
	)
	if err != nil {
		return "", err
	}

	routerCASrtName := OpenshiftIngressDefaultCertName
	// check if custom default certificate is provided or not
	if ingressOperator.Spec.DefaultCertificate != nil {
		routerCASrtName = ingressOperator.Spec.DefaultCertificate.Name
	}

	routerCASecret := &corev1.Secret{}
	err = client.Get(
		context.TODO(),
		types.NamespacedName{Name: routerCASrtName, Namespace: OpenshiftIngressNamespace},
		routerCASecret,
	)
	if err != nil {
		return "", err
	}
	return string(routerCASecret.Data["tls.crt"]), nil
}

// GetAlertmanagerCA is used to get the CA of Alertmanager.
func GetAlertmanagerCA(client client.Client) (string, error) {
	amCAConfigmap := &corev1.ConfigMap{}
	err := client.Get(
		context.TODO(),
		types.NamespacedName{Name: AlertmanagersDefaultCaBundleName, Namespace: GetDefaultNamespace()},
		amCAConfigmap,
	)
	if err != nil {
		return "", err
	}
	return string(amCAConfigmap.Data["service-ca.crt"]), nil
}

func GetDefaultNamespace() string {
	return defaultNamespace
}

// GetMonitoringCRName returns monitoring cr name.
func GetMonitoringCRName() string {
	return monitoringCRName
}

// SetMonitoringCRName sets the cr name.
func SetMonitoringCRName(crName string) {
	monitoringCRName = crName
}

func infrastructureConfigNameNsN() types.NamespacedName {
	return types.NamespacedName{
		Name: infrastructureConfigName,
	}
}

// GetKubeAPIServerAddress is used to get the api server url.
func GetKubeAPIServerAddress(client client.Client) (string, error) {
	infraConfig := &ocinfrav1.Infrastructure{}
	if err := client.Get(context.TODO(), infrastructureConfigNameNsN(), infraConfig); err != nil {
		return "", err
	}

	return infraConfig.Status.APIServerURL, nil
}

// GetClusterID is used to get the cluster uid.
func GetClusterID(ocpClient ocpClientSet.Interface) (string, error) {
	clusterVersion, err := ocpClient.ConfigV1().ClusterVersions().Get(context.TODO(), "version", v1.GetOptions{})
	if err != nil {
		log.Error(err, "Failed to get clusterVersion")
		return "", err
	}

	return string(clusterVersion.Spec.ClusterID), nil
}

// checkIsIBMCloud detects if the current cloud vendor is ibm or not
// we know we are on OCP already, so if it's also ibm cloud, it's roks.
func CheckIsIBMCloud(c client.Client) (bool, error) {
	nodes := &corev1.NodeList{}
	err := c.List(context.TODO(), nodes)
	if err != nil {
		log.Error(err, "Failed to get nodes list")
		return false, err
	}
	if len(nodes.Items) == 0 {
		log.Error(err, "Failed to list any nodes")
		return false, nil
	}

	providerID := nodes.Items[0].Spec.ProviderID
	if strings.Contains(providerID, "ibm") {
		return true, nil
	}

	return false, nil
}

// GetDefaultCRName is used to get default CR name.
func GetDefaultCRName() string {
	return defaultCRName
}

// IsPaused returns true if the multiclusterobservability instance is labeled as paused, and false otherwise.
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

// GetTenantUID returns tenant uid.
func GetTenantUID() string {
	if tenantUID == "" {
		tenantUID = string(uuid.NewUUID())
	}
	return tenantUID
}

// GetObsAPISvc returns observatorium api service.
func GetObsAPISvc(instanceName string) string {
	return instanceName + "-observatorium-api." + defaultNamespace + ".svc.cluster.local"
}

// SetCustomRuleConfigMap set true if there is custom rule configmap.
func SetCustomRuleConfigMap(hasConfigMap bool) {
	hasCustomRuleConfigMap = hasConfigMap
}

// HasCustomRuleConfigMap returns true if there is custom rule configmap.
func HasCustomRuleConfigMap() bool {
	return hasCustomRuleConfigMap
}

func GetCertDuration() time.Duration {
	return certDuration
}

func SetCertDuration(annotations map[string]string) {
	if annotations != nil && annotations[AnnotationCertDuration] != "" {
		d, err := time.ParseDuration(annotations[AnnotationCertDuration])
		if err != nil {
			log.Error(
				err,
				"Failed to parse cert duration, use default one",
				"annotation",
				annotations[AnnotationCertDuration],
			)
		} else {
			certDuration = d
			return
		}
	}
	certDuration = time.Hour * 24 * 365
}

func GetOperandNamePrefix() string {
	return operandNamePrefix
}

func GetImagePullPolicy(mco observabilityv1beta2.MultiClusterObservabilitySpec) corev1.PullPolicy {
	if mco.ImagePullPolicy != "" {
		return mco.ImagePullPolicy
	} else {
		return DefaultImagePullPolicy
	}
}

func GetImagePullSecret(mco observabilityv1beta2.MultiClusterObservabilitySpec) string {
	if mco.ImagePullSecret != "" {
		return mco.ImagePullSecret
	} else {
		return DefaultImagePullSecret
	}
}

func GetOperandName(name string) string {
	log.V(1).Info("operand is", "key", name, "name", operandNames[name])
	return operandNames[name]
}

func SetOperandNames(c client.Client) error {
	if len(operandNames) != 0 {
		return nil
	}
	// set the default values.
	operandNames[Grafana] = GetOperandNamePrefix() + Grafana
	operandNames[RBACQueryProxy] = GetOperandNamePrefix() + RBACQueryProxy
	operandNames[Alertmanager] = GetOperandNamePrefix() + Alertmanager
	operandNames[ObservatoriumOperator] = GetOperandNamePrefix() + ObservatoriumOperator
	operandNames[Observatorium] = GetDefaultCRName()
	operandNames[ObservatoriumAPI] = GetOperandNamePrefix() + ObservatoriumAPI
	operandNames[MultiClusterObservabilityAddon] = GetOperandNamePrefix() + MultiClusterObservabilityAddon

	// Check if the Observatorium CR already exists
	opts := &client.ListOptions{
		Namespace: GetDefaultNamespace(),
	}

	observatoriumList := &obsv1alpha1.ObservatoriumList{}
	err := c.List(context.TODO(), observatoriumList, opts)
	if err != nil {
		return err
	}
	if len(observatoriumList.Items) != 0 {
		for _, observatorium := range observatoriumList.Items {
			for _, ownerRef := range observatorium.OwnerReferences {
				if ownerRef.Kind == "MultiClusterObservability" && ownerRef.Name == GetMonitoringCRName() {
					if observatorium.Name != GetDefaultCRName() {
						// this is for upgrade case.
						operandNames[Grafana] = Grafana
						operandNames[RBACQueryProxy] = RBACQueryProxy
						operandNames[Alertmanager] = Alertmanager
						operandNames[ObservatoriumOperator] = ObservatoriumOperator
						operandNames[Observatorium] = observatorium.Name
						operandNames[ObservatoriumAPI] = observatorium.Name + "-" + ObservatoriumAPI
						operandNames[MultiClusterObservabilityAddon] = MultiClusterObservabilityAddon
					}
					break
				}
			}
		}
	}

	return nil
}

// CleanUpOperandNames delete all the operand name items.
// Should be called when the MCO CR is deleted.
func CleanUpOperandNames() {
	for k := range operandNames {
		delete(operandNames, k)
	}
}

// GetValidatingWebhookConfigurationForMCO return the ValidatingWebhookConfiguration for the MCO validaing webhook.
func GetValidatingWebhookConfigurationForMCO() *admissionregistrationv1.ValidatingWebhookConfiguration {
	validatingWebhookPath := "/validate-observability-open-cluster-management-io-v1beta2-multiclusterobservability"
	noSideEffects := admissionregistrationv1.SideEffectClassNone
	allScopeType := admissionregistrationv1.AllScopes
	webhookServiceNamespace := GetMCONamespace()
	webhookServicePort := int32(443)
	return &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: v1.ObjectMeta{
			Name: ValidatingWebhookConfigurationName,
			Labels: map[string]string{
				"name": ValidatingWebhookConfigurationName,
			},
			Annotations: map[string]string{
				"service.beta.openshift.io/inject-cabundle": "true",
			},
		},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{
			{
				AdmissionReviewVersions: []string{"v1", "v1beta1"},
				Name:                    "vmulticlusterobservability.observability.open-cluster-management.io",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Name:      WebhookServiceName,
						Namespace: webhookServiceNamespace,
						Path:      &validatingWebhookPath,
						Port:      &webhookServicePort,
					},
					CABundle: []byte(""),
				},
				SideEffects: &noSideEffects,
				Rules: []admissionregistrationv1.RuleWithOperations{
					{
						Operations: []admissionregistrationv1.OperationType{
							admissionregistrationv1.Create,
							admissionregistrationv1.Update,
						},
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{"observability.open-cluster-management.io"},
							APIVersions: []string{"v1beta2"},
							Resources:   []string{"multiclusterobservabilities"},
							Scope:       &allScopeType,
						},
					},
				},
			},
		},
	}
}

// GetMulticloudConsoleHost is used to get the URL for multicloud-console route.
func GetMulticloudConsoleHost(client client.Client, isStandalone bool) (string, error) {
	if multicloudConsoleRouteHost != "" {
		return multicloudConsoleRouteHost, nil
	}

	if isStandalone {
		return "", nil
	}

	namespace := GetMCONamespace()
	found := &routev1.Route{}

	err := client.Get(context.TODO(), types.NamespacedName{
		Name: MulticloudConsoleRouteName, Namespace: namespace,
	}, found)
	if err != nil {
		return "", err
	}
	return found.Spec.Host, nil
}

// Set AnnotationMCOAlerting.
func SetAlertingDisabled(status bool) {
	isAlertingDisabled = status
}

func IsAlertingDisabled() bool {
	return isAlertingDisabled
}

// Get AnnotationMCOAlerting.
func IsAlertingDisabledInSpec(mco *observabilityv1beta2.MultiClusterObservability) bool {
	if mco == nil {
		return false
	}

	annotations := mco.GetAnnotations()
	return annotations != nil && annotations[AnnotationDisableMCOAlerting] == "true"
}

func GetOauthProxyImage(imageClient imagev1client.ImageV1Interface) (bool, string) {
	if imageClient != nil && !reflect.ValueOf(imageClient).IsNil() {
		// set oauth-proxy from imagestream.image.openshift.io
		oauthImageStream, err := imageClient.ImageStreams(OauthProxyImageStreamNamespace).
			Get(context.TODO(), OauthProxyImageStreamName, v1.GetOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				return false, ""
			}
			// do not expect error = IsNotFound in OCP environment.
			// But for e2e test, it can be. for this case, just ignore
		} else {
			if oauthImageStream.Spec.Tags != nil {
				tag := oauthImageStream.Spec.Tags[0]
				if tag.From != nil && tag.From.Kind == "DockerImage" && len(tag.From.Name) > 0 {
					return true, tag.From.Name
				}
			}
		}
	}
	return false, ""
}

func GetMCOASupportedCRDNames() []string {
	var names []string
	for name := range mcoaSupportedCRDs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func GetMCOASupportedCRDVersion(name string) string {
	version, ok := mcoaSupportedCRDs[name]
	if !ok {
		return ""
	}

	return version
}

func GetMCOASupportedCRDFQDN(name string) string {
	version, ok := mcoaSupportedCRDs[name]
	if !ok {
		return ""
	}

	parts := strings.SplitN(name, ".", 2)

	return fmt.Sprintf("%s.%s.%s", parts[0], version, parts[1])
}

func SetCachedImageManifestData(data map[string]string) {
	imageManifestCache.Store("mch-image-manifest", data)
}

func GetCachedImageManifestData() (map[string]string, bool) {
	if value, ok := imageManifestCache.Load("mch-image-manifest"); ok {
		if cachedData, valid := value.(map[string]string); valid {
			return cachedData, true
		}
	}
	return nil, false
}
