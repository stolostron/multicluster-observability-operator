// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"fmt"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-observability-operator/tests/pkg/kustomize"
	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
)

var _ = Describe("", func() {
	BeforeEach(func() {

		cloudProvider := strings.ToLower(os.Getenv("CLOUD_PROVIDER"))
		if strings.Contains(cloudProvider, "ibmz") {
			Skip("skip on IMB-z as victoria-metrics image not available")
		}

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
		if os.Getenv("IS_CANARY_ENV") != trueStr {
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

		// Get name of the hub cluster
		hubClusterName := "local-cluster"
		for _, cluster := range testOptions.ManagedClusters {
			if cluster.BaseDomain == testOptions.HubCluster.BaseDomain {
				hubClusterName = cluster.Name
			}
		}

		By("Waiting for metrics acm_remote_write_requests_total on grafana console")
		Eventually(func() error {
			query := fmt.Sprintf("acm_remote_write_requests_total{cluster=\"%s\"} offset 1m", hubClusterName)
			res, err := utils.QueryGrafana(
				testOptions,
				query,
			)
			if err != nil {
				return err
			}
			if len(res.Data.Result) == 0 {
				return fmt.Errorf("metric %s not found in response", query)
			}

			// Check if the metric is forwarded to thanos-receiver
			labelSet := map[string]string{"code": "200", "name": "thanos-receiver"}
			if !res.ContainsLabelsSet(labelSet) {
				return fmt.Errorf("labels %v not found in response: %v", labelSet, res)
			}

			// Check if the metric is forwarded to victoriametrics
			labelSet = map[string]string{"code": "204", "name": "victoriametrics"}
			if !res.ContainsLabelsSet(labelSet) {
				return fmt.Errorf("labels %v not found in response: %v", labelSet, res)
			}

			return nil
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
	})

	JustAfterEach(func() {
		if CurrentSpecReport().Failed() {
			utils.LogFailingTestStandardDebugInfo(testOptions)
		}
		testFailed = testFailed || CurrentGinkgoTestDescription().Failed
	})

	AfterEach(func() {

		cloudProvider := strings.ToLower(os.Getenv("CLOUD_PROVIDER"))
		if !strings.Contains(cloudProvider, "ibmz") {
			Expect(utils.CleanExportResources(testOptions)).NotTo(HaveOccurred())
			Expect(utils.IntegrityChecking(testOptions)).NotTo(HaveOccurred())
		}
	})
})
