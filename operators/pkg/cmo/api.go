// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

// This code was copied from
// https://github.com/openshift/cluster-monitoring-operator/blob/f15addc8c1bd77851d395c662d452a365fb05370/pkg/manifests/types.go
// in order to avoid a dependency on the cluster-monitoring-operator repository for this specific version.
// This was required due to conflicts in imported k8s code base between the two repositories.
// Future releases have included an upgrade of the cmo package itself to the latest version.

package cmo

import (
	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	v1 "k8s.io/api/core/v1"
	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
)

// ClusterMonitoringConfiguration resource defines settings that
// customize the default platform monitoring stack through the
// `cluster-monitoring-config` config map in the `openshift-monitoring`
// namespace.
type ClusterMonitoringConfiguration struct {
	// `AlertmanagerMainConfig` defines settings for the
	// Alertmanager component in the `openshift-monitoring` namespace.
	AlertmanagerMainConfig *AlertmanagerMainConfig `json:"alertmanagerMain,omitempty"`
	// `UserWorkloadEnabled` is a Boolean flag that enables monitoring for user-defined projects.
	UserWorkloadEnabled *bool `json:"enableUserWorkload,omitempty"`
	// OmitFromDoc
	HTTPConfig *HTTPConfig `json:"http,omitempty"`
	// `K8sPrometheusAdapter` defines settings for the Prometheus Adapter component.
	K8sPrometheusAdapter *K8sPrometheusAdapter `json:"k8sPrometheusAdapter,omitempty"`
	// `MetricsServer` defines settings for the MetricsServer component.
	MetricsServerConfig *MetricsServerConfig `json:"metricsServer,omitempty"`
	// `KubeStateMetricsConfig` defines settings for the `kube-state-metrics` agent.
	KubeStateMetricsConfig *KubeStateMetricsConfig `json:"kubeStateMetrics,omitempty"`
	// `PrometheusK8sConfig` defines settings for the Prometheus component.
	PrometheusK8sConfig *PrometheusK8sConfig `json:"prometheusK8s,omitempty"`
	// `PrometheusOperatorConfig` defines settings for the Prometheus Operator component.
	PrometheusOperatorConfig *PrometheusOperatorConfig `json:"prometheusOperator,omitempty"`
	// `PrometheusOperatorAdmissionWebhookConfig` defines settings for the Prometheus Operator's admission webhook component.
	PrometheusOperatorAdmissionWebhookConfig *PrometheusOperatorAdmissionWebhookConfig `json:"prometheusOperatorAdmissionWebhook,omitempty"`
	// `OpenShiftMetricsConfig` defines settings for the `openshift-state-metrics` agent.
	OpenShiftMetricsConfig *OpenShiftStateMetricsConfig `json:"openshiftStateMetrics,omitempty"`
	// `TelemeterClientConfig` defines settings for the Telemeter Client
	// component.
	TelemeterClientConfig *TelemeterClientConfig `json:"telemeterClient,omitempty"`
	// `ThanosQuerierConfig` defines settings for the Thanos Querier component.
	ThanosQuerierConfig *ThanosQuerierConfig `json:"thanosQuerier,omitempty"`
	// `NodeExporterConfig` defines settings for the `node-exporter` agent.
	NodeExporterConfig NodeExporterConfig `json:"nodeExporter,omitempty"`
	// `MonitoringPluginConfig` defines settings for the monitoring `console-plugin`.
	MonitoringPluginConfig *MonitoringPluginConfig `json:"monitoringPlugin,omitempty"`
}

// AlertmanagerMainConfig resource defines settings for the
// Alertmanager component in the `openshift-monitoring` namespace.
type AlertmanagerMainConfig struct {
	// A Boolean flag that enables or disables the main Alertmanager instance
	// in the `openshift-monitoring` namespace.
	// The default value is `true`.
	Enabled *bool `json:"enabled,omitempty"`
	// A Boolean flag that enables or disables user-defined namespaces
	// to be selected for `AlertmanagerConfig` lookups. This setting only
	// applies if the user workload monitoring instance of Alertmanager
	// is not enabled.
	// The default value is `false`.
	EnableUserAlertManagerConfig bool `json:"enableUserAlertmanagerConfig,omitempty"`
	// Defines the log level setting for Alertmanager.
	// The possible values are: `error`, `warn`, `info`, `debug`.
	// The default value is `info`.
	LogLevel string `json:"logLevel,omitempty"`
	// Defines the nodes on which the Pods are scheduled.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Defines resource requests and limits for the Alertmanager container.
	Resources *v1.ResourceRequirements `json:"resources,omitempty"`
	// Defines a list of secrets that need to be mounted into the Alertmanager.
	// The secrets must reside within the same namespace as the Alertmanager object.
	// They will be added as volumes named secret-<secret-name> and mounted at
	// /etc/alertmanager/secrets/<secret-name> within the 'alertmanager' container of
	// the Alertmanager Pods.
	Secrets []string `json:"secrets,omitempty"`
	// Defines tolerations for the pods.
	Tolerations []v1.Toleration `json:"tolerations,omitempty"`
	// Defines a pod's topology spread constraints.
	TopologySpreadConstraints []v1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
	// Defines persistent storage for Alertmanager. Use this setting to
	// configure the persistent volume claim, including storage class, volume
	// size, and name.
	VolumeClaimTemplate *monv1.EmbeddedPersistentVolumeClaim `json:"volumeClaimTemplate,omitempty"`
}

// HTTPConfig resource defines settings for the HTTP proxy.
type HTTPConfig struct {
	HTTPProxy  string `json:"httpProxy"`
	HTTPSProxy string `json:"httpsProxy"`
	NoProxy    string `json:"noProxy"`
}

// K8sPrometheusAdapter resource defines settings for the Prometheus Adapter component.
// This is deprecated and will be removed in a future version.
type K8sPrometheusAdapter struct {
	// Defines the audit configuration used by the Prometheus Adapter instance.
	// Possible profile values are: `metadata`, `request`, `requestresponse`, and `none`.
	// The default value is `metadata`.
	Audit *Audit `json:"audit,omitempty"`
	// Defines the nodes on which the pods are scheduled.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Defines resource requests and limits for the PrometheusAdapter container.
	Resources *v1.ResourceRequirements `json:"resources,omitempty"`
	// Defines tolerations for the pods.
	Tolerations []v1.Toleration `json:"tolerations,omitempty"`
	// Defines a pod's topology spread constraints.
	TopologySpreadConstraints []v1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
}

// MetricsServerConfig resource defines settings for the Metrics Server component.
type MetricsServerConfig struct {
	// Defines the audit configuration used by the Metrics Server instance.
	// Possible profile values are: `metadata`, `request`, `requestresponse`, and `none`.
	// The default value is `metadata`.
	Audit *Audit `json:"audit,omitempty"`
	// Defines the nodes on which the pods are scheduled.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Defines tolerations for the pods.
	Tolerations []v1.Toleration `json:"tolerations,omitempty"`
	// Defines resource requests and limits for the Metrics Server container.
	Resources *v1.ResourceRequirements `json:"resources,omitempty"`
	// Defines a pod's topology spread constraints.
	TopologySpreadConstraints []v1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
}

// KubeStateMetricsConfig resource defines settings for the `kube-state-metrics` agent.
type KubeStateMetricsConfig struct {
	// Defines the nodes on which the pods are scheduled.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Defines resource requests and limits for the KubeStateMetrics container.
	Resources *v1.ResourceRequirements `json:"resources,omitempty"`
	// Defines tolerations for the pods.
	Tolerations []v1.Toleration `json:"tolerations,omitempty"`
	// Defines a pod's topology spread constraints.
	TopologySpreadConstraints []v1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
}

// PrometheusK8sConfig resource defines settings for the Prometheus component.
type PrometheusK8sConfig struct {
	// Configures additional Alertmanager instances that receive alerts from
	// the Prometheus component. By default, no additional Alertmanager
	// instances are configured.
	AlertmanagerConfigs []AdditionalAlertmanagerConfig `json:"additionalAlertmanagerConfigs,omitempty"`
	// Enforces a body size limit for Prometheus scraped metrics. If a scraped
	// target's body response is larger than the limit, the scrape will fail.
	// The following values are valid:
	// an empty value to specify no limit,
	// a numeric value in Prometheus size format (such as `64MB`), or
	// the string `automatic`, which indicates that the limit will be
	// automatically calculated based on cluster capacity.
	// The default value is empty, which indicates no limit.
	EnforcedBodySizeLimit string `json:"enforcedBodySizeLimit,omitempty"`
	// Defines labels to be added to any time series or alerts when
	// communicating with external systems such as federation, remote storage,
	// and Alertmanager. By default, no labels are added.
	ExternalLabels map[string]string `json:"externalLabels,omitempty"`
	// Defines the log level setting for Prometheus.
	// The possible values are: `error`, `warn`, `info`, and `debug`.
	// The default value is `info`.
	LogLevel string `json:"logLevel,omitempty"`
	// Defines the nodes on which the pods are scheduled.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Specifies the file to which PromQL queries are logged.
	// This setting can be either a filename, in which
	// case the queries are saved to an `emptyDir` volume
	// at `/var/log/prometheus`, or a full path to a location where
	// an `emptyDir` volume will be mounted and the queries saved.
	// Writing to `/dev/stderr`, `/dev/stdout` or `/dev/null` is supported, but
	// writing to any other `/dev/` path is not supported. Relative paths are
	// also not supported.
	// By default, PromQL queries are not logged.
	QueryLogFile string `json:"queryLogFile,omitempty"`
	// Defines the remote write configuration, including URL, authentication,
	// and relabeling settings.
	RemoteWrite []RemoteWriteSpec `json:"remoteWrite,omitempty"`
	// Defines resource requests and limits for the Prometheus container.
	Resources *v1.ResourceRequirements `json:"resources,omitempty"`
	// Defines the duration for which Prometheus retains data.
	// This definition must be specified using the following regular
	// expression pattern: `[0-9]+(ms|s|m|h|d|w|y)` (ms = milliseconds,
	// s= seconds,m = minutes, h = hours, d = days, w = weeks, y = years).
	// The default value is `15d`.
	Retention string `json:"retention,omitempty"`
	// Defines the maximum amount of disk space used by data blocks plus the
	// write-ahead log (WAL).
	// Supported values are `B`, `KB`, `KiB`, `MB`, `MiB`, `GB`, `GiB`, `TB`,
	// `TiB`, `PB`, `PiB`, `EB`, and `EiB`.
	// By default, no limit is defined.
	RetentionSize string `json:"retentionSize,omitempty"`
	// OmitFromDoc
	TelemetryMatches []string `json:"-"`
	// Defines tolerations for the pods.
	Tolerations []v1.Toleration `json:"tolerations,omitempty"`
	// Defines the pod's topology spread constraints.
	TopologySpreadConstraints []v1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
	// Defines the metrics collection profile that Prometheus uses to collect
	// metrics from the platform components. Supported values are `full` or
	// `minimal`. In the `full` profile (default), Prometheus collects all
	// metrics that are exposed by the platform components. In the `minimal`
	// profile, Prometheus only collects metrics necessary for the default
	// platform alerts, recording rules, telemetry and console dashboards.
	CollectionProfile CollectionProfile `json:"collectionProfile,omitempty"`
	// Defines persistent storage for Prometheus. Use this setting to
	// configure the persistent volume claim, including storage class,
	// volume size and name.
	VolumeClaimTemplate *monv1.EmbeddedPersistentVolumeClaim `json:"volumeClaimTemplate,omitempty"`
}

// PrometheusOperatorConfig resource defines settings for the Prometheus Operator component.
type PrometheusOperatorConfig struct {
	// Defines the log level settings for Prometheus Operator.
	// The possible values are `error`, `warn`, `info`, and `debug`.
	// The default value is `info`.
	LogLevel string `json:"logLevel,omitempty"`
	// Defines the nodes on which the pods are scheduled.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Defines resource requests and limits for the PrometheusOperator container.
	Resources *v1.ResourceRequirements `json:"resources,omitempty"`
	// Defines tolerations for the pods.
	Tolerations []v1.Toleration `json:"tolerations,omitempty"`
	// Defines a pod's topology spread constraints.
	TopologySpreadConstraints []v1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
}

// PrometheusOperatorAdmissionWebhookConfig resource defines settings for the admission webhook workload.
type PrometheusOperatorAdmissionWebhookConfig struct {
	// Defines resource requests and limits for the prometheus-operator-admission-webhook container.
	Resources *v1.ResourceRequirements `json:"resources,omitempty"`
	// Defines a pod's topology spread constraints.
	TopologySpreadConstraints []v1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
}

// OpenShiftStateMetricsConfig resource defines settings for the `openshift-state-metrics` agent.
type OpenShiftStateMetricsConfig struct {
	// Defines the nodes on which the pods are scheduled.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Defines resource requests and limits for the OpenShiftStateMetrics container.
	Resources *v1.ResourceRequirements `json:"resources,omitempty"`
	// Defines tolerations for the pods.
	Tolerations []v1.Toleration `json:"tolerations,omitempty"`
	// Defines a pod's topology spread constraints.
	TopologySpreadConstraints []v1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
}

// TelemeterClientConfig defines settings for the Telemeter Client component.
type TelemeterClientConfig struct {
	// OmitFromDoc
	ClusterID string `json:"clusterID,omitempty"`
	// OmitFromDoc
	Enabled *bool `json:"enabled,omitempty"`
	// Defines the nodes on which the pods are scheduled.
	NodeSelector map[string]string `json:"nodeSelector"`
	// Defines resource requests and limits for the TelemeterClient container.
	Resources *v1.ResourceRequirements `json:"resources,omitempty"`
	// OmitFromDoc
	TelemeterServerURL string `json:"telemeterServerURL,omitempty"`
	// OmitFromDoc
	Token string `json:"token,omitempty"`
	// Defines tolerations for the pods.
	Tolerations []v1.Toleration `json:"tolerations"`
	// Defines a pod's topology spread constraints.
	TopologySpreadConstraints []v1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
}

// ThanosQuerierConfig resource defines settings for the Thanos Querier component.
type ThanosQuerierConfig struct {
	// A Boolean flag that enables or disables request logging.
	// The default value is `false`.
	EnableRequestLogging bool `json:"enableRequestLogging,omitempty"`
	// Defines the log level setting for Thanos Querier.
	// The possible values are `error`, `warn`, `info`, and `debug`.
	// The default value is `info`.
	LogLevel string `json:"logLevel,omitempty"`
	// A Boolean flag that enables setting CORS headers.
	// The headers would allow access from any origin.
	// The default value is `false`.
	EnableCORS bool `json:"enableCORS,omitempty"`
	// Defines the nodes on which the pods are scheduled.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Defines resource requests and limits for the Thanos Querier container.
	Resources *v1.ResourceRequirements `json:"resources,omitempty"`
	// Defines tolerations for the pods.
	Tolerations []v1.Toleration `json:"tolerations,omitempty"`
	// Defines a pod's topology spread constraints.
	TopologySpreadConstraints []v1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
}

// The `NodeExporterConfig` resource defines settings for the `node-exporter` agent.
type NodeExporterConfig struct {
	// Defines which collectors are enabled and their additional configuration parameters.
	Collectors NodeExporterCollectorConfig `json:"collectors,omitempty"`
	// The target number of CPUs on which the Node Exporter's process will run.
	// Use this setting to override the default value, which is set either to `4` or to the number of CPUs on the host, whichever is smaller.
	// The default value is computed at runtime and set via the `GOMAXPROCS` environment variable before Node Exporter is launched.
	// If a kernel deadlock occurs or if performance degrades when reading from `sysfs` concurrently,
	// you can change this value to `1`, which limits Node Exporter to running on one CPU.
	// For nodes with a high CPU count, setting the limit to a low number saves resources by preventing Go routines from being scheduled to run on all CPUs.
	// However, I/O performance degrades if the `maxProcs` value is set too low, and there are many metrics to collect.
	MaxProcs uint32 `json:"maxProcs,omitempty"`
	// A list of network devices, as regular expressions, to be excluded from the relevant collector configuration such as `netdev` and `netclass`.
	// When not set, the Cluster Monitoring Operator uses a predefined list of devices to be excluded to minimize the impact on memory usage.
	// When set as an empty list, no devices are excluded.
	// If you modify this setting, monitor the `prometheus-k8s` deployment closely for excessive memory usage.
	IgnoredNetworkDevices *[]string `json:"ignoredNetworkDevices,omitempty"`
	// Defines resource requests and limits for the NodeExporter container.
	Resources *v1.ResourceRequirements `json:"resources,omitempty"`
}

// NodeExporterCollectorConfig resource defines settings for individual collectors of the `node-exporter` agent.
type NodeExporterCollectorConfig struct {
	// Defines the configuration of the `cpufreq` collector, which collects CPU frequency statistics.
	// Disabled by default.
	CpuFreq NodeExporterCollectorCpufreqConfig `json:"cpufreq,omitempty"`
	// Defines the configuration of the `tcpstat` collector, which collects TCP connection statistics.
	// Disabled by default.
	TcpStat NodeExporterCollectorTcpStatConfig `json:"tcpstat,omitempty"`
	// Defines the configuration of the `netdev` collector, which collects network devices statistics.
	// Enabled by default.
	NetDev NodeExporterCollectorNetDevConfig `json:"netdev,omitempty"`
	// Defines the configuration of the `netclass` collector, which collects information about network devices.
	// Enabled by default.
	NetClass NodeExporterCollectorNetClassConfig `json:"netclass,omitempty"`
	// Defines the configuration of the `buddyinfo` collector, which collects statistics about memory fragmentation from the `node_buddyinfo_blocks` metric. This metric collects data from `/proc/buddyinfo`.
	// Disabled by default.
	BuddyInfo NodeExporterCollectorBuddyInfoConfig `json:"buddyinfo,omitempty"`
	// Defines the configuration of the `mountstats` collector, which collects statistics about NFS volume I/O activities.
	// Disabled by default.
	MountStats NodeExporterCollectorMountStatsConfig `json:"mountstats,omitempty"`
	// Defines the configuration of the `ksmd` collector, which collects statistics from the kernel same-page merger daemon.
	// Disabled by default.
	Ksmd NodeExporterCollectorKSMDConfig `json:"ksmd,omitempty"`
	// Defines the configuration of the `processes` collector, which collects statistics from processes and threads running in the system.
	// Disabled by default.
	Processes NodeExporterCollectorProcessesConfig `json:"processes,omitempty"`
	// Defines the configuration of the `systemd` collector, which collects statistics on the systemd daemon and its managed services.
	// Disabled by default.
	Systemd NodeExporterCollectorSystemdConfig `json:"systemd,omitempty"`
}

// The `NodeExporterCollectorCpufreqConfig` resource works as an on/off switch for
// the `cpufreq` collector of the `node-exporter` agent.
// By default, the `cpufreq` collector is disabled.
// Under certain circumstances, enabling the cpufreq collector increases CPU usage on machines with many cores.
// If you enable this collector and have machines with many cores, monitor your systems closely for excessive CPU usage.
// Please refer to https://github.com/prometheus/node_exporter/issues/1880 for more details.
// A related bug: https://bugzilla.redhat.com/show_bug.cgi?id=1972076
type NodeExporterCollectorCpufreqConfig struct {
	// A Boolean flag that enables or disables the `cpufreq` collector.
	Enabled bool `json:"enabled,omitempty"`
}

// The `NodeExporterCollectorTcpStatConfig` resource works as an on/off switch for
// the `tcpstat` collector of the `node-exporter` agent.
// By default, the `tcpstat` collector is disabled.
type NodeExporterCollectorTcpStatConfig struct {
	// A Boolean flag that enables or disables the `tcpstat` collector.
	Enabled bool `json:"enabled,omitempty"`
}

// The `NodeExporterCollectorNetDevConfig` resource works as an on/off switch for
// the `netdev` collector of the `node-exporter` agent.
// By default, the `netdev` collector is enabled.
// If disabled, these metrics become unavailable:
// `node_network_receive_bytes_total`,
// `node_network_receive_compressed_total`,
// `node_network_receive_drop_total`,
// `node_network_receive_errs_total`,
// `node_network_receive_fifo_total`,
// `node_network_receive_frame_total`,
// `node_network_receive_multicast_total`,
// `node_network_receive_nohandler_total`,
// `node_network_receive_packets_total`,
// `node_network_transmit_bytes_total`,
// `node_network_transmit_carrier_total`,
// `node_network_transmit_colls_total`,
// `node_network_transmit_compressed_total`,
// `node_network_transmit_drop_total`,
// `node_network_transmit_errs_total`,
// `node_network_transmit_fifo_total`,
// `node_network_transmit_packets_total`.
type NodeExporterCollectorNetDevConfig struct {
	// A Boolean flag that enables or disables the `netdev` collector.
	Enabled bool `json:"enabled,omitempty"`
}

// The `NodeExporterCollectorNetClassConfig` resource works as an on/off switch for
// the `netclass` collector of the `node-exporter` agent.
// By default, the `netclass` collector is enabled.
// If disabled, these metrics become unavailable:
// `node_network_info`,
// `node_network_address_assign_type`,
// `node_network_carrier`,
// `node_network_carrier_changes_total`,
// `node_network_carrier_up_changes_total`,
// `node_network_carrier_down_changes_total`,
// `node_network_device_id`,
// `node_network_dormant`,
// `node_network_flags`,
// `node_network_iface_id`,
// `node_network_iface_link`,
// `node_network_iface_link_mode`,
// `node_network_mtu_bytes`,
// `node_network_name_assign_type`,
// `node_network_net_dev_group`,
// `node_network_speed_bytes`,
// `node_network_transmit_queue_length`,
// `node_network_protocol_type`.
type NodeExporterCollectorNetClassConfig struct {
	// A Boolean flag that enables or disables the `netclass` collector.
	Enabled bool `json:"enabled,omitempty"`
	// A Boolean flag that activates the `netlink` implementation of the `netclass` collector.
	// Its default value is `true`: activating the netlink mode.
	// This implementation improves the performance of the `netclass` collector.
	UseNetlink bool `json:"useNetlink,omitempty"`
}

// The `NodeExporterCollectorBuddyInfoConfig` resource works as an on/off switch for
// the `buddyinfo` collector of the `node-exporter` agent.
// By default, the `buddyinfo` collector is disabled.
type NodeExporterCollectorBuddyInfoConfig struct {
	// A Boolean flag that enables or disables the `buddyinfo` collector.
	Enabled bool `json:"enabled,omitempty"`
}

// The `NodeExporterCollectorMountStatsConfig` resource works as an on/off switch for
// the `mountstats` collector of the `node-exporter` agent.
// By default, the `mountstats` collector is disabled.
// If enabled, these metrics become available:
//
//	`node_mountstats_nfs_read_bytes_total`,
//	`node_mountstats_nfs_write_bytes_total`,
//	`node_mountstats_nfs_operations_requests_total`.
//
// Please be aware that these metrics can have a high cardinality.
// If you enable this collector, closely monitor any increases in memory usage for the `prometheus-k8s` pods.
type NodeExporterCollectorMountStatsConfig struct {
	// A Boolean flag that enables or disables the `mountstats` collector.
	Enabled bool `json:"enabled,omitempty"`
}

// The `NodeExporterCollectorKSMDConfig` resource works as an on/off switch for
// the `ksmd` collector of the `node-exporter` agent.
// By default, the `ksmd` collector is disabled.
type NodeExporterCollectorKSMDConfig struct {
	// A Boolean flag that enables or disables the `ksmd` collector.
	Enabled bool `json:"enabled,omitempty"`
}

// The `NodeExporterCollectorProcessesConfig` resource works as an on/off switch for
// the `processes` collector of the `node-exporter` agent.
// If enabled, these metrics become available:
// `node_processes_max_processes`,
// `node_processes_pids`,
// `node_processes_state`,
// `node_processes_threads`,
// `node_processes_threads_state`.
// The metric `node_processes_state` and `node_processes_threads_state` can have up to 5 series each,
// depending on the state of the processes and threads.
// The possible states of a process or a thread are:
// 'D' (UNINTERRUPTABLE_SLEEP),
// 'R' (RUNNING & RUNNABLE),
// 'S' (INTERRRUPTABLE_SLEEP),
// 'T' (STOPPED),
// 'Z' (ZOMBIE).
// By default, the `processes` collector is disabled.
type NodeExporterCollectorProcessesConfig struct {
	// A Boolean flag that enables or disables the `processes` collector.
	Enabled bool `json:"enabled,omitempty"`
}

// The `NodeExporterCollectorSystemdConfig` resource works as an on/off switch for
// the `systemd` collector of the `node-exporter` agent.
// By default, the `systemd` collector is disabled.
// If enabled, the following metrics become available:
// `node_systemd_system_running`,
// `node_systemd_units`,
// `node_systemd_version`.
// If the unit uses a socket, it also generates these 3 metrics:
// `node_systemd_socket_accepted_connections_total`,
// `node_systemd_socket_current_connections`,
// `node_systemd_socket_refused_connections_total`.
// You can use the `units` parameter to select the systemd units to be included by the `systemd` collector.
// The selected units are used to generate the `node_systemd_unit_state` metric, which shows the state of each systemd unit.
// The timer units such as `logrotate.timer` generate one more metric `node_systemd_timer_last_trigger_seconds`.
// However, this metric's cardinality might be high (at least 5 series per unit per node).
// If you enable this collector with a long list of selected units, closely monitor the `prometheus-k8s` deployment for excessive memory usage.
type NodeExporterCollectorSystemdConfig struct {
	// A Boolean flag that enables or disables the `systemd` collector.
	Enabled bool `json:"enabled,omitempty"`
	// A list of regular expression (regex) patterns that match systemd units to be included by the `systemd` collector.
	// By default, the list is empty, so the collector exposes no metrics for systemd units.
	Units []string `json:"units,omitempty"`
}

// Audit profile configurations
type Audit struct {
	// The Profile to set for audit logs. This currently matches the various
	// audit log levels such as: "metadata, request, requestresponse, none".
	// The default audit log level is "metadata"
	//
	// see: https://kubernetes.io/docs/tasks/debug-application-cluster/audit/#audit-policy
	// for more information about auditing and log levels.
	Profile auditv1.Level `json:"profile"`
}

// AdditionalAlertmanagerConfig resource defines settings for how a
// component communicates with additional Alertmanager instances.
type AdditionalAlertmanagerConfig struct {
	// Defines the API version of Alertmanager. Possible values are `v1` or
	// `v2`.
	// The default is `v2`.
	APIVersion string `json:"apiVersion"`
	// Defines the secret key reference containing the bearer token
	// to use when authenticating to Alertmanager.
	BearerToken *v1.SecretKeySelector `json:"bearerToken,omitempty"`
	// Defines the path prefix to add in front of the push endpoint path.
	PathPrefix string `json:"pathPrefix,omitempty"`
	// Defines the URL scheme to use when communicating with Alertmanager
	// instances.
	// Possible values are `http` or `https`. The default value is `http`.
	Scheme string `json:"scheme,omitempty"`
	// A list of statically configured Alertmanager endpoints in the form
	// of `<hosts>:<port>`.
	StaticConfigs []string `json:"staticConfigs,omitempty"`
	// Defines the timeout value used when sending alerts.
	Timeout *string `json:"timeout,omitempty"`
	// Defines the TLS settings to use for Alertmanager connections.
	TLSConfig TLSConfig `json:"tlsConfig,omitempty"`
}

// TLSConfig resource configures the settings for TLS connections.
type TLSConfig struct {
	// Defines the secret key reference containing the Certificate Authority
	// (CA) to use for the remote host.
	CA *v1.SecretKeySelector `json:"ca,omitempty"`
	// Defines the secret key reference containing the public certificate to
	// use for the remote host.
	Cert *v1.SecretKeySelector `json:"cert,omitempty"`
	// Defines the secret key reference containing the private key to use for
	// the remote host.
	Key *v1.SecretKeySelector `json:"key,omitempty"`
	// Used to verify the hostname on the returned certificate.
	ServerName string `json:"serverName,omitempty"`
	// When set to `true`, disables the verification of the remote host's
	// certificate and name.
	InsecureSkipVerify bool `json:"insecureSkipVerify"`
}

// The `MonitoringPluginConfig` resource defines settings for the
// Console Plugin component in the `openshift-monitoring` namespace.
type MonitoringPluginConfig struct {
	// Defines the nodes on which the Pods are scheduled.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Defines resource requests and limits for the console-plugin container.
	Resources *v1.ResourceRequirements `json:"resources,omitempty"`
	// Defines tolerations for the pods.
	Tolerations []v1.Toleration `json:"tolerations,omitempty"`
	// Defines a pod's topology spread constraints.
	TopologySpreadConstraints []v1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
}

// RemoteWriteSpec resource defines the settings for remote write storage.
type RemoteWriteSpec struct {
	// Defines the authorization settings for remote write storage.
	Authorization *monv1.SafeAuthorization `json:"authorization,omitempty"`
	// Defines basic authentication settings for the remote write endpoint URL.
	BasicAuth *monv1.BasicAuth `json:"basicAuth,omitempty"`
	// Defines the file that contains the bearer token for the remote write
	// endpoint.
	// However, because you cannot mount secrets in a pod, in practice
	// you can only reference the token of the service account.
	BearerTokenFile string `json:"bearerTokenFile,omitempty"`
	// Specifies the custom HTTP headers to be sent along with each remote write request.
	// Headers set by Prometheus cannot be overwritten.
	Headers map[string]string `json:"headers,omitempty"`
	// Defines settings for sending series metadata to remote write storage.
	MetadataConfig *monv1.MetadataConfig `json:"metadataConfig,omitempty"`
	// Defines the name of the remote write queue. This name is used in
	// metrics and logging to differentiate queues.
	// If specified, this name must be unique.
	Name string `json:"name,omitempty"`
	// Defines OAuth2 authentication settings for the remote write endpoint.
	OAuth2 *monv1.OAuth2 `json:"oauth2,omitempty"`
	// Defines an optional proxy URL.
	ProxyURL string `json:"proxyUrl,omitempty"`
	// Allows tuning configuration for remote write queue parameters.
	QueueConfig *monv1.QueueConfig `json:"queueConfig,omitempty"`
	// Defines the timeout value for requests to the remote write endpoint.
	RemoteTimeout string `json:"remoteTimeout,omitempty"`
	// Enables sending exemplars via remote write. When enabled, Prometheus is
	// configured to store a maximum of 100,000 exemplars in memory.
	// Note that this setting only applies to user-defined monitoring. It is not applicable
	// to default in-cluster monitoring.
	SendExemplars *bool `json:"sendExemplars,omitempty"`
	// Defines AWS Signature Version 4 authentication settings.
	Sigv4 *monv1.Sigv4 `json:"sigv4,omitempty"`
	// Defines TLS authentication settings for the remote write endpoint.
	TLSConfig *monv1.SafeTLSConfig `json:"tlsConfig,omitempty"`
	// Defines the URL of the remote write endpoint to which samples will be sent.
	URL string `json:"url"`
	// Defines the list of remote write relabel configurations.
	WriteRelabelConfigs []monv1.RelabelConfig `json:"writeRelabelConfigs,omitempty"`
}

type CollectionProfile string
type CollectionProfiles []CollectionProfile
