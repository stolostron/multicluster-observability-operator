// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tests

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/open-cluster-management/multicluster-observability-operator/tests/pkg/utils"
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

	Context("[P2][Sev2][Observability][Stable] Should be automatically created within 1 minute when delete manifestwork (manifestwork/g0) -", func() {
		manifestWorkName := "endpoint-observability-work"
		clientDynamic := utils.GetKubeClientDynamic(testOptions, true)
		clusterName := utils.GetManagedClusterName(testOptions)
		if clusterName != "" {
			oldManifestWorkResourceVersion := ""
			oldCollectorPodName := ""
			_, podList := utils.GetPodList(testOptions, false, MCO_ADDON_NAMESPACE, "component=metrics-collector")
			if podList != nil && len(podList.Items) > 0 {
				oldCollectorPodName = podList.Items[0].Name
			}

			Eventually(func() error {
				oldManifestWork, err := clientDynamic.Resource(utils.NewOCMManifestworksGVR()).Namespace(clusterName).Get(context.TODO(), manifestWorkName, metav1.GetOptions{})
				oldManifestWorkResourceVersion = oldManifestWork.GetResourceVersion()
				return err
			}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*5).Should(Succeed())

			By("Waiting for manifestwork to be deleted")
			Eventually(func() error {
				err := clientDynamic.Resource(utils.NewOCMManifestworksGVR()).Namespace(clusterName).Delete(context.TODO(), manifestWorkName, metav1.DeleteOptions{})
				return err
			}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*5).Should(Succeed())

			By("Waiting for manifestwork to be created automatically")
			Eventually(func() error {
				newManifestWork, err := clientDynamic.Resource(utils.NewOCMManifestworksGVR()).Namespace(clusterName).Get(context.TODO(), manifestWorkName, metav1.GetOptions{})
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
					_, podList := utils.GetPodList(testOptions, false, MCO_ADDON_NAMESPACE, "component=metrics-collector")
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
					err, _ = utils.ContainManagedClusterMetric(testOptions, `timestamp(node_memory_MemAvailable_bytes{cluster="`+clusterName+`}) - timestamp(node_memory_MemAvailable_bytes{cluster=`+clusterName+`"} offset 1m) > 59`, []string{`"__name__":"node_memory_MemAvailable_bytes"`})
					return err
				}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*3).Should(Succeed())
			})
		}
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
