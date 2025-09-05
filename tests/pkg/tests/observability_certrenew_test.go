// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
	})

	It("RHACM4K-3073: Observability: Verify Observability Certificate rotation - Should have metrics collector pod restart if cert secret re-generated [P1][Sev1][Observability][Integration]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release (certrenew/g0)", func() {

		if len(testOptions.ManagedClusters) > 0 &&
			utils.GetManagedClusterName(testOptions) != hubManagedClusterName {
			Skip("Skipping unreliable cert-test on multi-spoke systems")
		}

		By("Waiting for pods ready: observability-observatorium-api, observability-rbac-query-proxy, metrics-collector-deployment")
		// sleep 30s to wait for installation is ready
		time.Sleep(30 * time.Second)
		collectorPodNameSpoke := ""
		collectorPodNameHub := ""
		hubPodsName := []string{}
		Eventually(func() bool {
			// check metrics-collector on spoke, unless it's local-cluster
			if len(testOptions.ManagedClusters) > 0 &&
				utils.GetManagedClusterName(testOptions) != hubManagedClusterName {
				if collectorPodNameSpoke == "" {
					_, podList := utils.GetPodList(
						testOptions,
						false,
						MCO_ADDON_NAMESPACE,
						"component=metrics-collector",
					)
					if podList != nil && len(podList.Items) > 0 {
						collectorPodNameSpoke = podList.Items[0].Name
					}
				}
				if collectorPodNameSpoke == "" {
					return false
				}
			}

			// Check obs/api, rbac-query-proxy, metrics collector on hub
			if collectorPodNameHub == "" {
				_, podList := utils.GetPodList(
					testOptions,
					true,
					MCO_NAMESPACE,
					"component=metrics-collector",
				)
				if podList != nil && len(podList.Items) > 0 {
					collectorPodNameHub = podList.Items[0].Name
				}
			}
			if collectorPodNameHub == "" {
				return false
			}
			_, apiPodList := utils.GetPodList(
				testOptions,
				true,
				MCO_NAMESPACE,
				"app.kubernetes.io/name=observatorium-api",
			)
			if apiPodList != nil && len(apiPodList.Items) != 0 {
				for _, pod := range apiPodList.Items {
					hubPodsName = append(hubPodsName, pod.Name)
				}
			} else {
				return false
			}
			_, rbacPodList := utils.GetPodList(testOptions, true, MCO_NAMESPACE, "app=rbac-query-proxy")
			if rbacPodList != nil && len(rbacPodList.Items) != 0 {
				for _, pod := range rbacPodList.Items {
					hubPodsName = append(hubPodsName, pod.Name)
				}
			} else {
				return false
			}

			return true
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(BeTrue())

		By("Deleting certificate secret to simulate certificate renew")
		err := utils.DeleteCertSecret(testOptions)
		Expect(err).ToNot(HaveOccurred())

		// Wait for 40s to ensure the rbac-query-proxy readiness probe has had enough time to fail if it was going to.
		// The readiness probe has a period of 10s and a failure threshold of 3, so it takes 30s to fail.
		By("Waiting 40s for readiness probes to settle")
		time.Sleep(40 * time.Second)

		By("Waiting for observatorium-api pods to be recreated and rbac-query-proxy to be ready")
		Eventually(func() bool {
			err1, appPodList := utils.GetPodList(
				testOptions,
				true,
				MCO_NAMESPACE,
				"app.kubernetes.io/name=observatorium-api",
			)
			err2, rbacPodList := utils.GetPodList(testOptions, true, MCO_NAMESPACE, "app=rbac-query-proxy")
			if err1 != nil || err2 != nil {
				return false
			}

			// Check that observatorium-api pods are restarted and ready
			for _, oldPodName := range hubPodsName {
				if strings.Contains(oldPodName, "observatorium-api") {
					// Check it has been removed
					for _, pod := range appPodList.Items {
						if oldPodName == pod.Name {
							klog.V(1).Infof("<%s> not removed yet", oldPodName)
							return false
						}
					}
				}
			}
			for _, pod := range appPodList.Items {
				if pod.Status.Phase != "Running" {
					klog.V(1).Infof("<%s> not in Running status yet", pod.Name)
					return false
				}
				for _, cs := range pod.Status.ContainerStatuses {
					if !cs.Ready {
						klog.V(1).Infof("container <%s> in pod <%s> is not ready", cs.Name, pod.Name)
						return false
					}
				}
			}

			// Check that rbac-query-proxy pods are still running and ready
			for _, pod := range rbacPodList.Items {
				isOldPod := false
				for _, oldPodName := range hubPodsName {
					if pod.Name == oldPodName {
						isOldPod = true
						break
					}
				}
				if !isOldPod {
					klog.V(1).Infof("A new rbac-query-proxy pod <%s> was created, which is not expected.", pod.Name)
					return false
				}

				if pod.Status.Phase != "Running" {
					klog.V(1).Infof("<%s> not in Running status yet", pod.Name)
					return false
				}
				for _, cs := range pod.Status.ContainerStatuses {
					if !cs.Ready {
						klog.V(1).Infof("container <%s> in pod <%s> is not ready", cs.Name, pod.Name)
						return false
					}
				}
			}
			return true
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(BeTrue())

		// check metric collector spoke
		if len(testOptions.ManagedClusters) > 0 &&
			utils.GetManagedClusterName(testOptions) != hubManagedClusterName {
			By(fmt.Sprintf("Waiting for old pod <%s> removed and new pod created on spoke", collectorPodNameSpoke))
			Eventually(func() bool {
				err, podList := utils.GetPodList(
					testOptions,
					false,
					MCO_ADDON_NAMESPACE,
					"component=metrics-collector",
				)

				if len(podList.Items) != 1 {
					klog.Infof("Wrong number of pods: <%d> metrics-collector pods, 1 expected",
						len(podList.Items))
					return false
				}
				if err != nil {
					klog.Infof("Failed to get pod list: %v", err)
				}
				for _, pod := range podList.Items {
					if pod.Name != collectorPodNameSpoke {
						if pod.Status.Phase != "Running" {
							klog.Infof("<%s> not in Running status yet", pod.Name)
							return false
						}
						return true
					}
				}

				// debug code to check label "cert/time-restarted"
				deployment, err := utils.GetDeployment(
					testOptions,
					false,
					"metrics-collector-deployment",
					MCO_ADDON_NAMESPACE,
				)
				if err == nil {
					klog.V(1).Infof("labels: <%v>", deployment.Spec.Template.ObjectMeta.Labels)
				}
				return false
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(BeTrue())
		}

		By(fmt.Sprintf("Waiting for old pod <%s> removed and new pod created on Hub", collectorPodNameHub))
		Eventually(func() bool {
			err, podList := utils.GetPodList(
				testOptions,
				true,
				MCO_NAMESPACE,
				"component=metrics-collector",
			)
			if err != nil {
				klog.V(1).Infof("Failed to get pod list: %v", err)
			}
			for _, pod := range podList.Items {
				if pod.Name != collectorPodNameHub {
					if pod.Status.Phase != "Running" {
						klog.V(1).Infof("<%s> not in Running status yet", pod.Name)
						return false
					}
					return true
				}
			}

			// debug code to check label "cert/time-restarted"
			deployment, err := utils.GetDeployment(
				testOptions,
				true,
				"metrics-collector-deployment",
				MCO_NAMESPACE,
			)
			if err == nil {
				klog.V(1).Infof("labels: <%v>", deployment.Spec.Template.ObjectMeta.Labels)
			}
			return false
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(BeTrue())
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
})
