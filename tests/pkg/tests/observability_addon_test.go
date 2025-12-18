// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"context"
	"fmt"
	"strings"
	"time"

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
		if utils.GetManagedClusterName(testOptions) == hubManagedClusterName {
			Skip("Skip the case for local-cluster since no observability addon")
		}
	})

	Context("RHACM4K-1260: Observability: Verify monitoring operator and deployment status when metrics collection disabled [P2][Sev2][Observability]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @pre-upgrade  (addon/g0) -", func() {
		It("[Stable] Should have resource requirement defined in CR", func() {
			By("Check addon resource requirement")
			res, err := utils.GetMCOAddonSpecResources(testOptions)
			Expect(err).ToNot(HaveOccurred())
			limits := res["limits"].(map[string]any)
			requests := res["requests"].(map[string]any)
			Expect(limits["cpu"]).To(Equal("200m"))
			Expect(limits["memory"]).To(Equal("700Mi"))
			Expect(requests["cpu"]).To(Equal("10m"))
			Expect(requests["memory"]).To(Equal("100Mi"))
		})

		It("[Stable] Should have resource requirement in metrics-collector", func() {
			By("Check metrics-collector resource requirement")
			Eventually(func() error {
				return utils.CheckMCOAddonResources(testOptions)
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
		})

		It("[Stable] Should not have the expected MCO addon pods when disable observabilityaddon", func() {
			Eventually(func() error {
				return utils.ModifyMCOAddonSpecMetrics(testOptions, false)
			}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*5).Should(Succeed())

			By("Waiting for MCO addon components scales to 0")
			Eventually(func() error {
				err = utils.CheckAllOBAsDeleted(testOptions)

				if err != nil {
					return fmt.Errorf("Failed to disable observability addon")
				}
				return nil
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

		})
		// // it takes Prometheus 5m to notice a metric is not available -
		// // https://github.com/prometheus/prometheus/issues/1810
		// // the corret way is use timestamp, for example:
		// // timestamp(node_memory_MemAvailable_bytes{cluster="local-cluster"}) -
		// // timestamp(node_memory_MemAvailable_bytes{cluster="local-cluster"} offset 1m) > 59
		// It("[Stable] Waiting for check no metric data in grafana console", func() {
		// 	Eventually(func() error {
		// 		clusters, clusterError = utils.ListManagedClusters(testOptions)
		// 		if clusterError != nil {
		// 			return clusterError
		// 		}
		// 		for _, cluster := range clusters {
		// 			res, err := utils.QueryGrafana(
		// 				testOptions,
		// 				`timestamp(node_memory_MemAvailable_bytes{cluster="`+cluster.Name+`}) - timestamp(node_memory_MemAvailable_bytes{cluster=`+cluster.Name+`"} offset 1m) > 59`,
		// 			)
		// 			if err != nil {
		// 				return err
		// 			}
		// 			if len(res.Data.Result) != 0 {
		// 				return fmt.Errorf("Grafa console still has metric data: %v", res.Data.Result)
		// 			}
		// 		}
		// 		return nil
		// 	}, EventuallyTimeoutMinute*2, EventuallyIntervalSecond*5).Should(Succeed())
		// })
		It("RHACM4K-1418: Observability: Verify clustermanagementaddon CR for Observability - Modifying MCO cr to enable observabilityaddon [P2][Sev2][Stable][Observability]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @pre-upgrade (addon/g0)", func() {
			By("Waiting for 1 minute to make sure the registration controller correctly takes into account the changes")
			time.Sleep(60 * time.Second)
			Eventually(func() error {
				return utils.ModifyMCOAddonSpecMetrics(testOptions, true)
			}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*5).Should(Succeed())

			By("Waiting for MCO addon components ready")
			Eventually(func() bool {
				err, podList := utils.GetPodList(
					testOptions,
					true,
					MCO_NAMESPACE,
					"component=metrics-collector",
				)
				// starting with OCP 4.13, userWorkLoadMonitoring is enabled by default
				if len(podList.Items) >= 1 && err == nil {
					return true
				}
				return false
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(BeTrue())
		})
		It("RHACM4K-1074: Observability: Verify ObservabilityEndpoint operator deployment - Modifying MCO cr to enable observabilityaddon [P2][Sev2][Stable][Observability]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release @pre-upgrade (addon/g0)", func() {
			// Eventually(func() error {
			// 	return utils.ModifyMCOAddonSpecMetrics(testOptions, true)
			// }, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*5).Should(Succeed())

			By("Checking the status in managedclusteraddon reflects the endpoint operator status correctly")
			Eventually(func() error {
				err = utils.CheckAllOBAsEnabled(testOptions)
				if err != nil {
					return err
				}
				return nil
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
		})
	})

	It("RHACM4K-6923: Observability: Verify default scrap interval change to 5 minutes - [P2][Sev2][Observability][Stable]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release @pre-upgrade (addon/g2)", func() {
		By("Check default interval value is 300")
		// get the current interval, so we can revert to it after the test
		mco, getErr := dynClient.Resource(utils.NewMCOGVRV1BETA2()).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
		Expect(getErr).NotTo(HaveOccurred())

		observabilityAddonSpec := mco.Object["spec"].(map[string]any)["observabilityAddonSpec"].(map[string]any)
		oldInterval := observabilityAddonSpec["interval"]
		// set the interval to 0 (null) to ensure the default interval is applied
		err := utils.ModifyMCOAddonSpecInterval(testOptions, int64(0))
		Expect(err).NotTo(HaveOccurred())

		// Test the interval is now 300, which should be the default
		Eventually(func() bool {
			mco, getErr := dynClient.Resource(utils.NewMCOGVRV1BETA2()).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
			Expect(getErr).NotTo(HaveOccurred())

			observabilityAddonSpec := mco.Object["spec"].(map[string]any)["observabilityAddonSpec"].(map[string]any)
			return observabilityAddonSpec["interval"] == int64(300)
		}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*1).Should(BeTrue())

		// revert to original interval
		err = utils.ModifyMCOAddonSpecInterval(testOptions, oldInterval.(int64))
		Expect(err).NotTo(HaveOccurred())
	})

	It("RHACM4K-1235: Observability: Verify metrics data global setting on the managed cluster - Should not set interval to values beyond scope [P3][Sev3][Observability][Stable]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release @pre-upgrade (addon/g0)", func() {
		By("Set interval to 14")
		Eventually(func() bool {
			err := utils.ModifyMCOAddonSpecInterval(testOptions, int64(14))
			if strings.Contains(err.Error(), "Invalid value") &&
				strings.Contains(err.Error(), "15") {
				return true
			}
			klog.V(1).Infof("error message: <%s>\n", err.Error())
			return false
		}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*1).Should(BeTrue())

		By("Set interval to 3601")
		Eventually(func() bool {
			err := utils.ModifyMCOAddonSpecInterval(testOptions, int64(3601))
			if strings.Contains(err.Error(), "Invalid value") &&
				strings.Contains(err.Error(), "3600") {
				return true
			}
			klog.V(1).Infof("error message: <%s>\n", err.Error())
			return false
		}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*1).Should(BeTrue())
	})

	It("RHACM4K-1259: Observability: Verify imported cluster is observed [P3][Sev3][Observability][Stable]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore (deploy/g1)", func() {

		// Eventually(func() error {
		// 	return utils.UpdateObservabilityFromManagedCluster(testOptions, true)
		// }, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

		klog.V(1).Infof("managedcluster number is <%d>", len(testOptions.ManagedClusters))
		if len(testOptions.ManagedClusters) >= 1 {
			By("Waiting for ObservabilityAddon to be enabled and ready")
			Eventually(func() error {
				return utils.CheckAllOBAsEnabled(testOptions)
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

			By("Waiting for MCO addon components to be running")
			Eventually(func() bool {
				err, podList := utils.GetPodList(
					testOptions,
					false,
					MCO_ADDON_NAMESPACE,
					"component=metrics-collector",
				)
				if err != nil {
					klog.V(1).Infof("Failed to get pod list: %v", err)
					return false
				}
				if len(podList.Items) < 1 {
					klog.V(1).Infof("No metrics-collector pods found yet")
					return false
				}
				// Verify all pods are in Running state
				for _, pod := range podList.Items {
					if pod.Status.Phase != "Running" {
						klog.V(1).Infof("Pod %s is not running yet: %s", pod.Name, pod.Status.Phase)
						return false
					}
				}
				return true
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(BeTrue())
		}
	})

	// Context("RHACM4K-7518: Observability: Disable the Observability by updating managed cluster label [P2][Sev2][Observability]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore (addon/g1) -", func() {
	// 	It("[Stable] Modifying managedcluster cr to disable observability", func() {
	// 		Eventually(func() error {
	// 			return utils.UpdateObservabilityFromManagedCluster(testOptions, false)
	// 		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

	// 		klog.V(1).Infof("managedcluster number is <%d>", len(testOptions.ManagedClusters))
	// 		if len(testOptions.ManagedClusters) > 0 {
	// 			By("Waiting for MCO addon components scales to 0")
	// 			Eventually(func() bool {
	// 				err, obaNS := utils.GetNamespace(testOptions, false, MCO_ADDON_NAMESPACE)
	// 				if err == nil && obaNS == nil {
	// 					return true
	// 				}
	// 				return false
	// 			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(BeTrue())
	// 		}
	// 	})

	// 	It("[Stable] Remove disable observability label from the managed cluster", func() {
	// 		Eventually(func() error {
	// 			return utils.UpdateObservabilityFromManagedCluster(testOptions, true)
	// 		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

	// 		if len(testOptions.ManagedClusters) > 0 {
	// 			By("Waiting for MCO addon components ready")
	// 			Eventually(func() bool {
	// 				err, podList := utils.GetPodList(
	// 					testOptions,
	// 					false,
	// 					MCO_ADDON_NAMESPACE,
	// 					"component=metrics-collector",
	// 				)
	// 				// starting with OCP 4.13, userWorkLoadMonitoring is enabled by default
	// 				if len(podList.Items) >= 1 && err == nil {
	// 					return true
	// 				}
	// 				return false
	// 			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(BeTrue())
	// 		}
	// 	})
	// },
	// )

	JustAfterEach(func() {
		Expect(utils.IntegrityChecking(testOptions)).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			utils.LogFailingTestStandardDebugInfo(testOptions)
		}
		testFailed = testFailed || CurrentSpecReport().Failed()
	})
})
