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

		By("Enabling user workload monitoring on all openshift managed clusters", func() {
			Expect(utils.EnableUWLMonitoringOnManagedClusters(testOptions)).NotTo(HaveOccurred())
		})
		By("Disabling MCOA", func() {
			Expect(utils.SetMCOACapabilities(testOptions, false, false)).NotTo(HaveOccurred())
			utils.CheckStatefulSetAvailabilityOnClusters(managedClustersWithHub, platformPrometheusAgentStatefulSetName, utils.MCO_AGENT_ADDON_NAMESPACE, false)
		})
	})

	Context("when enabling and disabling platform collection", func() {
		BeforeAll(func() {
			By("The metrics collector should be running", func() {
				utils.CheckDeploymentAvailability(testOptions.HubCluster, metricsCollectorDeploymentName, utils.MCO_NAMESPACE, true)
				utils.CheckDeploymentAvailabilityOnClusters(managedClusters, metricsCollectorDeploymentName, utils.MCO_ADDON_NAMESPACE, true)
			})
		})

		It("should be deployed and then cleaned up successfully", SpecTimeout(15*time.Minute), func(ctx context.Context) {
			By("Enabling MCOA", func() {
				Expect(utils.SetMCOACapabilities(testOptions, true, false)).NotTo(HaveOccurred())
			})
			By("The metrics collector should be deleted", func() {
				utils.CheckDeploymentAvailability(testOptions.HubCluster, metricsCollectorDeploymentName, utils.MCO_NAMESPACE, false)
				utils.CheckDeploymentAvailabilityOnClusters(managedClusters, metricsCollectorDeploymentName, utils.MCO_ADDON_NAMESPACE, false)
			})
			By("MCOA should be available", func() {
				utils.CheckDeploymentAvailability(testOptions.HubCluster, mcoaManagerDeploymentName, utils.MCO_NAMESPACE, true)
				utils.CheckStatefulSetAvailabilityOnClusters(managedClustersWithHub, platformPrometheusAgentStatefulSetName, utils.MCO_AGENT_ADDON_NAMESPACE, true)
				utils.CheckManagedClusterAddonStatus(testOptions, mcoaAddonName)
			})
			By("Disabling MCOA", func() {
				Expect(utils.SetMCOACapabilities(testOptions, false, false)).NotTo(HaveOccurred())
			})
			By("MCOA should be deleted", func() {
				utils.CheckDeploymentAvailability(testOptions.HubCluster, mcoaManagerDeploymentName, utils.MCO_NAMESPACE, false)
				utils.CheckDeploymentAvailabilityOnClusters(managedClustersWithHub, oboPrometheusOperatorDeploymentName, utils.MCO_AGENT_ADDON_NAMESPACE, false)
				utils.CheckStatefulSetAvailabilityOnClusters(managedClustersWithHub, platformPrometheusAgentStatefulSetName, utils.MCO_AGENT_ADDON_NAMESPACE, false)
			})
			By("The metrics collector should be running and forwarding metrics", func() {
				utils.CheckDeploymentAvailability(testOptions.HubCluster, metricsCollectorDeploymentName, utils.MCO_NAMESPACE, true)
				utils.CheckDeploymentAvailabilityOnClusters(managedClusters, metricsCollectorDeploymentName, utils.MCO_ADDON_NAMESPACE, true)
				// We don't check the addon status as it has no managedClusterAddon for the hub
			})
		})
	})

	Context("when only platform metrics are enabled", func() {
		BeforeAll(func() {
			By("Enabling only platform metrics for MCOA", func() {
				Expect(utils.SetMCOACapabilities(testOptions, true, false)).NotTo(HaveOccurred())
			})
			By("Configuring the platform scrape interval to 1 min", func() {
				Eventually(func() error {
					return utils.UpdatePlatformPrometheusAgent(testOptions, "60s")
				}, 30, 2).Should(Not(HaveOccurred()))
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
				return nil
			}, 300, 2).Should(Not(HaveOccurred()))
		})

		It("should allow updating the metrics list", SpecTimeout(10*time.Minute), func(ctx context.Context) {
			customMetricName := "go_memstats_alloc_bytes"
			customScrapeConfigCR := "test-custom-metric"
			By("Creating a new ScrapeConfig for a custom metric", func() {
				Expect(utils.CreateScrapeConfig(testOptions, customScrapeConfigCR, "platform-metrics-collector", []string{fmt.Sprintf(`{__name__="%s"}`, customMetricName)})).NotTo(HaveOccurred())
				Expect(utils.AddConfigToPlacementInClusterManagementAddon(testOptions, utils.MCOA_CLUSTER_MANAGEMENT_ADDON_NAME, globalPlacementName, utils.NewScrapeConfigGVR(), customScrapeConfigCR, utils.MCO_NAMESPACE)).NotTo(HaveOccurred())
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
					return nil
				}, 600, 2).Should(Not(HaveOccurred()))
			})

			By("Deleting the custom ScrapeConfig", func() {
				Expect(utils.DeleteScrapeConfig(testOptions, customScrapeConfigCR)).NotTo(HaveOccurred())
				Expect(utils.RemoveConfigFromPlacementInClusterManagementAddon(testOptions, utils.MCOA_CLUSTER_MANAGEMENT_ADDON_NAME, globalPlacementName, utils.NewScrapeConfigGVR(), customScrapeConfigCR, utils.MCO_NAMESPACE)).NotTo(HaveOccurred())
			})
		})

		It("should allow adding prometheus rules", func() {
			ruleName := "test-prom-rule"
			ruleMetricName := "test_platform_metric_from_rule"
			scrapeConfigName := "test-prom-rule-metric"
			By("Creating a new PrometheusRule on the hub", func() {
				Expect(utils.CreatePrometheusRule(testOptions, ruleName, utils.MCO_NAMESPACE, "platform-metrics-collector", ruleMetricName)).NotTo(HaveOccurred())
				Expect(utils.AddConfigToPlacementInClusterManagementAddon(testOptions, utils.MCOA_CLUSTER_MANAGEMENT_ADDON_NAME, globalPlacementName, utils.NewPrometheusRuleGVR(), ruleName, utils.MCO_NAMESPACE)).NotTo(HaveOccurred())
			})

			By("Creating a new ScrapeConfig for the rule's metric", func() {
				Expect(utils.CreateScrapeConfig(testOptions, scrapeConfigName, "platform-metrics-collector", []string{fmt.Sprintf(`{__name__="%s"}`, ruleMetricName)})).NotTo(HaveOccurred())
				Expect(utils.AddConfigToPlacementInClusterManagementAddon(testOptions, utils.MCOA_CLUSTER_MANAGEMENT_ADDON_NAME, globalPlacementName, utils.NewScrapeConfigGVR(), scrapeConfigName, utils.MCO_NAMESPACE)).NotTo(HaveOccurred())
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
					return nil
				}, 300, 2).Should(Not(HaveOccurred()))
			})

			By("Deleting the PrometheusRule", func() {
				Expect(utils.DeletePrometheusRule(testOptions, ruleName, utils.MCO_NAMESPACE)).NotTo(HaveOccurred())
				Expect(utils.RemoveConfigFromPlacementInClusterManagementAddon(testOptions, utils.MCOA_CLUSTER_MANAGEMENT_ADDON_NAME, globalPlacementName, utils.NewPrometheusRuleGVR(), ruleName, utils.MCO_NAMESPACE)).NotTo(HaveOccurred())
			})

			By("Deleting the custom ScrapeConfig", func() {
				Expect(utils.DeleteScrapeConfig(testOptions, scrapeConfigName)).NotTo(HaveOccurred())
				Expect(utils.RemoveConfigFromPlacementInClusterManagementAddon(testOptions, utils.MCOA_CLUSTER_MANAGEMENT_ADDON_NAME, globalPlacementName, utils.NewScrapeConfigGVR(), scrapeConfigName, utils.MCO_NAMESPACE)).NotTo(HaveOccurred())
			})
		})

	})

	Context("when platform and user workload metrics are enabled", func() {
		uwlCustomMetricName := "go_threads"
		uwlCustomScrapeConfigCR := "test-uwl-custom-metric"

		BeforeAll(func() {
			By("Enabling platform and user workload metrics for MCOA", func() {
				Expect(utils.SetMCOACapabilities(testOptions, true, true)).NotTo(HaveOccurred())
			})
			By("Configuring the platform scrape interval to 1 min", func() {
				Eventually(func() error {
					return utils.UpdatePlatformPrometheusAgent(testOptions, "60s")
				}, 30, 2).Should(Not(HaveOccurred()))
			})
		})

		It("should deploy the user workload agent only when required", func() {
			By("The user workload prometheus agent should NOT be running", func() {
				utils.CheckStatefulSetAvailabilityOnClusters(managedClustersWithHub, uwlPrometheusAgentStatefulSetName, utils.MCO_AGENT_ADDON_NAMESPACE, false)
			})

			By("Creating a new ScrapeConfig for a custom UWL metric", func() {
				Expect(utils.CreateScrapeConfig(testOptions, uwlCustomScrapeConfigCR, "user-workload-metrics-collector", []string{fmt.Sprintf(`{__name__="%s"}`, uwlCustomMetricName)})).NotTo(HaveOccurred())
			})

			By("The user workload prometheus agent should be running", func() {
				utils.CheckStatefulSetAvailabilityOnClusters(managedClustersWithHub, uwlPrometheusAgentStatefulSetName, utils.MCO_AGENT_ADDON_NAMESPACE, true)
			})

			By("Verifying the custom UWL metric is forwarded to the hub", func() {
				Eventually(func() error {
					res, err := utils.QueryGrafana(testOptions, uwlCustomMetricName)
					if err != nil {
						return err
					}
					if len(res.Data.Result) == 0 {
						return fmt.Errorf("No results found for metric %q", uwlCustomMetricName)
					}
					return nil
				}, 300, 1).Should(Not(HaveOccurred()))
			})

			By("Deleting the custom UWL ScrapeConfig", func() {
				Expect(utils.DeleteScrapeConfig(testOptions, uwlCustomScrapeConfigCR)).NotTo(HaveOccurred())
			})

			By("The user workload prometheus agent should be terminated", func() {
				utils.CheckStatefulSetAvailabilityOnClusters(managedClustersWithHub, uwlPrometheusAgentStatefulSetName, utils.MCO_AGENT_ADDON_NAMESPACE, false)
			})
		})

		It("should allow adding prometheus rules for user workloads", func() {
			ruleName := "test-uwl-prom-rule"
			ruleMetricName := "test_uwl_metric_from_rule"
			scrapeConfigName := "test-uwl-prom-rule-metric"
			By("Creating a new UWL PrometheusRule on the hub", func() {
				Expect(utils.CreatePrometheusRule(testOptions, ruleName, utils.MCO_NAMESPACE, "user-workload-metrics-collector", ruleMetricName)).NotTo(HaveOccurred())
				Expect(utils.AddConfigToPlacementInClusterManagementAddon(testOptions, utils.MCOA_CLUSTER_MANAGEMENT_ADDON_NAME, globalPlacementName, utils.NewPrometheusRuleGVR(), ruleName, utils.MCO_NAMESPACE)).NotTo(HaveOccurred())
			})

			By("Creating a new ScrapeConfig for the rule's metric", func() {
				Expect(utils.CreateScrapeConfig(testOptions, scrapeConfigName, "user-workload-metrics-collector", []string{fmt.Sprintf(`{__name__="%s"}`, ruleMetricName)})).NotTo(HaveOccurred())
				Expect(utils.AddConfigToPlacementInClusterManagementAddon(testOptions, utils.MCOA_CLUSTER_MANAGEMENT_ADDON_NAME, globalPlacementName, utils.NewScrapeConfigGVR(), scrapeConfigName, utils.MCO_NAMESPACE)).NotTo(HaveOccurred())
			})

			By("The user workload prometheus agent should be running", func() {
				utils.CheckStatefulSetAvailabilityOnClusters(managedClustersWithHub, uwlPrometheusAgentStatefulSetName, utils.MCO_AGENT_ADDON_NAMESPACE, true)
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
					return nil
				}, 300, 1).Should(Not(HaveOccurred()))
			})

			By("Deleting the PrometheusRule", func() {
				Expect(utils.DeletePrometheusRule(testOptions, ruleName, utils.MCO_NAMESPACE)).NotTo(HaveOccurred())
				Expect(utils.RemoveConfigFromPlacementInClusterManagementAddon(testOptions, utils.MCOA_CLUSTER_MANAGEMENT_ADDON_NAME, globalPlacementName, utils.NewPrometheusRuleGVR(), ruleName, utils.MCO_NAMESPACE)).NotTo(HaveOccurred())
			})

			By("Deleting the custom ScrapeConfig", func() {
				Expect(utils.DeleteScrapeConfig(testOptions, scrapeConfigName)).NotTo(HaveOccurred())
				Expect(utils.RemoveConfigFromPlacementInClusterManagementAddon(testOptions, utils.MCOA_CLUSTER_MANAGEMENT_ADDON_NAME, globalPlacementName, utils.NewScrapeConfigGVR(), scrapeConfigName, utils.MCO_NAMESPACE)).NotTo(HaveOccurred())
			})

			By("The user workload prometheus agent should be terminated", func() {
				utils.CheckStatefulSetAvailabilityOnClusters(managedClustersWithHub, uwlPrometheusAgentStatefulSetName, utils.MCO_AGENT_ADDON_NAMESPACE, false)
			})
		})
	})

	Context("with Cluster Observability Operator (COO) installed", func() {
		BeforeAll(func() {
			if len(ocpClusters) == 0 {
				Skip("Skipping due to no ocp managed clusters found outside of the hub itself")
			}
			By("Disabling MCOA", func() {
				Expect(utils.SetMCOACapabilities(testOptions, false, false)).NotTo(HaveOccurred())
			})
			By("Deleting COO subscription if it exists", func() {
				utils.DeleteCOOSubscription(ocpClusters)
			})
			By("Deleting monitoring CRDs", func() {
				Expect(utils.DeleteMonitoringCRDs(testOptions, ocpClusters)).NotTo(HaveOccurred())
			})
		})

		It("should ingest metrics from hub and spoke clusters", SpecTimeout(15*time.Minute), func(ctx context.Context) {
			By("Installing COO on all clusters", func() {
				Expect(utils.CreateCOOSubscription(ocpClusters)).NotTo(HaveOccurred())
			})

			By("Waiting for COO to be installed", func() {
				utils.CheckCOODeployment(ocpClusters)
			})

			By("Enabling MCOA", func() {
				Expect(utils.SetMCOACapabilities(testOptions, true, true)).NotTo(HaveOccurred())
			})

			By("MCOA components should be running", func() {
				utils.CheckStatefulSetAvailabilityOnClusters(ocpClusters, platformPrometheusAgentStatefulSetName, utils.MCO_AGENT_ADDON_NAMESPACE, true)
				utils.CheckStatefulSetAvailabilityOnClusters(ocpClusters, uwlPrometheusAgentStatefulSetName, utils.MCO_AGENT_ADDON_NAMESPACE, true)
				utils.CheckManagedClusterAddonStatus(testOptions, mcoaAddonName)
			})

			By("Checking for obo-prometheus-operator deployment on managed clusters", func() {
				// It should use the COO operator, we check that the prometheus operator is not deployed
				utils.CheckDeploymentAvailabilityOnClusters(ocpClusters, oboPrometheusOperatorDeploymentName, utils.MCO_AGENT_ADDON_NAMESPACE, false)
			})
		})

		AfterAll(func() {
			By("Deleting COO subscription", func() {
				utils.DeleteCOOSubscription(ocpClusters)
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
		})
		By("The MCOA components should be deleted", func() {
			utils.CheckStatefulSetAvailabilityOnClusters(managedClustersWithHub, platformPrometheusAgentStatefulSetName, utils.MCO_AGENT_ADDON_NAMESPACE, false)
			utils.CheckDeploymentAvailability(testOptions.HubCluster, mcoaManagerDeploymentName, utils.MCO_NAMESPACE, false)
		})
	})
})
