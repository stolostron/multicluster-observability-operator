// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/yaml"
)

var _ = Describe("Right-sizing: independently enabled defaults", Ordered, func() {
	mcoGVR := utils.NewMCOGVRV1BETA2()

	BeforeAll(func() {
		if hubClient == nil {
			hubClient = utils.NewKubeClient(
				testOptions.HubCluster.ClusterServerURL,
				testOptions.KubeConfig,
				testOptions.HubCluster.KubeContext,
			)
		}
		if dynClient == nil {
			dynClient = utils.NewKubeClientDynamic(
				testOptions.HubCluster.ClusterServerURL,
				testOptions.KubeConfig,
				testOptions.HubCluster.KubeContext,
			)
		}
	})

	It("Should default all right-sizing flags to enabled", func() {
		By("Simulating a fresh-install state by removing right-sizing enabled fields (if present) and letting the operator default them")
		Eventually(func() error {
			mco, err := dynClient.Resource(mcoGVR).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
			if err != nil {
				return err
			}

			_ = unstructured.SetNestedMap(mco.Object, map[string]any{}, "spec", "capabilities", "platform", "analytics")
			unstructured.RemoveNestedField(mco.Object, "spec", "capabilities", "platform", "analytics", "namespaceRightSizingRecommendation", "enabled")
			unstructured.RemoveNestedField(mco.Object, "spec", "capabilities", "platform", "analytics", "virtualizationRightSizingRecommendation", "enabled")

			_, err = dynClient.Resource(mcoGVR).Update(context.TODO(), mco, metav1.UpdateOptions{})
			return err
		}, 2*time.Minute, 10*time.Second).Should(Succeed())

		Eventually(func() error {
			mco, err := dynClient.Resource(mcoGVR).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
			if err != nil {
				return err
			}

			nsEnabled, nsFound, err := unstructured.NestedBool(mco.Object,
				"spec", "capabilities", "platform", "analytics", "namespaceRightSizingRecommendation", "enabled")
			if err != nil {
				return err
			}
			if !nsFound || !nsEnabled {
				return fmt.Errorf("expected namespaceRightSizingRecommendation.enabled to be defaulted to true (found=%v enabled=%v)", nsFound, nsEnabled)
			}

			virtEnabled, virtFound, err := unstructured.NestedBool(mco.Object,
				"spec", "capabilities", "platform", "analytics", "virtualizationRightSizingRecommendation", "enabled")
			if err != nil {
				return err
			}
			if !virtFound || !virtEnabled {
				return fmt.Errorf("expected virtualizationRightSizingRecommendation.enabled to be defaulted to true (found=%v enabled=%v)", virtFound, virtEnabled)
			}

			for field, key := range map[string]string{
				"workloadRightSizingRecommendation": "platformWorkloadRightSizing",
				"podRightSizingRecommendation":      "platformPodRightSizing",
			} {
				enabled, found, err := unstructured.NestedBool(mco.Object, "spec", "capabilities", "platform", "analytics", field, "enabled")
				if err != nil {
					return err
				}
				if !found || !enabled {
					return fmt.Errorf("expected %s.enabled to default to true", field)
				}
				value, err := getADCCustomVar(dynClient, "multicluster-observability-addon", MCO_NAMESPACE, key)
				if err != nil {
					return err
				}
				if value != "enabled" {
					return fmt.Errorf("expected ADC %s=enabled, got %q", key, value)
				}
			}

			return nil
		}, 2*time.Minute, 10*time.Second).Should(Succeed())
	})
})

var _ = Describe("RHACM4K-55205: Enable and teardown namespace right-sizing recommendation (rightsizing/g0)", Ordered, func() {
	var (
		mcoGVR       = utils.NewMCOGVRV1BETA2()
		policyGVR    = schema.GroupVersionResource{Group: "policy.open-cluster-management.io", Version: "v1", Resource: "policies"}
		configMapGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
		prGVR        = schema.GroupVersionResource{Group: "monitoring.coreos.com", Version: "v1", Resource: "prometheusrules"}
	)

	BeforeAll(func() {
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

		By("Ensuring namespace 'open-cluster-management-global-set' exists")
		Eventually(func() error {
			_, err := hubClient.CoreV1().Namespaces().Get(context.TODO(), "open-cluster-management-global-set", metav1.GetOptions{})
			if err == nil {
				return nil
			}
			if apierrors.IsNotFound(err) {
				_, createErr := hubClient.CoreV1().Namespaces().Create(context.TODO(), &v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "open-cluster-management-global-set"},
				}, metav1.CreateOptions{})
				return createErr
			}
			return err
		}, 2*time.Minute, 10*time.Second).Should(Succeed())

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

	It("Should set ADC right-sizing delegation to 'true' and namespace RS to 'enabled'", func() {
		Eventually(func() error {
			delegated, err := getADCCustomVar(dynClient, "multicluster-observability-addon", MCO_NAMESPACE, "rightSizingDelegated")
			if err != nil {
				return err
			}
			if delegated != "true" {
				return fmt.Errorf("expected ADC rightSizingDelegated=%q, got %q", "true", delegated)
			}
			nsVal, err := getADCCustomVar(dynClient, "multicluster-observability-addon", MCO_NAMESPACE, "platformNamespaceRightSizing")
			if err != nil {
				return err
			}
			if nsVal != "enabled" {
				return fmt.Errorf("expected ADC platformNamespaceRightSizing=%q, got %q", "enabled", nsVal)
			}
			return nil
		}, 3*time.Minute, 10*time.Second).Should(Succeed())
	})

	It("Should NOT create any Policy resources (always-MCOA mode)", func() {
		Consistently(func() bool {
			_, err := dynClient.Resource(policyGVR).
				Namespace("open-cluster-management-global-set").
				Get(context.TODO(), "rs-prom-rules-policy", metav1.GetOptions{})
			return apierrors.IsNotFound(err)
		}, 30*time.Second, 5*time.Second).Should(BeTrue(), "rs-prom-rules-policy should NOT exist in always-MCOA mode")
	})

	It("Should create the ConfigMap 'rs-namespace-config' in namespace 'open-cluster-management-observability'", func() {
		Eventually(func() error {
			_, err := dynClient.Resource(configMapGVR).
				Namespace("open-cluster-management-observability").
				Get(context.TODO(), "rs-namespace-config", metav1.GetOptions{})
			return err
		}, 2*time.Minute, 10*time.Second).Should(Succeed())
	})

	// Timeout is 5 minutes because the preceding "defaults" test removes RS fields,
	// triggering MCOA teardown. ensureRightSizingDefaults re-enables RS, but the full
	// rebuild chain (CMA → addon-manager → MCA → ManifestWork → work-agent → PrometheusRule)
	// takes 3-5 minutes.
	It("Should find the PrometheusRule 'acm-rs-namespace-prometheus-rules' in namespace 'openshift-monitoring'", func() {
		Eventually(func() error {
			_, err := dynClient.Resource(prGVR).
				Namespace("openshift-monitoring").
				Get(context.TODO(), "acm-rs-namespace-prometheus-rules", metav1.GetOptions{})
			return err
		}, 5*time.Minute, 10*time.Second).Should(Succeed())
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
				found := slices.Contains(ml.Names, name)
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

	It("Should have cpu_profile and memory_profile variables in the namespace Grafana dashboard", func() {
		Eventually(func() error {
			return validateDashboardProfileVars(dynClient, "grafana-dashboard-acm-right-sizing-namespaces")
		}, 2*time.Minute, 10*time.Second).Should(Succeed())
	})

	AfterAll(func() {
		By("Disabling namespace right-sizing recommendation and cleaning up resources")

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

		By("Verifying ADC reflects disabled state")
		Eventually(func() error {
			nsVal, err := getADCCustomVar(dynClient, "multicluster-observability-addon", MCO_NAMESPACE, "platformNamespaceRightSizing")
			if err != nil {
				return err
			}
			if nsVal != "disabled" {
				return fmt.Errorf("expected ADC platformNamespaceRightSizing=%q, got %q", "disabled", nsVal)
			}
			return nil
		}, 3*time.Minute, 10*time.Second).Should(Succeed())

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

var _ = Describe("RHACM4K-58751: Enable and teardown virtualization right-sizing recommendation (rightsizing/g1)", Ordered, func() {
	var (
		mcoGVR       = utils.NewMCOGVRV1BETA2()
		policyGVR    = schema.GroupVersionResource{Group: "policy.open-cluster-management.io", Version: "v1", Resource: "policies"}
		configMapGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
		prGVR        = schema.GroupVersionResource{Group: "monitoring.coreos.com", Version: "v1", Resource: "prometheusrules"}
	)

	BeforeAll(func() {
		if hubClient == nil {
			hubClient = utils.NewKubeClient(
				testOptions.HubCluster.ClusterServerURL,
				testOptions.KubeConfig,
				testOptions.HubCluster.KubeContext,
			)
		}
		if dynClient == nil {
			dynClient = utils.NewKubeClientDynamic(
				testOptions.HubCluster.ClusterServerURL,
				testOptions.KubeConfig,
				testOptions.HubCluster.KubeContext,
			)
		}

		By("Ensuring namespace 'open-cluster-management-global-set' exists")
		Eventually(func() error {
			_, err := hubClient.CoreV1().Namespaces().Get(context.TODO(), "open-cluster-management-global-set", metav1.GetOptions{})
			if err == nil {
				return nil
			}
			if apierrors.IsNotFound(err) {
				_, createErr := hubClient.CoreV1().Namespaces().Create(context.TODO(), &v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "open-cluster-management-global-set"},
				}, metav1.CreateOptions{})
				return createErr
			}
			return err
		}, 2*time.Minute, 10*time.Second).Should(Succeed())

		By("Enabling virtualization right-sizing recommendation in the MCO CR")
		Eventually(func() error {
			mco, err := dynClient.Resource(mcoGVR).
				Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if err := unstructured.SetNestedField(
				mco.Object,
				true,
				"spec", "capabilities", "platform", "analytics", "virtualizationRightSizingRecommendation", "enabled",
			); err != nil {
				return err
			}
			_, err = dynClient.Resource(mcoGVR).
				Update(context.TODO(), mco, metav1.UpdateOptions{})
			return err
		}, 2*time.Minute, 10*time.Second).Should(Succeed())
	})

	It("Should set ADC right-sizing delegation to 'true' and virtualization RS to 'enabled'", func() {
		Eventually(func() error {
			delegated, err := getADCCustomVar(dynClient, "multicluster-observability-addon", MCO_NAMESPACE, "rightSizingDelegated")
			if err != nil {
				return err
			}
			if delegated != "true" {
				return fmt.Errorf("expected ADC rightSizingDelegated=%q, got %q", "true", delegated)
			}
			virtVal, err := getADCCustomVar(dynClient, "multicluster-observability-addon", MCO_NAMESPACE, "platformVirtualizationRightSizing")
			if err != nil {
				return err
			}
			if virtVal != "enabled" {
				return fmt.Errorf("expected ADC platformVirtualizationRightSizing=%q, got %q", "enabled", virtVal)
			}
			return nil
		}, 3*time.Minute, 10*time.Second).Should(Succeed())
	})

	It("Should NOT create any Policy resources (always-MCOA mode)", func() {
		Consistently(func() bool {
			_, err := dynClient.Resource(policyGVR).
				Namespace("open-cluster-management-global-set").
				Get(context.TODO(), "rs-virt-prom-rules-policy", metav1.GetOptions{})
			return apierrors.IsNotFound(err)
		}, 30*time.Second, 5*time.Second).Should(BeTrue(), "rs-virt-prom-rules-policy should NOT exist in always-MCOA mode")
	})

	It("Should create the ConfigMap 'rs-virt-config' in namespace 'open-cluster-management-observability'", func() {
		Eventually(func() error {
			_, err := dynClient.Resource(configMapGVR).
				Namespace("open-cluster-management-observability").
				Get(context.TODO(), "rs-virt-config", metav1.GetOptions{})
			return err
		}, 2*time.Minute, 10*time.Second).Should(Succeed())
	})

	It("Should find the PrometheusRule 'acm-rs-virt-prometheus-rules' in namespace 'openshift-monitoring'", func() {
		Eventually(func() error {
			_, err := dynClient.Resource(prGVR).
				Namespace("openshift-monitoring").
				Get(context.TODO(), "acm-rs-virt-prometheus-rules", metav1.GetOptions{})
			return err
		}, 2*time.Minute, 10*time.Second).Should(Succeed())
	})

	It("Should validate VM metrics in the 'observability-metrics-allowlist' ConfigMap in namespace 'open-cluster-management-observability'", func() {
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
				"acm_rs_vm:namespace:cpu_request",
				"acm_rs_vm:namespace:cpu_usage",
				"acm_rs_vm:namespace:cpu_recommendation",
				"acm_rs_vm:namespace:memory_request",
				"acm_rs_vm:namespace:memory_usage",
				"acm_rs_vm:namespace:memory_recommendation",
				"kubevirt_vm_running_status_last_transition_timestamp_seconds",
				"acm_rs_vm:cluster:cpu_request",
				"acm_rs_vm:cluster:cpu_usage",
				"acm_rs_vm:cluster:cpu_recommendation",
				"acm_rs_vm:cluster:memory_request",
				"acm_rs_vm:cluster:memory_usage",
				"acm_rs_vm:cluster:memory_recommendation",
			}
			for _, name := range expected {
				if !slices.Contains(ml.Names, name) {
					return fmt.Errorf("expected VM metric %s not found in list", name)
				}
			}
			return nil
		}, 2*time.Minute, 10*time.Second).Should(Succeed())
	})

	It("Should create all virtualization Grafana dashboards in namespace 'open-cluster-management-observability'", func() {
		dashboardCMs := []string{
			"grafana-dashboard-acm-right-sizing-virt-main",
			"grafana-dashboard-acm-right-sizing-virt-overestimation",
			"grafana-dashboard-acm-right-sizing-virt-underestimation",
		}
		Eventually(func() error {
			for _, name := range dashboardCMs {
				if _, err := dynClient.Resource(configMapGVR).
					Namespace("open-cluster-management-observability").
					Get(context.TODO(), name, metav1.GetOptions{}); err != nil {
					return fmt.Errorf("configmap %s not found: %w", name, err)
				}
			}
			return nil
		}, 2*time.Minute, 10*time.Second).Should(Succeed(),
			"Expected virtualization dashboard ConfigMaps to exist in namespace 'open-cluster-management-observability'",
		)
	})

	It("Should have cpu_profile and memory_profile variables in all virtualization Grafana dashboards", func() {
		dashboardCMs := []string{
			"grafana-dashboard-acm-right-sizing-virt-main",
			"grafana-dashboard-acm-right-sizing-virt-overestimation",
			"grafana-dashboard-acm-right-sizing-virt-underestimation",
		}
		Eventually(func() error {
			for _, name := range dashboardCMs {
				if err := validateDashboardProfileVars(dynClient, name); err != nil {
					return fmt.Errorf("dashboard %s: %w", name, err)
				}
			}
			return nil
		}, 2*time.Minute, 10*time.Second).Should(Succeed())
	})

	AfterAll(func() {
		By("Disabling virtualization right-sizing recommendation and cleaning up resources")

		Eventually(func() error {
			mco, err := dynClient.Resource(mcoGVR).
				Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if err := unstructured.SetNestedField(
				mco.Object,
				false,
				"spec", "capabilities", "platform", "analytics", "virtualizationRightSizingRecommendation", "enabled",
			); err != nil {
				return err
			}
			_, err = dynClient.Resource(mcoGVR).
				Update(context.TODO(), mco, metav1.UpdateOptions{})
			return err
		}, 2*time.Minute, 10*time.Second).Should(Succeed())

		By("Verifying ADC reflects disabled state")
		Eventually(func() error {
			virtVal, err := getADCCustomVar(dynClient, "multicluster-observability-addon", MCO_NAMESPACE, "platformVirtualizationRightSizing")
			if err != nil {
				return err
			}
			if virtVal != "disabled" {
				return fmt.Errorf("expected ADC platformVirtualizationRightSizing=%q, got %q", "disabled", virtVal)
			}
			return nil
		}, 3*time.Minute, 10*time.Second).Should(Succeed())

		Eventually(func() bool {
			_, err := dynClient.Resource(configMapGVR).
				Namespace("open-cluster-management-observability").
				Get(context.TODO(), "rs-virt-config", metav1.GetOptions{})
			return apierrors.IsNotFound(err)
		}, 2*time.Minute, 10*time.Second).Should(BeTrue(), "rs-virt-config should be deleted")

		Eventually(func() bool {
			_, err := dynClient.Resource(prGVR).
				Namespace("openshift-monitoring").
				Get(context.TODO(), "acm-rs-virt-prometheus-rules", metav1.GetOptions{})
			return apierrors.IsNotFound(err)
		}, 2*time.Minute, 10*time.Second).Should(BeTrue(), "acm-rs-virt-prometheus-rules should be deleted")
	})
})

// getADCCustomVar extracts a customized variable value from an AddOnDeploymentConfig.
func getADCCustomVar(dynClient dynamic.Interface, adcName, namespace, varName string) (string, error) {
	adcGVR := utils.NewMCOAddOnDeploymentConfigGVR()
	obj, err := dynClient.Resource(adcGVR).Namespace(namespace).
		Get(context.TODO(), adcName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get ADC %s/%s: %w", namespace, adcName, err)
	}

	adc := &addonapiv1alpha1.AddOnDeploymentConfig{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, adc); err != nil {
		return "", fmt.Errorf("failed to convert ADC: %w", err)
	}

	for _, v := range adc.Spec.CustomizedVariables {
		if v.Name == varName {
			return v.Value, nil
		}
	}
	return "", fmt.Errorf("variable %q not found in ADC %s/%s", varName, namespace, adcName)
}

var _ = Describe("Always-MCOA right-sizing: ADC reflects CR spec state (rightsizing/g2)", Ordered, func() {
	const (
		adcName       = "multicluster-observability-addon"
		adcNamespace  = MCO_NAMESPACE
		nsRSKey       = "platformNamespaceRightSizing"
		virtRSKey     = "platformVirtualizationRightSizing"
		workloadRSKey = "platformWorkloadRightSizing"
		podRSKey      = "platformPodRightSizing"
		delegatedKey  = "rightSizingDelegated"
	)

	var (
		mcoGVR             = utils.NewMCOGVRV1BETA2()
		policyGVR          = schema.GroupVersionResource{Group: "policy.open-cluster-management.io", Version: "v1", Resource: "policies"}
		configMapGVR       = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
		prometheusRuleGVR  = schema.GroupVersionResource{Group: "monitoring.coreos.com", Version: "v1", Resource: "prometheusrules"}
		persesDashboardGVR = schema.GroupVersionResource{Group: "perses.dev", Version: "v1alpha1", Resource: "persesdashboards"}
	)

	BeforeAll(func() {
		if hubClient == nil {
			hubClient = utils.NewKubeClient(
				testOptions.HubCluster.ClusterServerURL,
				testOptions.KubeConfig,
				testOptions.HubCluster.KubeContext,
			)
		}
		if dynClient == nil {
			dynClient = utils.NewKubeClientDynamic(
				testOptions.HubCluster.ClusterServerURL,
				testOptions.KubeConfig,
				testOptions.HubCluster.KubeContext,
			)
		}

		By("Enabling both RS features in MCO CR")
		Eventually(func() error {
			mco, err := dynClient.Resource(mcoGVR).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if err := unstructured.SetNestedField(mco.Object, true,
				"spec", "capabilities", "platform", "analytics", "namespaceRightSizingRecommendation", "enabled"); err != nil {
				return err
			}
			if err := unstructured.SetNestedField(mco.Object, true,
				"spec", "capabilities", "platform", "analytics", "virtualizationRightSizingRecommendation", "enabled"); err != nil {
				return err
			}
			for _, field := range []string{"workloadRightSizingRecommendation", "podRightSizingRecommendation"} {
				if err := unstructured.SetNestedField(mco.Object, true,
					"spec", "capabilities", "platform", "analytics", field, "enabled"); err != nil {
					return err
				}
			}
			_, err = dynClient.Resource(mcoGVR).Update(context.TODO(), mco, metav1.UpdateOptions{})
			return err
		}, 2*time.Minute, 10*time.Second).Should(Succeed())
	})

	Context("when all RS features are enabled", Ordered, func() {
		It("Should always set rightSizingDelegated to 'true'", func() {
			Eventually(func() error {
				val, err := getADCCustomVar(dynClient, adcName, adcNamespace, delegatedKey)
				if err != nil {
					return err
				}
				if val != "true" {
					return fmt.Errorf("expected ADC %s=%q, got %q", delegatedKey, "true", val)
				}
				return nil
			}, 3*time.Minute, 10*time.Second).Should(Succeed())
		})

		It("Should set all ADC RS variables to 'enabled'", func() {
			Eventually(func() error {
				nsVal, err := getADCCustomVar(dynClient, adcName, adcNamespace, nsRSKey)
				if err != nil {
					return err
				}
				if nsVal != "enabled" {
					return fmt.Errorf("expected ADC %s=%q, got %q", nsRSKey, "enabled", nsVal)
				}
				virtVal, err := getADCCustomVar(dynClient, adcName, adcNamespace, virtRSKey)
				if err != nil {
					return err
				}
				if virtVal != "enabled" {
					return fmt.Errorf("expected ADC %s=%q, got %q", virtRSKey, "enabled", virtVal)
				}
				workloadVal, err := getADCCustomVar(dynClient, adcName, adcNamespace, workloadRSKey)
				if err != nil {
					return err
				}
				if workloadVal != "enabled" {
					return fmt.Errorf("expected ADC %s=%q, got %q", workloadRSKey, "enabled", workloadVal)
				}
				podVal, err := getADCCustomVar(dynClient, adcName, adcNamespace, podRSKey)
				if err != nil {
					return err
				}
				if podVal != "enabled" {
					return fmt.Errorf("expected ADC %s=enabled, got %q", podRSKey, podVal)
				}
				return nil
			}, 3*time.Minute, 10*time.Second).Should(Succeed())
		})

		It("Should create separate workload and pod right-sizing resources", func() {
			Eventually(func() error {
				for _, name := range []string{"rs-workload-config", "rs-pod-config"} {
					if _, err := dynClient.Resource(configMapGVR).Namespace(MCO_NAMESPACE).
						Get(context.TODO(), name, metav1.GetOptions{}); err != nil {
						return fmt.Errorf("%s: %w", name, err)
					}
				}
				for _, name := range []string{"acm-rs-workload-prometheus-rules", "acm-rs-pod-prometheus-rules"} {
					if _, err := dynClient.Resource(prometheusRuleGVR).Namespace("openshift-monitoring").
						Get(context.TODO(), name, metav1.GetOptions{}); err != nil {
						return fmt.Errorf("%s: %w", name, err)
					}
				}
				for _, name := range []string{"acm-rs-workload-overview", "acm-rs-workload-detail", "acm-rs-pod-overview", "acm-rs-pod-detail"} {
					if _, err := dynClient.Resource(persesDashboardGVR).Namespace("observability-analytics").
						Get(context.TODO(), name, metav1.GetOptions{}); err != nil {
						return fmt.Errorf("PersesDashboard %s: %w", name, err)
					}
				}
				return nil
			}, 5*time.Minute, 10*time.Second).Should(Succeed())
		})

		It("Should deploy a pod-only detail dashboard without tables and with all query variables", func() {
			Eventually(func() error {
				dashboard, err := dynClient.Resource(persesDashboardGVR).Namespace("observability-analytics").
					Get(context.TODO(), "acm-rs-pod-detail", metav1.GetOptions{})
				if err != nil {
					return err
				}
				variables, found, err := unstructured.NestedSlice(dashboard.Object, "spec", "config", "variables")
				if err != nil || !found {
					return fmt.Errorf("pod detail variables missing: %v", err)
				}
				names := []string{}
				for _, variable := range variables {
					v, ok := variable.(map[string]any)
					if !ok {
						return fmt.Errorf("invalid dashboard variable: %v", variable)
					}
					name, _, err := unstructured.NestedString(v, "spec", "name")
					if err != nil {
						return err
					}
					names = append(names, name)
				}
				for _, name := range []string{"days", "cluster", "namespace", "pod", "workload", "workload_type", "cpu_profile", "memory_profile"} {
					if !slices.Contains(names, name) {
						return fmt.Errorf("pod detail variable %s missing", name)
					}
				}
				raw, err := json.Marshal(dashboard.Object["spec"])
				if err != nil {
					return err
				}
				if !strings.Contains(string(raw), "acm_rs:pod:") || strings.Contains(string(raw), "acm_rs:workload:") {
					return fmt.Errorf("pod detail must query only pod-level right-sizing data")
				}
				if !strings.Contains(string(raw), "acm_rs:pod:info") {
					return fmt.Errorf("pod detail selectors must include pods without resource samples")
				}
				panels, found, err := unstructured.NestedMap(dashboard.Object, "spec", "config", "panels")
				if err != nil || !found {
					return fmt.Errorf("pod detail panels missing: %v", err)
				}
				if len(panels) != 13 {
					return fmt.Errorf("expected back link, ten stats, and two time-series charts, got %d panels", len(panels))
				}
				for name, value := range panels {
					panel, ok := value.(map[string]any)
					if !ok {
						return fmt.Errorf("invalid pod detail panel %s", name)
					}
					kind, found, err := unstructured.NestedString(panel, "spec", "plugin", "kind")
					if err != nil || !found {
						return fmt.Errorf("pod detail panel %s plugin kind missing: %v", name, err)
					}
					if kind == "Table" {
						return fmt.Errorf("pod detail must not contain table panel %s", name)
					}
				}
				return nil
			}, 5*time.Minute, 10*time.Second).Should(Succeed())
		})

		It("Should forward workload and pod metrics through the legacy allowlist", func() {
			Eventually(func() error {
				cm, err := dynClient.Resource(configMapGVR).Namespace(MCO_NAMESPACE).
					Get(context.TODO(), "observability-metrics-allowlist", metav1.GetOptions{})
				if err != nil {
					return err
				}
				raw, found, err := unstructured.NestedString(cm.Object, "data", "metrics_list.yaml")
				if err != nil || !found {
					return fmt.Errorf("missing metrics_list.yaml: %v", err)
				}
				var allowlist struct {
					Names []string `json:"names"`
				}
				if err := yaml.Unmarshal([]byte(raw), &allowlist); err != nil {
					return err
				}
				if !slices.Contains(allowlist.Names, "acm_rs:pod:info") {
					return fmt.Errorf("pod inventory is not forwarded")
				}
				for _, level := range []string{"workload", "pod"} {
					for _, metric := range []string{"cpu_request", "cpu_limit", "cpu_usage", "cpu_recommendation", "memory_request", "memory_limit", "memory_usage", "memory_recommendation"} {
						name := "acm_rs:" + level + ":" + metric
						if !slices.Contains(allowlist.Names, name) {
							return fmt.Errorf("metric %s is not forwarded", name)
						}
					}
				}
				return nil
			}, 3*time.Minute, 10*time.Second).Should(Succeed())
		})

		It("Should NOT create any Policy resources", func() {
			Consistently(func() bool {
				_, err := dynClient.Resource(policyGVR).
					Namespace(MCO_GLOBAL_SET_NAMESPACE).
					Get(context.TODO(), "rs-prom-rules-policy", metav1.GetOptions{})
				return apierrors.IsNotFound(err)
			}, 30*time.Second, 5*time.Second).Should(BeTrue(), "rs-prom-rules-policy should NOT exist")
		})
	})

	Context("when namespace RS is disabled but virtualization RS remains enabled", Ordered, func() {
		BeforeAll(func() {
			By("Disabling only namespace right-sizing")
			Eventually(func() error {
				mco, err := dynClient.Resource(mcoGVR).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if err := unstructured.SetNestedField(mco.Object, false,
					"spec", "capabilities", "platform", "analytics", "namespaceRightSizingRecommendation", "enabled"); err != nil {
					return err
				}
				_, err = dynClient.Resource(mcoGVR).Update(context.TODO(), mco, metav1.UpdateOptions{})
				return err
			}, 2*time.Minute, 10*time.Second).Should(Succeed())
		})

		It("Should set namespace RS to 'disabled' and keep virtualization RS 'enabled'", func() {
			Eventually(func() error {
				nsVal, err := getADCCustomVar(dynClient, adcName, adcNamespace, nsRSKey)
				if err != nil {
					return err
				}
				if nsVal != "disabled" {
					return fmt.Errorf("expected ADC %s=%q, got %q", nsRSKey, "disabled", nsVal)
				}
				virtVal, err := getADCCustomVar(dynClient, adcName, adcNamespace, virtRSKey)
				if err != nil {
					return err
				}
				if virtVal != "enabled" {
					return fmt.Errorf("expected ADC %s=%q, got %q", virtRSKey, "enabled", virtVal)
				}
				return nil
			}, 3*time.Minute, 10*time.Second).Should(Succeed())
		})

		It("Should keep rightSizingDelegated as 'true' regardless of individual feature state", func() {
			Eventually(func() error {
				val, err := getADCCustomVar(dynClient, adcName, adcNamespace, delegatedKey)
				if err != nil {
					return err
				}
				if val != "true" {
					return fmt.Errorf("expected ADC %s=%q, got %q", delegatedKey, "true", val)
				}
				return nil
			}, 2*time.Minute, 10*time.Second).Should(Succeed())
		})
	})

	for _, state := range []struct {
		name          string
		workload, pod bool
	}{
		{"workload only", true, false},
		{"pod only", false, true},
	} {
		It("Should independently reconcile "+state.name, func() {
			Eventually(func() error {
				mco, err := dynClient.Resource(mcoGVR).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
				if err != nil {
					return err
				}
				for field, enabled := range map[string]bool{"workloadRightSizingRecommendation": state.workload, "podRightSizingRecommendation": state.pod} {
					if err := unstructured.SetNestedField(mco.Object, enabled, "spec", "capabilities", "platform", "analytics", field, "enabled"); err != nil {
						return err
					}
				}
				_, err = dynClient.Resource(mcoGVR).Update(context.TODO(), mco, metav1.UpdateOptions{})
				return err
			}, 2*time.Minute, 10*time.Second).Should(Succeed())
			Eventually(func() error {
				for _, feature := range []struct {
					level, key string
					enabled    bool
				}{{"workload", workloadRSKey, state.workload}, {"pod", podRSKey, state.pod}} {
					value, err := getADCCustomVar(dynClient, adcName, adcNamespace, feature.key)
					if err != nil {
						return err
					}
					want := "disabled"
					if feature.enabled {
						want = "enabled"
					}
					if value != want {
						return fmt.Errorf("expected %s=%s, got %s", feature.key, want, value)
					}
					for _, resource := range []struct {
						gvr             schema.GroupVersionResource
						namespace, name string
					}{
						{configMapGVR, MCO_NAMESPACE, "rs-" + feature.level + "-config"},
						{prometheusRuleGVR, "openshift-monitoring", "acm-rs-" + feature.level + "-prometheus-rules"},
						{persesDashboardGVR, "observability-analytics", "acm-rs-" + feature.level + "-overview"},
						{persesDashboardGVR, "observability-analytics", "acm-rs-" + feature.level + "-detail"},
					} {
						_, err := dynClient.Resource(resource.gvr).Namespace(resource.namespace).Get(context.TODO(), resource.name, metav1.GetOptions{})
						if feature.enabled && err != nil {
							return fmt.Errorf("%s should exist: %w", resource.name, err)
						}
						if !feature.enabled && !apierrors.IsNotFound(err) {
							return fmt.Errorf("%s should be deleted, got: %v", resource.name, err)
						}
					}
				}
				return nil
			}, 5*time.Minute, 10*time.Second).Should(Succeed())
		})
	}

	Context("when all RS features are disabled", Ordered, func() {
		BeforeAll(func() {
			By("Disabling both right-sizing features")
			Eventually(func() error {
				mco, err := dynClient.Resource(mcoGVR).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if err := unstructured.SetNestedField(mco.Object, false,
					"spec", "capabilities", "platform", "analytics", "namespaceRightSizingRecommendation", "enabled"); err != nil {
					return err
				}
				if err := unstructured.SetNestedField(mco.Object, false,
					"spec", "capabilities", "platform", "analytics", "virtualizationRightSizingRecommendation", "enabled"); err != nil {
					return err
				}
				for _, field := range []string{"workloadRightSizingRecommendation", "podRightSizingRecommendation"} {
					if err := unstructured.SetNestedField(mco.Object, false,
						"spec", "capabilities", "platform", "analytics", field, "enabled"); err != nil {
						return err
					}
				}
				_, err = dynClient.Resource(mcoGVR).Update(context.TODO(), mco, metav1.UpdateOptions{})
				return err
			}, 2*time.Minute, 10*time.Second).Should(Succeed())
		})

		It("Should set all ADC RS variables to 'disabled'", func() {
			Eventually(func() error {
				nsVal, err := getADCCustomVar(dynClient, adcName, adcNamespace, nsRSKey)
				if err != nil {
					return err
				}
				if nsVal != "disabled" {
					return fmt.Errorf("expected ADC %s=%q, got %q", nsRSKey, "disabled", nsVal)
				}
				virtVal, err := getADCCustomVar(dynClient, adcName, adcNamespace, virtRSKey)
				if err != nil {
					return err
				}
				if virtVal != "disabled" {
					return fmt.Errorf("expected ADC %s=%q, got %q", virtRSKey, "disabled", virtVal)
				}
				workloadVal, err := getADCCustomVar(dynClient, adcName, adcNamespace, workloadRSKey)
				if err != nil {
					return err
				}
				if workloadVal != "disabled" {
					return fmt.Errorf("expected ADC %s=%q, got %q", workloadRSKey, "disabled", workloadVal)
				}
				podVal, err := getADCCustomVar(dynClient, adcName, adcNamespace, podRSKey)
				if err != nil {
					return err
				}
				if podVal != "disabled" {
					return fmt.Errorf("expected ADC %s=disabled, got %q", podRSKey, podVal)
				}
				return nil
			}, 3*time.Minute, 10*time.Second).Should(Succeed())
		})

		It("Should keep rightSizingDelegated as 'true' even when both features are disabled", func() {
			Eventually(func() error {
				val, err := getADCCustomVar(dynClient, adcName, adcNamespace, delegatedKey)
				if err != nil {
					return err
				}
				if val != "true" {
					return fmt.Errorf("expected ADC %s=%q, got %q", delegatedKey, "true", val)
				}
				return nil
			}, 2*time.Minute, 10*time.Second).Should(Succeed())
		})
	})

	AfterAll(func() {
		By("Re-enabling both RS features (restore to defaults)")
		Eventually(func() error {
			mco, err := dynClient.Resource(mcoGVR).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if err := unstructured.SetNestedField(mco.Object, true,
				"spec", "capabilities", "platform", "analytics", "namespaceRightSizingRecommendation", "enabled"); err != nil {
				return err
			}
			if err := unstructured.SetNestedField(mco.Object, true,
				"spec", "capabilities", "platform", "analytics", "virtualizationRightSizingRecommendation", "enabled"); err != nil {
				return err
			}
			for _, field := range []string{"workloadRightSizingRecommendation", "podRightSizingRecommendation"} {
				if err := unstructured.SetNestedField(mco.Object, true,
					"spec", "capabilities", "platform", "analytics", field, "enabled"); err != nil {
					return err
				}
			}
			_, err = dynClient.Resource(mcoGVR).Update(context.TODO(), mco, metav1.UpdateOptions{})
			return err
		}, 2*time.Minute, 10*time.Second).Should(Succeed())
	})
})

// staleGrafanaProfileRef matches leftover Grafana interpolations of the old
// "profile" variable ($profile or ${profile}) without matching $cpu_profile /
// $memory_profile.
var staleGrafanaProfileRef = regexp.MustCompile(`\$\{profile(?::[^}]*)?\}|\$profile([^a-zA-Z0-9_]|$)`)

// validateDashboardProfileVars fetches a Grafana dashboard ConfigMap and verifies that
// the embedded JSON contains cpu_profile and memory_profile template variables,
// with no stale $profile or ${profile} references.
func validateDashboardProfileVars(dynClient dynamic.Interface, cmName string) error {
	configMapGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	cm, err := dynClient.Resource(configMapGVR).
		Namespace("open-cluster-management-observability").
		Get(context.TODO(), cmName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get ConfigMap %s: %w", cmName, err)
	}

	data, found, err := unstructured.NestedStringMap(cm.Object, "data")
	if err != nil {
		return fmt.Errorf("read ConfigMap %s data: %w", cmName, err)
	}
	if !found {
		return fmt.Errorf("ConfigMap %s has no data field", cmName)
	}

	for key, dashJSON := range data {
		if !strings.HasSuffix(key, ".json") {
			continue
		}

		var dashboard struct {
			Templating struct {
				List []struct {
					Name string `json:"name"`
				} `json:"list"`
			} `json:"templating"`
		}
		if err := json.Unmarshal([]byte(dashJSON), &dashboard); err != nil {
			return fmt.Errorf("failed to parse dashboard JSON in %s/%s: %w", cmName, key, err)
		}

		varNames := make(map[string]bool)
		for _, v := range dashboard.Templating.List {
			varNames[v.Name] = true
		}

		if !varNames["cpu_profile"] {
			return fmt.Errorf("dashboard %s/%s missing cpu_profile variable", cmName, key)
		}
		if !varNames["memory_profile"] {
			return fmt.Errorf("dashboard %s/%s missing memory_profile variable", cmName, key)
		}
		if varNames["profile"] {
			return fmt.Errorf("dashboard %s/%s still has stale 'profile' variable (should be split into cpu_profile/memory_profile)", cmName, key)
		}

		if stale := staleGrafanaProfileRef.FindString(dashJSON); stale != "" {
			return fmt.Errorf("dashboard %s/%s still references %q in queries", cmName, key, stale)
		}
	}

	return nil
}
