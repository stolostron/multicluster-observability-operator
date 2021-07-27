// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tests

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/open-cluster-management/multicluster-observability-operator/tests/pkg/utils"
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

	Context("[P1][Sev1][Observability] Should revert any manual changes on observatorium cr (observatorium_preserve/g0) -", func() {
		It("[Stable] Updating observatorium cr (spec.thanos.compact.retentionResolution1h) should be automatically reverted", func() {
			oldResourceVersion := ""
			updateRetention := "10d"
			Eventually(func() error {
				cr, err := dynClient.Resource(utils.NewMCOMObservatoriumGVR()).Namespace(MCO_NAMESPACE).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
				if err != nil {
					return err
				}
				cr.Object["spec"].(map[string]interface{})["thanos"].(map[string]interface{})["compact"].(map[string]interface{})["retentionResolution1h"] = updateRetention
				oldResourceVersion = cr.Object["metadata"].(map[string]interface{})["resourceVersion"].(string)
				_, err = dynClient.Resource(utils.NewMCOMObservatoriumGVR()).Namespace(MCO_NAMESPACE).Update(context.TODO(), cr, metav1.UpdateOptions{})
				return err
			}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*1).Should(Succeed())

			Eventually(func() bool {
				cr, err := dynClient.Resource(utils.NewMCOMObservatoriumGVR()).Namespace(MCO_NAMESPACE).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
				if err == nil {
					replicasNewRetention := cr.Object["spec"].(map[string]interface{})["thanos"].(map[string]interface{})["compact"].(map[string]interface{})["retentionResolution1h"]
					newResourceVersion := cr.Object["metadata"].(map[string]interface{})["resourceVersion"].(string)
					if newResourceVersion != oldResourceVersion &&
						replicasNewRetention != updateRetention {
						return true
					}
				}
				return false
			}, EventuallyTimeoutMinute*3, EventuallyIntervalSecond*1).Should(BeTrue())

			// wait for pod restarting
			time.Sleep(10 * time.Second)

			By("Wait for thanos compact pods are ready")
			sts, err := utils.GetStatefulSetWithLabel(testOptions, true, THANOS_COMPACT_LABEL, MCO_NAMESPACE)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(sts.Items)).NotTo(Equal(0))
			// ensure the thanos rule pods are restarted successfully before processing
			Eventually(func() error {
				err = utils.CheckStatefulSetPodReady(testOptions, (*sts).Items[0].Name)
				if err != nil {
					return err
				}
				return nil
			}, EventuallyTimeoutMinute*10, EventuallyIntervalSecond*5).Should(Succeed())
		})
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
