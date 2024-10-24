// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-observability-operator/tests/pkg/kustomize"
	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
)

const (
	dashboardName                 = "sample-dashboard"
	dashboardTitle                = "Sample Dashboard for E2E"
	updateDashboardTitle          = "Update Sample Dashboard for E2E"
	clusterOverviewTitle          = "ACM - Clusters Overview"
	clusterOverviewOptimizedTitle = "ACM - Clusters Overview (Optimized)"
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

	It("[P2][Sev2][observability][Stable] Should have custom dashboard which defined in configmap (dashboard/g0)", func() {
		By("Creating custom dashboard configmap")
		yamlB, _ := kustomize.Render(
			kustomize.Options{KustomizationPath: "../../../examples/dashboards/sample_custom_dashboard"},
		)
		Expect(
			utils.Apply(
				testOptions.HubCluster.ClusterServerURL,
				testOptions.KubeConfig,
				testOptions.HubCluster.KubeContext,
				yamlB)).NotTo(HaveOccurred())
		Eventually(func() bool {
			_, result := utils.ContainDashboard(testOptions, dashboardTitle)
			return result
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(BeTrue())
	})

	It("[P2][Sev2][observability][Stable] Should have update custom dashboard after configmap updated (dashboard/g0)", func() {
		By("Updating custom dashboard configmap")
		yamlB, _ := kustomize.Render(
			kustomize.Options{KustomizationPath: "../../../examples/dashboards/update_sample_custom_dashboard"},
		)
		Expect(
			utils.Apply(
				testOptions.HubCluster.ClusterServerURL,
				testOptions.KubeConfig,
				testOptions.HubCluster.KubeContext,
				yamlB)).NotTo(HaveOccurred())
		Eventually(func() bool {
			_, result := utils.ContainDashboard(testOptions, dashboardTitle)
			return result
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(BeFalse())
		Eventually(func() bool {
			_, result := utils.ContainDashboard(testOptions, updateDashboardTitle)
			return result
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(BeTrue())
	})

	It("[P2][Sev2][observability][Stable] Should have no custom dashboard in grafana after related configmap removed (dashboard/g0)", func() {
		By("Deleting custom dashboard configmap")
		err = utils.DeleteConfigMap(testOptions, true, dashboardName, MCO_NAMESPACE)
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() bool {
			_, result := utils.ContainDashboard(testOptions, updateDashboardTitle)
			return result
		}, EventuallyTimeoutMinute*3, EventuallyIntervalSecond*5).Should(BeFalse())
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

	It("[P2][Sev2][observability][Stable] Should have default overview dashboards (dashboard/g0)", func() {
		// Check Original dash exists
		Eventually(func() bool {
			_, result := utils.ContainDashboard(testOptions, clusterOverviewTitle)
			return result
		}, EventuallyTimeoutMinute*3, EventuallyIntervalSecond*5).Should(BeTrue())
		// Check optimized dash
		Eventually(func() bool {
			_, result := utils.ContainDashboard(testOptions, clusterOverviewOptimizedTitle)
			return result
		}, EventuallyTimeoutMinute*3, EventuallyIntervalSecond*5).Should(BeTrue())

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
