// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package config

const (
	ClusterNameKey                  = "cluster-name"
	HubInfoSecretName               = "hub-info-secret"
	HubInfoSecretKey                = "hub-info.yaml" // #nosec G101 -- Not a hardcoded credential.
	ObservatoriumAPIRemoteWritePath = "/api/metrics/v1/default/api/v1/receive"
	AnnotationSkipCreation          = "skip-creation-if-exist"
	ObservabilityAddonName          = "observability-addon"

	ClusterLabelKeyForAlerts = "managed_cluster"

	CollectorImage               = "COLLECTOR_IMAGE"
	InstallPrometheus            = "INSTALL_PROM"
	PullSecret                   = "PULL_SECRET"
	ImageConfigMap               = "images-list"
	AllowlistConfigMapName       = "observability-metrics-allowlist"
	AllowlistCustomConfigMapName = "observability-metrics-custom-allowlist"
	MetricsConfigMapKey          = "metrics_list.yaml"
	UwlMetricsConfigMapKey       = "uwl_metrics_list.yaml"
	PrometheusUserWorkload       = "prometheus-user-workload"
	MetricsOcp311ConfigMapKey    = "ocp311_metrics_list.yaml"
	CaConfigmapName              = "metrics-collector-serving-certs-ca-bundle"
	HubMetricsCollectorMtlsCert  = "observability-controller-open-cluster-management.io-observability-signer-client-cert"
	ClientCACertificateCN        = "observability-client-ca-certificate"
)

const (
	OCPClusterMonitoringNamespace         = "openshift-monitoring"
	OCPClusterMonitoringConfigMapName     = "cluster-monitoring-config"
	OCPClusterMonitoringConfigMapKey      = "config.yaml"
	OCPClusterMonitoringPrometheusService = "prometheus-k8s"
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

// Annotations to uspport OpenShift workload partitioning.
const (
	WorkloadPartitioningPodAnnotationKey = "target.workload.openshift.io/management"
	WorkloadPodExpectedValueJSON         = "{\"effect\":\"PreferredDuringScheduling\"}"
	WorkloadPartitioningNSAnnotationsKey = "workload.openshift.io/allowed"
	WorkloadPartitioningNSExpectedValue  = "management"
)

var ImageKeyNameMap = map[string]string{
	PrometheusKey:                  PrometheusKey,
	KubeStateMetricsKey:            KubeStateMetricsImgName,
	NodeExporterKey:                NodeExporterImgName,
	KubeRbacProxyKey:               KubeRbacProxyImgName,
	MetricsCollectorKey:            MetricsCollectorImgName,
	PrometheusConfigmapReloaderKey: PrometheusConfigmapReloaderImgName,
}

var (
	IsMCOTerminating = false
)

const (
	DefaultClusterType  = ""
	OcpThreeClusterType = "ocp3"
	SnoClusterType      = "SNO"
)
