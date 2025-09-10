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
	CaConfigmapName              = "metrics-collector-serving-certs-ca-bundle"
	HubMetricsCollectorMtlsCert  = "observability-controller-open-cluster-management.io-observability-signer-client-cert"
	ClientCACertificateCN        = "observability-client-ca-certificate"
)

const (
	OCPClusterMonitoringNamespace         = "openshift-monitoring"
	OCPClusterMonitoringConfigMapName     = "cluster-monitoring-config"
	OCPClusterMonitoringPrometheusService = "prometheus-k8s"
	OCPUserWorkloadMonitoringNamespace    = "openshift-user-workload-monitoring"
	OCPUserWorkloadMonitoringConfigMap    = "user-workload-monitoring-config"
)

const (
	MetricsCollectorKey            = "metrics_collector"
	PrometheusKey                  = "prometheus"
	KubeStateMetricsKey            = "kube_state_metrics"
	NodeExporterKey                = "node_exporter"
	KubeRbacProxyKey               = "kube_rbac_proxy"
	PrometheusOperatorKey          = "prometheus_operator"
	PrometheusConfigmapReloaderKey = "prometheus_config_reloader"
)

// Annotations to uspport OpenShift workload partitioning.
const (
	WorkloadPartitioningPodAnnotationKey = "target.workload.openshift.io/management"
	WorkloadPodExpectedValueJSON         = "{\"effect\":\"PreferredDuringScheduling\"}"
	WorkloadPartitioningNSAnnotationsKey = "workload.openshift.io/allowed"
	WorkloadPartitioningNSExpectedValue  = "management"
)

var (
	IsMCOTerminating = false
)

const (
	DefaultClusterType = ""
	SnoClusterType     = "SNO"
)
