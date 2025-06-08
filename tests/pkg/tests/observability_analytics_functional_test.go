// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"context"
	"flag"
	"fmt"
	"sync"
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
var once sync.Once

func init() {
	once.Do(func() {
		if flag.Lookup("kubeconfig") == nil {
			flag.String("kubeconfig", "", "Path to the kubeconfig file")
		}
	})
}

var _ = Describe("RHACM4K-XXXXX: Analytics Right-Sizing Functional Test [P1][Observability][Analytics] @e2e", Ordered, func() {
	const (
		rsPolicySetName                   = "rs-policyset"
		rsPlacementName                   = "rs-placement"
		rsPlacementBindingName            = "rs-policyset-binding"
		rsPrometheusRulePolicyName        = "rs-prom-rules-policy"
		rsPrometheusRulePolicyConfigName  = "rs-prometheus-rules-policy-config"
		rsPrometheusRuleName              = "acm-rs-namespace-prometheus-rules"
		rsConfigMapName                   = "rs-namespace-config"
		rsDefaultNamespace                = "open-cluster-management-global-set"
		rsMonitoringNamespace             = "openshift-monitoring"
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

		fmt.Println("🔁 Cleaning up previous resources")
		_ = hubClient.CoreV1().ConfigMaps(mcoNamespace).Delete(context.TODO(), rsConfigMapName, metav1.DeleteOptions{})
		_ = dynClient.Resource(utils.NewMCOGVRV1BETA2()).Namespace(mcoNamespace).Delete(context.TODO(), mcoCRName, metav1.DeleteOptions{})

		fmt.Println("📦 Creating new MCO CR with analytics enabled")
		mco := map[string]interface{}{
			"apiVersion": "observability.open-cluster-management.io/v1beta2",
			"kind":       "MultiClusterObservability",
			"metadata": map[string]interface{}{
				"name":      mcoCRName,
				"namespace": mcoNamespace,
			},
			"spec": map[string]interface{}{
				"enableDownsampling": true,
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
		_, err = dynClient.Resource(utils.NewMCOGVRV1BETA2()).Namespace(mcoNamespace).Create(context.TODO(), &unstructured.Unstructured{Object: mco}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		fmt.Println("📝 Creating right-sizing config map")
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rsConfigMapName,
				Namespace: mcoNamespace,
			},
			Data: map[string]string{
				"placementConfiguration": `typemeta:
  kind: ""
  apiversion: ""
spec:
  tolerations:
  - key: cluster.open-cluster-management.io/unreachable
    operator: Exists
  - key: cluster.open-cluster-management.io/unavailable
    operator: Exists`,
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
		fmt.Println("✅ ConfigMap and MCO CR created")

		fmt.Println("⏳ Waiting for MCO CR status to become Ready")
		Eventually(func() error {
			obj, err := dynClient.Resource(utils.NewMCOGVRV1BETA2()).Namespace(mcoNamespace).Get(context.TODO(), mcoCRName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			status, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
			if !found {
				return fmt.Errorf("status.conditions not found in MCO CR")
			}
			for _, cond := range status {
				if m, ok := cond.(map[string]interface{}); ok {
					if m["type"] == "Ready" && m["status"] == "True" {
						return nil
					}
				}
			}
			return fmt.Errorf("MCO CR not Ready yet")
		}, 5*time.Minute, 10*time.Second).Should(Succeed())
		fmt.Println("✅ MCO CR is in Ready state")
	})

	It("should create the PrometheusRule for namespace right-sizing with all expected records", func() {
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
					return fmt.Errorf("missing expected rule record: %s", expected)
				}
			}
			return nil
		}, 5*time.Minute, 10*time.Second).Should(Succeed())

		fmt.Println("✅ All expected PrometheusRule records are present")
	})

	It("should create the corresponding Policy for the PrometheusRule", func() {
		Eventually(func() error {
			var policy policyv1.Policy
			err := k8sClient.Get(context.TODO(), types.NamespacedName{
				Name:      rsPrometheusRulePolicyName,
				Namespace: mcoNamespace,
			}, &policy)
			if err != nil {
				return err
			}
			if policy.Spec.RemediationAction == "" || len(policy.Spec.PolicyTemplates) == 0 {
				return fmt.Errorf("Policy %q is missing required fields", rsPrometheusRulePolicyName)
			}
			return nil
		}, 3*time.Minute, 5*time.Second).Should(Succeed())
		fmt.Println("✅ Corresponding Policy created and valid")
	})

	AfterAll(func() {
		fmt.Println("🧹 Cleaning up test resources")
		_ = hubClient.CoreV1().ConfigMaps(mcoNamespace).Delete(context.TODO(), rsConfigMapName, metav1.DeleteOptions{})
		_ = dynClient.Resource(utils.NewMCOGVRV1BETA2()).Namespace(mcoNamespace).Delete(context.TODO(), mcoCRName, metav1.DeleteOptions{})
		fmt.Println("🧼 Cleanup complete")
	})
})
