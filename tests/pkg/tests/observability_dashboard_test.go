// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tests

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-observability-operator/tests/pkg/kustomize"
	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
)

const (
	dashboardName        = "sample-dashboard"
	dashboardTitle       = "Sample Dashboard for E2E"
	updateDashboardTitle = "Update Sample Dashboard for E2E"
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

	It("RHACM4K-1669: Observability: Verify new customized Grafana dashboard - Should have custom dashboard which defined in configmap [P2][Sev2][Observability][Stable]@ocpInterop @e2e (dashboard/g0)", func() {
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
		}, EventuallyTimeoutMinute*3, EventuallyIntervalSecond*5).Should(BeTrue())
	})

	It("RHACM4K-1669: Observability: Verify new customized Grafana dashboard - Should have update custom dashboard after configmap updated [P2][Sev2][Observability][Stable]@ocpInterop @e2e (dashboard/g0)", func() {
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
		}, EventuallyTimeoutMinute*3, EventuallyIntervalSecond*5).Should(BeFalse())
		Eventually(func() bool {
			_, result := utils.ContainDashboard(testOptions, updateDashboardTitle)
			return result
		}, EventuallyTimeoutMinute*3, EventuallyIntervalSecond*5).Should(BeTrue())
	})

	It("RHACM4K-1669: Observability: Verify new customized Grafana dashboard - Should have no custom dashboard in grafana after related configmap removed [P2][Sev2][Observability][Stable]@ocpInterop @e2e (dashboard/g0)", func() {
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
			utils.PrintMCOObject(testOptions)
			utils.PrintAllMCOPodsStatus(testOptions)
			utils.PrintAllOBAPodsStatus(testOptions)
		}
		testFailed = testFailed || CurrentGinkgoTestDescription().Failed
	})
})
