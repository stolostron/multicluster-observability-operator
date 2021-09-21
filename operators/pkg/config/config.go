// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

const (
	ClusterNameKey                  = "cluster-name"
	HubInfoSecretName               = "hub-info-secret"
	HubInfoSecretKey                = "hub-info.yaml" // #nosec
	ObservatoriumAPIRemoteWritePath = "/api/metrics/v1/default/api/v1/receive"
	AnnotationSkipCreation          = "skip-creation-if-exist"

	CollectorImage         = "COLLECTOR_IMAGE"
	InstallPrometheus      = "INSTALL_PROM"
	PullSecret             = "PULL_SECRET"
	ImageConfigMap         = "images-list"
	AllowlistConfigMapName = "observability-metrics-allowlist"
)

const (
	MetricsCollectorImgName = "metrics-collector"
	MetricsCollectorKey     = "metrics_collector"

	PrometheusImgName = "prometheus"
	PrometheusKey     = "prometheus"

	KubeStateMetricsImgName = "kube-state-metrics"
	KubeStateMetricsKey     = "kube_state_metrics"

	NodeExporterImgName = "node-exporter"
	NodeExporterKey     = "node_exporter"

	KubeRbacProxyImgName = "kube-rbac-proxy"
	KubeRbacProxyKey     = "kube_rbac_proxy"

	ConfigmapReloaderImgName = "origin-configmap-reloader"
	ConfigmapReloaderKey     = "prometheus-config-reloader"
)

var (
	ImageKeyNameMap = map[string]string{
		PrometheusKey:        PrometheusKey,
		KubeStateMetricsKey:  KubeStateMetricsImgName,
		NodeExporterKey:      NodeExporterImgName,
		KubeRbacProxyKey:     KubeRbacProxyImgName,
		MetricsCollectorKey:  MetricsCollectorImgName,
		ConfigmapReloaderKey: ConfigmapReloaderImgName,
	}
)

// HubInfo is the struct that contains the common information about the hub
// cluster, for example the name of managed cluster on the hub, the URL of
// observatorium api gateway, the URL of hub alertmanager and the CA for the
// hub router
type HubInfo struct {
	ClusterName              string `yaml:"cluster-name"`
	ObservatoriumAPIEndpoint string `yaml:"observatorium-api-endpoint"`
	AlertmanagerEndpoint     string `yaml:"alertmanager-endpoint"`
	AlertmanagerRouterCA     string `yaml:"alertmanager-router-ca"`
}
