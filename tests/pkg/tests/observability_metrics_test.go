// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stolostron/multicluster-observability-operator/tests/pkg/kustomize"
	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	allowlistCMname = "observability-metrics-custom-allowlist"
)

var (
	clusters     []utils.ClustersInfo
	clusterError error
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

	It("RHACM4K-1658: Observability: Customized metrics data are collected [P2][Sev2][Observability][Integration]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release @pre-upgrade (metrics/g0)", func() {
		By("Adding custom metrics allowlist configmap")
		yamlB, err := kustomize.Render(kustomize.Options{KustomizationPath: "../../../examples/metrics/allowlist"})
		Expect(err).ToNot(HaveOccurred())
		Expect(
			utils.ApplyRetryOnConflict(
				testOptions.HubCluster.ClusterServerURL,
				testOptions.KubeConfig,
				testOptions.HubCluster.KubeContext,
				yamlB,
			)).NotTo(HaveOccurred())

		By("Waiting for new added metrics on grafana console")
		Eventually(func() error {
			for _, cluster := range clusters {
				query := fmt.Sprintf("node_memory_Active_bytes{cluster=\"%s\"} offset 1m", cluster.Name)
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
					cluster.Name,
					cluster.Name,
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
					cluster.Name,
					cluster.Name,
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
					cluster.Name,
					cluster.Name,
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
		metricList := []string{
			// Check a random sample of the metrics we expect to be always present in e2e envs
			"ALERTS",
			"container_spec_cpu_quota",
			"kube_node_status_allocatable",
			"up",
			// Check some of our own rules
			"apiserver_request_duration_seconds:histogram_quantile_99",
			"cluster:kube_pod_container_resource_requests:memory:sum",
		}
		// Ensure that expected metrics are being collected
		Eventually(func() error {
			for _, cluster := range clusters {
				for _, name := range metricList {
					query := fmt.Sprintf("%s{cluster=\"%s\"}", name, cluster.Name)
					res, err := utils.QueryGrafana(testOptions, query)
					if err != nil {
						return fmt.Errorf("failed to get metrics %s in cluster %s: %v", name, cluster.Name, err)
					}

					if len(res.Data.Result) == 0 {
						return fmt.Errorf("no data found for %s in cluster %s", name, cluster.Name)
					}
				}
			}
			return nil
		}, EventuallyTimeoutMinute*3, EventuallyIntervalSecond*5).Should(Succeed())

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
