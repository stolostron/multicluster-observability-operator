// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"bytes"
	"fmt"
	"gopkg.in/yaml.v2"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/klog"

	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
)

var _ = Describe("Observability:", func() {
	It("RHACM4K-1406 - Observability - RBAC - only authorized user could query managed cluster metrics data [Observability][Integration]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release (obs_rbac/g0)", func() {
		By("Setting up users creation and rolebindings for RBAC", func() {
			cmd := exec.Command("../../../tools/setup_rbac-test.sh")
			var out bytes.Buffer
			cmd.Stdout = &out
			err := cmd.Run()
			klog.V(1).Infof("the output of setup_rbac_test.sh: %v", out.String())
			Expect(err).NotTo(HaveOccurred())
		})
		By("Logging in as admin and querying managed cluster metrics data", func() {

			grafanaConsoleURL := utils.GetGrafanaURL(testOptions)
			path := "/api/datasources/proxy/uid/000000001/api/v1/query?"
			query := fmt.Sprintf("node_memory_MemAvailable_bytes{cluster=*}")
			queryParams := url.PathEscape(fmt.Sprintf("query=%s", query))
			req, err := http.NewRequest(
				"GET",
				grafanaConsoleURL+path+queryParams,
				nil)
			Expect(err).NotTo(HaveOccurred())

			adminToken := os.Getenv("ADMIN_TOKEN")
			req.Header.Set("Authorization", "Bearer "+adminToken)

			client := &http.Client{}
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			respBody, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			metricResult := utils.GrafanaResponse{}
			err = yaml.Unmarshal(respBody, &metricResult)
			Expect(err).NotTo(HaveOccurred())
			Expect(metricResult.Status).To(Equal("success"))

		})
		By("Logging in as user1 and querying managed cluster metrics data", func() {
			// Query grafana with token from environment variable
			//user1Token := os.Getenv("USER1_TOKEN")
		})
		By("Logging in as user2 and querying managed cluster metrics data", func() {
			//cmd := exec.Command("oc login -u user2 -p user2")
			//var out bytes.Buffer
			//cmd.Stdout = &out
			//err := cmd.Run()
			//Expect(err).NotTo(HaveOccurred())
		})

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
