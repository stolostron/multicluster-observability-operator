// Copyright (c) 2020 Red Hat, Inc.

package multiclustermonitoring

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	dashboardMetricsConfigMapKey  = "metrics.yaml"
	dashboardMetricsConfigMapName = "grafana-dashboards-metrics"
	dashboardMetricsConfigMapNS   = "open-cluster-management-monitoring"
)

type DashboardMetricConfig struct {
	// default metrics are used for the dashboard to show cluster status
	DefalutMetrics []string
	// additional metrics are used for user customization
	AdditionalMetrics []string
}

func getDashboardMetrics(client client.Client) []string {
	internalDefaultMetrics := []string{
		"cluster:capacity_cpu_cores:sum",
		"cluster:capacity_memory_bytes:sum",
		"cluster:container_cpu_usage:ratio",
		"cluster:container_spec_cpu_shares:ratio",
		"cluster:cpu_usage_cores:sum",
		"cluster:memory_usage:ratio",
		"cluster:memory_usage_bytes:sum",
		"cluster:usage:resources:sum",
		"cluster_infrastructure_provider",
		"cluster_version",
		"cluster_version_payload",
		"container_cpu_cfs_throttled_periods_total",
		"container_memory_cache",
		"container_memory_rss",
		"container_memory_swap",
		"container_memory_working_set_bytes",
		"container_network_receive_bytes_total",
		"container_network_receive_packets_dropped_total",
		"container_network_receive_packets_total",
		"container_network_transmit_bytes_total",
		"container_network_transmit_packets_dropped_total",
		"container_network_transmit_packets_total",
		"haproxy_backend_connections_total",
		"instance:node_cpu_utilisation:rate1m",
		"instance:node_load1_per_cpu:ratio",
		"instance:node_memory_utilisation:ratio",
		"instance:node_network_receive_bytes_excluding_lo:rate1m",
		"instance:node_network_receive_drop_excluding_lo:rate1m",
		"instance:node_network_transmit_bytes_excluding_lo:rate1m",
		"instance:node_network_transmit_drop_excluding_lo:rate1m",
		"instance:node_num_cpu:sum",
		"instance:node_vmstat_pgmajfault:rate1m",
		"instance_device:node_disk_io_time_seconds:rate1m",
		"instance_device:node_disk_io_time_weighted_seconds:rate1m",
		"kube_node_status_allocatable_cpu_cores",
		"kube_node_status_allocatable_memory_bytes",
		"kube_pod_container_resource_limits_cpu_cores",
		"kube_pod_container_resource_limits_memory_bytes",
		"kube_pod_container_resource_requests_cpu_cores",
		"kube_pod_container_resource_requests_memory_bytes",
		"kube_pod_info",
		"kube_resourcequota",
		"machine_cpu_cores",
		"machine_memory_bytes",
		"mixin_pod_workload",
		"node_cpu_seconds_total",
		"node_filesystem_avail_bytes",
		"node_filesystem_size_bytes",
		"node_memory_MemAvailable_bytes",
		"node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate",
		"node_namespace_pod_container:container_memory_cache",
		"node_namespace_pod_container:container_memory_rss",
		"node_namespace_pod_container:container_memory_swap",
		"node_namespace_pod_container:container_memory_working_set_bytes",
		"node_netstat_Tcp_OutSegs",
		"node_netstat_Tcp_RetransSegs",
		"node_netstat_TcpExt_TCPSynRetrans",
		"up",
	}

	found := &corev1.ConfigMap{}
	namespacedName := types.NamespacedName{
		Name:      dashboardMetricsConfigMapName,
		Namespace: dashboardMetricsConfigMapNS,
	}

	err := client.Get(context.TODO(), namespacedName, found)
	if err != nil {
		log.Error(err, "failed to get metrics config, use default value")
		return internalDefaultMetrics
	}

	var metricConf DashboardMetricConfig
	err = yaml.Unmarshal([]byte(found.Data[dashboardMetricsConfigMapKey]), &metricConf)
	if err != nil {
		log.Error(err, "metrics config is invalid, use default value")
		return internalDefaultMetrics
	}

	internalDefaultMetrics = append(internalDefaultMetrics, metricConf.DefalutMetrics...)
	internalDefaultMetrics = append(internalDefaultMetrics, metricConf.AdditionalMetrics...)

	return internalDefaultMetrics
}
