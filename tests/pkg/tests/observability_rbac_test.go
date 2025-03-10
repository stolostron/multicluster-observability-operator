// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"bytes"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/klog"

	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
)

var _ = Describe("Observability:", func() {
	It("RHACM4K-1406 - Observability - RBAC - only authorized user could query managed cluster metrics data [Observability][Integration]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release (obs_rbac/g0)", func() {
		By("Setting up users creation and rolebindings for RBAC", func() {
			cmd := exec.Command("../../setup_rbac_test.sh")
			var out bytes.Buffer
			cmd.Stdout = &out
			_ = cmd.Run()
			klog.V(1).Infof("the output of setup_rbac_test.sh: %v", out.String())
			//Expect(err).NotTo(HaveOccurred())
		})
		By("Logging in as admin and querying managed cluster metrics data", func() {
			err = utils.LoginOCUser(testOptions, "admin", "admin")
			Expect(err).NotTo(HaveOccurred())
			res, err := utils.QueryGrafana(testOptions, "node_memory_MemAvailable_bytes")
			Expect(err).NotTo(HaveOccurred())
			klog.V(1).Infof("the result of query: %v", res)

		})
		By("Logging in as user1 with view role and querying managed cluster metrics data", func() {
			err = utils.LoginOCUser(testOptions, "user1", "user1")
			Expect(err).NotTo(HaveOccurred())
			res, err := utils.QueryGrafana(testOptions, "node_memory_MemAvailable_bytes")
			Expect(err).NotTo(HaveOccurred())
			klog.V(1).Infof("the result of query: %v", res)
		})
		By("Logging in as user2 with edit role and querying managed cluster metrics data", func() {
			err = utils.LoginOCUser(testOptions, "user2", "user2")
			Expect(err).NotTo(HaveOccurred())
			res, err := utils.QueryGrafana(testOptions, "node_memory_MemAvailable_bytes")
			Expect(err).NotTo(HaveOccurred())
			klog.V(1).Infof("the result of query: %v", res)
		})

	})

	JustAfterEach(func() {
		Expect(utils.IntegrityChecking(testOptions)).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			//utils.LogFailingTestStandardDebugInfo(testOptions)
		}
		testFailed = testFailed || CurrentSpecReport().Failed()
	})
})
