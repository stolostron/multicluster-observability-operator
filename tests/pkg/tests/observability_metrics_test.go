// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tests

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stolostron/multicluster-observability-operator/tests/pkg/kustomize"
	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
)

const (
	allowlistCMname = "observability-metrics-custom-allowlist"
)

var (
	clusters         []string
	clusterError     error
	metricslistError error
)

var _ = Describe("", func() {
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

	It("RHACM4K-1449 - Observability - Verify metrics data consistency [P2][Sev2][Observability][Integration]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release @pre-upgrade (metrics/g1)", func() {
		metricList := utils.GetDefaultMetricList(testOptions)
		ignoreMetricMap := utils.GetIgnoreMetricMap()
		_, etcdPodList := utils.GetPodList(
			testOptions,
			true,
			"openshift-etcd",
			"app=etcd",
		)
		// ignore etcd network peer metrics for SNO cluster
		if etcdPodList != nil && len(etcdPodList.Items) <= 0 {
			ignoreMetricMap["etcd_network_peer_received_bytes_total"] = true
			ignoreMetricMap["etcd_network_peer_sent_bytes_total"] = true
		}
		for _, name := range metricList {
			_, ok := ignoreMetricMap[name]
			if !ok {
				Eventually(func() error {
					err, _ := utils.ContainManagedClusterMetric(testOptions, name, []string{name})
					return err
				}, EventuallyTimeoutMinute*2, EventuallyIntervalSecond*3).Should(Succeed())
			}
		}
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
				err, _ := utils.ContainManagedClusterMetric(
					testOptions,
					query,
					[]string{`"__name__":"node_memory_Active_bytes"`},
				)
				if err != nil {
					return err
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
				metricslistError, _ = utils.ContainManagedClusterMetric(testOptions, query, []string{})
				if metricslistError == nil {
					return nil
				}
			}
			return metricslistError
		}, EventuallyTimeoutMinute*10, EventuallyIntervalSecond*5).Should(MatchError("Failed to find metric name from response"))
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
				metricslistError, _ = utils.ContainManagedClusterMetric(testOptions, query, []string{})
				if metricslistError == nil {
					return nil
				}
			}
			return metricslistError
		}, EventuallyTimeoutMinute*10, EventuallyIntervalSecond*5).Should(MatchError("Failed to find metric name from response"))
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
				metricslistError, _ = utils.ContainManagedClusterMetric(testOptions, query, []string{})
				if metricslistError == nil {
					return nil
				}
			}
			return metricslistError
		}, EventuallyTimeoutMinute*10, EventuallyIntervalSecond*5).Should(MatchError("Failed to find metric name from response"))
	})

	It("RHACM4K-3339: Observability: Verify recording rule - Should have metrics which used grafana dashboard [P2][Sev2][Observability][Integration]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release @pre-upgrade (ssli/g1)", func() {
		metricList := utils.GetDefaultMetricList(testOptions)
		ignoreMetricMap := utils.GetIgnoreMetricMap()
		_, etcdPodList := utils.GetPodList(
			testOptions,
			true,
			"openshift-etcd",
			"app=etcd",
		)
		// ignore etcd network peer metrics for SNO cluster
		if etcdPodList != nil && len(etcdPodList.Items) <= 0 {
			ignoreMetricMap["etcd_network_peer_received_bytes_total"] = true
			ignoreMetricMap["etcd_network_peer_sent_bytes_total"] = true
		}
		for _, name := range metricList {
			_, ok := ignoreMetricMap[name]
			if !ok {
				Eventually(func() error {
					err, _ := utils.ContainManagedClusterMetric(testOptions, name, []string{name})
					return err
				}, EventuallyTimeoutMinute*2, EventuallyIntervalSecond*3).Should(Succeed())
			}
		}
	})

	JustAfterEach(func() {
		Expect(utils.IntegrityChecking(testOptions)).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if CurrentGinkgoTestDescription().Failed {
			utils.PrintMCOObject(testOptions)
			utils.PrintAllMCOPodsStatus(testOptions)
			utils.PrintAllOBAPodsStatus(testOptions)
		}
		testFailed = testFailed || CurrentGinkgoTestDescription().Failed
	})
})
