// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
)

var _ = Describe("", func() {
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

	It("RHACM4K-1066: Observability: Verify Grafana - Should have metric data in grafana console @BVT - [P1][Sev1][Observability][Stable]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release (grafana/g0)", func() {
		Eventually(func() error {
			clusters, err := utils.ListManagedClusters(testOptions)
			if err != nil {
				return err
			}
			for _, cluster := range clusters {
				query := fmt.Sprintf("node_memory_MemAvailable_bytes{cluster=\"%s\"}", cluster.Name)
				res, err := utils.QueryGrafana(
					testOptions,
					query,
				)
				if err != nil {
					return err
				}

				if len(res.Data.Result) == 0 {
					return fmt.Errorf("no data found for %s", query)
				}
			}
			return nil
		}, EventuallyTimeoutMinute*6, EventuallyIntervalSecond*5).Should(Succeed())
	})

	It("RHACM4K-23537: Observability: Verify managed cluster labels in Grafana dashboards(2.7) - [P1][Sev1][Observability][Stable]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release (grafana/g1)", func() {
		Eventually(func() bool {
			clientDynamic := utils.GetKubeClientDynamic(testOptions, true)
			objs, err := clientDynamic.Resource(utils.NewOCMManagedClustersGVR()).List(context.TODO(), metav1.ListOptions{})
			if err != nil {
				klog.V(1).Infof("Get the managedcluster failed, The err is: %s\n", err)
			}

			for _, obj := range objs.Items {
				metadata := obj.Object["metadata"].(map[string]interface{})
				labels := metadata["labels"].(map[string]interface{})
				if labels["local-cluster"] == "true" {
					labels := metadata["labels"].(map[string]interface{})
					labels["autolabel"] = "grafanacm"
					klog.V(1).Infof("The cluster with new label: %s\n", labels)
					_, updateErr := clientDynamic.Resource(utils.NewOCMManagedClustersGVR()).Update(context.TODO(), &obj, metav1.UpdateOptions{})
					if updateErr != nil {
						klog.V(1).Infof("Update label failed, updateErr is : %s\n", updateErr)
					}
				}

			}

			var (
				errcm error
				cm    *v1.ConfigMap
			)
			errcm, cm = utils.GetConfigMap(
				testOptions,
				true,
				"observability-managed-cluster-label-allowlist",
				MCO_NAMESPACE,
			)
			if errcm != nil {
				klog.V(1).Infof("The errcm is: %s\n", errcm)
			}

			data, err := json.Marshal(cm)
			if err != nil {
				klog.V(1).Infof("The err is: %s\n", err)
			}
			if !strings.Contains(string(data), "autolabel") {
				klog.V(1).Infof("new managedcluster label autolabel is NOT added into configmap  observability-managed-cluster-label-allowlist")
				return false
			} else {
				klog.V(1).Infof("new managedcluster label autolabel is added into configmap  observability-managed-cluster-label-allowlist")
				return true
			}
		}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*10).Should(BeTrue())
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
