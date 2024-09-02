// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

	Context("[P2][Sev2][observability][Stable] Should be automatically created within 1 minute when delete manifestwork (manifestwork/g0) -", func() { // move to unit tests or integration ALL
		manifestWorkName := "endpoint-observability-work"
		clientDynamic := utils.GetKubeClientDynamic(testOptions, true)
		clusterName := utils.GetManagedClusterName(testOptions)
		if clusterName != "" && clusterName != "local-cluster" {
			// ACM 8509 : Special case for local-cluster
			// We do not create manifestwork for local-cluster
			oldManifestWorkResourceVersion := ""
			oldCollectorPodName := ""
			_, podList := utils.GetPodList(testOptions, false, MCO_ADDON_NAMESPACE, "component=metrics-collector")
			if podList != nil && len(podList.Items) > 0 {
				oldCollectorPodName = podList.Items[0].Name
			}

			Eventually(func() error {
				oldManifestWork, err := clientDynamic.Resource(utils.NewOCMManifestworksGVR()).
					Namespace(clusterName).
					Get(context.TODO(), manifestWorkName, metav1.GetOptions{})
				oldManifestWorkResourceVersion = oldManifestWork.GetResourceVersion()
				return err
			}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*5).Should(Succeed())

			By("Waiting for manifestwork to be deleted")
			Eventually(func() error {
				err := clientDynamic.Resource(utils.NewOCMManifestworksGVR()).
					Namespace(clusterName).
					Delete(context.TODO(), manifestWorkName, metav1.DeleteOptions{})
				return err
			}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*5).Should(Succeed())

			By("Waiting for manifestwork to be created automatically")
			Eventually(func() error {
				newManifestWork, err := clientDynamic.Resource(utils.NewOCMManifestworksGVR()).
					Namespace(clusterName).
					Get(context.TODO(), manifestWorkName, metav1.GetOptions{})
				if err == nil {
					if newManifestWork.GetResourceVersion() != oldManifestWorkResourceVersion {
						return nil
					} else {
						return errors.New("No new manifestwork generated")
					}
				} else {
					return err
				}
			}, EventuallyTimeoutMinute*2, EventuallyIntervalSecond*5).Should(Succeed())

			It("[Stable] Waiting for metrics collector to be created automatically", func() {
				Eventually(func() error {
					_, podList := utils.GetPodList(
						testOptions,
						false,
						MCO_ADDON_NAMESPACE,
						"component=metrics-collector",
					)
					if podList != nil && len(podList.Items) > 0 {
						if oldCollectorPodName != podList.Items[0].Name {
							return nil
						}
					}
					return errors.New("No new metrics collector generated")
				}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
			})

			It("[Stable] Checking OBA components are ready", func() {
				Eventually(func() error {
					err = utils.CheckOBAComponents(testOptions)
					if err != nil {
						return err
					}
					return nil
				}, EventuallyTimeoutMinute*3, EventuallyIntervalSecond*5).Should(Succeed())
			})

			It("[Stable] Checking metric to ensure that no data is lost in 1 minute", func() {
				Eventually(func() error {
					query := fmt.Sprintf(`timestamp(node_memory_MemAvailable_bytes{cluster="%s"}) - timestamp(node_memory_MemAvailable_bytes{cluster="%s"} offset 1m) > 59`, clusterName, clusterName)
					res, err := utils.QueryGrafana(
						testOptions,
						query,
					)
					if err != nil {
						return err
					}

					if len(res.Data.Result) == 0 {
						return fmt.Errorf("no data found for %s", query)
					}

					return nil
				}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*3).Should(Succeed())
			})
		}
	})

	JustAfterEach(func() {
		Expect(utils.IntegrityChecking(testOptions)).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if CurrentGinkgoTestDescription().Failed {
			utils.LogFailingTestStandardDebugInfo(testOptions)
		}
		testFailed = testFailed || CurrentGinkgoTestDescription().Failed
	})
})
