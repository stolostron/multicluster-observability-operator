// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tests

import (
	"bytes"
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/klog"

	"github.com/open-cluster-management/multicluster-observability-operator/tests/pkg/utils"
)

var _ = Describe("Observability:", func() {

	// Do not need to run this case in canary environment
	// If we really need it in canary, ensure the grafana-dev-test.sh is available in observability-e2e-test image and all required commands exist
	It("[P1][Sev1][Observability][Integration] Should run grafana-dev test successfully (grafana-dev/g0)", func() {
		Skip("Skip grafana-dev temporarily. need more tests")
		cmd := exec.Command("../../cicd-scripts/grafana-dev-test.sh")
		var out bytes.Buffer
		cmd.Stdout = &out
		err := cmd.Run()
		if err != nil {
			klog.V(1).Infof("Failed to run grafana-dev-test.sh: %v", out.String())
		}
		Expect(err).NotTo(HaveOccurred())
	})

	JustAfterEach(func() {
		Eventually(func() error {
			return utils.IntegrityChecking(testOptions)
			// alertmanager takes 4 minutes to start, so we need to set a timeout for IntegrityChecking
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
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
