// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
	"k8s.io/klog"
)

var _ = Describe("Observability:", Ordered, func() {
	BeforeAll(func() {
		cmd := exec.Command("../../setup_rbac_test.sh")
		var out bytes.Buffer
		cmd.Stdout = &out
		err = cmd.Run()
		Expect(err).To(BeNil())
		klog.V(1).Infof("the output of setup_rbac_test.sh: %v", out.String())
	})
	It("RHACM4K-1406 - Observability - RBAC - only authorized user could query managed cluster metrics data [Observability][Integration]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release (requires-ocp/g0) (obs_rbac/g0)", func() {
		By("Logging in as admin and querying managed cluster metrics data", func() {
			Eventually(func() error {
				err = utils.LoginOCUser(testOptions, "e2eadmin", "e2eadmin")
				if err != nil {
					klog.Errorf("Failed to login as admin: %v", err)
					return err
				}

				res, err := utils.QueryGrafana(testOptions, "node_memory_MemAvailable_bytes")
				if err != nil {
					return err
				}
				if len(res.Data.Result) == 0 {
					return fmt.Errorf("no data found for node_memory_MemAvailable_bytes")
				}
				return nil
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
		})
		By("Logging in as user1 with view role in local-cluster ns and querying metrics data", func() {
			Eventually(func() error {
				err = utils.LoginOCUser(testOptions, "user1", "user1")
				if err != nil {
					klog.Errorf("Failed to login as user1: %v", err)
					return err
				}

				res, err := utils.QueryGrafana(testOptions, "node_memory_MemAvailable_bytes{cluster=\"local-cluster\"}")
				if err != nil {
					return err
				}
				if len(res.Data.Result) != 1 {
					return fmt.Errorf("no data found for node_memory_MemAvailable_bytes{cluster=\"local-cluster\"}")
				}
				return nil
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
		})
		By("Logging in as user2 with no role binding to access managed cluster metrics data", func() {
			Eventually(func() error {
				err = utils.LoginOCUser(testOptions, "user2", "user2")
				if err != nil {
					klog.Errorf("Failed to login as user2: %v", err)
					return err
				}
				res, err := utils.QueryGrafana(testOptions, "node_memory_MemAvailable_bytes{cluster=\"local-cluster\"}")
				if err != nil {
					return err
				}
				if len(res.Data.Result) != 0 {
					return fmt.Errorf("data found for node_memory_MemAvailable_bytes{cluster=\"local-cluster\"}")
				}
				return nil
			}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
		})

	})

	It("RHACM4K-1439 - Observability - RBAC - Verify only cluster-manager-admin role can deploy MCO CR [Observability][Integration]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release (requires-ocp/g0) (obs_rbac/g0)", func() {
		By("Logging as kube:admin checking if MCO can be deleted by user1 and e2eadmin", func() {
			Eventually(func() error {
				_, err = exec.Command("oc", "config", "use-context", testOptions.HubCluster.KubeContext).CombinedOutput()
				if err != nil {
					return err
				}

				cmd := exec.Command("oc", "policy", "who-can", "delete", "mco")
				var out bytes.Buffer
				cmd.Stdout = &out
				err = cmd.Run()
				if err != nil {
					return err
				}
				if bytes.Contains(out.Bytes(), []byte("user1")) {
					return fmt.Errorf("user1 can delete multiclusterobservabilities.observability.open-cluster-management.io CR")
				}
				if !bytes.Contains(out.Bytes(), []byte("e2eadmin")) {
					return fmt.Errorf("e2eadmin can't delete multiclusterobservabilities.observability.open-cluster-management.io CR")
				}
				return nil
			}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*5).Should(Succeed())
		})
	})

	AfterEach(func() {
		os.Unsetenv("USER_TOKEN")
		_, _ = exec.Command("oc", "config", "use-context", testOptions.HubCluster.KubeContext).CombinedOutput()
		if CurrentSpecReport().Failed() {
			utils.LogFailingTestStandardDebugInfo(testOptions)
		}
		testFailed = testFailed || CurrentSpecReport().Failed()
	})
})
