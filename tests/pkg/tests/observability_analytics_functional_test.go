// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"context"
	"flag"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
)

var k8sClient client.Client

func init() {
	if flag.Lookup("kubeconfig") == nil {
		var kubeconfig string
		flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to the kubeconfig file")
	}
}

var _ = Describe("RHACM4K-XXXXX: Analytics Right-Sizing Functional Test [P1][Observability][Analytics] @e2e", Ordered, func() {
	const (
		mcoNamespace    = "open-cluster-management"
		mcoCRName       = "open-cluster-management-observability"
		rsConfigMapName = "rs-namespace-config"
		promRuleName    = "acm-rs-namespace-prometheus-rules"
		policyName      = "rs-prom-rules-policy"
	)

	expectedRecords := []string{
		"acm_managed_cluster_labels",
		"acm_rs:namespace:cpu_request_hard",
		"acm_rs:namespace:cpu_request",
		"acm_rs:namespace:cpu_usage",
		"acm_rs:namespace:cpu_recommendation",
		"acm_rs:namespace:memory_request_hard",
		"acm_rs:namespace:memory_request",
		"acm_rs:namespace:memory_usage",
		"acm_rs:namespace:memory_recommendation",
		"acm_rs:cluster:cpu_request",
		"acm_rs:cluster:cpu_usage",
		"acm_rs:cluster:cpu_recommendation",
		"acm_rs:cluster:memory_request",
		"acm_rs:cluster:memory_usage",
		"acm_rs:cluster:memory_recommendation",
	}

	BeforeAll(func() {
		cfg := ctrl.GetConfigOrDie()
		var err error
		k8sClient, err = client.New(cfg, client.Options{})
		Expect(err).NotTo(HaveOccurred())

		_ = hubClient.CoreV1().ConfigMaps(mcoNamespace).Delete(context.TODO(), rsConfigMapName, metav1.DeleteOptions{})
		_ = dynClient.Resource(utils.NewMCOGVRV1BETA2()).Namespace(mcoNamespace).Delete(context.TODO(), mcoCRName, metav1.DeleteOptions{})

		mco := map[string]interface{}{
			"apiVersion": "observability.open-cluster-management.io/v1beta2",
			"kind":       "MultiClusterObservability",
			"metadata": map[string]interface{}{
				"name":      mcoCRName,
				"namespace": mcoNamespace,
			},
			"spec": map[string]interface{}{
				"enableDownsampling": true,
				"enableHubAlerting":  true,
				"capabilities": map[string]interface{}{
					"platform": map[string]interface{}{
						"analytics": map[string]interface{}{
							"namespaceRightSizingRecommendation": map[string]interface{}{
								"enabled": true,
							},
						},
					},
				},
			},
		}
		_, err = dynClient.Resource(utils.NewMCOGVRV1BETA2()).
			Namespace(mcoNamespace).
			Create(context.TODO(), &unstructured.Unstructured{Object: mco}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rsConfigMapName,
				Namespace: mcoNamespace,
			},
			Data: map[string]string{
				"config.yaml": `namespaceFilterCriteria:
  inclusionCriteria:
    - "default"
labelFilterCriteria:
  - label: "team"
    inclusionCriteria:
      - "platform"
recommendationPercentage: 80
placementConfiguration:
  placementRuleName: "acm-placement"`,
			},
		}
		_, err = hubClient.CoreV1().ConfigMaps(mcoNamespace).Create(context.TODO(), configMap, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		fmt.Println("✅ Created rs-namespace-config ConfigMap and MCO CR")
	})

	It("should create the PrometheusRule for namespace right-sizing with all expected records", func() {
		Eventually(func() error {
			var rule monitoringv1.PrometheusRule
			err := k8sClient.Get(context.TODO(), types.NamespacedName{
				Name:      promRuleName,
				Namespace: mcoNamespace,
			}, &rule)
			if err != nil {
				return err
			}
			found := map[string]bool{}
			for _, group := range rule.Spec.Groups {
				for _, r := range group.Rules {
					found[r.Record] = true
				}
			}
			for _, expected := range expectedRecords {
				if !found[expected] {
					return fmt.Errorf("missing expected rule record: %s", expected)
				}
			}
			return nil
		}, 3*time.Minute, 5*time.Second).Should(Succeed())
		fmt.Println("✅ All expected PrometheusRule records found")
	})

	It("should create the corresponding Policy for the PrometheusRule", func() {
		Eventually(func() error {
			var policy policyv1.Policy
			err := k8sClient.Get(context.TODO(), types.NamespacedName{
				Name:      policyName,
				Namespace: mcoNamespace,
			}, &policy)
			if err != nil {
				return err
			}
			if policy.Spec.RemediationAction == "" || len(policy.Spec.PolicyTemplates) == 0 {
				return fmt.Errorf("Policy %q is missing required spec fields", policyName)
			}
			return nil
		}, 3*time.Minute, 5*time.Second).Should(Succeed())
		fmt.Println("✅ Corresponding policy found and valid")
	})

	AfterAll(func() {
		_ = hubClient.CoreV1().ConfigMaps(mcoNamespace).Delete(context.TODO(), rsConfigMapName, metav1.DeleteOptions{})
		_ = dynClient.Resource(utils.NewMCOGVRV1BETA2()).Namespace(mcoNamespace).Delete(context.TODO(), mcoCRName, metav1.DeleteOptions{})
		fmt.Println("🧹 Cleaned up ConfigMap and MCO CR")
	})
})
