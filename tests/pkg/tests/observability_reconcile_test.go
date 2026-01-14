// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

var (
	EventuallyTimeoutMinute  time.Duration = 60 * time.Second
	EventuallyIntervalSecond time.Duration = 1 * time.Second

	hubClient kubernetes.Interface
	dynClient dynamic.Interface
	err       error
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

	It("RHACM4K-2881: Observability: Check and tune backup retention settings in MCO CR - tune retention settings in MCO CR [P2][Sev2][Observability][Stable] @e2e @post-release @post-upgrade @post-restore (reconcile/g0)", func() {
		cloudProvider := strings.ToLower(os.Getenv("CLOUD_PROVIDER"))
		substring1 := "vmware"
		substring2 := "ibm"
		if strings.Contains(cloudProvider, substring1) || strings.Contains(cloudProvider, substring2) {
			Skip("Skip the case due to it's not supported on the VMWARE and IBM")
		}
		By("Modifying MCO CR for reconciling")
		err := utils.ModifyMCOCR(testOptions)
		Expect(err).ToNot(HaveOccurred())

		By("Waiting for MCO retentionResolutionRaw filed to take effect")
		advRetentionCon, err := utils.CheckAdvRetentionConfig(testOptions)
		if !advRetentionCon {
			Skip("Skip the case since " + err.Error())
		}
		Eventually(func() error {
			compacts, _ := hubClient.AppsV1().StatefulSets(MCO_NAMESPACE).List(context.TODO(), metav1.ListOptions{
				LabelSelector: THANOS_COMPACT_LABEL,
			})
			Expect(len(compacts.Items)).NotTo(Equal(0))

			argList := (*compacts).Items[0].Spec.Template.Spec.Containers[0].Args
			if slices.Contains(argList, "--retention.resolution-raw=3d") {
				return nil
			}
			return fmt.Errorf("Failed to find modified retention field, the current args is: %v", argList)
		}, EventuallyTimeoutMinute*10, EventuallyIntervalSecond*5).Should(Succeed())

		By("Wait for thanos compact pods are ready")
		compacts, _ := hubClient.AppsV1().StatefulSets(MCO_NAMESPACE).List(context.TODO(), metav1.ListOptions{
			LabelSelector: THANOS_COMPACT_LABEL,
		})
		Expect(len(compacts.Items)).NotTo(Equal(0))

		// ensure the thanos rule pods are restarted successfully before processing
		Eventually(func() error {
			err = utils.CheckStatefulSetPodReady(testOptions, (*compacts).Items[0].Name)
			if err != nil {
				return err
			}
			return nil
		}, EventuallyTimeoutMinute*10, EventuallyIntervalSecond*5).Should(Succeed())
	})

	It("RHACM4K-1655: Observability: Verify nodeSelector setting effects for Observability components [P2][Sev2][Observability][Stable] @e2e @post-release @post-upgrade @post-restore (reconcile/g0)", func() {
		By("Checking node selector spec in MCO CR")
		mcoSC, err := dynClient.Resource(utils.NewMCOGVRV1BETA2()).
			Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		spec := mcoSC.Object["spec"].(map[string]any)
		if _, ok := spec["nodeSelector"]; !ok {
			Skip("Skip the case since the MCO CR did not set the nodeSelector")
		}

		By("Checking node selector for all pods")
		Eventually(func() error {
			err = utils.CheckAllPodNodeSelector(testOptions, spec["nodeSelector"].(map[string]any))
			if err != nil {
				return err
			}
			return nil
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
	})

	It("RHACM4K-1657: Observability: Check affinity rule takes effect on Observability components [P2][Sev2][Observability][Stable] @e2e @post-release @post-upgrade @post-restore (reconcile/g0)", func() {
		By("Checking podAntiAffinity for all pods")
		Eventually(func() error {
			err := utils.CheckAllPodsAffinity(testOptions)
			if err != nil {
				return err
			}
			return nil
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
	})

	It("RHACM4K-2821: Observability: Customize the Observability components storage size [P2][Sev2][Observability][Stable] @e2e @post-release @post-upgrade @post-restore (reconcile/g0)", func() {
		cloudProvider := strings.ToLower(os.Getenv("CLOUD_PROVIDER"))
		substring1 := "vmware"
		substring2 := "ibm"
		if strings.Contains(cloudProvider, substring1) || strings.Contains(cloudProvider, substring2) {
			// if strings.Contains(string(os.Getenv("CLOUD_PROVIDER")), "VMWARE") {
			Skip("Skip the case due to it's not supported on the VMWARE and IBM")
		}
		By("Resizing alertmanager storage")
		alertmans, _ := hubClient.AppsV1().StatefulSets(MCO_NAMESPACE).List(context.TODO(), metav1.ListOptions{
			LabelSelector: ALERTMANAGER_LABEL,
		})
		Expect(len(alertmans.Items)).NotTo(Equal(0))

		Eventually(func() error {
			err := utils.CheckStorageResize(testOptions, (*alertmans).Items[0].Name, "3Gi")
			if err != nil {
				return err
			}
			return nil
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
	})

	It("RHACM4K-2881: Observability: Check and tune backup retention settings in MCO CR - Revert MCO CR changes [P2][Sev2][Observability][Stable] @e2e @post-release @post-upgrade @post-restore (reconcile/g0)", func() {
		cloudProvider := strings.ToLower(os.Getenv("CLOUD_PROVIDER"))
		substring1 := "vmware"
		substring2 := "ibm"
		if strings.Contains(cloudProvider, substring1) || strings.Contains(cloudProvider, substring2) {
			// if strings.Contains(string(os.Getenv("CLOUD_PROVIDER")), "VMWARE") {
			Skip("Skip the case due to it's not supported on the VMWARE and IBM")
		}
		advRetentionCon, err := utils.CheckAdvRetentionConfig(testOptions)
		if !advRetentionCon {
			Skip("Skip the case since " + err.Error())
		}
		By("Revert MCO CR changes")
		err = utils.RevertMCOCRModification(testOptions)
		Expect(err).ToNot(HaveOccurred())

		By("Waiting for MCO retentionResolutionRaw filed to take effect")
		Eventually(func() error {
			compacts, _ := hubClient.AppsV1().StatefulSets(MCO_NAMESPACE).List(context.TODO(), metav1.ListOptions{
				LabelSelector: THANOS_COMPACT_LABEL,
			})
			Expect(len(compacts.Items)).NotTo(Equal(0))

			argList := (*compacts).Items[0].Spec.Template.Spec.Containers[0].Args
			if slices.Contains(argList, "--retention.resolution-raw=6d") {
				return nil
			}
			return fmt.Errorf("Failed to find modified retention field, the current args is: %v", argList)
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

		By("Wait for thanos compact pods are ready")
		// ensure the thanos rule pods are restarted successfully before processing
		compacts, _ := hubClient.AppsV1().StatefulSets(MCO_NAMESPACE).List(context.TODO(), metav1.ListOptions{
			LabelSelector: THANOS_COMPACT_LABEL,
		})
		Expect(len(compacts.Items)).NotTo(Equal(0))

		Eventually(func() error {
			err = utils.CheckStatefulSetPodReady(testOptions, (*compacts).Items[0].Name)
			if err != nil {
				return err
			}
			return nil
		}, EventuallyTimeoutMinute*10, EventuallyIntervalSecond*5).Should(Succeed())

		By("Checking MCO components in default HA mode")
		Eventually(func() error {
			err = utils.CheckMCOComponents(testOptions)
			if err != nil {
				return err
			}
			return nil
		}, EventuallyTimeoutMinute*15, EventuallyIntervalSecond*5).Should(Succeed())
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
