// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/mcoa"
	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
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

			It("should configure hub Thanos components with the correct CLI arguments", func() {
				By("Checking Thanos Receive arguments for out-of-order flag", func() {
					Eventually(func() error {
						stsInfo, err := utils.GetStatefulSet(testOptions, true, "observability-thanos-receive-default", utils.MCO_NAMESPACE)
						if err != nil {
							return err
						}
						args := stsInfo.Spec.Template.Spec.Containers[0].Args
						if !slices.Contains(args, "--tsdb.out-of-order.time-window=1h") {
							return fmt.Errorf("expected out-of-order flag not found in thanos-receive args: %v", args)
						}
						return nil
					}, 60, 2).Should(Not(HaveOccurred()))
				})

				By("Checking Thanos Compact arguments for vertical-compaction flag", func() {
					Eventually(func() error {
						stsInfo, err := utils.GetStatefulSet(testOptions, true, "observability-thanos-compact", utils.MCO_NAMESPACE)
						if err != nil {
							return err
						}
						args := stsInfo.Spec.Template.Spec.Containers[0].Args
						if !slices.Contains(args, "--compact.enable-vertical-compaction") {
							return fmt.Errorf("expected vertical-compaction flag not found in thanos-compact args: %v", args)
						}
						return nil
					}, 60, 2).Should(Not(HaveOccurred()))
				})
			})
			It("should allow updating the metrics list", SpecTimeout(10*time.Minute), func(ctx context.Context) {
				customMetricName := "go_memstats_alloc_bytes"
				customScrapeConfigCR := "test-custom-metric"
				By("Creating a new ScrapeConfig for a custom metric", func() {
					Expect(utils.CreateScrapeConfig(testOptions, customScrapeConfigCR, "platform-metrics-collector", []string{fmt.Sprintf(`{__name__="%s"}`, customMetricName)})).NotTo(HaveOccurred())
					Expect(
						utils.AddConfigToPlacementInClusterManagementAddon(
							ctx,
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
							ctx,
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

			It("should allow adding prometheus rules", func(ctx context.Context) {
				ruleName := "test-prom-rule"
				ruleMetricName := "test_platform_metric_from_rule"
				scrapeConfigName := "test-prom-rule-metric"
				By("Creating a new PrometheusRule on the hub", func() {
					Expect(utils.CreatePrometheusRule(testOptions, ruleName, utils.MCO_NAMESPACE, "platform-metrics-collector", ruleMetricName, "")).NotTo(HaveOccurred())
					Expect(
						utils.AddConfigToPlacementInClusterManagementAddon(
							ctx,
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
							ctx,
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
							ctx,
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
							ctx,
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
							ctx,
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
							ctx,
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
							ctx,
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
							ctx,
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

	// Context("with Cluster Observability Operator (COO) installed [P1][Sev1][Observability][Stable]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release @pre-upgrade (mcoa/g0)", func() {
	// 	// We retrict this test to the hub for simplification purpose. The processing is similar for the spokes.
	// 	onlyTheHub := []utils.Cluster{testOptions.HubCluster}

	// 	BeforeAll(func() {
	// 		By("Disabling MCOA", func() {
	// 			Expect(utils.SetMCOACapabilities(testOptions, false, false)).NotTo(HaveOccurred())

	// 			By("Waiting for 1 minute to make sure the registration controller correctly takes into account the changes")
	// 			time.Sleep(60 * time.Second)

	// 			// Wait for the metrics collector to be running
	// 			utils.CheckDeploymentAvailability(testOptions.HubCluster, metricsCollectorDeploymentName, utils.MCO_NAMESPACE, true)
	// 			utils.CheckDeploymentAvailabilityOnClusters(managedClusters, metricsCollectorDeploymentName, utils.MCO_ADDON_NAMESPACE, true)
	// 			Expect(utils.CheckAllOBAsEnabled(testOptions)).NotTo(HaveOccurred())
	// 		})
	// 	})

	// 	It("should ingest metrics from hub and spoke clusters", SpecTimeout(15*time.Minute), func(ctx context.Context) {
	// 		By("Installing COO on the hub", func() {
	// 			Expect(utils.CreateCOOSubscription(onlyTheHub)).NotTo(HaveOccurred())
	// 			// Wait for COO to be running
	// 			utils.CheckCOODeployment(onlyTheHub)
	// 		})

	// 		By("Enabling MCOA", func() {
	// 			Expect(utils.SetMCOACapabilities(testOptions, true, true)).NotTo(HaveOccurred())
	// 		})

	// 		By("MCOA components should be running", func() {
	// 			utils.CheckStatefulSetAvailabilityOnClusters(onlyTheHub, platformPrometheusAgentStatefulSetName, utils.MCO_AGENT_ADDON_NAMESPACE, true)
	// 			utils.CheckStatefulSetAvailabilityOnClusters(onlyTheHub, uwlPrometheusAgentStatefulSetName, utils.MCO_AGENT_ADDON_NAMESPACE, true)
	// 			utils.CheckManagedClusterAddonStatus(testOptions, mcoaAddonName)
	// 		})

	// 		By("Checking for obo-prometheus-operator deployment on managed clusters", func() {
	// 			// It should use the COO operator, we check that the prometheus operator is not deployed
	// 			utils.CheckDeploymentAvailabilityOnClusters(onlyTheHub, oboPrometheusOperatorDeploymentName, utils.MCO_AGENT_ADDON_NAMESPACE, false)
	// 		})
	// 	})

	// 	AfterAll(func() {
	// 		By("Disabling MCOA", func() {
	// 			Expect(utils.SetMCOACapabilities(testOptions, false, false)).NotTo(HaveOccurred())
	// 			utils.CheckStatefulSetAvailabilityOnClusters(onlyTheHub, platformPrometheusAgentStatefulSetName, utils.MCO_AGENT_ADDON_NAMESPACE, false)
	// 			utils.CheckDeploymentAvailability(testOptions.HubCluster, mcoaManagerDeploymentName, utils.MCO_NAMESPACE, false)
	// 		})
	// 		By("Deleting COO subscription", func() {
	// 			utils.DeleteCOOSubscription(onlyTheHub)
	// 			Expect(utils.DeleteMonitoringCRDs(testOptions, onlyTheHub)).NotTo(HaveOccurred())
	// 		})
	// 	})
	// })

	Context(
		"when alert forwarding is enabled for MCOA [P1][Sev1][Observability][Stable]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release @pre-upgrade (mcoa_alerts/g0)",
		func() {
			var hubClient kubernetes.Interface

			BeforeAll(func() {
				hubClient = utils.NewKubeClient(
					testOptions.HubCluster.ClusterServerURL,
					testOptions.KubeConfig,
					testOptions.HubCluster.KubeContext)

				By("Disabling legacy alert forwarding to avoid interference with MCOA CMO configuration", func() {
					Expect(utils.SetLegacyAlertForwardingDisabled(testOptions, true)).NotTo(HaveOccurred())
				})

				By("Enabling user workload monitoring on all openshift managed clusters", func() {
					Expect(utils.EnableUWLMonitoringOnManagedClusters(testOptions, accessibleOCPClusters)).NotTo(HaveOccurred())
				})
			})

			AfterAll(func() {
				By("Re-enabling legacy alert forwarding after MCOA alert forwarding tests", func() {
					Expect(utils.SetLegacyAlertForwardingDisabled(testOptions, false)).NotTo(HaveOccurred())
				})
			})

			It("should allow enabling and forwarding alerts to the hub", SpecTimeout(10*time.Minute), func(ctx SpecContext) {
				By("Enabling platform and user workload metrics with alert forwarding for MCOA", func() {
					Expect(utils.SetMCOAAllCapabilities(testOptions, true, true, true, true)).NotTo(HaveOccurred())
				})

				By("Configuring the platform and user workload scrape intervals to 30s", func() {
					Eventually(func() error {
						return utils.UpdatePrometheusAgentScrapeInterval(testOptions, "platform-metrics-collector", "30s")
					}, 120, 2).Should(Not(HaveOccurred()))
					Eventually(func() error {
						return utils.UpdatePrometheusAgentScrapeInterval(testOptions, "user-workload-metrics-collector", "30s")
					}, 120, 2).Should(Not(HaveOccurred()))
				})

				By("Checking Watchdog alerts are forwarded to the hub when alert forwarding is enabled")
				Eventually(func() error {
					var amURL *url.URL
					cloudProvider := strings.ToLower(os.Getenv("CLOUD_PROVIDER"))
					if strings.Contains(cloudProvider, "rosa") && strings.Contains(cloudProvider, "hcp") {
						amURL = &url.URL{
							Scheme: "https",
							Host:   "alertmanager-open-cluster-management-observability.apps.rosa." + testOptions.HubCluster.BaseDomain,
							Path:   "/api/v2/alerts",
						}
					} else {
						amURL = &url.URL{
							Scheme: "https",
							Host:   "alertmanager-open-cluster-management-observability.apps." + testOptions.HubCluster.BaseDomain,
							Path:   "/api/v2/alerts",
						}
					}
					q := amURL.Query()
					q.Set("filter", "alertname=Watchdog")
					amURL.RawQuery = q.Encode()

					caCrt, err := utils.GetRouterCA(hubClient)
					if err != nil {
						return err
					}
					pool := x509.NewCertPool()
					pool.AppendCertsFromPEM(caCrt)

					client := &http.Client{
						Timeout: 30 * time.Second,
						Transport: &http.Transport{
							Proxy:           http.ProxyFromEnvironment,
							TLSClientConfig: &tls.Config{RootCAs: pool},
						},
					}

					alertGetReq, err := http.NewRequestWithContext(ctx, "GET", amURL.String(), nil)
					if err != nil {
						return err
					}

					if os.Getenv("IS_KIND_ENV") != "true" {
						if BearerToken == "" {
							BearerToken, err = utils.FetchBearerToken(testOptions)
							if err != nil {
								return err
							}
						}
						alertGetReq.Header.Set("Authorization", "Bearer "+BearerToken)
					}

					resp, err := client.Do(alertGetReq)
					if err != nil {
						return err
					}
					defer resp.Body.Close()

					if resp.StatusCode != http.StatusOK {
						return fmt.Errorf("failed to get alerts via alertmanager route: HTTP %d", resp.StatusCode)
					}

					alertResult, err := io.ReadAll(resp.Body)
					if err != nil {
						return err
					}

					postableAlerts := models.PostableAlerts{}
					if err := json.Unmarshal(alertResult, &postableAlerts); err != nil {
						return err
					}

					clusterIDsInAlerts := []string{}
					for _, alt := range postableAlerts {
						if alt.Labels != nil {
							labelSets := map[string]string(alt.Labels)
							clusterID := labelSets["managed_cluster"]
							if clusterID != "" {
								clusterIDsInAlerts = append(clusterIDsInAlerts, clusterID)
							}
						}
					}

					expectedOCPClusterIDs, err := utils.ListAvailableOCPManagedClusterIDs(testOptions)
					if err != nil {
						return err
					}

					missingClusters := []string{}
					for _, expectedID := range expectedOCPClusterIDs {
						if !slices.Contains(clusterIDsInAlerts, expectedID) {
							missingClusters = append(missingClusters, expectedID)
						}
					}

					if len(missingClusters) > 0 {
						return fmt.Errorf("Watchdog alerts are still missing from these clusters: %q. Found clusters: %q", missingClusters, clusterIDsInAlerts)
					}

					return nil
				}, 600, 10).Should(Not(HaveOccurred()))
			})

			It("should stop forwarding alerts and clean up config when alert forwarding or MCOA is disabled", SpecTimeout(10*time.Minute), func(ctx SpecContext) {
				By("Disabling alert forwarding for MCOA", func() {
					Expect(utils.SetMCOAAlertForwardingCapabilities(testOptions, false, false)).NotTo(HaveOccurred())
				})

				By("Checking that CMO ConfigMap is cleaned of additional alert forwarding config on disable", func() {
					namespace := "openshift-monitoring"
					configMapName := "cluster-monitoring-config"

					Eventually(func() error {
						cm, err := hubClient.CoreV1().ConfigMaps(namespace).Get(context.TODO(), configMapName, metav1.GetOptions{})
						if err != nil {
							if apierrors.IsNotFound(err) {
								return nil
							}
							return err
						}
						configContent := cm.Data["config.yaml"]
						if strings.Contains(configContent, "additionalAlertmanagerConfigs:") {
							return fmt.Errorf("ConfigMap still contains additionalAlertmanagerConfigs: %s", configContent)
						}
						return nil
					}, 120, 5).Should(Succeed())
				})

				By("Checking that UWL ConfigMap is cleaned of additional alert forwarding config on disable", func() {
					namespace := "openshift-user-workload-monitoring"
					configMapName := "user-workload-monitoring-config"

					Eventually(func() error {
						cm, err := hubClient.CoreV1().ConfigMaps(namespace).Get(context.TODO(), configMapName, metav1.GetOptions{})
						if err != nil {
							if apierrors.IsNotFound(err) {
								return nil
							}
							return err
						}
						configContent := cm.Data["config.yaml"]
						if strings.Contains(configContent, "additionalAlertmanagerConfigs:") {
							return fmt.Errorf("UWL ConfigMap still contains additionalAlertmanagerConfigs: %s", configContent)
						}
						return nil
					}, 120, 5).Should(Succeed())
				})

				By("Configuring alert forwarding back to enabled first", func() {
					Expect(utils.SetMCOAAlertForwardingCapabilities(testOptions, true, true)).NotTo(HaveOccurred())
				})

				By("Waiting for additional alert forwarding config to be recreated", func() {
					namespace := "openshift-monitoring"
					configMapName := "cluster-monitoring-config"

					Eventually(func() error {
						cm, err := hubClient.CoreV1().ConfigMaps(namespace).Get(context.TODO(), configMapName, metav1.GetOptions{})
						if err != nil {
							return err
						}
						configContent := cm.Data["config.yaml"]
						if !strings.Contains(configContent, "additionalAlertmanagerConfigs:") {
							return fmt.Errorf("ConfigMap does not contain additionalAlertmanagerConfigs yet: %s", configContent)
						}
						return nil
					}, 120, 5).Should(Succeed())
				})

				By("Disabling MCOA capabilities entirely", func() {
					Expect(utils.SetMCOACapabilities(testOptions, false, false)).NotTo(HaveOccurred())
				})

				By("Checking that CMO ConfigMap is cleaned of additional alert forwarding config when addon is disabled", func() {
					namespace := "openshift-monitoring"
					configMapName := "cluster-monitoring-config"

					Eventually(func() error {
						cm, err := hubClient.CoreV1().ConfigMaps(namespace).Get(context.TODO(), configMapName, metav1.GetOptions{})
						if err != nil {
							if apierrors.IsNotFound(err) {
								return nil
							}
							return err
						}
						configContent := cm.Data["config.yaml"]
						if strings.Contains(configContent, "additionalAlertmanagerConfigs:") {
							return fmt.Errorf("ConfigMap still contains additionalAlertmanagerConfigs: %s", configContent)
						}
						return nil
					}, 120, 5).Should(Succeed())
				})

				By("Checking that UWL ConfigMap is cleaned of additional alert forwarding config when addon is disabled", func() {
					namespace := "openshift-user-workload-monitoring"
					configMapName := "user-workload-monitoring-config"

					Eventually(func() error {
						cm, err := hubClient.CoreV1().ConfigMaps(namespace).Get(context.TODO(), configMapName, metav1.GetOptions{})
						if err != nil {
							if apierrors.IsNotFound(err) {
								return nil
							}
							return err
						}
						configContent := cm.Data["config.yaml"]
						if strings.Contains(configContent, "additionalAlertmanagerConfigs:") {
							return fmt.Errorf("UWL ConfigMap still contains additionalAlertmanagerConfigs: %s", configContent)
						}
						return nil
					}, 120, 5).Should(Succeed())
				})
			})
		},
	)

	JustAfterEach(func() {
		if CurrentSpecReport().Failed() {
			utils.LogFailingTestStandardDebugInfo(testOptions, true)
		}
		testFailed = testFailed || CurrentSpecReport().Failed()

		Expect(utils.IntegrityChecking(testOptions)).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		By("Disabling MCOA", func() {
			Expect(utils.SetMCOACapabilities(testOptions, false, false)).NotTo(HaveOccurred())
			utils.CheckStatefulSetAvailabilityOnClusters(managedClustersWithHub, platformPrometheusAgentStatefulSetName, utils.MCO_AGENT_ADDON_NAMESPACE, false)
			utils.CheckDeploymentAvailability(testOptions.HubCluster, mcoaManagerDeploymentName, utils.MCO_NAMESPACE, false)

			By("Checking that OBO/COO CRDs are deleted from managed clusters when addon is disabled", func() {
				expectedCRDs := mcoa.GetManagedCRDNames()
				for _, cluster := range accessibleOCPClusters {
					clientAPIExtension := utils.NewKubeClientAPIExtension(cluster.ClusterServerURL, cluster.KubeConfig, cluster.KubeContext)
					clientAPIExtensionV1 := clientAPIExtension.ApiextensionsV1()
					Eventually(func() error {
						for _, crd := range expectedCRDs {
							getRequestCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
							crdObj, err := clientAPIExtensionV1.CustomResourceDefinitions().Get(getRequestCtx, crd, metav1.GetOptions{})
							cancel()
							if err != nil {
								if apierrors.IsNotFound(err) {
									continue
								}
								return err
							}
							// On clusters like the Hub, some OBO/COO CRDs are installed and managed by other
							// controllers (e.g., MCO). We should only fail if a CRD remains and carries
							// our specific endpoint operator management label.
							labels := crdObj.GetLabels()
							if labels != nil && labels[mcoa.ManagedByLabelKey] == mcoa.ManagedByLabelValue {
								return fmt.Errorf("OBO CRD %s managed by MCOA was not cleaned up on cluster %s after addon was disabled", crd, cluster.Name)
							}
						}
						return nil
					}, 120, 5).Should(Succeed(), "All MCOA-managed CRDs should be cleaned up on cluster %s", cluster.Name)
				}
			})

			By("Waiting for 1 minute to make sure the registration controller correctly takes into account the changes")
			time.Sleep(60 * time.Second)

			// Wait for the metrics collector to be up to avoid race conditions with other tests setups
			utils.CheckDeploymentAvailability(testOptions.HubCluster, metricsCollectorDeploymentName, utils.MCO_NAMESPACE, true)
			utils.CheckDeploymentAvailabilityOnClusters(managedClusters, metricsCollectorDeploymentName, utils.MCO_ADDON_NAMESPACE, true)
		})
	})
})
