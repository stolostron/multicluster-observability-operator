// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
	"k8s.io/klog/v2"
)

var _ = Describe("", Ordered, func() {
	BeforeAll(func() {
		// This test is currently not working well on shared enviroments
		// as the test setup script does not take into account that
		// others might add similar htaccess identity providers.
		// Therefore we skip it for now on the QE test envs which
		// are shared with other teams.
		cloudProvider := strings.ToLower(os.Getenv("CLOUD_PROVIDER"))
		if len(cloudProvider) > 0 {
			Skip("Skipping RBAC test on QE Jenkins test-run")
		}

		cmd := exec.Command("../../setup_rbac_test.sh")
		var out bytes.Buffer
		cmd.Stdout = &out
		err = cmd.Run()
		klog.V(1).Infof("the output of setup_rbac_test.sh: %v", out.String())
		Expect(err).To(BeNil())
		time.Sleep(2 * time.Minute)
	})
	It(
		"RHACM4K-1406 - Observability - RBAC - only authorized user could query managed cluster metrics data [Observability][Integration]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release (requires-ocp/g0) (obs_rbac/g0)",
		func() {
			By("Logging in as admin and querying managed cluster metrics data", func() {
				Eventually(func() error {
					err = utils.LoginOCUser(testOptions, "admin", "admin")
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
					if len(res.Data.Result) == 0 {
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
		},
	)

	It(
		"RHACM4K-1439 - Observability - RBAC - Verify only cluster-manager-admin role can deploy MCO CR [Observability][Integration]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release (requires-ocp/g0) (obs_rbac/g0)",
		func() {
			By("Logging as kube:admin checking if MCO can be deleted by user1 and admin", func() {
				Eventually(func() error {
					if len(testOptions.HubCluster.KubeContext) > 0 {
						_, err = exec.Command("oc", "config", "use-context", testOptions.HubCluster.KubeContext).CombinedOutput()
						if err != nil {
							return fmt.Errorf("Unable to log in as kube:admin after rbac test using kube-context: %v", err)
						}
					} else {
						user := os.Getenv("OC_CLUSTER_USER")
						password := os.Getenv("OC_HUB_CLUSTER_PASS")
						err = utils.LoginOCUser(testOptions, user, password)
						if err != nil {
							return fmt.Errorf("Unable to log in as kube:admin after rbac test using username/pw: %v", err)
						}
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
					if !bytes.Contains(out.Bytes(), []byte("admin")) {
						return fmt.Errorf("admin can't delete multiclusterobservabilities.observability.open-cluster-management.io CR")
					}
					return nil
				}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*5).Should(Succeed())
			})
		},
	)

	AfterEach(func() {
		if CurrentSpecReport().State.Is(types.SpecStateSkipped) {
			return
		}
		// make sure we login as kube admin again
		if len(testOptions.HubCluster.KubeContext) > 0 {
			_, err = exec.Command("oc", "config", "use-context", testOptions.HubCluster.KubeContext).CombinedOutput()
			if err != nil {
				klog.Error("Unable to log in as kube:admin after rbac test using kube-context", err)
			}
		} else {
			user := os.Getenv("OC_CLUSTER_USER")
			password := os.Getenv("OC_HUB_CLUSTER_PASS")
			err = utils.LoginOCUser(testOptions, user, password)
			if err != nil {
				klog.Error("Unable to log in as kube:admin after rbac test using username/pw", err)
			}
		}

		os.Unsetenv("USER_TOKEN")
		if CurrentSpecReport().Failed() {
			utils.LogFailingTestStandardDebugInfo(testOptions)
		}
		testFailed = testFailed || CurrentSpecReport().Failed()
	})
})
