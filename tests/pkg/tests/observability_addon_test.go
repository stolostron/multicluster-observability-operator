// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"fmt"
	"strings"

	"errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/klog"

	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
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
		if utils.GetManagedClusterName(testOptions) == hubManagedClusterName {
			Skip("Skip the case for local-cluster since no observability addon")
		}
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

	Context("[P2][Sev2][observability] Modifying MCO cr to disable observabilityaddon (addon/g0) -", func() {
		It("[Stable] Should have resource requirement defined in CR", func() {
			By("Check addon resource requirement")
			res, err := utils.GetMCOAddonSpecResources(testOptions)
			Expect(err).ToNot(HaveOccurred())
			limits := res["limits"].(map[string]interface{})
			requests := res["requests"].(map[string]interface{})
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
				err, podList := utils.GetPodList(
					testOptions,
					false,
					MCO_ADDON_NAMESPACE,
					"component=metrics-collector",
				)
				if err != nil {
					return errors.New("Failed to disable observability addon")
				}
				if len(podList.Items) != 0 {
					for _, po := range podList.Items {
						if po.Status.Phase == "Running" {
							return errors.New("Failed to disable observability addon, there is still metrics-collector pod in Running")
						}
					}
				}
				return nil
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

			Eventually(func() error {
				err = utils.CheckAllOBAsDeleted(testOptions)
				if err != nil {
					return err
				}
				return nil
			}, EventuallyTimeoutMinute*20, EventuallyIntervalSecond*5).Should(Succeed())
		})
		// it takes Prometheus 5m to notice a metric is not available -
		// https://github.com/prometheus/prometheus/issues/1810
		// the corret way is use timestamp, for example:
		// timestamp(node_memory_MemAvailable_bytes{cluster="local-cluster"}) -
		// timestamp(node_memory_MemAvailable_bytes{cluster="local-cluster"} offset 1m) > 59
		It("[Stable] Waiting for check no metric data in grafana console", func() {
			Eventually(func() error {
				for _, cluster := range clusters {
					res, err := utils.QueryGrafana(
						testOptions,
						`timestamp(node_memory_MemAvailable_bytes{cluster="`+cluster+`}) - timestamp(node_memory_MemAvailable_bytes{cluster=`+cluster+`"} offset 1m) > 59`,
					)
					if err != nil {
						return err
					}
					if len(res.Data.Result) != 0 {
						return fmt.Errorf("Grafa console still has metric data: %v", res.Data.Result)
					}
				}
				return nil
			}, EventuallyTimeoutMinute*2, EventuallyIntervalSecond*5).Should(Succeed())
		})

		It("[Stable] Modifying MCO cr to enable observabilityaddon", func() {
			Eventually(func() error {
				return utils.ModifyMCOAddonSpecMetrics(testOptions, true)
			}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*5).Should(Succeed())

			By("Waiting for MCO addon components ready")
			Eventually(func() bool {
				err, podList := utils.GetPodList(
					testOptions,
					false,
					MCO_ADDON_NAMESPACE,
					"component=metrics-collector",
				)
				// starting with OCP 4.13, userWorkLoadMonitoring is enabled by default
				if len(podList.Items) >= 1 && err == nil {
					return true
				}
				return false
			}, EventuallyTimeoutMinute*6, EventuallyIntervalSecond*5).Should(BeTrue())

			By("Checking the status in managedclusteraddon reflects the endpoint operator status correctly")
			Eventually(func() error {
				err = utils.CheckAllOBAsEnabled(testOptions)
				if err != nil {
					return err
				}
				return nil
			}, EventuallyTimeoutMinute*15, EventuallyIntervalSecond*5).Should(Succeed())
		})
	})

	It("[P3][Sev3][observability][Stable] Should not set interval to values beyond scope (addon/g0)", func() {
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

	Context("[P2][Sev2][observability] Should not have the expected MCO addon pods when disable observability from managedcluster (addon/g0) -", func() {
		It("[Stable] Modifying managedcluster cr to disable observability", func() {
			Eventually(func() error {
				return utils.UpdateObservabilityFromManagedCluster(testOptions, false)
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

			By("Waiting for MCO addon components scales to 0")
			Eventually(func() bool {
				err, obaNS := utils.GetNamespace(testOptions, false, MCO_ADDON_NAMESPACE)
				if err == nil && obaNS == nil {
					return true
				}
				return false
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(BeTrue())
		})

		It("[Stable] Remove disable observability label from the managed cluster", func() {
			Eventually(func() error {
				return utils.UpdateObservabilityFromManagedCluster(testOptions, true)
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

			By("Waiting for MCO addon components ready")
			Eventually(func() bool {
				err, podList := utils.GetPodList(
					testOptions,
					false,
					MCO_ADDON_NAMESPACE,
					"component=metrics-collector",
				)
				// starting with OCP 4.13, userWorkLoadMonitoring is enabled by default
				if len(podList.Items) >= 1 && err == nil {
					return true
				}
				return false
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(BeTrue())
		})
	},
	)

	JustAfterEach(func() {
		Expect(utils.IntegrityChecking(testOptions)).NotTo(HaveOccurred())
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
