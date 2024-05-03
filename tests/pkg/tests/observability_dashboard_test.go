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
	dashboardName        = "sample-dashboard"
	dashboardTitle       = "Sample Dashboard for E2E"
	updateDashboardTitle = "Update Sample Dashboard for E2E"
)

var _ = Describe("Observability:", func() {
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
		}, EventuallyTimeoutMinute*3, EventuallyIntervalSecond*5).Should(BeTrue())
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
		}, EventuallyTimeoutMinute*3, EventuallyIntervalSecond*5).Should(BeFalse())
		Eventually(func() bool {
			_, result := utils.ContainDashboard(testOptions, updateDashboardTitle)
			return result
		}, EventuallyTimeoutMinute*3, EventuallyIntervalSecond*5).Should(BeTrue())
	})

	It("[P2][Sev2][observability][Stable] Should have no custom dashboard in grafana after related configmap removed (dashboard/g0)", func() {
		By("Deleting custom dashboard configmap")
		err = utils.DeleteConfigMap(testOptions.ManagedClusters[0], true, dashboardName, MCO_NAMESPACE)
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
