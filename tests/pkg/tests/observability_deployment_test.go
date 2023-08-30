// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tests

import (
	"context"
	"os"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

	It("RHACM4K-1064: Observability: Verify MCO deployment - [P1][Sev1][Observability][Stable]@ocpInterop @e2e @post-release (deployment/g0)", func() {
		By("Check MCO in ready status")
		Eventually(func() error {
			err = utils.CheckMCOComponents(testOptions)
			if err != nil {
				testFailed = true
				utils.PrintAllMCOPodsStatus(testOptions)
				return err
			}
			testFailed = false
			return nil
		}, EventuallyTimeoutMinute*25, EventuallyIntervalSecond*10).Should(Succeed())

		By("Check clustermanagementaddon CR is created")
		Eventually(func() error {
			_, err := dynClient.Resource(utils.NewMCOClusterManagementAddonsGVR()).Get(context.TODO(), "observability-controller", metav1.GetOptions{})
			if err != nil {
				testFailed = true
				return err
			}
			testFailed = false
			return nil
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

	})

	It("RHACM4K-1288: Observability: Verify Observability function working on the hub cluster - [P1][Sev1][Observability][Stable]@ocpInterop @e2e @post-release (deployment/g0)", func() {
		By("Check endpoint-operator and metrics-collector pods are ready")
		Eventually(func() error {
			err = utils.CheckAllOBAsEnabledLocal(testOptions)
			if err != nil {
				testFailed = true
				return err
			}
			testFailed = false
			return nil
		}, EventuallyTimeoutMinute*20, EventuallyIntervalSecond*10).Should(Succeed())

	})

	It("RHACM4K-30645: Observability: Verify setting in CM cluster-monitoring-config is not removed after MCO enabled - [P1][Sev1][Observability][Stable]@ocpInterop @e2e @post-release (deployment/g1)", func() {
		By("Check enableUserAlertmanagerConfig value is not replaced in the CM cluster-monitoring-config")
		if os.Getenv("SKIP_INSTALL_STEP") == "true" {
			Skip("Skip the case due to this case is only available before MCOCR deployment")
		}
		Eventually(func() bool {

			cm, err := hubClient.CoreV1().ConfigMaps("openshift-monitoring").Get(context.TODO(), "cluster-monitoring-config", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			if strings.Contains(cm.String(), "enableUserAlertmanagerConfig: true") {
				return true
			}
			return false
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(BeTrue())
	})

	AfterEach(func() {
		if CurrentGinkgoTestDescription().Failed {
			utils.PrintMCOObject(testOptions)
			utils.PrintAllMCOPodsStatus(testOptions)
			utils.PrintAllOBAPodsStatus(testOptions)
			utils.PrintManagedClusterOBAObject(testOptions)
		}
		testFailed = testFailed || CurrentGinkgoTestDescription().Failed
	})

})
