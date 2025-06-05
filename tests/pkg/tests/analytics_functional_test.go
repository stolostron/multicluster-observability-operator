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

// Prevent flag redefinition panic (only needed if running standalone)
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
		rsConfigMapName = "right-sizing-config"
		promRuleName    = "acm-ns-rightsizing-prometheus-rule"
		policyName      = "acm-ns-rightsizing-prometheus-rule-policy"
	)

	BeforeAll(func() {
		cfg := ctrl.GetConfigOrDie()
		var err error
		k8sClient, err = client.New(cfg, client.Options{})
		Expect(err).NotTo(HaveOccurred())

		By("Deleting previous ConfigMap and MCO CR (if any)")
		_ = hubClient.CoreV1().ConfigMaps(mcoNamespace).Delete(context.TODO(), rsConfigMapName, metav1.DeleteOptions{})
		_ = dynClient.Resource(utils.NewMCOGVRV1BETA2()).Namespace(mcoNamespace).Delete(context.TODO(), mcoCRName, metav1.DeleteOptions{})

		By("Creating a minimal MCO CR to trigger analytics component")
		mco := map[string]interface{}{
			"apiVersion": "observability.open-cluster-management.io/v1beta2",
			"kind":       "MultiClusterObservability",
			"metadata": map[string]interface{}{
				"name":      mcoCRName,
				"namespace": mcoNamespace,
			},
			"spec": map[string]interface{}{
				"enableDownsampling": true,
			},
		}
		_, err = dynClient.Resource(utils.NewMCOGVRV1BETA2()).
			Namespace(mcoNamespace).
			Create(context.TODO(), &unstructured.Unstructured{Object: mco}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		By("Creating Right-Sizing ConfigMap")
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
	})

	It("should create the PrometheusRule for namespace right-sizing", func() {
		Eventually(func() error {
			var rule monitoringv1.PrometheusRule
			err := k8sClient.Get(context.TODO(), types.NamespacedName{
				Name:      promRuleName,
				Namespace: mcoNamespace,
			}, &rule)
			if err != nil {
				return err
			}
			if len(rule.Spec.Groups) == 0 {
				return fmt.Errorf("PrometheusRule %q has no rule groups", promRuleName)
			}
			return nil
		}, 3*time.Minute, 5*time.Second).Should(Succeed())
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
	})

	AfterAll(func() {
		By("Cleaning up Right-Sizing ConfigMap and MCO CR")
		_ = hubClient.CoreV1().ConfigMaps(mcoNamespace).Delete(context.TODO(), rsConfigMapName, metav1.DeleteOptions{})
		_ = dynClient.Resource(utils.NewMCOGVRV1BETA2()).Namespace(mcoNamespace).Delete(context.TODO(), mcoCRName, metav1.DeleteOptions{})
	})
})
