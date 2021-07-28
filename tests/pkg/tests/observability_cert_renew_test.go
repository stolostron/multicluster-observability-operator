// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/klog"

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

	It("[P1][Sev1][Observability][Integration] Should have metrics collector pod restart if cert secret re-generated (certrenew/g0)", func() {
		By("Waiting for pods ready: observability-observatorium-api, observability-rbac-query-proxy, metrics-collector-deployment")
		// sleep 30s to wait for installation is ready
		time.Sleep(30 * time.Second)
		collectorPodName := ""
		hubPodsName := []string{}
		Eventually(func() bool {
			if collectorPodName == "" {
				_, podList := utils.GetPodList(testOptions, false, MCO_ADDON_NAMESPACE, "component=metrics-collector")
				if podList != nil && len(podList.Items) > 0 {
					collectorPodName = podList.Items[0].Name
				}
			}
			if collectorPodName == "" {
				return false
			}
			hubPodsName = []string{}
			_, apiPodList := utils.GetPodList(testOptions, true, MCO_NAMESPACE, "app.kubernetes.io/name=observatorium-api")
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
			err1, appPodList := utils.GetPodList(testOptions, true, MCO_NAMESPACE, "app.kubernetes.io/name=observatorium-api")
			err2, rbacPodList := utils.GetPodList(testOptions, true, MCO_NAMESPACE, "app=rbac-query-proxy")
			if err1 == nil && err2 == nil {
				if len(hubPodsName) != len(appPodList.Items)+len(rbacPodList.Items) {
					klog.V(1).Infof("Wrong number of pods: <%d> observatorium-api pods and <%d> rbac-query-proxy pods", len(appPodList.Items), len(rbacPodList.Items))
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

		By(fmt.Sprintf("Waiting for old pod <%s> removed and new pod created", collectorPodName))
		Eventually(func() bool {
			err, podList := utils.GetPodList(testOptions, false, MCO_ADDON_NAMESPACE, "component=metrics-collector")
			if err == nil {
				for _, pod := range podList.Items {
					if pod.Name != collectorPodName {
						if pod.Status.Phase != "Running" {
							klog.V(1).Infof("<%s> not in Running status yet", pod.Name)
							return false
						}
						return true
					}
				}

			}
			// debug code to check label "cert/time-restarted"
			deployment, err := utils.GetDeployment(testOptions, false, "metrics-collector-deployment", MCO_ADDON_NAMESPACE)
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
			utils.PrintMCOObject(testOptions)
			utils.PrintAllMCOPodsStatus(testOptions)
			utils.PrintAllOBAPodsStatus(testOptions)
		}
		testFailed = testFailed || CurrentGinkgoTestDescription().Failed
	})
})
