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
	"github.com/stolostron/multicluster-observability-operator/tests/pkg/kustomize"
	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// Locally redeclared constants to avoid build dependency on dashboard loader package
	customDashboardLabelKey = "grafana-custom-dashboard"
	customFolderKey         = "observability.open-cluster-management.io/dashboard-folder"
	homeDashboardUIDKey     = "home-dashboard-uid"
	setHomeDashboardKey     = "set-home-dashboard"

	dashboardName                 = "sample-dashboard"
	dashboardTitle                = "Sample Dashboard for E2E"
	updateDashboardTitle          = "Update Sample Dashboard for E2E"
	clusterOverviewTitle          = "ACM - Clusters Overview"
	clusterOverviewOptimizedTitle = "ACM - Clusters Overview (Optimized)"

	syncTimeout      = 5 * time.Minute
	syncInterval     = 5 * time.Second
	cleanupTimeout   = 2 * time.Minute
	cleanupInterval  = 5 * time.Second
	metadataTimeout  = 1 * time.Minute
	metadataInterval = 5 * time.Second
)

var _ = Describe("Observability: Dashboard Lifecycle", func() {
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

	It(
		"RHACM4K-1669: Observability: Verify new customized Grafana dashboard - Should have custom dashboard which defined in configmap [P2][Sev2][Observability][Stable]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release @pre-upgrade (dashboard/g0)",
		func() {
			By("Creating custom dashboard configmap")
			yamlB, err := kustomize.Render(
				kustomize.Options{KustomizationPath: "../../../examples/dashboards/sample_custom_dashboard"},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(
				utils.ApplyRetryOnConflict(
					testOptions.HubCluster.ClusterServerURL,
					testOptions.KubeConfig,
					testOptions.HubCluster.KubeContext,
					yamlB)).NotTo(HaveOccurred())

			DeferCleanup(func() {
				By("Cleaning up custom dashboard")
				_ = utils.DeleteConfigMap(testOptions, true, dashboardName, MCO_NAMESPACE)
			})

			Eventually(func() bool {
				result, _ := utils.ContainDashboard(testOptions, dashboardTitle)
				return result
			}, syncTimeout, syncInterval).Should(BeTrue())
		},
	)

	It(
		"RHACM4K-1669: Observability: Verify new customized Grafana dashboard - Should have update custom dashboard after configmap updated [P2][Sev2][Observability][Stable]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release @pre-upgrade (dashboard/g0)",
		func() {
			By("Creating custom dashboard configmap")
			yamlB, err := kustomize.Render(
				kustomize.Options{KustomizationPath: "../../../examples/dashboards/sample_custom_dashboard"},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(
				utils.ApplyRetryOnConflict(
					testOptions.HubCluster.ClusterServerURL,
					testOptions.KubeConfig,
					testOptions.HubCluster.KubeContext,
					yamlB)).NotTo(HaveOccurred())

			DeferCleanup(func() {
				By("Cleaning up custom dashboard")
				_ = utils.DeleteConfigMap(testOptions, true, dashboardName, MCO_NAMESPACE)
			})

			By("Updating custom dashboard configmap")
			yamlB, err = kustomize.Render(
				kustomize.Options{KustomizationPath: "../../../examples/dashboards/update_sample_custom_dashboard"},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(
				utils.ApplyRetryOnConflict(
					testOptions.HubCluster.ClusterServerURL,
					testOptions.KubeConfig,
					testOptions.HubCluster.KubeContext,
					yamlB)).NotTo(HaveOccurred())

			Eventually(func(g Gomega) bool {
				result, err := utils.ContainDashboard(testOptions, dashboardTitle)
				g.Expect(err).ToNot(HaveOccurred())
				return result
			}, cleanupTimeout, cleanupInterval).Should(BeFalse())

			Eventually(func() bool {
				result, _ := utils.ContainDashboard(testOptions, updateDashboardTitle)
				return result
			}, syncTimeout, syncInterval).Should(BeTrue())
		},
	)

	It(
		"RHACM4K-1669: Observability: Verify new customized Grafana dashboard - Should have no custom dashboard in grafana after related configmap removed [P2][Sev2][Observability][Stable]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release @pre-upgrade (dashboard/g0)",
		func() {
			By("Creating custom dashboard configmap")
			yamlB, err := kustomize.Render(
				kustomize.Options{KustomizationPath: "../../../examples/dashboards/sample_custom_dashboard"},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(
				utils.ApplyRetryOnConflict(
					testOptions.HubCluster.ClusterServerURL,
					testOptions.KubeConfig,
					testOptions.HubCluster.KubeContext,
					yamlB)).NotTo(HaveOccurred())

			By("Deleting custom dashboard configmap")
			err = utils.DeleteConfigMap(testOptions, true, dashboardName, MCO_NAMESPACE)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) bool {
				result, err := utils.ContainDashboard(testOptions, dashboardTitle)
				g.Expect(err).ToNot(HaveOccurred())
				return result
			}, syncTimeout, syncInterval).Should(BeFalse())
		},
	)

	It(
		"Observability: Verify dashboard move and folder reaping - Should move dashboard and delete empty folder [P2][Sev2][Observability][Stable]@e2e (dashboard/g1)",
		func() {
			cmName := "move-test-dashboard-unique"
			folderA := "Folder-A"
			folderB := "Folder-B"
			dashTitle := "Move Test Dashboard"
			dashUID := "move-test-uid"

			By("Creating dashboard in Folder-A")
			cm := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cmName,
					Namespace: MCO_NAMESPACE,
					Labels:    map[string]string{customDashboardLabelKey: "true"},
					Annotations: map[string]string{
						customFolderKey: folderA,
					},
				},
				Data: map[string]string{
					"test.json": fmt.Sprintf("{\"title\": \"%s\", \"uid\": \"%s\"}", dashTitle, dashUID),
				},
			}
			_, err := hubClient.CoreV1().ConfigMaps(MCO_NAMESPACE).Create(context.Background(), cm, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				By("Cleaning up move-test-dashboard")
				_ = hubClient.CoreV1().ConfigMaps(MCO_NAMESPACE).Delete(context.Background(), cmName, metav1.DeleteOptions{})
			})

			Eventually(func() string {
				meta, _ := utils.GetDashboardMetadata(context.Background(), testOptions, dashTitle)
				if meta != nil {
					return meta.FolderTitle
				}
				return ""
			}, metadataTimeout, metadataInterval).Should(Equal(folderA))

			By("Moving dashboard to Folder-B")
			cm, err = hubClient.CoreV1().ConfigMaps(MCO_NAMESPACE).Get(context.Background(), cmName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			cm.Annotations[customFolderKey] = folderB
			// Also change the title slightly to break any search caches
			updateTitle := "Move Test Dashboard Updated"
			cm.Data["test.json"] = fmt.Sprintf("{\"title\": \"%s\", \"uid\": \"%s\"}", updateTitle, dashUID)
			_, err = hubClient.CoreV1().ConfigMaps(MCO_NAMESPACE).Update(context.Background(), cm, metav1.UpdateOptions{})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() string {
				meta, _ := utils.GetDashboardMetadata(context.Background(), testOptions, updateTitle)
				if meta != nil {
					return meta.FolderTitle
				}
				return ""
			}, syncTimeout, syncInterval).Should(Equal(folderB))

			By("Deleting dashboard and verifying folder reaping")
			err = hubClient.CoreV1().ConfigMaps(MCO_NAMESPACE).Delete(context.Background(), cmName, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) bool {
				meta, err := utils.GetDashboardMetadata(context.Background(), testOptions, updateTitle)
				g.Expect(err).ToNot(HaveOccurred())
				return meta != nil
			}, cleanupTimeout, cleanupInterval).Should(BeFalse())

			// The folder should be reaped eventually (moving/deletion triggers immediate cleanup in the new code)
			Eventually(func(g Gomega) bool {
				exists, err := utils.FolderExists(context.Background(), testOptions, folderA)
				g.Expect(err).ToNot(HaveOccurred())
				return exists
			}, syncTimeout, syncInterval).Should(BeFalse())
			Eventually(func(g Gomega) bool {
				exists, err := utils.FolderExists(context.Background(), testOptions, folderB)
				g.Expect(err).ToNot(HaveOccurred())
				return exists
			}, syncTimeout, syncInterval).Should(BeFalse())
		},
	)

	It(
		"Observability: Verify multi-dashboard ConfigMap - Should sync multiple dashboards from one CM [P2][Sev2][Observability][Stable]@e2e (dashboard/g2)",
		func() {
			cmName := "multi-test-cm-unique"
			By("Creating multi-dashboard configmap")
			cm := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cmName,
					Namespace: MCO_NAMESPACE,
					Labels:    map[string]string{customDashboardLabelKey: "true"},
				},
				Data: map[string]string{
					"dash1.json": "{\"title\": \"Multi Dash 1\", \"uid\": \"multi-1\"}",
					"dash2.json": "{\"title\": \"Multi Dash 2\", \"uid\": \"multi-2\"}",
				},
			}
			_, err := hubClient.CoreV1().ConfigMaps(MCO_NAMESPACE).Create(context.Background(), cm, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				By("Cleaning up multi-dashboard CM")
				_ = hubClient.CoreV1().ConfigMaps(MCO_NAMESPACE).Delete(context.Background(), cmName, metav1.DeleteOptions{})
			})

			Eventually(func() bool {
				found1, _ := utils.ContainDashboard(testOptions, "Multi Dash 1")
				found2, _ := utils.ContainDashboard(testOptions, "Multi Dash 2")
				return found1 && found2
			}, syncTimeout, syncInterval).Should(BeTrue())

			By("Deleting the whole ConfigMap for immediate cleanup")
			err = hubClient.CoreV1().ConfigMaps(MCO_NAMESPACE).Delete(context.Background(), cmName, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) bool {
				found1, err1 := utils.ContainDashboard(testOptions, "Multi Dash 1")
				g.Expect(err1).ToNot(HaveOccurred())
				found2, err2 := utils.ContainDashboard(testOptions, "Multi Dash 2")
				g.Expect(err2).ToNot(HaveOccurred())
				return !found1 && !found2
			}, cleanupTimeout, cleanupInterval).Should(BeTrue())
		},
	)

	It(
		"Observability: Verify dashboard UID stability - Should preserve UID across CM recreation [P2][Sev2][Observability][Stable]@e2e (dashboard/g3)",
		func() {
			cmName := "stability-test-cm-unique"
			dashTitle := "Stability Test Dashboard"
			By("Creating dashboard without UID")
			cm := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cmName,
					Namespace: MCO_NAMESPACE,
					Labels:    map[string]string{customDashboardLabelKey: "true"},
				},
				Data: map[string]string{
					"test.json": fmt.Sprintf("{\"title\": \"%s\"}", dashTitle),
				},
			}
			_, err := hubClient.CoreV1().ConfigMaps(MCO_NAMESPACE).Create(context.Background(), cm, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				By("Cleaning up stability test CM")
				_ = hubClient.CoreV1().ConfigMaps(MCO_NAMESPACE).Delete(context.Background(), cmName, metav1.DeleteOptions{})
			})

			var initialUID string
			Eventually(func() bool {
				meta, _ := utils.GetDashboardMetadata(context.Background(), testOptions, dashTitle)
				if meta != nil {
					initialUID = meta.UID
					return true
				}
				return false
			}, syncTimeout, syncInterval).Should(BeTrue())

			Expect(initialUID).NotTo(BeEmpty())

			By("Deleting and recreating ConfigMap")
			err = hubClient.CoreV1().ConfigMaps(MCO_NAMESPACE).Delete(context.Background(), cmName, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())

			// Wait for deletion in Grafana
			Eventually(func(g Gomega) bool {
				found, err := utils.ContainDashboard(testOptions, dashTitle)
				g.Expect(err).ToNot(HaveOccurred())
				return found
			}, cleanupTimeout, cleanupInterval).Should(BeFalse())

			// Recreate exactly same
			_, err = hubClient.CoreV1().ConfigMaps(MCO_NAMESPACE).Create(context.Background(), cm, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() string {
				meta, _ := utils.GetDashboardMetadata(context.Background(), testOptions, dashTitle)
				if meta != nil {
					return meta.UID
				}
				return ""
			}, syncTimeout, syncInterval).Should(Equal(initialUID))
		},
	)

	It(
		"Observability: Verify home dashboard setting - Should set custom dashboard as home [P2][Sev2][Observability][Stable]@e2e (dashboard/g4)",
		func() {
			cmName := "home-test-dashboard-unique"
			dashTitle := "Home Test Dashboard"
			dashUID := "home-test-uid"
			By("Creating dashboard with home dashboard labels")
			cm := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cmName,
					Namespace: MCO_NAMESPACE,
					Labels: map[string]string{
						customDashboardLabelKey: "true",
						homeDashboardUIDKey:     dashUID,
					},
					Annotations: map[string]string{
						setHomeDashboardKey: "true",
					},
				},
				Data: map[string]string{
					"test.json": fmt.Sprintf("{\"title\": \"%s\", \"uid\": \"%s\"}", dashTitle, dashUID),
				},
			}
			_, err := hubClient.CoreV1().ConfigMaps(MCO_NAMESPACE).Create(context.Background(), cm, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				By("Cleaning up home-test-dashboard")
				_ = hubClient.CoreV1().ConfigMaps(MCO_NAMESPACE).Delete(context.Background(), cmName, metav1.DeleteOptions{})
			})

			Eventually(func() bool {
				meta, _ := utils.GetDashboardMetadata(context.Background(), testOptions, dashTitle)
				return meta != nil && meta.UID == dashUID
			}, syncTimeout, syncInterval).Should(BeTrue())

			By("Verifying Grafana home dashboard preference")
			isForbidden := false
			Eventually(func() string {
				id, err := utils.GetGrafanaHomeDashboard(context.Background(), testOptions)
				if err != nil {
					if strings.Contains(err.Error(), "403") {
						isForbidden = true
						return "403"
					}
					return ""
				}
				if id == 0 {
					return "none"
				}
				homeUID, _ := utils.GetDashboardUIDByID(context.Background(), testOptions, id)
				return homeUID
			}, syncTimeout, syncInterval).Should(SatisfyAny(
				Equal(dashUID),
				Equal("403"), // Handle 403 Access Denied
			))

			if isForbidden {
				GinkgoWriter.Printf("WARNING: Access denied (403) while checking home dashboard preference. This is expected if the token lacks organization preference read permissions.\n")
				Skip("Skipping home dashboard verification due to 403 Access Denied on preferences API")
			}
		},
	)

	// TODO: Need RHACM4K no
	It("[P2][Sev2][observability][Stable] Should have default overview dashboards (dashboard/g0)", func() {
		// Check Original dash exists
		Eventually(func() bool {
			result, _ := utils.ContainDashboard(testOptions, clusterOverviewTitle)
			return result
		}, syncTimeout, syncInterval).Should(BeTrue())
		// Check optimized dash
		Eventually(func() bool {
			result, _ := utils.ContainDashboard(testOptions, clusterOverviewOptimizedTitle)
			return result
		}, syncTimeout, syncInterval).Should(BeTrue())
	})

	JustAfterEach(func() {
		Expect(utils.IntegrityChecking(testOptions)).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			utils.LogFailingTestStandardDebugInfo(testOptions)
			// Force logs for Grafana pods
			hubClient := utils.NewKubeClient(
				testOptions.HubCluster.ClusterServerURL,
				testOptions.KubeConfig,
				testOptions.HubCluster.KubeContext)
			utils.CheckPodsInNamespace(hubClient, MCO_NAMESPACE, []string{"observability-grafana"}, map[string]string{
				"app": "multicluster-observability-grafana",
			})
		}
		testFailed = testFailed || CurrentSpecReport().Failed()
	})
})
