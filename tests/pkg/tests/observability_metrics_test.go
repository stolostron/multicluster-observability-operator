// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
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

	It("[P2][Sev2][observability][Integration] Should have metrics which defined in custom metrics allowlist (metrics/g0)", func() {
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

	It("[P2][Sev2][observability][Integration] Should have no metrics which have been marked for deletion in names section (metrics/g0)", func() {
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

	It("[P2][Sev2][observability][Integration] Should have no metrics which have been marked for deletion in matches section (metrics/g0)", func() {
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

	It("[P2][Sev2][observability][Integration] Should have no metrics after custom metrics allowlist deleted (metrics/g0)", func() {
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

		// Ensure that ignored metrics are not being collected
		// This is to ensure that the ignoredMetrics list is in sync with the actual metrics being collected
		Eventually(func() error {
			for _, cluster := range clusters {
				for name := range ignoredMetrics {
					query := fmt.Sprintf("%s{cluster=\"%s\"}", name, cluster)
					res, err := utils.QueryGrafana(testOptions, query)
					if err != nil {
						return fmt.Errorf("failed to get metrics %s in cluster %s: %v", name, cluster, err)
					}

					if len(res.Data.Result) != 0 {
						return fmt.Errorf("found data for %s in cluster %s", name, cluster)
					}
				}
			}

			return nil
		}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*5).Should(Succeed())

		// // wait for metrics to be available
		// klog.V(1).Infof("waiting for metrics to be available...")
		// time.Sleep(90 * time.Second)

		// // Check if the metrics are available
		// for _, name := range metricList {
		// 	res, err := utils.QueryGrafana(testOptions, name)
		// 	if err != nil {
		// 		klog.Errorf("failed to get metrics %s: %v", name, err)
		// 		continue
		// 	}
		// 	if len(res.Data.Result) == 0 {
		// 		klog.Errorf("no data found for %s", name)
		// 	}
		// }
	})

	JustAfterEach(func() {
		Expect(utils.IntegrityChecking(testOptions)).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if CurrentGinkgoTestDescription().Failed {
			utils.LogFailingTestStandardDebugInfo(testOptions)
		}
		testFailed = testFailed || CurrentGinkgoTestDescription().Failed
	})
})

// List of metrics that are not collected in the e2e environment
// It might be because they are deprecated or not relevant for our test environment
// These metrics are ignored in the test
var ignoredMetrics = map[string]struct{}{
	"cluster:policy_governance_info:propagated_count":                          {},
	"cluster:policy_governance_info:propagated_noncompliant_count":             {},
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
