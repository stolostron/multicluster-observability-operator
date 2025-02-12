// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"context"
	"fmt"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

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

	It("RHACM4K-1064: Observability: Verify MCO deployment - [P1][Sev1][Observability][Stable]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release @pre-upgrade (deployment/g0)", func() {
		By("Check MCO in ready status")
		Eventually(func() error {
			err = utils.CheckMCOComponents(testOptions)
			if err != nil {
				testFailed = true
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

	It("RHACM4K-1288: Observability: Verify Observability function working on the hub cluster - [P1][Sev1][Observability][Stable]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release @pre-upgrade (deployment/g0)", func() {
		By("Check etrics-collector pod is ready")
		Eventually(func() error {

			err, podList := utils.GetPodList(
				testOptions,
				true,
				"open-cluster-management-observability",
				"component=metrics-collector",
			)

			if err != nil {
				return fmt.Errorf("Failed to get the pod metrics-collector")
			}
			if len(podList.Items) != 0 {
				for _, po := range podList.Items {
					if po.Status.Phase == "Running" {
						klog.V(1).Infof("metrics-collector pod in Running")
						return nil
					}
				}
			}
			return nil
		}, EventuallyTimeoutMinute*20, EventuallyIntervalSecond*10).Should(Succeed())

	})

	It("RHACM4K-30645: Observability: Verify setting in CM cluster-monitoring-config is not removed after MCO enabled - [P1][Sev1][Observability][Stable]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release @pre-upgrade (deployment/g1)", func() {
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
			utils.LogFailingTestStandardDebugInfo(testOptions)
		}
		testFailed = testFailed || CurrentGinkgoTestDescription().Failed
	})

})
