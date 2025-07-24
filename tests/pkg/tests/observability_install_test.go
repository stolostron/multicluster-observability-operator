// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	"github.com/stolostron/multicluster-observability-operator/tests/pkg/kustomize"
	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
)

func installMCO() {
	if os.Getenv("SKIP_INSTALL_STEP") == trueStr {
		return
	}

	klog.V(5).Infof("Create kubeclient for url %s using kubeconfig path %s\n", testOptions.HubCluster.ClusterServerURL, testOptions.KubeConfig)
	hubClient := utils.NewKubeClient(
		testOptions.HubCluster.ClusterServerURL,
		testOptions.KubeConfig,
		testOptions.HubCluster.KubeContext)

	klog.V(5).Infof("Create kubeclient dynamic for url %s using kubeconfig path %s\n", testOptions.HubCluster.ClusterServerURL, testOptions.KubeConfig)
	dynClient := utils.NewKubeClientDynamic(
		testOptions.HubCluster.ClusterServerURL,
		testOptions.KubeConfig,
		testOptions.HubCluster.KubeContext)

	if os.Getenv("IS_KIND_ENV") != trueStr {
		By("Deploy CM cluster-monitoring-config")

		yamlBc, _ := kustomize.Render(
			kustomize.Options{KustomizationPath: "../../../examples/configmapcmc/cluster-monitoring-config"},
		)
		Expect(
			utils.ApplyRetryOnConflict(
				testOptions.HubCluster.ClusterServerURL,
				testOptions.KubeConfig,
				testOptions.HubCluster.KubeContext,
				yamlBc)).NotTo(HaveOccurred())
	}

	By("Checking MCO operator is started up and running")
	podList, err := hubClient.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{LabelSelector: MCO_LABEL})
	Expect(len(podList.Items)).To(Equal(1))
	Expect(err).NotTo(HaveOccurred())
	var (
		mcoPod = ""
		mcoNs  = ""
	)
	for _, pod := range podList.Items {
		mcoPod = pod.GetName()
		mcoNs = pod.GetNamespace()
		Expect(string(mcoPod)).NotTo(Equal(""))
		Expect(string(pod.Status.Phase)).To(Equal("Running"))
	}

	By("Checking Required CRDs are created")
	Eventually(func() error {
		return utils.HaveCRDs(testOptions.HubCluster, testOptions.KubeConfig,
			[]string{
				"multiclusterobservabilities.observability.open-cluster-management.io",
				"observatoria.core.observatorium.io",
				"observabilityaddons.observability.open-cluster-management.io",
			})
	}).Should(Succeed())

	Expect(utils.CreateMCONamespace(testOptions)).NotTo(HaveOccurred())
	if os.Getenv("IS_CANARY_ENV") == trueStr {
		Expect(utils.CreatePullSecret(testOptions, mcoNs)).NotTo(HaveOccurred())
		Expect(utils.CreateObjSecret(testOptions)).NotTo(HaveOccurred())
	} else {
		By("Creating Minio as object storage")
		//set resource quota and limit range for canary environment to avoid destruct the node
		yamlB, err := kustomize.Render(kustomize.Options{KustomizationPath: "../../../examples/minio"})
		Expect(err).NotTo(HaveOccurred())
		Expect(utils.ApplyRetryOnConflict(testOptions.HubCluster.ClusterServerURL, testOptions.KubeConfig, testOptions.HubCluster.KubeContext, yamlB)).NotTo(HaveOccurred())
	}

	//set resource quota and limit range for canary environment to avoid destruct the node
	yamlB, err := kustomize.Render(kustomize.Options{KustomizationPath: "../../../examples/policy"})
	Expect(err).NotTo(HaveOccurred())
	Expect(
		utils.ApplyRetryOnConflict(
			testOptions.HubCluster.ClusterServerURL,
			testOptions.KubeConfig,
			testOptions.HubCluster.KubeContext,
			yamlB,
		)).NotTo(HaveOccurred())

	By("Creating the MCO testing RBAC resources")
	Expect(utils.CreateMCOTestingRBAC(testOptions)).NotTo(HaveOccurred())

	if os.Getenv("SKIP_INTEGRATION_CASES") != trueStr {
		By("Creating MCO instance of v1beta1")
		v1beta1KustomizationPath := "../../../examples/mco/e2e/v1beta1"
		yamlB, err = kustomize.Render(kustomize.Options{KustomizationPath: v1beta1KustomizationPath})
		Expect(err).NotTo(HaveOccurred())
		Expect(
			utils.Apply(
				testOptions.HubCluster.ClusterServerURL,
				testOptions.KubeConfig,
				testOptions.HubCluster.KubeContext,
				yamlB,
			)).NotTo(HaveOccurred())

		By("Waiting for MCO ready status")
		allPodsIsReady := false
		Eventually(func() error {
			instance, err := dynClient.Resource(utils.NewMCOGVRV1BETA1()).
				Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
			if err == nil {
				allPodsIsReady = utils.StatusContainsTypeEqualTo(instance, "Ready")
				if allPodsIsReady {
					testFailed = false
					return nil
				}
			}
			testFailed = true
			if instance != nil && instance.Object != nil {
				return fmt.Errorf(
					"MCO componnets cannot be running in 20 minutes. check the MCO CR status for the details: %v",
					instance.Object["status"],
				)
			} else {
				return errors.New("Wait for reconciling.")
			}
		}, EventuallyTimeoutMinute*20, EventuallyIntervalSecond*5).Should(Succeed())

		By("Check clustermanagementaddon CR is created")
		Eventually(func() error {
			_, err := dynClient.Resource(utils.NewMCOClusterManagementAddonsGVR()).
				Get(context.TODO(), "observability-controller", metav1.GetOptions{})
			if err != nil {
				testFailed = true
				return err
			}
			testFailed = false
			return nil
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

		By("Check the api conversion is working as expected")
		v1beta1Tov1beta2GoldenPath := "../../../examples/mco/e2e/v1beta1/observability-v1beta1-to-v1beta2-golden.yaml"
		err = utils.CheckMCOConversion(testOptions, v1beta1Tov1beta2GoldenPath)
		Expect(err).NotTo(HaveOccurred())
	}

	if os.Getenv("IS_CANARY_ENV") != trueStr {
		By("Recreating Minio-tls as object storage")
		//set resource quota and limit range for canary environment to avoid destruct the node
		yamlB, err := kustomize.Render(kustomize.Options{KustomizationPath: "../../../examples/minio-tls"})
		Expect(err).NotTo(HaveOccurred())
		Expect(utils.ApplyRetryOnConflict(testOptions.HubCluster.ClusterServerURL, testOptions.KubeConfig, testOptions.HubCluster.KubeContext, yamlB)).NotTo(HaveOccurred())

		By("Apply MCO instance of v1beta2")
		v1beta2KustomizationPath := "../../../examples/mco/e2e/v1beta2/custom-certs"
		yamlB, err = kustomize.Render(kustomize.Options{KustomizationPath: v1beta2KustomizationPath})
		Expect(err).NotTo(HaveOccurred())
		// add retry for update mco object failure
		Eventually(func() error {
			return utils.ApplyRetryOnConflict(
				testOptions.HubCluster.ClusterServerURL,
				testOptions.KubeConfig,
				testOptions.HubCluster.KubeContext,
				yamlB,
			)
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*10).Should(Succeed())

	} else {
		By("Apply MCO instance of v1beta2")
		v1beta2KustomizationPath := "../../../examples/mco/e2e/v1beta2"
		yamlB, err = kustomize.Render(kustomize.Options{KustomizationPath: v1beta2KustomizationPath})
		Expect(err).NotTo(HaveOccurred())
		// add retry for update mco object failure
		Eventually(func() error {
			return utils.ApplyRetryOnConflict(
				testOptions.HubCluster.ClusterServerURL,
				testOptions.KubeConfig,
				testOptions.HubCluster.KubeContext,
				yamlB,
			)
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*10).Should(Succeed())
	}
	// wait for pod restarting
	time.Sleep(60 * time.Second)

	mcoTestFailed := false
	defer func() {
		if !mcoTestFailed {
			return
		}

		mcoLogs, err := utils.GetPodLogs(testOptions, true, mcoNs, mcoPod, "multicluster-observability-operator", false, 1000)
		Expect(err).NotTo(HaveOccurred())
		fmt.Fprintf(GinkgoWriter, "[DEBUG] MCO is installed failed, checking MCO operator logs:\n%s\n", mcoLogs)
		utils.LogFailingTestStandardDebugInfo(testOptions)

	}()
	By("Waiting for MCO ready status")
	Eventually(func() error {
		err = utils.CheckMCOComponents(testOptions)
		if err != nil {
			testFailed = true
			mcoTestFailed = true
			return err
		}
		fmt.Fprintf(GinkgoWriter, "[DEBUG] MCO is installed successfully!\n")
		testFailed = false
		mcoTestFailed = false
		return nil
	}, EventuallyTimeoutMinute*15, EventuallyIntervalSecond*20).Should(Succeed())

	obaTestFailed := false
	defer func() {
		if !obaTestFailed {
			return
		}

		fmt.Fprintf(GinkgoWriter, "[DEBUG] Addon failed, checking pods:\n")
		utils.LogFailingTestStandardDebugInfo(testOptions)
	}()
	By("Check endpoint-operator and metrics-collector pods are ready")
	Eventually(func() error {
		err = utils.CheckAllOBAsEnabled(testOptions)
		if err != nil {
			obaTestFailed = true
			testFailed = true
			return err
		}
		fmt.Fprintf(GinkgoWriter, "[DEBUG] Addon is installed successfully!\n")
		obaTestFailed = false
		testFailed = false
		return nil
	}, EventuallyTimeoutMinute*15, EventuallyIntervalSecond*20).Should(Succeed())

	By("Check clustermanagementaddon CR is created")
	Eventually(func() error {
		_, err := dynClient.Resource(utils.NewMCOClusterManagementAddonsGVR()).
			Get(context.TODO(), "observability-controller", metav1.GetOptions{})
		if err != nil {
			testFailed = true
			return err
		}
		testFailed = false
		return nil
	}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*10).Should(Succeed())

	Eventually(func() error {
		BearerToken, err = utils.FetchBearerToken(testOptions)
		if err != nil {
			klog.Errorf("fetch bearer token error: %v", err)
		}
		if BearerToken == "" {
			return fmt.Errorf("failed to get bearer token: %w", err)
		}
		return nil

	}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*10).Should(Succeed())
}
