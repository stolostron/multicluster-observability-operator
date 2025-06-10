// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"context"
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

var _ = Describe("RHACM4K-XXXXX: Analytics Right-Sizing Functional Test [P1][Observability][Analytics] @e2e", Ordered, func() {
	const (
		rsPrometheusRulePolicyName        = "rs-prom-rules-policy"
		rsPrometheusRuleName              = "acm-rs-namespace-prometheus-rules"
		rsConfigMapName                   = "rs-namespace-config"
		rsDefaultRecommendationPercentage = 110
		mcoCRName                         = "open-cluster-management-observability"
		mcoNamespace                      = "open-cluster-management"
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

		By("Patching the MCO CR to enable right-sizing analytics")
		Eventually(func() error {
			mco, err := dynClient.Resource(utils.NewMCOGVRV1BETA2()).Namespace(mcoNamespace).Get(context.TODO(), mcoCRName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			spec := mco.Object["spec"].(map[string]interface{})
			capabilities := spec["capabilities"].(map[string]interface{})
			platform := capabilities["platform"].(map[string]interface{})
			analytics := platform["analytics"].(map[string]interface{})
			analytics["namespaceRightSizingRecommendation"] = map[string]interface{}{
				"enabled": true,
			}
			_, err = dynClient.Resource(utils.NewMCOGVRV1BETA2()).Namespace(mcoNamespace).Update(context.TODO(), mco, metav1.UpdateOptions{})
			return err
		}, 2*time.Minute, 5*time.Second).Should(Succeed())

		By("Creating the RightSizing config map")
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rsConfigMapName,
				Namespace: mcoNamespace,
			},
			Data: map[string]string{
				"prometheusRuleConfig": fmt.Sprintf(`namespaceFilterCriteria:
  inclusionCriteria:
    - "default"
labelFilterCriteria:
  - label: "team"
    inclusionCriteria:
      - "platform"
recommendationPercentage: %d`, rsDefaultRecommendationPercentage),
			},
		}
		_, err = hubClient.CoreV1().ConfigMaps(mcoNamespace).Create(context.TODO(), configMap, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		By("Waiting for MCO CR to become Ready")
		Eventually(func() error {
			obj, err := dynClient.Resource(utils.NewMCOGVRV1BETA2()).Namespace(mcoNamespace).Get(context.TODO(), mcoCRName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			status, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
			if !found {
				return fmt.Errorf("status.conditions not found")
			}
			for _, cond := range status {
				if m, ok := cond.(map[string]interface{}); ok {
					if m["type"] == "Ready" && m["status"] == "True" {
						return nil
					}
				}
			}
			return fmt.Errorf("MCO CR not ready yet")
		}, 5*time.Minute, 10*time.Second).Should(Succeed())
	})

	It("should create all expected PrometheusRule records", func() {
		Eventually(func() error {
			var rule monitoringv1.PrometheusRule
			err := k8sClient.Get(context.TODO(), types.NamespacedName{
				Name:      rsPrometheusRuleName,
				Namespace: mcoNamespace,
			}, &rule)
			if err != nil {
				return err
			}
			found := map[string]bool{}
			for _, group := range rule.Spec.Groups {
				for _, r := range group.Rules {
					if r.Record != "" {
						found[r.Record] = true
					}
				}
			}
			for _, expected := range expectedRecords {
				if !found[expected] {
					return fmt.Errorf("missing expected record: %s", expected)
				}
			}
			return nil
		}, 5*time.Minute, 10*time.Second).Should(Succeed())
	})

	It("should create the right policy for the PrometheusRule", func() {
		Eventually(func() error {
			var policy policyv1.Policy
			err := k8sClient.Get(context.TODO(), types.NamespacedName{
				Name:      rsPrometheusRulePolicyName,
				Namespace: mcoNamespace,
			}, &policy)
			if err != nil {
				return err
			}
			if len(policy.Spec.PolicyTemplates) == 0 || policy.Spec.RemediationAction == "" {
				return fmt.Errorf("policy fields missing")
			}
			return nil
		}, 3*time.Minute, 5*time.Second).Should(Succeed())
	})

	AfterAll(func() {
		By("Cleaning up test resources")
		_ = hubClient.CoreV1().ConfigMaps(mcoNamespace).Delete(context.TODO(), rsConfigMapName, metav1.DeleteOptions{})
	})
})
