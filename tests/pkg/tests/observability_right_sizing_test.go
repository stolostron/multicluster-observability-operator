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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
)

var _ = Describe("RHACM4K-55205: Enable and teardown namespace right-sizing recommendation (rightsizing/g0)", Ordered, func() {
	var (
		mcoGVR       = utils.NewMCOGVRV1BETA2()
		policyGVR    = schema.GroupVersionResource{Group: "policy.open-cluster-management.io", Version: "v1", Resource: "policies"}
		configMapGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
		prGVR        = schema.GroupVersionResource{Group: "monitoring.coreos.com", Version: "v1", Resource: "prometheusrules"}
	)

	BeforeAll(func() {
		// initialize clients once
		hubClient = utils.NewKubeClient(
			testOptions.HubCluster.ClusterServerURL,
			testOptions.KubeConfig,
			testOptions.HubCluster.KubeContext,
		)
		dynClient = utils.NewKubeClientDynamic(
			testOptions.HubCluster.ClusterServerURL,
			testOptions.KubeConfig,
			testOptions.HubCluster.KubeContext,
		)

		By("Enabling namespace right-sizing recommendation in the MCO CR")
		Eventually(func() error {
			mco, err := dynClient.Resource(mcoGVR).
				Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if err := unstructured.SetNestedField(
				mco.Object,
				true,
				"spec", "capabilities", "platform", "analytics", "namespaceRightSizingRecommendation", "enabled",
			); err != nil {
				return err
			}
			_, err = dynClient.Resource(mcoGVR).
				Update(context.TODO(), mco, metav1.UpdateOptions{})
			return err
		}, 2*time.Minute, 10*time.Second).Should(Succeed())
	})

	// Verify resources are created
	It("Should find the rs-prom-rules-policy in the hub cluster namespace 'open-cluster-management-global-set'", func() {
		Eventually(func() error {
			_, err := dynClient.Resource(policyGVR).
				Namespace("open-cluster-management-global-set").
				Get(context.TODO(), "rs-prom-rules-policy", metav1.GetOptions{})
			return err
		}, 2*time.Minute, 10*time.Second).Should(Succeed())
	})

	It("Should create the ConfigMap 'rs-namespace-config' in namespace 'open-cluster-management-observability'", func() {
		Eventually(func() error {
			_, err := dynClient.Resource(configMapGVR).
				Namespace("open-cluster-management-observability").
				Get(context.TODO(), "rs-namespace-config", metav1.GetOptions{})
			return err
		}, 2*time.Minute, 10*time.Second).Should(Succeed())
	})

	It("Should find the PrometheusRule 'acm-rs-namespace-prometheus-rules' in namespace 'openshift-monitoring'", func() {
		Eventually(func() error {
			_, err := dynClient.Resource(prGVR).
				Namespace("openshift-monitoring").
				Get(context.TODO(), "acm-rs-namespace-prometheus-rules", metav1.GetOptions{})
			return err
		}, 2*time.Minute, 10*time.Second).Should(Succeed())
	})

	It("Should validate the 'observability-metrics-allowlist' ConfigMap in namespace 'open-cluster-management-observability'", func() {
		Eventually(func() error {
			cm, err := dynClient.Resource(configMapGVR).
				Namespace("open-cluster-management-observability").
				Get(context.TODO(), "observability-metrics-allowlist", metav1.GetOptions{})
			if err != nil {
				return err
			}

			raw, found, err := unstructured.NestedString(cm.Object, "data", "metrics_list.yaml")
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("metrics_list.yaml key not found in ConfigMap data")
			}

			var ml struct {
				Names []string `yaml:"names"`
			}
			if err := yaml.Unmarshal([]byte(raw), &ml); err != nil {
				return err
			}

			expected := []string{
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
			for _, name := range expected {
				found := false
				for _, v := range ml.Names {
					if v == name {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("expected metric %s not found in list", name)
				}
			}
			return nil
		}, 2*time.Minute, 10*time.Second).Should(Succeed())
	})

	It("Should create the Grafana dashboard ConfigMap 'grafana-dashboard-acm-right-sizing-namespaces' in namespace 'open-cluster-management-observability'", func() {
		Eventually(func() error {
			_, err := dynClient.Resource(configMapGVR).
				Namespace("open-cluster-management-observability").
				Get(context.TODO(), "grafana-dashboard-acm-right-sizing-namespaces", metav1.GetOptions{})
			return err
		}, 2*time.Minute, 10*time.Second).Should(Succeed(),
			"Expected ConfigMap 'grafana-dashboard-acm-right-sizing-namespaces' to exist in namespace 'open-cluster-management-observability'",
		)
	})

	// Teardown in AfterAll
	AfterAll(func() {
		By("Disabling namespace right-sizing recommendation and cleaning up resources")

		// Disable the feature
		Eventually(func() error {
			mco, err := dynClient.Resource(mcoGVR).
				Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if err := unstructured.SetNestedField(
				mco.Object,
				false,
				"spec", "capabilities", "platform", "analytics", "namespaceRightSizingRecommendation", "enabled",
			); err != nil {
				return err
			}
			_, err = dynClient.Resource(mcoGVR).
				Update(context.TODO(), mco, metav1.UpdateOptions{})
			return err
		}, 2*time.Minute, 10*time.Second).Should(Succeed())

		// Verify resources are removed
		Eventually(func() bool {
			_, err := dynClient.Resource(policyGVR).
				Namespace("open-cluster-management-global-set").
				Get(context.TODO(), "rs-prom-rules-policy", metav1.GetOptions{})
			return apierrors.IsNotFound(err)
		}, 2*time.Minute, 10*time.Second).Should(BeTrue(), "rs-prom-rules-policy should be deleted")

		Eventually(func() bool {
			_, err := dynClient.Resource(configMapGVR).
				Namespace("open-cluster-management-observability").
				Get(context.TODO(), "rs-namespace-config", metav1.GetOptions{})
			return apierrors.IsNotFound(err)
		}, 2*time.Minute, 10*time.Second).Should(BeTrue(), "rs-namespace-config should be deleted")

		Eventually(func() bool {
			_, err := dynClient.Resource(prGVR).
				Namespace("openshift-monitoring").
				Get(context.TODO(), "acm-rs-namespace-prometheus-rules", metav1.GetOptions{})
			return apierrors.IsNotFound(err)
		}, 2*time.Minute, 10*time.Second).Should(BeTrue(), "acm-rs-namespace-prometheus-rules should be deleted")
	})
})
