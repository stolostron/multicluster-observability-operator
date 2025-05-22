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
	obsv1beta2 "github.com/stolostron/multicluster-observability-operator/api/v1beta2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var k8sClient client.Client

var _ = Describe("Analytics Right-Sizing Functional Test", Ordered, func() {
	BeforeAll(func() {
		cfg := ctrl.GetConfigOrDie()
		var err error
		k8sClient, err = client.New(cfg, client.Options{})
		Expect(err).NotTo(HaveOccurred())

		// Apply the MultiClusterObservability CR to trigger reconciliation
		mco := &obsv1beta2.MultiClusterObservability{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "open-cluster-management-observability",
				Namespace: "open-cluster-management",
			},
			Spec: obsv1beta2.MultiClusterObservabilitySpec{
				EnableDownsampling: true,
				ObservabilityAddonSpec: obsv1beta2.ObservabilityAddonSpec{
					EnableMetrics: true,
				},
			},
		}
		_ = k8sClient.Delete(context.TODO(), mco)
		Expect(k8sClient.Create(context.TODO(), mco)).To(Succeed())

		// Apply a rich ConfigMap for enabling RightSizing feature
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "right-sizing-config",
				Namespace: "open-cluster-management",
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
		Expect(k8sClient.Create(context.TODO(), cm)).To(Succeed())
	})

	It("should create PrometheusRule and Policy resources", func(ctx SpecContext) {
		Eventually(func() error {
			var promRule monitoringv1.PrometheusRule
			err := k8sClient.Get(context.TODO(), types.NamespacedName{
				Name:      "acm-ns-rightsizing-prometheus-rule",
				Namespace: "open-cluster-management",
			}, &promRule)
			if err != nil {
				return err
			}
			if len(promRule.Spec.Groups) == 0 {
				return fmt.Errorf("PrometheusRule has no rule groups")
			}
			return nil
		}, 3*time.Minute, 5*time.Second).Should(Succeed(), "Expected PrometheusRule with rule groups to be created")

		Eventually(func() error {
			var policy policyv1.Policy
			err := k8sClient.Get(context.TODO(), types.NamespacedName{
				Name:      "acm-ns-rightsizing-prometheus-rule-policy",
				Namespace: "open-cluster-management",
			}, &policy)
			if err != nil {
				return err
			}
			if policy.Spec.RemediationAction == "" || len(policy.Spec.PolicyTemplates) == 0 {
				return fmt.Errorf("Policy is missing expected fields")
			}
			return nil
		}, 3*time.Minute, 5*time.Second).Should(Succeed(), "Expected Policy with valid spec to be created")

		report := CurrentSpecReport()
		fmt.Println("Test completed:", report.FullText())
	})

	AfterAll(func() {
		// Clean up the ConfigMap and MCO CR after the test
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "right-sizing-config",
				Namespace: "open-cluster-management",
			},
		}
		_ = k8sClient.Delete(context.TODO(), cm)

		mco := &obsv1beta2.MultiClusterObservability{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "open-cluster-management-observability",
				Namespace: "open-cluster-management",
			},
		}
		_ = k8sClient.Delete(context.TODO(), mco)
	})
})
