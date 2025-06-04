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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
)

var _ = Describe("RHACM4K-55205: Enable namespace right-sizing recommendation (rightsizing/g0)", Ordered, func() {
	BeforeEach(func() {
		// initialize both typed and dynamic clients
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
	})

	It("Should patch the MultiClusterObservability CR to enable right-sizing recommendation", func() {
		By("Updating spec.capabilities.platform.analytics.namespaceRightSizingRecommendation.enabled to true")

		mcoGVR := utils.NewMCOGVRV1BETA2()

		Eventually(func() error {
			// fetch the current CR
			mco, err := dynClient.Resource(mcoGVR).
				Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
			if err != nil {
				return err
			}

			// Safely set .spec.capabilities.platform.analytics.namespaceRightSizingRecommendation.enabled = true
			if err := unstructured.SetNestedField(
				mco.Object,
				true,
				"spec",
				"capabilities",
				"platform",
				"analytics",
				"namespaceRightSizingRecommendation",
				"enabled",
			); err != nil {
				return err
			}

			// push the update
			_, err = dynClient.Resource(mcoGVR).
				Update(context.TODO(), mco, metav1.UpdateOptions{})
			return err
		}, 2*time.Minute, 10*time.Second).Should(Succeed())
	})

	It("Should find the rs-prom-rules-policy in the hub cluster namespace 'open-cluster-management-global-set'", func() {
		policyGVR := schema.GroupVersionResource{Group: "policy.open-cluster-management.io", Version: "v1", Resource: "policies"}
		Eventually(func() error {
			_, err := dynClient.Resource(policyGVR).
				Namespace("open-cluster-management-global-set").
				Get(context.TODO(), "rs-prom-rules-policy", metav1.GetOptions{})
			return err
		}, 2*time.Minute, 10*time.Second).Should(Succeed())
	})

	It("Should create the ConfigMap 'rs-namespace-config' in namespace 'open-cluster-management-observability'", func() {
		configMapGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
		Eventually(func() error {
			_, err := dynClient.Resource(configMapGVR).
				Namespace("open-cluster-management-observability").
				Get(context.TODO(), "rs-namespace-config", metav1.GetOptions{})
			return err
		}, 2*time.Minute, 10*time.Second).Should(Succeed())
	})

	It("Should find the PrometheusRule 'acm-rs-namespace-prometheus-rules' in namespace 'openshift-monitoring'", func() {
		prGVR := schema.GroupVersionResource{Group: "monitoring.coreos.com", Version: "v1", Resource: "prometheusrules"}
		Eventually(func() error {
			_, err := dynClient.Resource(prGVR).
				Namespace("openshift-monitoring").
				Get(context.TODO(), "acm-rs-namespace-prometheus-rules", metav1.GetOptions{})
			return err
		}, 2*time.Minute, 10*time.Second).Should(Succeed())
	})

	It("Should validate the 'observability-metrics-allowlist' ConfigMap in namespace 'open-cluster-management-observability'", func() {
		cmGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
		Eventually(func() error {
			cm, err := dynClient.Resource(cmGVR).
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
		cmGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
		Eventually(func() error {
			_, err := dynClient.Resource(cmGVR).
				Namespace("open-cluster-management-observability").
				Get(context.TODO(), "grafana-dashboard-acm-right-sizing-namespaces", metav1.GetOptions{})
			return err
		}, 2*time.Minute, 10*time.Second).Should(Succeed(),
			"Expected ConfigMap 'grafana-dashboard-acm-right-sizing-namespaces' to exist in namespace 'open-cluster-management-observability'",
		)
	})
})
