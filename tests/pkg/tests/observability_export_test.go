// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tests

import (
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-observability-operator/tests/pkg/kustomize"
	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
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

	It("RHACM4K-11170: Observability: Verify metrics would be exported to corp tools(2.5)(draft)[P2][Sev2][observability][Integration] Should have acm_remote_write_requests_total metrics with correct labels/value  @e2e (export/g0)", func() {
		By("Adding victoriametrics deployment/service/secret")
		yamlB, err := kustomize.Render(kustomize.Options{KustomizationPath: "../../../examples/export"})
		Expect(err).ToNot(HaveOccurred())
		Expect(
			utils.Apply(
				testOptions.HubCluster.ClusterServerURL,
				testOptions.KubeConfig,
				testOptions.HubCluster.KubeContext,
				yamlB,
			)).NotTo(HaveOccurred())

		By("Updating mco cr to inject WriteStorage")
		templatePath := "../../../examples/export/v1beta2"
		if os.Getenv("IS_CANARY_ENV") != "true" {
			templatePath = "../../../examples/export/v1beta2/custom-certs"
		}
		yamlB, err = kustomize.Render(kustomize.Options{KustomizationPath: templatePath})
		Expect(err).ToNot(HaveOccurred())
		Expect(
			utils.Apply(
				testOptions.HubCluster.ClusterServerURL,
				testOptions.KubeConfig,
				testOptions.HubCluster.KubeContext,
				yamlB,
			)).NotTo(HaveOccurred())

		By("Waiting for metrics acm_remote_write_requests_total on grafana console")
		Eventually(func() error {
			//for _, cluster := range clusters {
			// wait for pod restarting
			time.Sleep(60 * time.Second)
			query := fmt.Sprintf("acm_remote_write_requests_total{cluster=\"%s\"} offset 1m", "local-cluster")
			err, _ := utils.ContainManagedClusterMetric(
				testOptions,
				query,
				[]string{`"__name__":"acm_remote_write_requests_total"`},
			)
			if err != nil {
				return err
			}
			err, _ = utils.ContainManagedClusterMetric(
				testOptions,
				query,
				[]string{`"__name__":"acm_remote_write_requests_total"`,
					`"code":"200`, `"name":"thanos-receiver"`},
			)
			if err != nil {
				return fmt.Errorf("metrics not forwarded to thanos-receiver")
			}
			err, _ = utils.ContainManagedClusterMetric(
				testOptions,
				query,
				[]string{`"__name__":"acm_remote_write_requests_total"`,
					`"code":"204`, `"name":"victoriametrics"`},
			)
			if err != nil {
				return fmt.Errorf("metrics not forwarded to victoriametrics")
			}
			//}
			return nil
		}, EventuallyTimeoutMinute*20, EventuallyIntervalSecond*5).Should(Succeed())
	})

	JustAfterEach(func() {

		Expect(utils.CleanExportResources(testOptions)).NotTo(HaveOccurred())
		time.Sleep(120 * time.Second)
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
