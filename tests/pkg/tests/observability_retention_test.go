// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tests

import (
	"fmt"
	"reflect"
	"strconv"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/open-cluster-management/observability-e2e-test/pkg/utils"
)

var _ = Describe("Observability:", func() {

	var (
		deleteDelay              = "48h"
		retentionInLocal         = "24h"
		blockDuration            = "2h"
		ignoreDeletionMarksDelay = ""
	)

	BeforeEach(func() {
		hubClient = utils.NewKubeClient(
			testOptions.HubCluster.MasterURL,
			testOptions.KubeConfig,
			testOptions.HubCluster.KubeContext)
		dynClient = utils.NewKubeClientDynamic(
			testOptions.HubCluster.MasterURL,
			testOptions.KubeConfig,
			testOptions.HubCluster.KubeContext)

		mcoRes, err := dynClient.Resource(utils.NewMCOGVRV1BETA2()).Get(MCO_CR_NAME, metav1.GetOptions{})
		if err != nil {
			panic(err.Error())
		}
		if _, adv := mcoRes.Object["spec"].(map[string]interface{})["advanced"]; adv {
			if _, rec := mcoRes.Object["spec"].(map[string]interface{})["advanced"].(map[string]interface{})["retentionConfig"]; rec {
				for k, v := range mcoRes.Object["spec"].(map[string]interface{})["advanced"].(map[string]interface{})["retentionConfig"].(map[string]interface{}) {
					switch k {
					case "deleteDelay":
						deleteDelay = reflect.ValueOf(v).String()
					case "retentionInLocal":
						retentionInLocal = reflect.ValueOf(v).String()
					case "blockDuration":
						blockDuration = reflect.ValueOf(v).String()
					}
				}
			}
		}
		idmk, err := strconv.Atoi(deleteDelay[:len(deleteDelay)-1])
		if err != nil {
			panic(err.Error())
		}
		ignoreDeletionMarksDelay = strconv.Itoa(idmk/2) + deleteDelay[len(deleteDelay)-1:]
	})

	It("[P2][Sev2][Observability][Stable] Check compact args: --delete-delay="+deleteDelay+" (retention/g0)", func() {
		Eventually(func() error {
			name := MCO_CR_NAME + "-thanos-compact"
			compact, err := hubClient.AppsV1().StatefulSets(MCO_NAMESPACE).Get(name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			argList := compact.Spec.Template.Spec.Containers[0].Args
			for _, arg := range argList {
				if arg == "--delete-delay="+deleteDelay {
					return nil
				}
			}
			return fmt.Errorf("Failed to check compact args: --delete-delay="+deleteDelay+". args is %v", argList)
		}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*5).Should(Succeed())
	})

	It("[P2][Sev2][Observability][Stable] Check store args: --ignore-deletion-marks-delay="+ignoreDeletionMarksDelay+" (retention/g0)", func() {
		Eventually(func() error {
			name := MCO_CR_NAME + "-thanos-store-shard-0"
			store, err := hubClient.AppsV1().StatefulSets(MCO_NAMESPACE).Get(name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			argList := store.Spec.Template.Spec.Containers[0].Args
			for _, arg := range argList {
				if arg == "--ignore-deletion-marks-delay="+ignoreDeletionMarksDelay {
					return nil
				}
			}
			return fmt.Errorf("Failed to check store args: --ignore-deletion-marks-delay="+ignoreDeletionMarksDelay+". args is %v", argList)
		}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*5).Should(Succeed())
	})

	It("[P2][Sev2][Observability][Stable] Check receive args: --tsdb.retention="+retentionInLocal+" (retention/g0)", func() {
		Eventually(func() error {
			name := MCO_CR_NAME + "-thanos-receive-default"
			receive, err := hubClient.AppsV1().StatefulSets(MCO_NAMESPACE).Get(name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			argList := receive.Spec.Template.Spec.Containers[0].Args
			for _, arg := range argList {
				if arg == "--tsdb.retention="+retentionInLocal {
					return nil
				}
			}
			return fmt.Errorf("Failed to check receive args: --tsdb.retention="+retentionInLocal+". args is %v", argList)
		}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*5).Should(Succeed())
	})

	It("[P2][Sev2][Observability][Stable] Check rule args: --tsdb.retention="+retentionInLocal+" (retention/g0)", func() {
		Eventually(func() error {
			name := MCO_CR_NAME + "-thanos-rule"
			rule, err := hubClient.AppsV1().StatefulSets(MCO_NAMESPACE).Get(name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			argList := rule.Spec.Template.Spec.Containers[0].Args
			for _, arg := range argList {
				if arg == "--tsdb.retention="+retentionInLocal {
					return nil
				}
			}
			return fmt.Errorf("Failed to check rule args: --tsdb.retention="+retentionInLocal+". args is %v", argList)
		}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*5).Should(Succeed())
	})

	It("[P2][Sev2][Observability][Stable] Check rule args: --tsdb.block-duration="+blockDuration+" (retention/g0)", func() {
		Eventually(func() error {
			name := MCO_CR_NAME + "-thanos-rule"
			rule, err := hubClient.AppsV1().StatefulSets(MCO_NAMESPACE).Get(name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			argList := rule.Spec.Template.Spec.Containers[0].Args
			for _, arg := range argList {
				if arg == "--tsdb.block-duration="+blockDuration {
					return nil
				}
			}
			return fmt.Errorf("Failed to check rule args: --tsdb.block-duration="+blockDuration+". args is %v", argList)
		}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*5).Should(Succeed())
	})

	AfterEach(func() {
		if CurrentGinkgoTestDescription().Failed {
			utils.PrintMCOObject(testOptions)
			utils.PrintAllMCOPodsStatus(testOptions)
			utils.PrintAllOBAPodsStatus(testOptions)
		} else {
			Expect(utils.IntegrityChecking(testOptions)).NotTo(HaveOccurred())
		}
		testFailed = testFailed || CurrentGinkgoTestDescription().Failed
	})
})
