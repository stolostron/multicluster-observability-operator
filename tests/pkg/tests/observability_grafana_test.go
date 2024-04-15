// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
)

var _ = Describe("Observability:", func() {
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

	It("@BVT - [P1][Sev1][observability][Stable] Should have metric data in grafana console (grafana/g0)", func() {
		clusterName := utils.GetManagedClusterName(testOptions)
		fmt.Printf("Coleen deleting metrics-collector deployment for namespace : %s  cluster: %s\n", namespace, clusterName)
		Eventually(func() error {
			clusters, err := utils.ListManagedClusters(testOptions)
			if err != nil {
				return err
			}
			for _, cluster := range clusters {
				query := fmt.Sprintf("node_memory_MemAvailable_bytes{cluster=\"%s\"}", cluster)
				err, _ = utils.ContainManagedClusterMetric(
					testOptions,
					query,
					[]string{`"__name__":"node_memory_MemAvailable_bytes"`},
				)
				if err != nil {
					return err
				}
			}
			return nil
		}, EventuallyTimeoutMinute*6, EventuallyIntervalSecond*5).Should(Succeed())
	})

	JustAfterEach(func() {
		Expect(utils.IntegrityChecking(testOptions)).NotTo(HaveOccurred())
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
