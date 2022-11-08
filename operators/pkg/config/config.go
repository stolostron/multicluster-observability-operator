// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

const (
	ClusterNameKey                  = "cluster-name"
	HubInfoSecretName               = "hub-info-secret"
	HubInfoSecretKey                = "hub-info.yaml" // #nosec
	ObservatoriumAPIRemoteWritePath = "/api/metrics/v1/default/api/v1/receive"
	AnnotationSkipCreation          = "skip-creation-if-exist"

	ClusterLabelKeyForAlerts = "managed_cluster"

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

	PrometheusOperatorImgName = "prometheus-operator"
	PrometheusOperatorKey     = "prometheus_operator"

	PrometheusConfigmapReloaderImgName = "prometheus-config-reloader"
	PrometheusConfigmapReloaderKey     = "prometheus_config_reloader"
)

var (
	ImageKeyNameMap = map[string]string{
		PrometheusKey:                  PrometheusKey,
		KubeStateMetricsKey:            KubeStateMetricsImgName,
		NodeExporterKey:                NodeExporterImgName,
		KubeRbacProxyKey:               KubeRbacProxyImgName,
		MetricsCollectorKey:            MetricsCollectorImgName,
		PrometheusConfigmapReloaderKey: PrometheusConfigmapReloaderImgName,
	}
)
