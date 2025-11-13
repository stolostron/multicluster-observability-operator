// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"context"
	"fmt"
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
		// it takes Prometheus 5m to notice a metric is not available -
		// https://github.com/prometheus/prometheus/issues/1810
		// the corret way is use timestamp, for example:
		// timestamp(node_memory_MemAvailable_bytes{cluster="local-cluster"}) -
		// timestamp(node_memory_MemAvailable_bytes{cluster="local-cluster"} offset 1m) > 59
		It("[Stable] Waiting for check no metric data in grafana console", func() {
			Eventually(func() error {
				clusters, clusterError = utils.ListManagedClusters(testOptions)
				if clusterError != nil {
					return clusterError
				}
				for _, cluster := range clusters {
					res, err := utils.QueryGrafana(
						testOptions,
						`timestamp(node_memory_MemAvailable_bytes{cluster="`+cluster.Name+`}) - timestamp(node_memory_MemAvailable_bytes{cluster=`+cluster.Name+`"} offset 1m) > 59`,
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
		It("RHACM4K-1418: Observability: Verify clustermanagementaddon CR for Observability - Modifying MCO cr to enable observabilityaddon [P2][Sev2][Stable][Observability]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @pre-upgrade (addon/g0)", func() {
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
			}, EventuallyTimeoutMinute*6, EventuallyIntervalSecond*5).Should(BeTrue())
		})
		It("RHACM4K-1074: Observability: Verify ObservabilityEndpoint operator deployment - Modifying MCO cr to enable observabilityaddon [P2][Sev2][Stable][Observability]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release @pre-upgrade (addon/g0)", func() {
			Eventually(func() error {
				return utils.ModifyMCOAddonSpecMetrics(testOptions, true)
			}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*5).Should(Succeed())

			By("Checking the status in managedclusteraddon reflects the endpoint operator status correctly")
			Eventually(func() error {
				err = utils.CheckAllOBAsEnabled(testOptions)
				if err != nil {
					return err
				}
				return nil
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
		})

		It("Observability: Verify AddOnDeploymentConfig specHash is managed by addon-framework", func() {
			testConfigName := "observability-e2e-test-config"
			managedClusterName := utils.GetManagedClusterName(testOptions)

			// Setup: Create a test AddOnDeploymentConfig
			By("Creating test AddOnDeploymentConfig")
			testConfig := map[string]interface{}{
				"spec": map[string]interface{}{
					"nodePlacement": map[string]interface{}{
						"nodeSelector": map[string]interface{}{
							"test-node": "e2e",
						},
					},
				},
			}
			err := utils.CreateClusterSpecificAddOnDeploymentConfig(testOptions, testConfigName, MCO_NAMESPACE, testConfig)
			Expect(err).NotTo(HaveOccurred())

			// Setup: Add this config as defaultConfig in CMA
			By("Adding test config as CMA defaultConfig")
			err = utils.UpdateClusterManagementAddOnDefaultConfig(testOptions, testConfigName, MCO_NAMESPACE)
			Expect(err).NotTo(HaveOccurred())

			// Test: Verify addon-framework populates specHash in CMA
			By("Validating CMA status.defaultConfigReferences has specHash")
			Eventually(func() error {
				_, err := utils.GetClusterManagementAddOnSpecHash(testOptions)
				return err
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

			// Test: Verify addon-framework populates specHash in MCA
			By("Validating MCA status.configReferences has specHash")
			Eventually(func() error {
				return utils.ValidateAddOnDeploymentConfigSpecHash(testOptions, managedClusterName)
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

			// Cleanup: Remove defaultConfig from CMA
			By("Cleanup: Clearing CMA defaultConfig")
			err = utils.ClearClusterManagementAddOnDefaultConfig(testOptions)
			Expect(err).NotTo(HaveOccurred())

			// Cleanup: Delete test config
			By("Cleanup: Deleting test AddOnDeploymentConfig")
			err = utils.DeleteAddOnDeploymentConfig(testOptions, testConfigName, MCO_NAMESPACE)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Observability: Framework updates specHash when AddOnDeploymentConfig changes", func() {
			testConfigName := "observability-e2e-test-config"
			managedClusterName := utils.GetManagedClusterName(testOptions)

			// Setup: Create a test AddOnDeploymentConfig
			By("Creating test AddOnDeploymentConfig")
			testConfig := map[string]interface{}{
				"spec": map[string]interface{}{
					"nodePlacement": map[string]interface{}{
						"nodeSelector": map[string]interface{}{
							"test-node": "e2e",
						},
					},
				},
			}
			err := utils.CreateClusterSpecificAddOnDeploymentConfig(testOptions, testConfigName, MCO_NAMESPACE, testConfig)
			Expect(err).NotTo(HaveOccurred())

			// Setup: Add this config as defaultConfig in CMA
			By("Adding test config as CMA defaultConfig")
			err = utils.UpdateClusterManagementAddOnDefaultConfig(testOptions, testConfigName, MCO_NAMESPACE)
			Expect(err).NotTo(HaveOccurred())

			// Test: Get initial specHash values
			By("Getting initial specHash values")
			var originalCMAHash, originalMCAHash string
			Eventually(func() error {
				var err error
				originalCMAHash, err = utils.GetClusterManagementAddOnSpecHash(testOptions)
				if err != nil {
					return err
				}
				originalMCAHash, err = utils.GetManagedClusterAddOnSpecHash(testOptions, managedClusterName)
				return err
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

			klog.V(1).Infof("Original specHash - CMA: %s, MCA: %s", originalCMAHash, originalMCAHash)

			// Test: Update config and verify specHash changes
			By("Updating AddOnDeploymentConfig nodeSelector")
			err = utils.UpdateAddOnDeploymentConfig(testOptions, testConfigName, MCO_NAMESPACE, func(config map[string]interface{}) {
				spec := config["spec"].(map[string]interface{})
				nodePlacement := spec["nodePlacement"].(map[string]interface{})
				nodeSelector := nodePlacement["nodeSelector"].(map[string]interface{})
				nodeSelector["test-label"] = "test-value-updated"
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying framework updates specHash in CMA and MCA")
			Eventually(func() error {
				return utils.ValidateSpecHashChanged(testOptions, managedClusterName, originalCMAHash, originalMCAHash)
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

			// Cleanup: Remove defaultConfig from CMA
			By("Cleanup: Clearing CMA defaultConfig")
			err = utils.ClearClusterManagementAddOnDefaultConfig(testOptions)
			Expect(err).NotTo(HaveOccurred())

			// Cleanup: Delete test config
			By("Cleanup: Deleting test AddOnDeploymentConfig")
			err = utils.DeleteAddOnDeploymentConfig(testOptions, testConfigName, MCO_NAMESPACE)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Observability: Framework manages different specHash for per-cluster AddOnDeploymentConfig override", func() {
			testConfigName := "observability-e2e-test-config"
			clusterConfigName := "observability-e2e-cluster-override"
			managedClusterName := utils.GetManagedClusterName(testOptions)

			// Setup: Create a test AddOnDeploymentConfig for CMA default
			By("Creating default test AddOnDeploymentConfig")
			defaultConfig := map[string]interface{}{
				"spec": map[string]interface{}{
					"nodePlacement": map[string]interface{}{
						"nodeSelector": map[string]interface{}{
							"test-node": "e2e-default",
						},
					},
				},
			}
			err := utils.CreateClusterSpecificAddOnDeploymentConfig(testOptions, testConfigName, MCO_NAMESPACE, defaultConfig)
			Expect(err).NotTo(HaveOccurred())

			// Setup: Add this config as defaultConfig in CMA
			By("Adding test config as CMA defaultConfig")
			err = utils.UpdateClusterManagementAddOnDefaultConfig(testOptions, testConfigName, MCO_NAMESPACE)
			Expect(err).NotTo(HaveOccurred())

			// Test: Get baseline specHash values
			By("Recording original CMA and MCA specHash")
			var originalCMAHash, originalMCAHash string
			Eventually(func() error {
				var err error
				originalCMAHash, err = utils.GetClusterManagementAddOnSpecHash(testOptions)
				if err != nil {
					return err
				}
				originalMCAHash, err = utils.GetManagedClusterAddOnSpecHash(testOptions, managedClusterName)
				return err
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

			klog.V(1).Infof("Original specHash - CMA: %s, MCA: %s", originalCMAHash, originalMCAHash)

			// Test: Create per-cluster config override
			By("Creating cluster-specific AddOnDeploymentConfig with different settings")
			clusterConfig := map[string]interface{}{
				"spec": map[string]interface{}{
					"nodePlacement": map[string]interface{}{
						"nodeSelector": map[string]interface{}{
							"test-node":        "e2e-default",
							"cluster-override": "true",
						},
					},
				},
			}
			err = utils.CreateClusterSpecificAddOnDeploymentConfig(testOptions, clusterConfigName, managedClusterName, clusterConfig)
			Expect(err).NotTo(HaveOccurred())

			// Test: Update MCA to use cluster-specific config
			By("Updating ManagedClusterAddOn to use cluster-specific config")
			err = utils.UpdateManagedClusterAddOnConfig(testOptions, managedClusterName, clusterConfigName, managedClusterName)
			Expect(err).NotTo(HaveOccurred())

			// Test: Verify MCA specHash differs from CMA
			By("Verifying framework updates MCA specHash to differ from CMA")
			Eventually(func() error {
				return utils.ValidateSpecHashDifferent(testOptions, managedClusterName)
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

			By("Verifying MCA specHash changed from original")
			newMCAHash, err := utils.GetManagedClusterAddOnSpecHash(testOptions, managedClusterName)
			Expect(err).NotTo(HaveOccurred())
			Expect(newMCAHash).NotTo(Equal(originalMCAHash), "MCA specHash should have changed")

			// Cleanup: Revert MCA to use default config
			By("Cleanup: Reverting to default config")
			err = utils.ClearManagedClusterAddOnConfig(testOptions, managedClusterName)
			Expect(err).NotTo(HaveOccurred())

			// Cleanup: Delete cluster-specific config
			By("Cleanup: Deleting cluster-specific config")
			err = utils.DeleteAddOnDeploymentConfig(testOptions, clusterConfigName, managedClusterName)
			Expect(err).NotTo(HaveOccurred())

			// Cleanup: Remove defaultConfig from CMA
			By("Cleanup: Clearing CMA defaultConfig")
			err = utils.ClearClusterManagementAddOnDefaultConfig(testOptions)
			Expect(err).NotTo(HaveOccurred())

			// Cleanup: Delete default test config
			By("Cleanup: Deleting default test config")
			err = utils.DeleteAddOnDeploymentConfig(testOptions, testConfigName, MCO_NAMESPACE)
			Expect(err).NotTo(HaveOccurred())

			// Verify cleanup worked
			By("Verifying MCA reverts to no config after cleanup")
			Eventually(func() error {
				currentMCAHash, err := utils.GetManagedClusterAddOnSpecHash(testOptions, managedClusterName)
				if err != nil {
					return err
				}
				if currentMCAHash == newMCAHash {
					return fmt.Errorf("MCA specHash still matches cluster override: %s", currentMCAHash)
				}
				return nil
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
		})

		It("Observability: Addon recovers specHash after disable and re-enable", func() {
			testConfigName := "observability-e2e-test-config"
			managedClusterName := utils.GetManagedClusterName(testOptions)

			// Setup: Create a test AddOnDeploymentConfig
			By("Creating test AddOnDeploymentConfig")
			testConfig := map[string]interface{}{
				"spec": map[string]interface{}{
					"nodePlacement": map[string]interface{}{
						"nodeSelector": map[string]interface{}{
							"test-node": "e2e",
						},
					},
				},
			}
			err := utils.CreateClusterSpecificAddOnDeploymentConfig(testOptions, testConfigName, MCO_NAMESPACE, testConfig)
			Expect(err).NotTo(HaveOccurred())

			// Setup: Add this config as defaultConfig in CMA
			By("Adding test config as CMA defaultConfig")
			err = utils.UpdateClusterManagementAddOnDefaultConfig(testOptions, testConfigName, MCO_NAMESPACE)
			Expect(err).NotTo(HaveOccurred())

			// Test: Get initial specHash
			By("Getting initial specHash")
			var originalMCAHash string
			Eventually(func() error {
				var err error
				originalMCAHash, err = utils.GetManagedClusterAddOnSpecHash(testOptions, managedClusterName)
				return err
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

			// Test: Disable addon
			By("Disabling observability addon")
			Eventually(func() error {
				return utils.ModifyMCOAddonSpecMetrics(testOptions, false)
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

			By("Waiting for addon to be disabled")
			Eventually(func() error {
				return utils.CheckAllOBAsDeleted(testOptions)
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

			// Test: Re-enable addon
			By("Re-enabling observability addon")
			Eventually(func() error {
				return utils.ModifyMCOAddonSpecMetrics(testOptions, true)
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

			By("Waiting for addon to be ready")
			Eventually(func() error {
				return utils.CheckAllOBAsEnabled(testOptions)
			}, EventuallyTimeoutMinute*6, EventuallyIntervalSecond*5).Should(Succeed())

			// Test: Verify framework repopulates specHash
			By("Verifying framework repopulates specHash in MCA")
			Eventually(func() error {
				newMCAHash, err := utils.GetManagedClusterAddOnSpecHash(testOptions, managedClusterName)
				if err != nil {
					return err
				}
				if newMCAHash == "" {
					return fmt.Errorf("MCA specHash is empty after re-enable")
				}
				klog.V(1).Infof("SpecHash after recovery - Original: %s, New: %s", originalMCAHash, newMCAHash)
				return nil
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

			// Cleanup: Remove defaultConfig from CMA
			By("Cleanup: Clearing CMA defaultConfig")
			err = utils.ClearClusterManagementAddOnDefaultConfig(testOptions)
			Expect(err).NotTo(HaveOccurred())

			// Cleanup: Delete test config
			By("Cleanup: Deleting test AddOnDeploymentConfig")
			err = utils.DeleteAddOnDeploymentConfig(testOptions, testConfigName, MCO_NAMESPACE)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Observability: Upgrade test: Observability controller works without defaultConfig (backward compatibility)", func() {
			By("Verifying ClusterManagementAddOn exists")
			cma, err := dynClient.Resource(utils.NewMCOClusterManagementAddonsGVR()).
				Get(context.TODO(), "observability-controller", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying lifecycle annotation is not present (removed during upgrade)")
			annotations := cma.Object["metadata"].(map[string]interface{})["annotations"]
			if annotations != nil {
				annotationsMap := annotations.(map[string]interface{})
				_, hasLifecycleAnnotation := annotationsMap["addon.open-cluster-management.io/lifecycle"]
				Expect(hasLifecycleAnnotation).To(BeFalse(), "Lifecycle annotation should have been removed during upgrade")
			}

			By("Verifying no defaultConfig is set (backward compatible)")
			spec := cma.Object["spec"].(map[string]interface{})
			supportedConfigs := spec["supportedConfigs"].([]interface{})
			for _, cfg := range supportedConfigs {
				cfgMap := cfg.(map[string]interface{})
				cgr := cfgMap["configGroupResource"].(map[string]interface{})
				if cgr["group"] == "addon.open-cluster-management.io" && cgr["resource"] == "addondeploymentconfigs" {
					_, hasDefaultConfig := cfgMap["defaultConfig"]
					Expect(hasDefaultConfig).To(BeFalse(), "DefaultConfig should not be set for backward compatibility")
					break
				}
			}

			By("Verifying observability-controller addon is operational without defaultConfig")
			managedClusterName := utils.GetManagedClusterName(testOptions)
			Eventually(func() error {
				return utils.CheckAllOBAsEnabled(testOptions)
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

			// Verify ManagedClusterAddOn exists
			mca, err := dynClient.Resource(utils.NewMCOManagedClusterAddonsGVR()).
				Namespace(managedClusterName).
				Get(context.TODO(), "observability-controller", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(mca).NotTo(BeNil())

			klog.V(1).Info("Observability-controller confirmed working without defaultConfig (backward compatible)")
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

		Eventually(func() error {
			return utils.UpdateObservabilityFromManagedCluster(testOptions, false)
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

		klog.V(1).Infof("managedcluster number is <%d>", len(testOptions.ManagedClusters))
		if len(testOptions.ManagedClusters) >= 1 {
			Eventually(func() bool {
				return true
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(BeTrue())
		}
	})

	Context("RHACM4K-7518: Observability: Disable the Observability by updating managed cluster label [P2][Sev2][Observability]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore (addon/g1) -", func() {
		It("[Stable] Modifying managedcluster cr to disable observability", func() {
			Eventually(func() error {
				return utils.UpdateObservabilityFromManagedCluster(testOptions, false)
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

			klog.V(1).Infof("managedcluster number is <%d>", len(testOptions.ManagedClusters))
			if len(testOptions.ManagedClusters) > 0 {
				By("Waiting for MCO addon components scales to 0")
				Eventually(func() bool {
					err, obaNS := utils.GetNamespace(testOptions, false, MCO_ADDON_NAMESPACE)
					if err == nil && obaNS == nil {
						return true
					}
					return false
				}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(BeTrue())
			}
		})

		It("[Stable] Remove disable observability label from the managed cluster", func() {
			Eventually(func() error {
				return utils.UpdateObservabilityFromManagedCluster(testOptions, true)
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

			if len(testOptions.ManagedClusters) > 0 {
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
			}
		})
	},
	)

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
