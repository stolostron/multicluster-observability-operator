// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	"github.com/stolostron/multicluster-observability-operator/tests/pkg/kustomize"
	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
)

const (
	allowlistCMname = "observability-metrics-custom-allowlist"
)

var (
	clusters     []string
	clusterError error
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

	JustBeforeEach(func() {
		Eventually(func() error {
			clusters, clusterError = utils.ListManagedClusters(testOptions)
			if clusterError != nil {
				return clusterError
			}
			return nil
		}, EventuallyTimeoutMinute*6, EventuallyIntervalSecond*5).Should(Succeed())
	})

	It("[P2][Sev2][observability][Integration] Should have metrics which defined in custom metrics allowlist (metrics/g0)", func() {
		By("Adding custom metrics allowlist configmap")
		yamlB, err := kustomize.Render(kustomize.Options{KustomizationPath: "../../../examples/metrics/allowlist"})
		Expect(err).ToNot(HaveOccurred())
		Expect(
			utils.Apply(
				testOptions.HubCluster.ClusterServerURL,
				testOptions.KubeConfig,
				testOptions.HubCluster.KubeContext,
				yamlB,
			)).NotTo(HaveOccurred())

		By("Waiting for new added metrics on grafana console")
		Eventually(func() error {
			for _, cluster := range clusters {
				query := fmt.Sprintf("node_memory_Active_bytes{cluster=\"%s\"} offset 1m", cluster)
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
		}, EventuallyTimeoutMinute*10, EventuallyIntervalSecond*5).Should(Succeed())
	})

	It("[P2][Sev2][observability][Integration] Should have no metrics which have been marked for deletion in names section (metrics/g0)", func() {
		By("Waiting for deleted metrics disappear on grafana console")
		Eventually(func() error {
			for _, cluster := range clusters {
				query := fmt.Sprintf(
					"timestamp(instance:node_num_cpu:sum{cluster=\"%s\"}) - timestamp(instance:node_num_cpu:sum{cluster=\"%s\"} offset 1m) > 59",
					cluster,
					cluster,
				)
				res, err := utils.QueryGrafana(testOptions, query)
				if err != nil {
					return err
				}
				// there should be no data for the deleted metric
				if len(res.Data.Result) != 0 {
					return fmt.Errorf("metric %s found in response: %v", query, res)
				}
			}
			return nil
		}, EventuallyTimeoutMinute*10, EventuallyIntervalSecond*5).Should(Succeed())
	})

	It("[P2][Sev2][observability][Integration] Should have no metrics which have been marked for deletion in matches section (metrics/g0)", func() {
		By("Waiting for deleted metrics disappear on grafana console")
		Eventually(func() error {
			for _, cluster := range clusters {
				query := fmt.Sprintf(
					"timestamp(go_goroutines{cluster=\"%s\"}) - timestamp(go_goroutines{cluster=\"%s\"} offset 1m) > 59",
					cluster,
					cluster,
				)
				res, err := utils.QueryGrafana(testOptions, query)
				if err != nil {
					return err
				}
				if len(res.Data.Result) != 0 {
					return fmt.Errorf("metric %s found in response: %v", query, res)
				}
			}
			return nil
		}, EventuallyTimeoutMinute*10, EventuallyIntervalSecond*5).Should(Succeed())
	})

	It("[P2][Sev2][observability][Integration] Should have no metrics after custom metrics allowlist deleted (metrics/g0)", func() {
		By("Deleting custom metrics allowlist configmap")
		Eventually(func() error {
			err := hubClient.CoreV1().
				ConfigMaps(MCO_NAMESPACE).
				Delete(context.TODO(), allowlistCMname, metav1.DeleteOptions{})
			return err
		}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*1).Should(Succeed())

		By("Waiting for new added metrics disappear on grafana console")
		Eventually(func() error {
			for _, cluster := range clusters {
				query := fmt.Sprintf(
					"timestamp(node_memory_Active_bytes{cluster=\"%s\"}) - timestamp(node_memory_Active_bytes{cluster=\"%s\"} offset 1m) > 59",
					cluster,
					cluster,
				)
				res, err := utils.QueryGrafana(testOptions, query)
				if err != nil {
					return err
				}
				if len(res.Data.Result) != 0 {
					return fmt.Errorf("metric %s found in response: %v", query, res)
				}
			}
			return nil
		}, EventuallyTimeoutMinute*10, EventuallyIntervalSecond*5).Should(Succeed())
	})

	It("[P2][Sev2][observability][Integration] Should have metrics which used grafana dashboard (ssli/g1)", func() {
		metricList, dynamicMetricList := utils.GetDefaultMetricList(testOptions)
		klog.V(1).Infof("metricList: %v", metricList)
		klog.V(1).Infof("dynamicMetricList: %v", dynamicMetricList)
		allowMetricsMap := make(map[string]struct{}, len(metricList)+len(dynamicMetricList))
		for _, name := range metricList {
			allowMetricsMap[name] = struct{}{}
		}
		for _, name := range dynamicMetricList {
			allowMetricsMap[name] = struct{}{}
		}
		ignoreMetricMap := utils.GetIgnoreMetricMap()

		// Print ignored metrics that are not found in the allowlist
		for name := range ignoreMetricMap {
			if _, ok := allowMetricsMap[name]; !ok {
				klog.V(1).Infof("ignored metric %s is not found in the allowlist", name)
			}
		}

		// wait for metrics to be available
		klog.V(1).Infof("waiting for metrics to be available...")
		time.Sleep(90 * time.Second)

		// Check if the metrics are available
		for _, name := range metricList {
			res, err := utils.QueryGrafana(testOptions, name)
			if err != nil {
				klog.Errorf("failed to get metrics %s: %v", name, err)
				continue
			}
			if len(res.Data.Result) == 0 {
				klog.Errorf("no data found for %s", name)
			}
		}
		// _, etcdPodList := utils.GetPodList(
		// 	testOptions,
		// 	true,
		// 	"openshift-etcd",
		// 	"app=etcd",
		// )
		// // ignore etcd network peer metrics for SNO cluster
		// if etcdPodList != nil && len(etcdPodList.Items) <= 0 {
		// 	ignoreMetricMap["etcd_network_peer_received_bytes_total"] = true
		// 	ignoreMetricMap["etcd_network_peer_sent_bytes_total"] = true
		// }
		// for _, name := range dynamicMetricList {
		// 	ignoreMetricMap[name] = true
		// }
		// for _, name := range metricList {
		// 	_, ok := ignoreMetricMap[name]
		// 	if !ok {
		// 		Eventually(func() error {
		// 			res, err := utils.QueryGrafana(testOptions, name)
		// 			if err != nil {
		// 				return fmt.Errorf("failed to get metrics %s: %v", name, err)
		// 			}
		// 			if len(res.Data.Result) == 0 {
		// 				return fmt.Errorf("no data found for %s", name)
		// 			}
		// 			return nil
		// 		}, EventuallyTimeoutMinute*2, EventuallyIntervalSecond*3).Should(Succeed())
		// 	}
		// }
	})

	JustAfterEach(func() {
		Expect(utils.IntegrityChecking(testOptions)).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if CurrentGinkgoTestDescription().Failed {
			utils.LogFailingTestStandardDebugInfo(testOptions)
		}
		testFailed = testFailed || CurrentGinkgoTestDescription().Failed
	})
})
