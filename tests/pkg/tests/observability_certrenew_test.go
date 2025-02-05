// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
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
	})

	It("RHACM4K-3073: Observability: Verify Observability Certificate rotation - Should have metrics collector pod restart if cert secret re-generated [P1][Sev1][Observability][Integration]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release (certrenew/g0)", func() {
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

		By(fmt.Sprintf("Waiting for old pods removed: %v and new pods created", hubPodsName))
		Eventually(func() bool {
			err1, appPodList := utils.GetPodList(
				testOptions,
				true,
				MCO_NAMESPACE,
				"app.kubernetes.io/name=observatorium-api",
			)
			err2, rbacPodList := utils.GetPodList(testOptions, true, MCO_NAMESPACE, "app=rbac-query-proxy")
			if err1 == nil && err2 == nil {
				if len(hubPodsName) != len(appPodList.Items)+len(rbacPodList.Items) {
					klog.V(1).Infof("Wrong number of pods: <%d> observatorium-api pods and <%d> rbac-query-proxy pods",
						len(appPodList.Items),
						len(rbacPodList.Items))
					return false
				}
				for _, oldPodName := range hubPodsName {
					for _, pod := range appPodList.Items {
						if oldPodName == pod.Name {
							klog.V(1).Infof("<%s> not removed yet", oldPodName)
							return false
						}
						if pod.Status.Phase != "Running" {
							klog.V(1).Infof("<%s> not in Running status yet", pod.Name)
							return false
						}
					}
					for _, pod := range rbacPodList.Items {
						if oldPodName == pod.Name {
							klog.V(1).Infof("<%s> not removed yet", oldPodName)
							return false
						}
						if pod.Status.Phase != "Running" {
							klog.V(1).Infof("<%s> not in Running status yet", pod.Name)
							return false
						}
					}
				}
				return true
			}

			// debug code to check label "cert/time-restarted"
			deploys, err := utils.GetDeploymentWithLabel(testOptions, true, OBSERVATORIUM_API_LABEL, MCO_NAMESPACE)
			if err == nil {
				for _, deployInfo := range (*deploys).Items {
					klog.V(1).Infof("labels: <%v>", deployInfo.Spec.Template.ObjectMeta.Labels)
				}
			}

			deploys, err = utils.GetDeploymentWithLabel(testOptions, true, RBAC_QUERY_PROXY_LABEL, MCO_NAMESPACE)
			if err == nil {
				for _, deployInfo := range (*deploys).Items {
					klog.V(1).Infof("labels: <%v>", deployInfo.Spec.Template.ObjectMeta.Labels)
				}
			}

			return false
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
				if err != nil {
					klog.V(1).Infof("Failed to get pod list: %v", err)
				}
				for _, pod := range podList.Items {
					if pod.Name != collectorPodNameSpoke {
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

		By(fmt.Sprintf("Waiting for old pod <%s> removed and new pod created on Hub", collectorPodNameSpoke))
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
				if pod.Name != collectorPodNameSpoke {
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
		if CurrentGinkgoTestDescription().Failed {
			utils.LogFailingTestStandardDebugInfo(testOptions)
		}
		testFailed = testFailed || CurrentGinkgoTestDescription().Failed
	})
})
