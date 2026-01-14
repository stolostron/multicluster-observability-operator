// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("", func() {
	var (
		deleteDelay              = "48h"
		retentionInLocal         = "24h"
		blockDuration            = "2h"
		ignoreDeletionMarksDelay = "24h"
	)

	BeforeEach(func() {
		hubClient = utils.NewKubeClient(
			testOptions.HubCluster.ClusterServerURL,
			testOptions.KubeConfig,
			testOptions.HubCluster.KubeContext)
		dynClient = utils.NewKubeClientDynamic(
			testOptions.HubCluster.ClusterServerURL,
			testOptions.KubeConfig,
			testOptions.HubCluster.KubeContext)

		mcoRes, err := dynClient.Resource(utils.NewMCOGVRV1BETA2()).
			Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
		if err != nil {
			panic(err.Error())
		}

		if _, adv := mcoRes.Object["spec"].(map[string]any)["advanced"]; adv {
			if _, rec := mcoRes.Object["spec"].(map[string]any)["advanced"].(map[string]any)["retentionConfig"]; rec {
				for k, v := range mcoRes.Object["spec"].(map[string]any)["advanced"].(map[string]any)["retentionConfig"].(map[string]any) {
					switch k {
					case "deleteDelay":
						deleteDelay = reflect.ValueOf(v).String()
						idmk, _ := strconv.Atoi(deleteDelay[:len(deleteDelay)-1])
						ignoreDeletionMarksDelay = fmt.Sprintf(
							"%.f",
							math.Ceil(float64(idmk)/float64(2)),
						) + deleteDelay[len(deleteDelay)-1:]
					case "retentionInLocal":
						retentionInLocal = reflect.ValueOf(v).String()
					case "blockDuration":
						blockDuration = reflect.ValueOf(v).String()
					}
				}
			}
		}
	})

	It(
		"RHACM4K-2881: Observability: Check and tune backup retention settings in MCO CR - Check compact args [P2][Sev2][Observability][Stable] @e2e @post-release @post-upgrade @post-restore (retention/g0):",
		func() {
			By("--delete-delay=" + deleteDelay)
			Eventually(func() error {
				compacts, err := hubClient.AppsV1().StatefulSets(MCO_NAMESPACE).List(context.TODO(), metav1.ListOptions{
					LabelSelector: THANOS_COMPACT_LABEL,
				})
				if err != nil {
					return err
				}
				argList := (*compacts).Items[0].Spec.Template.Spec.Containers[0].Args
				if slices.Contains(argList, "--delete-delay="+deleteDelay) {
					return nil
				}
				return fmt.Errorf("Failed to check compact args: --delete-delay="+deleteDelay+". args is %v", argList)
			}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*5).Should(Succeed())
		},
	)

	It(
		"RHACM4K-2881: Observability: Check and tune backup retention settings in MCO CR - Check store args [P2][Sev2][Observability][Stable] @e2e @post-release @post-upgrade @post-restore (retention/g0):",
		func() {
			By("--ignore-deletion-marks-delay=" + ignoreDeletionMarksDelay)
			Eventually(func() error {
				stores, err := hubClient.AppsV1().StatefulSets(MCO_NAMESPACE).List(context.TODO(), metav1.ListOptions{
					LabelSelector: THANOS_STORE_LABEL,
				})
				if err != nil {
					return err
				}
				argList := (*stores).Items[0].Spec.Template.Spec.Containers[0].Args
				if slices.Contains(argList, "--ignore-deletion-marks-delay="+ignoreDeletionMarksDelay) {
					return nil
				}
				return fmt.Errorf(
					"Failed to check store args: --ignore-deletion-marks-delay="+ignoreDeletionMarksDelay+". The args is: %v",
					argList,
				)
			}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*5).Should(Succeed())
		},
	)

	It(
		"RHACM4K-2881: Observability: Check and tune backup retention settings in MCO CR - Check receive args [P2][Sev2][Observability][Stable] @e2e @post-release @post-upgrade @post-restore (retention/g0):",
		func() {
			By("--tsdb.retention=" + retentionInLocal)
			Eventually(func() error {
				receives, err := hubClient.AppsV1().StatefulSets(MCO_NAMESPACE).List(context.TODO(), metav1.ListOptions{
					LabelSelector: THANOS_RECEIVE_LABEL,
				})
				if err != nil {
					return err
				}
				argList := (*receives).Items[0].Spec.Template.Spec.Containers[0].Args
				if slices.Contains(argList, "--tsdb.retention="+retentionInLocal) {
					return nil
				}
				return fmt.Errorf(
					"Failed to check receive args: --tsdb.retention="+retentionInLocal+". The args is: %v",
					argList,
				)
			}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*5).Should(Succeed())
		},
	)

	It(
		"RHACM4K-2881: Observability: Check and tune backup retention settings in MCO CR - Check rule args [P2][Sev2][Observability][Stable] @e2e @post-release @post-upgrade @post-restore (retention/g0):",
		func() {
			By("--tsdb.retention=" + retentionInLocal)
			Eventually(func() error {
				rules, err := hubClient.AppsV1().StatefulSets(MCO_NAMESPACE).List(context.TODO(), metav1.ListOptions{
					LabelSelector: THANOS_RULE_LABEL,
				})
				if err != nil {
					return err
				}
				argList := (*rules).Items[0].Spec.Template.Spec.Containers[0].Args
				if slices.Contains(argList, "--tsdb.retention="+retentionInLocal) {
					return nil
				}
				return fmt.Errorf(
					"Failed to check rule args: --tsdb.retention="+retentionInLocal+". The args is: %v",
					argList,
				)
			}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*5).Should(Succeed())
		},
	)

	It(
		"RHACM4K-2881: Observability: Check and tune backup retention settings in MCO CR - Check rule args [P2][Sev2][Observability][Stable] @e2e @post-release @post-upgrade @post-restore (retention/g0):",
		func() {
			By("--tsdb.block-duration=" + blockDuration)
			Eventually(func() error {
				rules, err := hubClient.AppsV1().StatefulSets(MCO_NAMESPACE).List(context.TODO(), metav1.ListOptions{
					LabelSelector: THANOS_RULE_LABEL,
				})
				if err != nil {
					return err
				}
				argList := (*rules).Items[0].Spec.Template.Spec.Containers[0].Args
				if slices.Contains(argList, "--tsdb.block-duration="+blockDuration) {
					return nil
				}
				return fmt.Errorf(
					"Failed to check rule args: --tsdb.block-duration="+blockDuration+". The args is: %v",
					argList,
				)
			}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*5).Should(Succeed())
		},
	)

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
