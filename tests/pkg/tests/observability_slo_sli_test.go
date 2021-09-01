// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tests

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	"github.com/open-cluster-management/multicluster-observability-operator/tests/pkg/utils"
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

	configmap := [...]string{
		"observability-metrics-allowlist",
		"thanos-ruler-default-rules",
	}

	// Done - Checking to see if configmaps are available.
	for i := range configmap {
		It("[P2][Sev2][Observability][Stable] Should have the expected configmap: "+configmap[i]+"(slo/g0)", func() {
			By("Checking if CM: " + configmap[i] + "is existed")
			cm, err := hubClient.CoreV1().ConfigMaps(MCO_NAMESPACE).Get(context.TODO(), configmap[i], metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			Expect(cm.ResourceVersion).ShouldNot(BeEmpty())
			klog.V(3).Infof("Configmap %s does exist", configmap[i])
		})
	}
})
