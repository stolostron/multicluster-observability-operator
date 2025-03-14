// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	"github.com/stolostron/multicluster-observability-operator/tests/pkg/kustomize"
	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
)

const (
	allowlistCMname = "observability-metrics-custom-allowlist"
)

var (
	clusters     []string
	clusterError error
)

var _ = Describe("Observability:", func() {
	BeforeEach(func() {
		hubClient = utils.NewKubeClient(
			testOptions.HubCluster.ClusterServerURL,
			testOptions.KubeConfig,
			testOptions.HubCluster.KubeContext)

		dynClient = utils.NewKubeClientDynamic(
			testOptions.HubCluster.ClusterServerURL,
			testOptions.KubeConfig,
			testOptions.HubCluster.KubeContext)
	})

	JustBeforeEach(func() {
		Eventually(func() error {
			clusters, clusterError = utils.ListManagedClusters(testOptions)
			if clusterError != nil {
				return clusterError
			}
			return nil
		}, EventuallyTimeoutMinute*6, EventuallyIntervalSecond*5).Should(Succeed())
	})

	It("RHACM4K-1658: Observability: Customized metrics data are collected [P2][Sev2][Observability][Integration]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release @pre-upgrade (metrics/g0)", func() {
		By("Adding custom metrics allowlist configmap")
		yamlB, err := kustomize.Render(kustomize.Options{KustomizationPath: "../../../examples/metrics/allowlist"})
		Expect(err).ToNot(HaveOccurred())
		Expect(
			utils.Apply(
				testOptions.HubCluster.ClusterServerURL,
				testOptions.KubeConfig,
				testOptions.HubCluster.KubeContext,
				yamlB,
			)).NotTo(HaveOccurred())

		By("Waiting for new added metrics on grafana console")
		Eventually(func() error {
			for _, cluster := range clusters {
				query := fmt.Sprintf("node_memory_Active_bytes{cluster=\"%s\"} offset 1m", cluster)
				res, err := utils.QueryGrafana(
					testOptions,
					query,
				)
				if err != nil {
					return err
				}

				if len(res.Data.Result) == 0 {
					return fmt.Errorf("no data found for %s", query)
				}
			}
			return nil
		}, EventuallyTimeoutMinute*10, EventuallyIntervalSecond*5).Should(Succeed())
	})

	It("RHACM4K-3063: Observability: Metrics removal from default allowlist [P2][Sev2][Observability][Integration]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release @pre-upgrade (metrics/g0)", func() {
		By("Waiting for deleted metrics disappear on grafana console")
		Eventually(func() error {
			for _, cluster := range clusters {
				query := fmt.Sprintf(
					"timestamp(instance:node_num_cpu:sum{cluster=\"%s\"}) - timestamp(instance:node_num_cpu:sum{cluster=\"%s\"} offset 1m) > 59",
					cluster,
					cluster,
				)
				res, err := utils.QueryGrafana(testOptions, query)
				if err != nil {
					return err
				}
				// there should be no data for the deleted metric
				if len(res.Data.Result) != 0 {
					return fmt.Errorf("metric %s found in response: %v", query, res)
				}
			}
			return nil
		}, EventuallyTimeoutMinute*10, EventuallyIntervalSecond*5).Should(Succeed())
	})

	It("RHACM4K-3063: Observability: Metrics removal from default allowlist [P2][Sev2][Observability][Integration]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release @pre-upgrade (metrics/g0)", func() {
		By("Waiting for deleted metrics disappear on grafana console")
		Eventually(func() error {
			for _, cluster := range clusters {
				query := fmt.Sprintf(
					"timestamp(go_goroutines{cluster=\"%s\"}) - timestamp(go_goroutines{cluster=\"%s\"} offset 1m) > 59",
					cluster,
					cluster,
				)
				res, err := utils.QueryGrafana(testOptions, query)
				if err != nil {
					return err
				}
				if len(res.Data.Result) != 0 {
					return fmt.Errorf("metric %s found in response: %v", query, res)
				}
			}
			return nil
		}, EventuallyTimeoutMinute*10, EventuallyIntervalSecond*5).Should(Succeed())
	})

	It("RHACM4K-3063: Observability: Metrics removal from default allowlist [P2][Sev2][Observability][Integration]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release @pre-upgrade (metrics/g0)", func() {
		By("Deleting custom metrics allowlist configmap")
		Eventually(func() error {
			err := hubClient.CoreV1().
				ConfigMaps(MCO_NAMESPACE).
				Delete(context.TODO(), allowlistCMname, metav1.DeleteOptions{})
			return err
		}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*1).Should(Succeed())

		By("Waiting for new added metrics disappear on grafana console")
		Eventually(func() error {
			for _, cluster := range clusters {
				query := fmt.Sprintf(
					"timestamp(node_memory_Active_bytes{cluster=\"%s\"}) - timestamp(node_memory_Active_bytes{cluster=\"%s\"} offset 1m) > 59",
					cluster,
					cluster,
				)
				res, err := utils.QueryGrafana(testOptions, query)
				if err != nil {
					return err
				}
				if len(res.Data.Result) != 0 {
					return fmt.Errorf("metric %s found in response: %v", query, res)
				}
			}
			return nil
		}, EventuallyTimeoutMinute*10, EventuallyIntervalSecond*5).Should(Succeed())
	})

	// TODO: Needs RHACM4K number
	// Ensures that the allowList is current by checking that the metrics are being collected
	It("[P2][Sev2][observability][Integration] Should collect expected metrics from spokes (metrics/g0)", func() {
		// Get the metrics from the deployed allowList configMap
		metricList, dynamicMetricList := utils.GetDefaultMetricList(testOptions)
		allowMetricsMap := make(map[string]struct{}, len(metricList)+len(dynamicMetricList))
		for _, name := range metricList {
			allowMetricsMap[name] = struct{}{}
		}
		for _, name := range dynamicMetricList {
			allowMetricsMap[name] = struct{}{}
		}

		// Log ignored metrics that are not found in the allowlist to verify that both lists are in sync
		for name := range ignoredMetrics {
			if _, ok := allowMetricsMap[name]; !ok {
				klog.V(1).Infof("ignored metric %s is not found in the allowlist", name)
			}
		}

		// Ensure that expected metrics are being collected
		Eventually(func() error {
			for _, cluster := range clusters {
				for _, name := range metricList {
					if _, ok := ignoredMetrics[name]; ok {
						continue
					}

					query := fmt.Sprintf("%s{cluster=\"%s\"}", name, cluster)
					res, err := utils.QueryGrafana(testOptions, query)
					if err != nil {
						return fmt.Errorf("failed to get metrics %s in cluster %s: %v", name, cluster, err)
					}

					if len(res.Data.Result) == 0 {
						return fmt.Errorf("no data found for %s in cluster %s", name, cluster)
					}

					return nil
				}
			}
			return nil
		}, EventuallyTimeoutMinute*3, EventuallyIntervalSecond*5).Should(Succeed())

	})

	It("RHACM4K-3339: Observability: Verify recording rule - Should have metrics which used grafana dashboard [P2][Sev2][Observability][Integration]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release @pre-upgrade (ssli/g1)", func() {
		metricList, _ := utils.GetDefaultMetricList(testOptions)

		for _, name := range metricList {

			if _, ok := ignoredMetrics[name]; ok {
				continue
			}

			Eventually(func() error {
				query := fmt.Sprintf("%s", name)
				res, err := utils.QueryGrafana(testOptions, query)

				if err != nil {
					return err
				}
				if len(res.Data.Result) == 0 {
					return fmt.Errorf("no data found for %s", query)
				}
				return nil
			}, EventuallyTimeoutMinute*2, EventuallyIntervalSecond*3).Should(Succeed())
		}
	})

	JustAfterEach(func() {
		Expect(utils.IntegrityChecking(testOptions)).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			utils.LogFailingTestStandardDebugInfo(testOptions)
		}
		testFailed = testFailed || CurrentSpecReport().Failed()
	})
})

// List of metrics that are not collected in the e2e environment
// It might be because they are deprecated or not relevant for our test environment
// These metrics are ignored in the test
var ignoredMetrics = map[string]struct{}{
	"cluster:policy_governance_info:propagated_count":                          {},
	"cluster:policy_governance_info:propagated_noncompliant_count":             {},
	"cluster_policy_governance_info":                                           {},
	"cnv:vmi_status_running:count":                                             {},
	"container_cpu_cfs_periods_total":                                          {},
	"container_cpu_cfs_throttled_periods_total":                                {},
	"container_memory_cache":                                                   {},
	"container_memory_rss":                                                     {},
	"container_memory_swap":                                                    {},
	"container_memory_working_set_bytes":                                       {},
	"coredns_forward_responses_total":                                          {},
	"csv_abnormal":                                                             {},
	"etcd_mvcc_db_total_size_in_bytes":                                         {},
	"etcd_network_peer_received_bytes_total":                                   {},
	"etcd_network_peer_sent_bytes_total":                                       {},
	"etcd_object_counts":                                                       {},
	"instance:node_filesystem_usage:sum":                                       {},
	"kube_node_status_allocatable_cpu_cores":                                   {},
	"kube_node_status_allocatable_memory_bytes":                                {},
	"kube_node_status_capacity_cpu_cores":                                      {},
	"kube_node_status_capacity_pods":                                           {},
	"kube_pod_container_resource_limits":                                       {},
	"kube_pod_container_resource_limits_cpu_cores":                             {},
	"kube_pod_container_resource_limits_memory_bytes":                          {},
	"kube_pod_container_resource_requests":                                     {},
	"kube_pod_container_resource_requests_cpu_cores":                           {},
	"kube_pod_container_resource_requests_memory_bytes":                        {},
	"kubelet_running_container_count":                                          {},
	"kubelet_runtime_operations":                                               {},
	"kubevirt_hyperconverged_operator_health_status":                           {},
	"kubevirt_hco_system_health_status":                                        {},
	"kubevirt_vmi_info":                                                        {},
	"kubevirt_vm_running_status_last_transition_timestamp_seconds":             {},
	"kubevirt_vm_non_running_status_last_transition_timestamp_seconds":         {},
	"kubevirt_vm_error_status_last_transition_timestamp_seconds":               {},
	"kubevirt_vm_starting_status_last_transition_timestamp_seconds":            {},
	"kubevirt_vm_migrating_status_last_transition_timestamp_seconds":           {},
	"kubevirt_vmi_memory_available_bytes":                                      {},
	"kubevirt_vmi_memory_unused_bytes":                                         {},
	"kubevirt_vmi_memory_cached_bytes":                                         {},
	"kubevirt_vmi_memory_used_bytes":                                           {},
	"kubevirt_vmi_phase_count":                                                 {},
	"kubevirt_vmi_cpu_usage_seconds_total":                                     {},
	"kubevirt_vmi_network_receive_bytes_total":                                 {},
	"kubevirt_vmi_network_transmit_bytes_total":                                {},
	"kubevirt_vmi_network_receive_packets_total":                               {},
	"kubevirt_vmi_network_transmit_packets_total":                              {},
	"kubevirt_vmi_storage_iops_read_total":                                     {},
	"kubevirt_vmi_storage_iops_write_total":                                    {},
	"kubevirt_vm_resource_requests":                                            {},
	"mce_hs_addon_hosted_control_planes_status_gauge":                          {},
	"mce_hs_addon_request_based_hcp_capacity_current_gauge":                    {},
	"mixin_pod_workload":                                                       {},
	"namespace:kube_pod_container_resource_requests_cpu_cores:sum":             {},
	"namespace_workload_pod:kube_pod_owner:relabel":                            {},
	"node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate": {},
	"node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate":  {},
	"policy:policy_governance_info:propagated_count":                           {},
	"policy:policy_governance_info:propagated_noncompliant_count":              {},
	"policyreport_info":                                                        {},
}
