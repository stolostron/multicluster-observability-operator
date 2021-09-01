// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tests

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	"github.com/open-cluster-management/multicluster-observability-operator/tests/pkg/utils"
)

var _ = Describe("Observability: RHACK-6733: ", func() {
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

	configmap := [...]string{
		"observability-metrics-allowlist",
		"thanos-ruler-default-rules",
	}

	for i := range configmap {
		It("[P2][Sev2][Observability][Stable] Should have the expected configmap: "+configmap[i]+"(sli/g0)", func() {
			By("Checking if CM: " + configmap[i] + "is existed")
			cm, err := hubClient.CoreV1().ConfigMaps(MCO_NAMESPACE).Get(context.TODO(), configmap[i], metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			Expect(cm.ResourceVersion).ShouldNot(BeEmpty())
			klog.V(3).Infof("Configmap %s does exist", configmap[i])
		})
	}

	// Check to see if recording rule metric is in the observability-metrics-allowlist
	// It("[P2][Sev2][Observability][Integration] Should verify SLI recording rule exist within metrics allowlist (sli/g0)", func() {

	// })

	It("[P2][Sev2][Observability][Integration] Should have recording rule data in grafana console (sli/g0)", func() {
		Eventually(func() error {
			clusters, err := utils.ListManagedClusters(testOptions)
			if err != nil {
				return err
			}
			for _, cluster := range clusters {
				query := fmt.Sprintf("sli:apiserver_request_duration_seconds:trend:5m{cluster=\"%s\"}", cluster)
				err, _ = utils.ContainManagedClusterMetric(testOptions, query, []string{`"__name__":"sli:apiserver_request_duration_seconds:trend:5m"`})
				if err != nil {
					return err
				}
			}
			return nil
		}, EventuallyTimeoutMinute*6, EventuallyIntervalSecond*5).Should(Succeed())
	})

	// Check to see if recording rule metric is in the thanos-ruler-default-rules configmap
	// It("[P2][Sev2][Observability][Integration] Should verify SLI recording rule exist in thanos-ruler-default-rules configmap (sli/g0)", func() {

	// })

	// Check to see if recording rule metric is able to be triggered as an alert
	// It("[P2][Sev2][Observability][Integration] Should verify SLI recording rule can be triggered as an alert (sli/g0)", func() {

	// })
})
