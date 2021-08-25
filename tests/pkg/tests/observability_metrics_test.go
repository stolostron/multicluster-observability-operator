// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tests

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/open-cluster-management/multicluster-observability-operator/tests/pkg/kustomize"
	"github.com/open-cluster-management/multicluster-observability-operator/tests/pkg/utils"
)

const (
	allowlistCMname = "observability-metrics-custom-allowlist"
)

var (
	clusters         []string
	clusterError     error
	metricslistError error
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
		clusters, clusterError = utils.ListManagedClusters(testOptions)
		Expect(clusterError).NotTo(HaveOccurred())
	})

	It("[P2][Sev2][Observability][Integration] Should have metrics which defined in custom metrics allowlist (metrics/g0)", func() {
		By("Adding custom metrics allowlist configmap")
		yamlB, err := kustomize.Render(kustomize.Options{KustomizationPath: "../../../examples/metrics/allowlist"})
		Expect(err).ToNot(HaveOccurred())
		Expect(utils.Apply(testOptions.HubCluster.ClusterServerURL, testOptions.KubeConfig, testOptions.HubCluster.KubeContext, yamlB)).NotTo(HaveOccurred())

		By("Waiting for new added metrics on grafana console")
		Eventually(func() error {
			for _, cluster := range clusters {
				query := fmt.Sprintf("node_memory_Active_bytes{cluster=\"%s\"} offset 1m", cluster)
				err, _ := utils.ContainManagedClusterMetric(testOptions, query, []string{`"__name__":"node_memory_Active_bytes"`})
				if err != nil {
					return err
				}
			}
			return nil
		}, EventuallyTimeoutMinute*10, EventuallyIntervalSecond*5).Should(Succeed())
	})

	It("[P2][Sev2][Observability][Integration] Should have no metrics which have been marked for deletion in names section (metrics/g0)", func() {
		By("Waiting for deleted metrics disappear on grafana console")
		Eventually(func() error {
			for _, cluster := range clusters {
				query := fmt.Sprintf("timestamp(instance:node_num_cpu:sum{cluster=\"%s\"}) - timestamp(instance:node_num_cpu:sum{cluster=\"%s\"} offset 1m) > 59",
					cluster, cluster)
				metricslistError, _ = utils.ContainManagedClusterMetric(testOptions, query, []string{})
				if metricslistError == nil {
					return nil
				}
			}
			return metricslistError
		}, EventuallyTimeoutMinute*10, EventuallyIntervalSecond*5).Should(MatchError("Failed to find metric name from response"))
	})

	It("[P2][Sev2][Observability][Integration] Should have no metrics which have been marked for deletion in matches section (metrics/g0)", func() {
		By("Waiting for deleted metrics disappear on grafana console")
		Eventually(func() error {
			for _, cluster := range clusters {
				query := fmt.Sprintf("timestamp(go_goroutines{cluster=\"%s\"}) - timestamp(go_goroutines{cluster=\"%s\"} offset 1m) > 59",
					cluster, cluster)
				metricslistError, _ = utils.ContainManagedClusterMetric(testOptions, query, []string{})
				if metricslistError == nil {
					return nil
				}
			}
			return metricslistError
		}, EventuallyTimeoutMinute*10, EventuallyIntervalSecond*5).Should(MatchError("Failed to find metric name from response"))
	})

	It("[P2][Sev2][Observability][Integration] Should have no metrics after custom metrics allowlist deleted (metrics/g0)", func() {
		By("Deleting custom metrics allowlist configmap")
		Eventually(func() error {
			err := hubClient.CoreV1().ConfigMaps(MCO_NAMESPACE).Delete(context.TODO(), allowlistCMname, metav1.DeleteOptions{})
			return err
		}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*1).Should(Succeed())

		By("Waiting for new added metrics disappear on grafana console")
		Eventually(func() error {
			for _, cluster := range clusters {
				query := fmt.Sprintf("timestamp(node_memory_Active_bytes{cluster=\"%s\"}) - timestamp(node_memory_Active_bytes{cluster=\"%s\"} offset 1m) > 59",
					cluster, cluster)
				metricslistError, _ = utils.ContainManagedClusterMetric(testOptions, query, []string{})
				if metricslistError == nil {
					return nil
				}
			}
			return metricslistError
		}, EventuallyTimeoutMinute*10, EventuallyIntervalSecond*5).Should(MatchError("Failed to find metric name from response"))
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
