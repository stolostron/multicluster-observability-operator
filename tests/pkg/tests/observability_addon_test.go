// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tests

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	"github.com/open-cluster-management/multicluster-observability-operator/tests/pkg/utils"
)

const (
	ManagedClusterAddOnMessage = "enableMetrics is set to False"
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

	Context("[P2][Sev2][Observability] Modifying MCO cr to disable observabilityaddon (addon/g0) -", func() {
		clusterName := utils.GetManagedClusterName(testOptions)
		It("[Stable] Should have endpoint-operator and metrics-collector being deployed", func() {
			By("Check enableMetrics is true")
			enable, err := utils.GetMCOAddonSpecMetrics(testOptions)
			Expect(err).ToNot(HaveOccurred())
			Expect(enable).To(Equal(true))

			By("Check ObservabilityAddon is created if there's managed OCP clusters on the hub")
			if clusterName != "" {
				Eventually(func() string {
					mco, err := dynClient.Resource(utils.NewMCOAddonGVR()).Namespace(string(clusterName)).Get(context.TODO(), "observability-addon", metav1.GetOptions{})
					if err != nil {
						panic(err.Error())
					}
					return fmt.Sprintf("%T", mco.Object["status"])
				}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).ShouldNot(Equal("nil"))
				Eventually(func() string {
					mco, err := dynClient.Resource(utils.NewMCOAddonGVR()).Namespace(string(clusterName)).Get(context.TODO(), "observability-addon", metav1.GetOptions{})
					if err != nil {
						panic(err.Error())
					}
					return mco.Object["status"].(map[string]interface{})["conditions"].([]interface{})[0].(map[string]interface{})["message"].(string)
				}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Equal("Metrics collector deployed and functional"))
			}

			By("Check endpoint-operator and metrics-collector pods are created")
			Eventually(func() error {
				return utils.CheckMCOAddon(testOptions)
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
		})

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
				err, podList := utils.GetPodList(testOptions, false, MCO_ADDON_NAMESPACE, "component=metrics-collector")
				if len(podList.Items) != 0 || err != nil {
					return fmt.Errorf("Failed to disable observability addon")
				}
				return nil
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

			if clusterName != "" {
				Eventually(func() string {
					mco, err := dynClient.Resource(utils.NewMCOAddonGVR()).Namespace(string(clusterName)).Get(context.TODO(), "observability-addon", metav1.GetOptions{})
					if err != nil {
						panic(err.Error())
					}
					if mco.Object["status"] != nil {
						return mco.Object["status"].(map[string]interface{})["conditions"].([]interface{})[0].(map[string]interface{})["message"].(string)
					} else {
						return ""
					}
				}, EventuallyTimeoutMinute*3, EventuallyIntervalSecond*5).Should(Equal(ManagedClusterAddOnMessage))

				Eventually(func() string {
					mco, err := dynClient.Resource(utils.NewMCOManagedClusterAddonsGVR()).Namespace(string(clusterName)).Get(context.TODO(), "observability-controller", metav1.GetOptions{})
					Expect(err).ToNot(HaveOccurred())
					conditions := mco.Object["status"].(map[string]interface{})["conditions"].([]interface{})
					for _, condition := range conditions {
						if condition.(map[string]interface{})["message"].(string) == ManagedClusterAddOnMessage {
							return condition.(map[string]interface{})["status"].(string)
						}
					}
					return ""
				}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*5).Should(Equal("True"))
			}
		})
		// it takes Prometheus 5m to notice a metric is not available - https://github.com/prometheus/prometheus/issues/1810
		// the corret way is use timestamp, for example:
		// timestamp(node_memory_MemAvailable_bytes{cluster="local-cluster"}) - timestamp(node_memory_MemAvailable_bytes{cluster="local-cluster"} offset 1m) > 59
		It("[Stable] Waiting for check no metric data in grafana console", func() {
			Eventually(func() error {
				err, hasMetric := utils.ContainManagedClusterMetric(testOptions, `timestamp(node_memory_MemAvailable_bytes{cluster="`+clusterName+`}) - timestamp(node_memory_MemAvailable_bytes{cluster=`+clusterName+`"} offset 1m) > 59`, []string{`"__name__":"node_memory_MemAvailable_bytes"`})
				if err != nil && !hasMetric && strings.Contains(err.Error(), "Failed to find metric name from response") {
					return nil
				}
				return fmt.Errorf("Check no metric data in grafana console error: %v", err)
			}, EventuallyTimeoutMinute*2, EventuallyIntervalSecond*5).Should(Succeed())
		})

		It("[Stable] Modifying MCO cr to enable observabilityaddon", func() {
			Eventually(func() error {
				return utils.ModifyMCOAddonSpecMetrics(testOptions, true)
			}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*5).Should(Succeed())

			By("Waiting for MCO addon components ready")
			Eventually(func() bool {
				err, podList := utils.GetPodList(testOptions, false, MCO_ADDON_NAMESPACE, "component=metrics-collector")
				if len(podList.Items) == 1 && err == nil {
					return true
				}
				return false
			}, EventuallyTimeoutMinute*6, EventuallyIntervalSecond*5).Should(BeTrue())

			By("Checking the status in managedclusteraddon reflects the endpoint operator status correctly")
			if clusterName != "" {
				Eventually(func() string {
					mco, err := dynClient.Resource(utils.NewMCOManagedClusterAddonsGVR()).Namespace(string(clusterName)).Get(context.TODO(), "observability-controller", metav1.GetOptions{})
					Expect(err).ToNot(HaveOccurred())
					conditions := mco.Object["status"].(map[string]interface{})["conditions"].([]interface{})
					for _, condition := range conditions {
						if condition.(map[string]interface{})["message"].(string) == "Send metrics successfully" {
							return condition.(map[string]interface{})["status"].(string)
						}
					}
					return ""
				}, EventuallyTimeoutMinute*3, EventuallyIntervalSecond*5).Should(Equal("True"))
			}
		})
	})

	It("[P3][Sev3][Observability][Stable] Should not set interval to values beyond scope (addon/g0)", func() {
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

	Context("[P2][Sev2][Observability] Should not have the expected MCO addon pods when disable observability from managedcluster (addon/g0) -", func() {
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
				err, podList := utils.GetPodList(testOptions, false, MCO_ADDON_NAMESPACE, "component=metrics-collector")
				if len(podList.Items) == 1 && err == nil {
					return true
				}
				return false
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(BeTrue())
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
			utils.PrintManagedClusterOBAObject(testOptions)
		}
		testFailed = testFailed || CurrentGinkgoTestDescription().Failed
	})
})
