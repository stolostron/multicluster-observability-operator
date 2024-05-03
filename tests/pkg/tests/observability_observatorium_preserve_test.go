// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
)

var _ = Describe("Observability:", func() {
	BeforeEach(func() {
		dynClient = utils.NewKubeClientDynamic(
			testOptions.HubCluster.ClusterServerURL,
			testOptions.KubeConfig,
			testOptions.HubCluster.KubeContext)
	})

	Context("[P1][Sev1][observability] Should revert any manual changes on observatorium cr (observatorium_preserve/g0) -", func() {
		It("[Stable] Updating observatorium cr (spec.thanos.compact.retentionResolution1h) should be automatically reverted", func() {
			oldCRResourceVersion := ""
			updateRetention := "10d"
			oldCompactResourceVersion := ""
			Eventually(func() error {
				cr, err := dynClient.Resource(utils.NewMCOMObservatoriumGVR()).
					Namespace(MCO_NAMESPACE).
					Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
				if err != nil {
					return err
				}
				oldCRResourceVersion = cr.Object["metadata"].(map[string]interface{})["resourceVersion"].(string)

				sts, err := utils.GetStatefulSetWithLabel(testOptions, true, THANOS_COMPACT_LABEL, MCO_NAMESPACE)
				if err != nil {
					return err
				}
				oldCompactResourceVersion = (*sts).Items[0].ResourceVersion

				cr.Object["spec"].(map[string]interface{})["thanos"].(map[string]interface{})["compact"].(map[string]interface{})["retentionResolution1h"] = updateRetention
				_, err = dynClient.Resource(utils.NewMCOMObservatoriumGVR()).
					Namespace(MCO_NAMESPACE).
					Update(context.TODO(), cr, metav1.UpdateOptions{})
				return err
			}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*1).Should(Succeed())

			Eventually(func() bool {
				cr, err := dynClient.Resource(utils.NewMCOMObservatoriumGVR()).
					Namespace(MCO_NAMESPACE).
					Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
				if err == nil {
					replicasNewRetention := cr.Object["spec"].(map[string]interface{})["thanos"].(map[string]interface{})["compact"].(map[string]interface{})["retentionResolution1h"]
					newResourceVersion := cr.Object["metadata"].(map[string]interface{})["resourceVersion"].(string)
					if newResourceVersion != oldCRResourceVersion &&
						replicasNewRetention != updateRetention {
						return true
					}
				}
				return false
			}, EventuallyTimeoutMinute*3, EventuallyIntervalSecond*1).Should(BeTrue())

			// ensure the thanos compact is restarted
			Eventually(func() bool {
				sts, err := utils.GetStatefulSetWithLabel(testOptions, true, THANOS_COMPACT_LABEL, MCO_NAMESPACE)
				if err == nil {
					if (*sts).Items[0].ResourceVersion != oldCompactResourceVersion {
						argList := (*sts).Items[0].Spec.Template.Spec.Containers[0].Args
						for _, arg := range argList {
							if arg != "--retention.resolution-raw="+updateRetention {
								return true
							}
						}
						return false
					}
				}
				return false
			}, EventuallyTimeoutMinute*10, EventuallyIntervalSecond*5).Should(BeTrue())

			By("Wait for thanos compact pods are ready")
			sts, err := utils.GetStatefulSetWithLabel(testOptions, true, THANOS_COMPACT_LABEL, MCO_NAMESPACE)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(sts.Items)).NotTo(Equal(0))
			// ensure the thanos rule pod is ready
			Eventually(func() error {
				err = utils.CheckStatefulSetPodReady(testOptions, (*sts).Items[0].Name)
				if err != nil {
					return err
				}
				return nil
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
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
