// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tests

import (
	"context"
	"fmt"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/open-cluster-management/multicluster-observability-operator/tests/pkg/utils"
)

func uninstallMCO() {
	if os.Getenv("SKIP_UNINSTALL_STEP") == "true" {
		return
	}

	hubClient := utils.NewKubeClient(
		testOptions.HubCluster.ClusterServerURL,
		testOptions.KubeConfig,
		testOptions.HubCluster.KubeContext)

	dynClient := utils.NewKubeClientDynamic(
		testOptions.HubCluster.ClusterServerURL,
		testOptions.KubeConfig,
		testOptions.HubCluster.KubeContext)

	By("Deleteing the MCO testing RBAC resources")
	Expect(utils.DeleteMCOTestingRBAC(testOptions)).NotTo(HaveOccurred())

	By("Uninstall MCO instance")
	err := utils.UninstallMCO(testOptions)
	Expect(err).ToNot(HaveOccurred())

	By("Waiting for delete all MCO components")
	Eventually(func() error {
		var podList, _ = hubClient.CoreV1().Pods(MCO_NAMESPACE).List(context.TODO(), metav1.ListOptions{})
		if len(podList.Items) != 0 {
			return err
		}
		return nil
	}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

	By("Waiting for delete MCO addon instance")
	Eventually(func() error {
		name := MCO_CR_NAME + "-addon"
		clientDynamic := utils.GetKubeClientDynamic(testOptions, false)
		// should check oba instance from managedcluster
		instance, _ := clientDynamic.Resource(utils.NewMCOAddonGVR()).Namespace(MCO_ADDON_NAMESPACE).Get(context.TODO(), name, metav1.GetOptions{})
		if instance != nil {
			utils.PrintManagedClusterOBAObject(testOptions)
			return fmt.Errorf("Failed to delete MCO addon instance")
		}
		return nil
	}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

	By("Waiting for delete manifestwork")
	Eventually(func() error {
		name := "endpoint-observability-work"
		_, err := dynClient.Resource(utils.NewOCMManifestworksGVR()).Namespace("local-cluster").Get(context.TODO(), name, metav1.GetOptions{})
		return err
	}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(MatchError(`manifestworks.work.open-cluster-management.io "endpoint-observability-work" not found`))

	By("Waiting for delete all MCO addon components")
	Eventually(func() error {
		var podList, _ = hubClient.CoreV1().Pods(MCO_ADDON_NAMESPACE).List(context.TODO(), metav1.ListOptions{})
		if len(podList.Items) != 0 {
			return err
		}
		return nil
	}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

	By("Waiting for delete MCO namespaces")
	Eventually(func() error {
		err := hubClient.CoreV1().Namespaces().Delete(context.TODO(), MCO_NAMESPACE, metav1.DeleteOptions{})
		if err != nil {
			return err
		}
		return nil
	}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
}
