// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
)

const (
	mcoaManagerDeploymentName              = "multicluster-observability-addon-manager"
	platformPrometheusAgentStatefulSetName = "prom-agent-platform-metrics-collector"
	uwlPrometheusAgentStatefulSetName      = "prom-agent-user-workload-metrics-collector"
	oboPrometheusOperatorDeploymentName    = "obo-prometheus-operator"
	metricsCollectorDeploymentName         = "metrics-collector-deployment"
	mcoaAddonName                          = "multicluster-observability-addon"
	globalPlacementName                    = "global"
)

var _ = Describe("Observability Addon (MCOA)", Ordered, func() {
	var managedClusters []utils.Cluster
	var managedClustersWithHub []utils.Cluster
	var ocpClusters []utils.Cluster
	var accessibleOCPClusters []utils.Cluster
	// var ocpClustersWithHub []utils.Cluster

	BeforeAll(func() {
		By("Getting available managed clusters")
		var err error
		managedClusters, err = utils.GetAvailableManagedClustersAsClusters(testOptions)
		Expect(err).ToNot(HaveOccurred())
		clusterNames := []string{}
		for _, cluster := range managedClusters {
			clusterNames = append(clusterNames, cluster.Name)
		}
		managedClustersWithHub = append(managedClusters, testOptions.HubCluster)
		By(fmt.Sprintf("Running tests against the following managed clusters (excluding the hub): %v", clusterNames))

		By("Getting available OCP managed clusters")
		ocpClusters, err = utils.GetOCPClusters(testOptions)
		Expect(err).ToNot(HaveOccurred())
		ocpClusterNames := []string{}
		for _, cluster := range ocpClusters {
			ocpClusterNames = append(ocpClusterNames, cluster.Name)
		}
		// ocpClustersWithHub = append(ocpClusters, testOptions.HubCluster)
		By(fmt.Sprintf("Running tests against the following OCP managed clusters: %v", ocpClusterNames))

		By("Getting OCP managed clusters with API access")
		accessibleOCPClusters, err = utils.GetOCPClustersWithAPIAccess(testOptions)
		Expect(err).ToNot(HaveOccurred())
		accessibleOCPClusterNames := []string{}
		for _, cluster := range accessibleOCPClusters {
			accessibleOCPClusterNames = append(accessibleOCPClusterNames, cluster.Name)
		}
		accessibleOCPClusters = append(accessibleOCPClusters, testOptions.HubCluster)
		accessibleOCPClusterNames = append(accessibleOCPClusterNames, testOptions.HubCluster.Name)
		By(fmt.Sprintf("Running tests against the following OCP managed clusters with API access: %v", accessibleOCPClusterNames))

		By("Disabling MCOA", func() {
			Expect(utils.SetMCOACapabilities(testOptions, false, false)).NotTo(HaveOccurred())
			utils.CheckStatefulSetAvailabilityOnClusters(managedClustersWithHub, platformPrometheusAgentStatefulSetName, utils.MCO_AGENT_ADDON_NAMESPACE, false)
		})
		By("Deleting COO subscription if it exists and CRDs", func() {
			utils.DeleteCOOSubscription(accessibleOCPClusters)
			Expect(utils.DeleteMonitoringCRDs(testOptions, accessibleOCPClusters)).NotTo(HaveOccurred())
		})
	})

	Context(
		"when only platform metrics are enabled [P1][Sev1][Observability][Stable]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release @pre-upgrade (mcoa/g0)",
		func() {
			BeforeAll(func() {
				By("Enabling only platform metrics for MCOA", func() {
					Expect(utils.SetMCOACapabilities(testOptions, true, false)).NotTo(HaveOccurred())
				})
				By("Configuring the platform scrape interval to 30s", func() {
					Eventually(func() error {
						return utils.UpdatePrometheusAgentScrapeInterval(testOptions, "platform-metrics-collector", "30s")
					}, 120, 2).Should(Not(HaveOccurred()))
				})
			})

			It("should deploy the correct agents", func() {
				By("The platform prometheus agent should be running", func() {
					utils.CheckStatefulSetAvailabilityOnClusters(managedClustersWithHub, platformPrometheusAgentStatefulSetName, utils.MCO_AGENT_ADDON_NAMESPACE, true)
				})
				By("The user workload prometheus agent should NOT be running", func() {
					utils.CheckStatefulSetAvailabilityOnClusters(managedClustersWithHub, uwlPrometheusAgentStatefulSetName, utils.MCO_AGENT_ADDON_NAMESPACE, false)
				})
				By("The addon status should be Available", func() {
					utils.CheckManagedClusterAddonStatus(testOptions, mcoaAddonName)
				})
			})
			It("should forward default platform metrics to the hub", func() {
				metricName := "up"
				Eventually(func() error {
					res, err := utils.QueryGrafana(testOptions, metricName)
					if err != nil {
						return err
					}
					if len(res.Data.Result) == 0 {
						return fmt.Errorf("No results found for metric %q", metricName)
					}
					return res.CheckMetricFromAllClusters(managedClustersWithHub)
				}, 300, 2).Should(Not(HaveOccurred()))
			})

			It("should allow updating the metrics list", SpecTimeout(10*time.Minute), func(ctx context.Context) {
				customMetricName := "go_memstats_alloc_bytes"
				customScrapeConfigCR := "test-custom-metric"
				By("Creating a new ScrapeConfig for a custom metric", func() {
					Expect(utils.CreateScrapeConfig(testOptions, customScrapeConfigCR, "platform-metrics-collector", []string{fmt.Sprintf(`{__name__="%s"}`, customMetricName)})).NotTo(HaveOccurred())
					Expect(
						utils.AddConfigToPlacementInClusterManagementAddon(
							testOptions,
							utils.MCOA_CLUSTER_MANAGEMENT_ADDON_NAME,
							globalPlacementName,
							utils.NewScrapeConfigGVR(),
							customScrapeConfigCR,
							utils.MCO_NAMESPACE,
						),
					).NotTo(HaveOccurred())
				})

				By("Verifying the custom metric is forwarded to the hub", func() {
					Eventually(func() error {
						res, err := utils.QueryGrafana(testOptions, customMetricName)
						if err != nil {
							return err
						}
						if len(res.Data.Result) == 0 {
							return fmt.Errorf("No results found for metric %q", customMetricName)
						}
						return res.CheckMetricFromAllClusters(managedClustersWithHub)
					}, 600, 2).Should(Not(HaveOccurred()))
				})

				By("Deleting the custom ScrapeConfig", func() {
					Expect(
						utils.RemoveConfigFromPlacementInClusterManagementAddon(
							testOptions,
							utils.MCOA_CLUSTER_MANAGEMENT_ADDON_NAME,
							globalPlacementName,
							utils.NewScrapeConfigGVR(),
							customScrapeConfigCR,
							utils.MCO_NAMESPACE,
						),
					).NotTo(HaveOccurred())
					Expect(utils.DeleteScrapeConfig(testOptions, customScrapeConfigCR)).NotTo(HaveOccurred())
				})
			})

			It("should allow adding prometheus rules", func() {
				ruleName := "test-prom-rule"
				ruleMetricName := "test_platform_metric_from_rule"
				scrapeConfigName := "test-prom-rule-metric"
				By("Creating a new PrometheusRule on the hub", func() {
					Expect(utils.CreatePrometheusRule(testOptions, ruleName, utils.MCO_NAMESPACE, "platform-metrics-collector", ruleMetricName, "")).NotTo(HaveOccurred())
					Expect(
						utils.AddConfigToPlacementInClusterManagementAddon(
							testOptions,
							utils.MCOA_CLUSTER_MANAGEMENT_ADDON_NAME,
							globalPlacementName,
							utils.NewPrometheusRuleGVR(),
							ruleName,
							utils.MCO_NAMESPACE,
						),
					).NotTo(HaveOccurred())
				})

				By("Creating a new ScrapeConfig for the rule's metric", func() {
					Expect(utils.CreateScrapeConfig(testOptions, scrapeConfigName, "platform-metrics-collector", []string{fmt.Sprintf(`{__name__="%s"}`, ruleMetricName)})).NotTo(HaveOccurred())
					Expect(
						utils.AddConfigToPlacementInClusterManagementAddon(
							testOptions,
							utils.MCOA_CLUSTER_MANAGEMENT_ADDON_NAME,
							globalPlacementName,
							utils.NewScrapeConfigGVR(),
							scrapeConfigName,
							utils.MCO_NAMESPACE,
						),
					).NotTo(HaveOccurred())
				})

				By("Verifying the rule's metric is forwarded to the hub", func() {
					Eventually(func() error {
						res, err := utils.QueryGrafana(testOptions, ruleMetricName)
						if err != nil {
							return err
						}
						if len(res.Data.Result) == 0 {
							return fmt.Errorf("No results found for metric %q", ruleMetricName)
						}
						return res.CheckMetricFromAllClusters(managedClustersWithHub)
					}, 300, 2).Should(Not(HaveOccurred()))
				})
				By("Deleting the PrometheusRule", func() {
					Expect(
						utils.RemoveConfigFromPlacementInClusterManagementAddon(
							testOptions,
							utils.MCOA_CLUSTER_MANAGEMENT_ADDON_NAME,
							globalPlacementName,
							utils.NewPrometheusRuleGVR(),
							ruleName,
							utils.MCO_NAMESPACE,
						),
					).NotTo(HaveOccurred())
					Expect(utils.DeletePrometheusRule(testOptions, ruleName, utils.MCO_NAMESPACE)).NotTo(HaveOccurred())
				})

				By("Deleting the custom ScrapeConfig", func() {
					Expect(
						utils.RemoveConfigFromPlacementInClusterManagementAddon(
							testOptions,
							utils.MCOA_CLUSTER_MANAGEMENT_ADDON_NAME,
							globalPlacementName,
							utils.NewScrapeConfigGVR(),
							scrapeConfigName,
							utils.MCO_NAMESPACE,
						),
					).NotTo(HaveOccurred())
					Expect(utils.DeleteScrapeConfig(testOptions, scrapeConfigName)).NotTo(HaveOccurred())
				})
			})
		},
	)

	Context(
		"when platform and user workload metrics are enabled [P1][Sev1][Observability][Stable]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release @pre-upgrade (mcoa/g0)",
		func() {
			BeforeAll(func() {
				By("Enabling user workload monitoring on all openshift managed clusters", func() {
					Expect(utils.EnableUWLMonitoringOnManagedClusters(testOptions, accessibleOCPClusters)).NotTo(HaveOccurred())
				})
			})

			It("should deploy the user workload agent only when required", SpecTimeout(10*time.Minute), func(ctx context.Context) {
				By("The user workload prometheus agent should NOT be running", func() {
					utils.CheckStatefulSetAvailabilityOnClusters(accessibleOCPClusters, uwlPrometheusAgentStatefulSetName, utils.MCO_AGENT_ADDON_NAMESPACE, false)
				})
				By("Enabling platform and user workload metrics for MCOA", func() {
					Expect(utils.SetMCOACapabilities(testOptions, true, true)).NotTo(HaveOccurred())
				})
				By("Configuring the user workload scrape interval to 30s", func() {
					Eventually(func() error {
						return utils.UpdatePrometheusAgentScrapeInterval(testOptions, "user-workload-metrics-collector", "30s")
					}, 600, 5).Should(Not(HaveOccurred()))
				})

				By("The user workload prometheus agent should be running", func() {
					utils.CheckStatefulSetAvailabilityOnClusters(accessibleOCPClusters, uwlPrometheusAgentStatefulSetName, utils.MCO_AGENT_ADDON_NAMESPACE, true)
				})
			})

			It("should allow collecting metrics for user workloads", SpecTimeout(10*time.Minute), func(ctx context.Context) {
				ruleName := "test-uwl-prom-rule"
				ruleMetricName := "test_uwl_metric_from_rule"
				scrapeConfigName := "test-uwl-prom-rule-metric"

				By("Creating a new PrometheusRule on the hub", func() {
					Expect(utils.CreatePrometheusRule(testOptions, ruleName, utils.MCO_NAMESPACE, "user-workload-metrics-collector", ruleMetricName, "default")).NotTo(HaveOccurred())
					Expect(
						utils.AddConfigToPlacementInClusterManagementAddon(
							testOptions,
							utils.MCOA_CLUSTER_MANAGEMENT_ADDON_NAME,
							globalPlacementName,
							utils.NewPrometheusRuleGVR(),
							ruleName,
							utils.MCO_NAMESPACE,
						),
					).NotTo(HaveOccurred())
				})

				By("Creating a new ScrapeConfig for the rule's metric", func() {
					Expect(utils.CreateScrapeConfig(testOptions, scrapeConfigName, "user-workload-metrics-collector", []string{fmt.Sprintf(`{__name__="%s"}`, ruleMetricName)})).NotTo(HaveOccurred())
					Expect(
						utils.AddConfigToPlacementInClusterManagementAddon(
							testOptions,
							utils.MCOA_CLUSTER_MANAGEMENT_ADDON_NAME,
							globalPlacementName,
							utils.NewScrapeConfigGVR(),
							scrapeConfigName,
							utils.MCO_NAMESPACE,
						),
					).NotTo(HaveOccurred())
				})

				By("The user workload prometheus agent should be running", func() {
					utils.CheckStatefulSetAvailabilityOnClusters(accessibleOCPClusters, uwlPrometheusAgentStatefulSetName, utils.MCO_AGENT_ADDON_NAMESPACE, true)
				})

				By("Verifying the rule's metric is forwarded to the hub", func() {
					Eventually(func() error {
						res, err := utils.QueryGrafana(testOptions, ruleMetricName)
						if err != nil {
							return err
						}
						if len(res.Data.Result) == 0 {
							return fmt.Errorf("No results found for metric %q", ruleMetricName)
						}
						return res.CheckMetricFromAllClusters(accessibleOCPClusters)
					}, 300, 2).Should(Not(HaveOccurred()))
				})

				By("Deleting the custom ScrapeConfig", func() {
					Expect(
						utils.RemoveConfigFromPlacementInClusterManagementAddon(
							testOptions,
							utils.MCOA_CLUSTER_MANAGEMENT_ADDON_NAME,
							globalPlacementName,
							utils.NewScrapeConfigGVR(),
							scrapeConfigName,
							utils.MCO_NAMESPACE,
						),
					).NotTo(HaveOccurred())
					Expect(utils.DeleteScrapeConfig(testOptions, scrapeConfigName)).NotTo(HaveOccurred())
				})

				By("Deleting the PrometheusRule", func() {
					Expect(
						utils.RemoveConfigFromPlacementInClusterManagementAddon(
							testOptions,
							utils.MCOA_CLUSTER_MANAGEMENT_ADDON_NAME,
							globalPlacementName,
							utils.NewPrometheusRuleGVR(),
							ruleName,
							utils.MCO_NAMESPACE,
						),
					).NotTo(HaveOccurred())
					Expect(utils.DeletePrometheusRule(testOptions, ruleName, utils.MCO_NAMESPACE)).NotTo(HaveOccurred())
				})
			})
		},
	)

	Context("with Cluster Observability Operator (COO) installed [P1][Sev1][Observability][Stable]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release @pre-upgrade (mcoa/g0)", func() {
		// We retrict this test to the hub for simplification purpose. The processing is similar for the spokes.
		onlyTheHub := []utils.Cluster{testOptions.HubCluster}

		BeforeAll(func() {
			By("Disabling MCOA", func() {
				// Disable the legacy addon to avoid entering bad states that mey happen when disabling and re-enabling too quickly
				utils.ModifyMCOAddonSpecMetrics(testOptions, false)
				// Then disable MCOA
				Expect(utils.SetMCOACapabilities(testOptions, false, false)).NotTo(HaveOccurred())

				By("Waiting for 30 seconds to enable resources deletion")
				time.Sleep(30 * time.Second)
			})
		})

		It("should ingest metrics from hub and spoke clusters", SpecTimeout(15*time.Minute), func(ctx context.Context) {
			By("Installing COO on the hub", func() {
				Expect(utils.CreateCOOSubscription(onlyTheHub)).NotTo(HaveOccurred())
				// Wait for COO to be running
				utils.CheckCOODeployment(onlyTheHub)
			})

			By("Enabling MCOA", func() {
				Expect(utils.SetMCOACapabilities(testOptions, true, true)).NotTo(HaveOccurred())
			})

			By("MCOA components should be running", func() {
				utils.CheckStatefulSetAvailabilityOnClusters(onlyTheHub, platformPrometheusAgentStatefulSetName, utils.MCO_AGENT_ADDON_NAMESPACE, true)
				utils.CheckStatefulSetAvailabilityOnClusters(onlyTheHub, uwlPrometheusAgentStatefulSetName, utils.MCO_AGENT_ADDON_NAMESPACE, true)
				utils.CheckManagedClusterAddonStatus(testOptions, mcoaAddonName)
			})

			By("Checking for obo-prometheus-operator deployment on managed clusters", func() {
				// It should use the COO operator, we check that the prometheus operator is not deployed
				utils.CheckDeploymentAvailabilityOnClusters(onlyTheHub, oboPrometheusOperatorDeploymentName, utils.MCO_AGENT_ADDON_NAMESPACE, false)
			})
		})

		AfterAll(func() {
			By("Disabling MCOA", func() {
				Expect(utils.SetMCOACapabilities(testOptions, false, false)).NotTo(HaveOccurred())
				utils.CheckStatefulSetAvailabilityOnClusters(onlyTheHub, platformPrometheusAgentStatefulSetName, utils.MCO_AGENT_ADDON_NAMESPACE, false)
				utils.CheckDeploymentAvailability(testOptions.HubCluster, mcoaManagerDeploymentName, utils.MCO_NAMESPACE, false)
			})
			By("Deleting COO subscription", func() {
				utils.DeleteCOOSubscription(onlyTheHub)
				Expect(utils.DeleteMonitoringCRDs(testOptions, onlyTheHub)).NotTo(HaveOccurred())
			})
		})
	})

	JustAfterEach(func() {
		Expect(utils.IntegrityChecking(testOptions)).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			utils.LogFailingTestStandardDebugInfo(testOptions)
		}
		testFailed = testFailed || CurrentSpecReport().Failed()
	})

	AfterAll(func() {
		By("Disabling MCOA", func() {
			Expect(utils.SetMCOACapabilities(testOptions, false, false)).NotTo(HaveOccurred())
			utils.CheckStatefulSetAvailabilityOnClusters(managedClustersWithHub, platformPrometheusAgentStatefulSetName, utils.MCO_AGENT_ADDON_NAMESPACE, false)
			utils.CheckDeploymentAvailability(testOptions.HubCluster, mcoaManagerDeploymentName, utils.MCO_NAMESPACE, false)
			// Re-enable the legacy collector
			utils.ModifyMCOAddonSpecMetrics(testOptions, true)

			By("Waiting for 30 seconds to make sure the registration controller correctly takes into account the changes")
			time.Sleep(30 * time.Second)

			// Wait for the metrics collector to be up to avoid race conditions with other tests setups
			utils.CheckDeploymentAvailability(testOptions.HubCluster, metricsCollectorDeploymentName, utils.MCO_NAMESPACE, true)
			utils.CheckDeploymentAvailabilityOnClusters(managedClusters, metricsCollectorDeploymentName, utils.MCO_ADDON_NAMESPACE, true)
		})
	})
})
