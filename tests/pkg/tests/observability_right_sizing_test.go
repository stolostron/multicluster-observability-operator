// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/rendering"
	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/utils/ptr"
	addonapiv1beta1 "open-cluster-management.io/api/addon/v1beta1"
	"sigs.k8s.io/yaml"
)

const (
	namespaceRightSizingFlag      = "namespaceRightSizingRecommendation"
	virtualizationRightSizingFlag = "virtualizationRightSizingRecommendation"
	workloadRightSizingFlag       = "workloadRightSizingRecommendation"
	podRightSizingFlag            = "podRightSizingRecommendation"
	rightSizingDelegatedKey       = "rightSizingDelegated"
	analyticsNamespace            = "observability-analytics"
	podDetailDashboardName        = "acm-rs-pod-detail"
	podDetailSpecName             = "Should deploy a pod-only detail dashboard without tables and with all query variables"
	workloadPodResourcesSpecName  = "Should create separate workload and pod right-sizing resources"
	workloadRightSizingMetric     = "acm_rs:workload:"
)

var rightSizingDashboardGVR = schema.GroupVersionResource{Group: "perses.dev", Version: "v1alpha2", Resource: "persesdashboards"}

var rightSizingADCKeys = map[string]string{
	namespaceRightSizingFlag:      "platformNamespaceRightSizing",
	virtualizationRightSizingFlag: "platformVirtualizationRightSizing",
	workloadRightSizingFlag:       "platformWorkloadRightSizing",
	podRightSizingFlag:            "platformPodRightSizing",
}

// A nil value requests that the enabled field be omitted, allowing the operator to default it.
type rightSizingFlags map[string]*bool

func allRightSizingFlags(enabled bool) rightSizingFlags {
	flags := make(rightSizingFlags, len(rightSizingADCKeys))
	for flag := range rightSizingADCKeys {
		flags[flag] = ptr.To(enabled)
	}
	return flags
}

func rightSizingFlagPath(flag string) []string {
	return []string{"spec", "capabilities", "platform", "analytics", flag, "enabled"}
}

func setRightSizingFlags(ctx context.Context, mcoClient dynamic.ResourceInterface, flags rightSizingFlags) {
	GinkgoHelper()
	Eventually(ctx, func(ctx context.Context) error {
		mco, err := mcoClient.Get(ctx, MCO_CR_NAME, metav1.GetOptions{})
		if err != nil {
			return err
		}
		changed := false
		for flag, enabled := range flags {
			path := rightSizingFlagPath(flag)
			value, found, err := unstructured.NestedBool(mco.Object, path...)
			if err != nil {
				return err
			}
			switch {
			case enabled == nil && found:
				unstructured.RemoveNestedField(mco.Object, path...)
				changed = true
			case enabled != nil && (!found || value != *enabled):
				if err := unstructured.SetNestedField(mco.Object, *enabled, path...); err != nil {
					return err
				}
				changed = true
			}
		}
		if !changed {
			return nil
		}
		// Each retry applies the requested leaves to a fresh resourceVersion.
		_, err = mcoClient.Update(ctx, mco, metav1.UpdateOptions{})
		return err
	}, 2*time.Minute, 5*time.Second).Should(Succeed())
}

func configureRightSizing(ctx context.Context, client dynamic.Interface, flags rightSizingFlags) {
	GinkgoHelper()
	mcoClient := client.Resource(utils.NewMCOGVRV1BETA2())
	original := make(rightSizingFlags, len(flags))
	Eventually(ctx, func(ctx context.Context) error {
		mco, err := mcoClient.Get(ctx, MCO_CR_NAME, metav1.GetOptions{})
		if err != nil {
			return err
		}
		for flag := range flags {
			enabled, found, err := unstructured.NestedBool(mco.Object, rightSizingFlagPath(flag)...)
			if err != nil {
				return err
			}
			original[flag] = nil
			if found {
				original[flag] = ptr.To(enabled)
			}
		}
		return nil
	}, 2*time.Minute, 5*time.Second).Should(Succeed())

	DeferCleanup(func(ctx SpecContext) {
		By("Restoring the original right-sizing flags")
		setRightSizingFlags(ctx, mcoClient, original)
		waitForRightSizingState(ctx, client, original)
	}, NodeTimeout(8*time.Minute))
	setRightSizingFlags(ctx, mcoClient, flags)
}

func waitForRightSizingState(ctx context.Context, client dynamic.Interface, flags rightSizingFlags) {
	GinkgoHelper()
	mcoClient := client.Resource(utils.NewMCOGVRV1BETA2())
	Eventually(ctx, func(ctx context.Context) error {
		mco, err := mcoClient.Get(ctx, MCO_CR_NAME, metav1.GetOptions{})
		if err != nil {
			return err
		}
		adcValues := map[string]string{rightSizingDelegatedKey: "true"}
		for flag, expected := range flags {
			value, found, err := unstructured.NestedBool(mco.Object, rightSizingFlagPath(flag)...)
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("waiting for the operator to default %s.enabled", flag)
			}
			if expected != nil && value != *expected {
				return fmt.Errorf("expected %s.enabled=%t, got %t", flag, *expected, value)
			}
			adcValues[rightSizingADCKeys[flag]] = rightSizingADCValue(value)
		}
		if err := checkRightSizingADC(ctx, client, adcValues); err != nil {
			return err
		}

		var instance mcov1beta2.MultiClusterObservability
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(mco.Object, &instance); err != nil {
			return fmt.Errorf("decode MCO capabilities: %w", err)
		}
		rightSizingEnabled := rendering.RightSizingEnabled(&instance)
		mcoaEnabled := rendering.MCOAEnabled(&instance)
		if !rightSizingEnabled && !mcoaEnabled {
			if err := checkRightSizingManagerStopped(ctx, client); err != nil {
				return err
			}
		}
		var incidentDetection, metricsUI bool
		if capabilities := instance.Spec.Capabilities; capabilities != nil && capabilities.Platform != nil {
			incidentDetection = capabilities.Platform.Analytics.IncidentDetection.Enabled
			metricsUI = rendering.MCOAPlatformMetricsEnabled(&instance) && capabilities.Platform.Metrics.UI.Enabled
		}
		// When the manager stays up, wait for it to act on the disabled flags even if deletion has not started yet.
		removeAnalytics := mcoaEnabled && !rightSizingEnabled && !incidentDetection
		if err := checkRightSizingNamespaces(ctx, client, removeAnalytics, removeAnalytics && !metricsUI); err != nil {
			return err
		}
		// Incident detection keeps the analytics namespace and the Perses API available.
		if !rightSizingEnabled && incidentDetection {
			if err := checkRightSizingFeatureResources(ctx, client, "workload", false); err != nil {
				return err
			}
			return checkRightSizingFeatureResources(ctx, client, "pod", false)
		}
		return nil
	}, 5*time.Minute, 5*time.Second).Should(Succeed())
}

func checkRightSizingManagerStopped(ctx context.Context, client dynamic.Interface) error {
	deployments := client.Resource(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}).Namespace(MCO_NAMESPACE)
	if err := checkRightSizingResource(ctx, deployments, mcoaManagerDeploymentName, false); err != nil {
		return err
	}
	// The Deployment can disappear before its pods finish shutting down.
	pods, err := client.Resource(schema.GroupVersionResource{Version: "v1", Resource: "pods"}).Namespace(MCO_NAMESPACE).
		List(ctx, metav1.ListOptions{LabelSelector: "app=" + mcoaManagerDeploymentName})
	if err != nil {
		return fmt.Errorf("list MCOA manager pods: %w", err)
	}
	if len(pods.Items) != 0 {
		return fmt.Errorf("MCOA manager pod %s still exists", pods.Items[0].GetName())
	}
	return nil
}

func checkRightSizingNamespaces(ctx context.Context, client dynamic.Interface, removeAnalytics, removeCOO bool) error {
	namespaces := client.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"})
	for _, expected := range []struct {
		name   string
		remove bool
	}{{analyticsNamespace, removeAnalytics}, {"openshift-cluster-observability-operator", removeCOO}} {
		namespace, err := namespaces.Get(ctx, expected.name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("get namespace %s: %w", expected.name, err)
		}
		if namespace.GetLabels()["app.kubernetes.io/managed-by"] == "mcoa-hub-resource-reconciler" &&
			(expected.remove || namespace.GetDeletionTimestamp() != nil) {
			return fmt.Errorf("waiting for MCOA-managed namespace %s to be removed (deletionTimestamp=%v)",
				expected.name, namespace.GetDeletionTimestamp())
		}
	}
	return nil
}

var _ = Describe("Right-sizing: independently enabled defaults", Ordered, func() {
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

	It("Should default all right-sizing flags to enabled", func(ctx SpecContext) {
		By("Omitting the right-sizing flags so the operator defaults them")
		omitted := make(rightSizingFlags, len(rightSizingADCKeys))
		for flag := range rightSizingADCKeys {
			omitted[flag] = nil
		}
		configureRightSizing(ctx, dynClient, omitted)
		waitForRightSizingState(ctx, dynClient, allRightSizingFlags(true))
	}, NodeTimeout(10*time.Minute))
})

var _ = Describe("RHACM4K-55205: Enable and teardown namespace right-sizing recommendation (rightsizing/g0)", Ordered, func() {
	var (
		policyGVR    = schema.GroupVersionResource{Group: "policy.open-cluster-management.io", Version: "v1", Resource: "policies"}
		configMapGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
		prGVR        = schema.GroupVersionResource{Group: "monitoring.coreos.com", Version: "v1", Resource: "prometheusrules"}
	)

	BeforeAll(func(ctx SpecContext) {
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
		Eventually(ctx, func(ctx context.Context) error {
			_, err := hubClient.CoreV1().Namespaces().Get(ctx, "open-cluster-management-global-set", metav1.GetOptions{})
			if err == nil {
				return nil
			}
			if apierrors.IsNotFound(err) {
				_, createErr := hubClient.CoreV1().Namespaces().Create(ctx, &v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "open-cluster-management-global-set"},
				}, metav1.CreateOptions{})
				return createErr
			}
			return err
		}, 2*time.Minute, 10*time.Second).Should(Succeed())

		By("Enabling namespace right-sizing")
		// Keep workload right-sizing active while checking removal of this feature's resources.
		configureRightSizing(ctx, dynClient, rightSizingFlags{
			namespaceRightSizingFlag: ptr.To(true),
			workloadRightSizingFlag:  ptr.To(true),
		})
		DeferCleanup(func(ctx SpecContext) {
			disableRightSizingFeature(ctx, dynClient, namespaceRightSizingFlag, "rs-namespace-config", "acm-rs-namespace-prometheus-rules")
		}, NodeTimeout(10*time.Minute))
	}, NodeTimeout(7*time.Minute))

	It("Should set ADC right-sizing delegation to 'true' and namespace RS to 'enabled'", func(ctx SpecContext) {
		Eventually(ctx, func(ctx context.Context) error {
			return checkRightSizingADC(ctx, dynClient, map[string]string{
				rightSizingDelegatedKey:                      "true",
				rightSizingADCKeys[namespaceRightSizingFlag]: "enabled",
			})
		}, 3*time.Minute, 10*time.Second).Should(Succeed())
	}, NodeTimeout(4*time.Minute))

	It("Should NOT create any Policy resources (always-MCOA mode)", func() {
		_, err := dynClient.Resource(policyGVR).
			Namespace("open-cluster-management-global-set").
			Get(context.TODO(), "rs-prom-rules-policy", metav1.GetOptions{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "rs-prom-rules-policy should NOT exist in always-MCOA mode")
	})

	It("Should create the ConfigMap 'rs-namespace-config' in namespace 'open-cluster-management-observability'", func() {
		Eventually(func() error {
			_, err := dynClient.Resource(configMapGVR).
				Namespace("open-cluster-management-observability").
				Get(context.TODO(), "rs-namespace-config", metav1.GetOptions{})
			return err
		}, 2*time.Minute, 10*time.Second).Should(Succeed())
	})

	// Rebuilding the addon and its work can take a few minutes when this group starts with MCOA disabled.
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
})

var _ = Describe("RHACM4K-58751: Enable and teardown virtualization right-sizing recommendation (rightsizing/g1)", Ordered, func() {
	var (
		policyGVR    = schema.GroupVersionResource{Group: "policy.open-cluster-management.io", Version: "v1", Resource: "policies"}
		configMapGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
		prGVR        = schema.GroupVersionResource{Group: "monitoring.coreos.com", Version: "v1", Resource: "prometheusrules"}
	)

	BeforeAll(func(ctx SpecContext) {
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
		Eventually(ctx, func(ctx context.Context) error {
			_, err := hubClient.CoreV1().Namespaces().Get(ctx, "open-cluster-management-global-set", metav1.GetOptions{})
			if err == nil {
				return nil
			}
			if apierrors.IsNotFound(err) {
				_, createErr := hubClient.CoreV1().Namespaces().Create(ctx, &v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "open-cluster-management-global-set"},
				}, metav1.CreateOptions{})
				return createErr
			}
			return err
		}, 2*time.Minute, 10*time.Second).Should(Succeed())

		By("Enabling virtualization right-sizing")
		// Keep workload right-sizing active while checking removal of this feature's resources.
		configureRightSizing(ctx, dynClient, rightSizingFlags{
			virtualizationRightSizingFlag: ptr.To(true),
			workloadRightSizingFlag:       ptr.To(true),
		})
		DeferCleanup(func(ctx SpecContext) {
			disableRightSizingFeature(ctx, dynClient, virtualizationRightSizingFlag, "rs-virt-config", "acm-rs-virt-prometheus-rules")
		}, NodeTimeout(10*time.Minute))
	}, NodeTimeout(7*time.Minute))

	It("Should set ADC right-sizing delegation to 'true' and virtualization RS to 'enabled'", func(ctx SpecContext) {
		Eventually(ctx, func(ctx context.Context) error {
			return checkRightSizingADC(ctx, dynClient, map[string]string{
				rightSizingDelegatedKey:                           "true",
				rightSizingADCKeys[virtualizationRightSizingFlag]: "enabled",
			})
		}, 3*time.Minute, 10*time.Second).Should(Succeed())
	}, NodeTimeout(4*time.Minute))

	It("Should NOT create any Policy resources (always-MCOA mode)", func() {
		_, err := dynClient.Resource(policyGVR).
			Namespace("open-cluster-management-global-set").
			Get(context.TODO(), "rs-virt-prom-rules-policy", metav1.GetOptions{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "rs-virt-prom-rules-policy should NOT exist in always-MCOA mode")
	})

	It("Should create the ConfigMap 'rs-virt-config' in namespace 'open-cluster-management-observability'", func() {
		Eventually(func() error {
			_, err := dynClient.Resource(configMapGVR).
				Namespace("open-cluster-management-observability").
				Get(context.TODO(), "rs-virt-config", metav1.GetOptions{})
			return err
		}, 2*time.Minute, 10*time.Second).Should(Succeed())
	})

	// Rebuilding the addon and its work can take a few minutes when this group starts with MCOA disabled.
	It("Should find the PrometheusRule 'acm-rs-virt-prometheus-rules' in namespace 'openshift-monitoring'", func() {
		Eventually(func() error {
			_, err := dynClient.Resource(prGVR).
				Namespace("openshift-monitoring").
				Get(context.TODO(), "acm-rs-virt-prometheus-rules", metav1.GetOptions{})
			return err
		}, 5*time.Minute, 10*time.Second).Should(Succeed())
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
})

func disableRightSizingFeature(ctx SpecContext, client dynamic.Interface, flag, configMapName, ruleName string) {
	GinkgoHelper()
	By(fmt.Sprintf("Disabling %s and checking resource removal", flag))
	setRightSizingFlags(ctx, client.Resource(utils.NewMCOGVRV1BETA2()), rightSizingFlags{flag: ptr.To(false)})
	Eventually(ctx, func(ctx context.Context) error {
		return checkRightSizingADC(ctx, client, map[string]string{rightSizingADCKeys[flag]: "disabled"})
	}, 3*time.Minute, 10*time.Second).Should(Succeed())

	configMaps := client.Resource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).Namespace(MCO_NAMESPACE)
	rules := client.Resource(utils.NewPrometheusRuleGVR()).Namespace("openshift-monitoring")
	Eventually(ctx, func(ctx context.Context) error {
		return checkRightSizingResource(ctx, configMaps, configMapName, false)
	}, 2*time.Minute, 10*time.Second).Should(Succeed())
	Eventually(ctx, func(ctx context.Context) error {
		return checkRightSizingResource(ctx, rules, ruleName, false)
	}, 2*time.Minute, 10*time.Second).Should(Succeed())
}

func rightSizingADCValue(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func checkRightSizingADC(ctx context.Context, client dynamic.Interface, expected map[string]string) error {
	adcName := utils.MCOA_CLUSTER_MANAGEMENT_ADDON_NAME
	obj, err := client.Resource(utils.NewMCOAddOnDeploymentConfigGVR()).Namespace(MCO_NAMESPACE).
		Get(ctx, adcName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get ADC %s/%s: %w", MCO_NAMESPACE, adcName, err)
	}

	var adc addonapiv1beta1.AddOnDeploymentConfig
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &adc); err != nil {
		return fmt.Errorf("decode ADC %s/%s: %w", MCO_NAMESPACE, adcName, err)
	}

	values := make(map[string]string, len(adc.Spec.CustomizedVariables))
	for _, variable := range adc.Spec.CustomizedVariables {
		values[variable.Name] = variable.Value
	}
	for key, want := range expected {
		got, found := values[key]
		if !found || got != want {
			return fmt.Errorf("expected ADC %s=%q, got %q (present=%t)", key, want, got, found)
		}
	}
	return nil
}

func checkRightSizingResource(ctx context.Context, client dynamic.ResourceInterface, name string, wantPresent bool) error {
	obj, err := client.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !wantPresent && apierrors.IsNotFound(err) {
			// An unserved API also returns 404. Require the error to identify the object.
			var status apierrors.APIStatus
			if errors.As(err, &status) {
				if details := status.Status().Details; details != nil && details.Name == name {
					return nil
				}
			}
		}
		return fmt.Errorf("get %s: %w", name, err)
	}
	if !wantPresent || obj.GetDeletionTimestamp() != nil {
		return fmt.Errorf("unexpected state for %s/%s %s (expected present=%t, deletionTimestamp=%v, finalizers=%v)",
			obj.GetNamespace(), name, obj.GetKind(), wantPresent, obj.GetDeletionTimestamp(), obj.GetFinalizers())
	}
	return nil
}

func checkRightSizingFeatureResources(ctx context.Context, client dynamic.Interface, level string, wantPresent bool) error {
	for _, resource := range []struct {
		gvr             schema.GroupVersionResource
		namespace, name string
	}{
		{schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}, MCO_NAMESPACE, "rs-" + level + "-config"},
		{utils.NewPrometheusRuleGVR(), "openshift-monitoring", "acm-rs-" + level + "-prometheus-rules"},
		{rightSizingDashboardGVR, analyticsNamespace, "acm-rs-" + level + "-overview"},
		{rightSizingDashboardGVR, analyticsNamespace, "acm-rs-" + level + "-detail"},
	} {
		resources := client.Resource(resource.gvr).Namespace(resource.namespace)
		if err := checkRightSizingResource(ctx, resources, resource.name, wantPresent); err != nil {
			return fmt.Errorf("check %s in %s: %w", resource.gvr.Resource, resource.namespace, err)
		}
	}
	return nil
}

var _ = Describe("Always-MCOA right-sizing: ADC reflects CR spec state (rightsizing/g2)", Ordered, func() {
	var (
		mcoClient    dynamic.ResourceInterface
		policyGVR    = schema.GroupVersionResource{Group: "policy.open-cluster-management.io", Version: "v1", Resource: "policies"}
		configMapGVR = schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	)

	BeforeAll(func(ctx SpecContext) {
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

		mcoClient = dynClient.Resource(utils.NewMCOGVRV1BETA2())
		By("Enabling all right-sizing features in the MCO CR")
		configureRightSizing(ctx, dynClient, allRightSizingFlags(true))
	}, NodeTimeout(5*time.Minute))

	Context("when all RS features are enabled", Ordered, func() {
		It("Should always set rightSizingDelegated to 'true'", func(ctx SpecContext) {
			Eventually(ctx, func(ctx context.Context) error {
				return checkRightSizingADC(ctx, dynClient, map[string]string{rightSizingDelegatedKey: "true"})
			}, 3*time.Minute, 10*time.Second).Should(Succeed())
		}, NodeTimeout(4*time.Minute))

		It("Should set all ADC RS variables to 'enabled'", func(ctx SpecContext) {
			waitForRightSizingState(ctx, dynClient, allRightSizingFlags(true))
		}, NodeTimeout(6*time.Minute))

		It(workloadPodResourcesSpecName, func(ctx SpecContext) {
			Eventually(ctx, func(ctx context.Context) error {
				if err := checkRightSizingFeatureResources(ctx, dynClient, "workload", true); err != nil {
					return err
				}
				return checkRightSizingFeatureResources(ctx, dynClient, "pod", true)
			}, 5*time.Minute, 10*time.Second).Should(Succeed())
		}, NodeTimeout(6*time.Minute))

		It(podDetailSpecName, func(ctx SpecContext) {
			Eventually(ctx, func(ctx context.Context) error {
				dashboard, err := dynClient.Resource(rightSizingDashboardGVR).Namespace(analyticsNamespace).
					Get(ctx, podDetailDashboardName, metav1.GetOptions{})
				if err != nil {
					return fmt.Errorf("read %s PersesDashboard %s/%s: %w", rightSizingDashboardGVR.Version, analyticsNamespace, podDetailDashboardName, err)
				}
				return validatePodDetailDashboard(dashboard)
			}, 5*time.Minute, 10*time.Second).Should(Succeed())
		}, NodeTimeout(6*time.Minute))

		JustAfterEach(func(ctx SpecContext) {
			report := CurrentSpecReport()
			if report.Failed() && (report.LeafNodeText == podDetailSpecName || report.LeafNodeText == workloadPodResourcesSpecName) {
				logPodDetailDashboardVersions(ctx, dynClient, rightSizingDashboardGVR)
			}
		}, NodeTimeout(30*time.Second))

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
			_, err := dynClient.Resource(policyGVR).
				Namespace(MCO_GLOBAL_SET_NAMESPACE).
				Get(context.TODO(), "rs-prom-rules-policy", metav1.GetOptions{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "rs-prom-rules-policy should NOT exist")
		})
	})

	Context("when namespace RS is disabled but virtualization RS remains enabled", Ordered, func() {
		BeforeAll(func(ctx SpecContext) {
			By("Disabling namespace right-sizing and keeping the other features enabled")
			flags := allRightSizingFlags(true)
			flags[namespaceRightSizingFlag] = ptr.To(false)
			setRightSizingFlags(ctx, mcoClient, flags)
		}, NodeTimeout(3*time.Minute))

		It("Should set namespace RS to 'disabled' and keep virtualization RS 'enabled'", func(ctx SpecContext) {
			Eventually(ctx, func(ctx context.Context) error {
				return checkRightSizingADC(ctx, dynClient, map[string]string{
					rightSizingADCKeys[namespaceRightSizingFlag]:      "disabled",
					rightSizingADCKeys[virtualizationRightSizingFlag]: "enabled",
				})
			}, 3*time.Minute, 10*time.Second).Should(Succeed())
		}, NodeTimeout(4*time.Minute))

		It("Should keep rightSizingDelegated as 'true' regardless of individual feature state", func(ctx SpecContext) {
			Eventually(ctx, func(ctx context.Context) error {
				return checkRightSizingADC(ctx, dynClient, map[string]string{rightSizingDelegatedKey: "true"})
			}, 2*time.Minute, 10*time.Second).Should(Succeed())
		}, NodeTimeout(3*time.Minute))
	})

	for _, state := range []struct {
		name          string
		workload, pod bool
	}{
		{"workload only", true, false},
		{"pod only", false, true},
	} {
		It("Should independently reconcile "+state.name, func(ctx SpecContext) {
			disabledLevel, disabledFlag := "workload", workloadRightSizingFlag
			if state.workload {
				disabledLevel, disabledFlag = "pod", podRightSizingFlag
			}
			// A retry can start after an earlier attempt has already disabled the feature.
			setRightSizingFlags(ctx, mcoClient, rightSizingFlags{disabledFlag: ptr.To(true)})
			By("Waiting for " + disabledLevel + " right-sizing resources to exist before disabling the feature")
			Eventually(ctx, func(ctx context.Context) error {
				return checkRightSizingFeatureResources(ctx, dynClient, disabledLevel, true)
			}, 5*time.Minute, 10*time.Second).Should(Succeed())

			flags := allRightSizingFlags(false)
			flags[workloadRightSizingFlag] = ptr.To(state.workload)
			flags[podRightSizingFlag] = ptr.To(state.pod)
			setRightSizingFlags(ctx, mcoClient, flags)

			expected := map[string]string{rightSizingDelegatedKey: "true"}
			for flag, value := range flags {
				expected[rightSizingADCKeys[flag]] = rightSizingADCValue(*value)
			}
			Eventually(ctx, func(ctx context.Context) error {
				if err := checkRightSizingADC(ctx, dynClient, expected); err != nil {
					return err
				}
				if err := checkRightSizingFeatureResources(ctx, dynClient, "workload", state.workload); err != nil {
					return err
				}
				return checkRightSizingFeatureResources(ctx, dynClient, "pod", state.pod)
			}, 5*time.Minute, 10*time.Second).Should(Succeed())
		}, NodeTimeout(15*time.Minute))
	}

	Context("when all RS features are disabled", Ordered, func() {
		disabledFlags := allRightSizingFlags(false)

		BeforeAll(func(ctx SpecContext) {
			By("Waiting for the MCOA manager and pod right-sizing resources before disabling all right-sizing features")
			setRightSizingFlags(ctx, mcoClient, rightSizingFlags{podRightSizingFlag: ptr.To(true)})
			Eventually(ctx, func(ctx context.Context) error {
				deployment, err := hubClient.AppsV1().Deployments(MCO_NAMESPACE).Get(ctx, mcoaManagerDeploymentName, metav1.GetOptions{})
				if err != nil {
					return fmt.Errorf("get MCOA manager Deployment: %w", err)
				}
				if deployment.DeletionTimestamp != nil || deployment.Status.ObservedGeneration < deployment.Generation || deployment.Status.AvailableReplicas == 0 {
					return fmt.Errorf("waiting for the MCOA manager Deployment to be available (generation=%d, observedGeneration=%d, availableReplicas=%d, deletionTimestamp=%v)",
						deployment.Generation, deployment.Status.ObservedGeneration, deployment.Status.AvailableReplicas, deployment.DeletionTimestamp)
				}
				return checkRightSizingFeatureResources(ctx, dynClient, "pod", true)
			}, 5*time.Minute, 5*time.Second).Should(Succeed())
			By("Disabling all right-sizing features")
			setRightSizingFlags(ctx, mcoClient, disabledFlags)
			By("Waiting for the disabled state to reconcile")
			waitForRightSizingState(ctx, dynClient, disabledFlags)
		}, NodeTimeout(15*time.Minute))

		It("Should set all ADC RS variables to 'disabled'", func(ctx SpecContext) {
			expected := make(map[string]string, len(rightSizingADCKeys))
			for _, key := range rightSizingADCKeys {
				expected[key] = "disabled"
			}
			Eventually(ctx, func(ctx context.Context) error {
				return checkRightSizingADC(ctx, dynClient, expected)
			}, 3*time.Minute, 10*time.Second).Should(Succeed())
		}, NodeTimeout(4*time.Minute))

		It("Should keep rightSizingDelegated as 'true' even when all features are disabled", func(ctx SpecContext) {
			Eventually(ctx, func(ctx context.Context) error {
				return checkRightSizingADC(ctx, dynClient, map[string]string{rightSizingDelegatedKey: "true"})
			}, 2*time.Minute, 10*time.Second).Should(Succeed())
		}, NodeTimeout(3*time.Minute))
	})
})

func validatePodDetailDashboard(dashboard *unstructured.Unstructured) error {
	if dashboard.GetDeletionTimestamp() != nil {
		return fmt.Errorf("pod detail dashboard is being deleted")
	}
	variables, found, err := unstructured.NestedSlice(dashboard.Object, "spec", "config", "variables")
	if err != nil {
		return fmt.Errorf("read pod detail spec.config.variables: %w", err)
	}
	if !found {
		return fmt.Errorf("pod detail spec.config.variables is missing")
	}
	names := make(map[string]struct{}, len(variables))
	for _, variable := range variables {
		v, ok := variable.(map[string]any)
		if !ok {
			return fmt.Errorf("unexpected pod detail variable type %T", variable)
		}
		name, _, err := unstructured.NestedString(v, "spec", "name")
		if err != nil {
			return fmt.Errorf("read pod detail variable name: %w", err)
		}
		names[name] = struct{}{}
		matchers, _, err := unstructured.NestedStringSlice(v, "spec", "plugin", "spec", "matchers")
		if err != nil {
			return fmt.Errorf("read pod detail %s matchers: %w", name, err)
		}
		if slices.ContainsFunc(matchers, func(matcher string) bool {
			return strings.Contains(matcher, workloadRightSizingMetric)
		}) {
			return fmt.Errorf("pod detail variable %s selects workload-level right-sizing data", name)
		}
		kind, _, err := unstructured.NestedString(v, "spec", "plugin", "kind")
		if err != nil {
			return fmt.Errorf("read pod detail %s variable plugin kind: %w", name, err)
		}
		if kind == "PrometheusPromQLVariable" {
			expr, _, err := unstructured.NestedString(v, "spec", "plugin", "spec", "expr")
			if err != nil {
				return fmt.Errorf("read pod detail %s PromQL variable expression: %w", name, err)
			}
			if strings.Contains(expr, workloadRightSizingMetric) {
				return fmt.Errorf("pod detail variable %s selects workload-level right-sizing data", name)
			}
		}
		switch name {
		case "cluster", "namespace", "pod":
			labelName, _, err := unstructured.NestedString(v, "spec", "plugin", "spec", "labelName")
			if err != nil {
				return fmt.Errorf("read pod detail %s selector label: %w", name, err)
			}
			if labelName != name {
				return fmt.Errorf("pod detail %s selector uses label %q", name, labelName)
			}
			if !slices.ContainsFunc(matchers, func(matcher string) bool {
				return matcher == "acm_rs:pod:info" || strings.HasPrefix(matcher, "acm_rs:pod:info{")
			}) {
				return fmt.Errorf("pod detail %s selector must use acm_rs:pod:info", name)
			}
		}
	}
	for _, name := range []string{"days", "cluster", "namespace", "pod", "cpu_profile", "memory_profile"} {
		if _, found := names[name]; !found {
			return fmt.Errorf("pod detail variable %q is missing", name)
		}
	}

	panels, _, err := unstructured.NestedMap(dashboard.Object, "spec", "config", "panels")
	if err != nil {
		return fmt.Errorf("read pod detail spec.config.panels: %w", err)
	}
	if len(panels) == 0 {
		return fmt.Errorf("pod detail spec.config.panels is empty or missing")
	}
	return validatePodDetailQueries(panels)
}

func validatePodDetailQueries(panels map[string]any) error {
	hasPodQuery := false
	for name, value := range panels {
		panel, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("unexpected type %T for pod detail panel %s", value, name)
		}
		kind, _, err := unstructured.NestedString(panel, "spec", "plugin", "kind")
		if err != nil {
			return fmt.Errorf("read pod detail panel %s plugin kind: %w", name, err)
		}
		if kind == "" || kind == "Table" {
			return fmt.Errorf("unexpected plugin kind %q for pod detail panel %s", kind, name)
		}
		queries, _, err := unstructured.NestedSlice(panel, "spec", "queries")
		if err != nil {
			return fmt.Errorf("read pod detail panel %s queries: %w", name, err)
		}
		for _, query := range queries {
			q, ok := query.(map[string]any)
			if !ok {
				return fmt.Errorf("unexpected query type %T in pod detail panel %s", query, name)
			}
			text, _, err := unstructured.NestedString(q, "spec", "plugin", "spec", "query")
			if err != nil {
				return fmt.Errorf("read pod detail panel %s query: %w", name, err)
			}
			if strings.Contains(text, workloadRightSizingMetric) {
				return fmt.Errorf("pod detail panel %s queries workload-level right-sizing data", name)
			}
			hasPodQuery = hasPodQuery || strings.Contains(text, "acm_rs:pod:")
		}
	}
	if !hasPodQuery {
		return fmt.Errorf("pod detail panels have no pod-level right-sizing query")
	}
	return nil
}

func logPodDetailDashboardVersions(ctx context.Context, client dynamic.Interface, gvr schema.GroupVersionResource) {
	for _, version := range []string{gvr.Version, "v1alpha1"} {
		gvr.Version = version
		readCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		dashboard, err := client.Resource(gvr).Namespace(analyticsNamespace).Get(readCtx, podDetailDashboardName, metav1.GetOptions{})
		cancel()
		if err != nil {
			GinkgoWriter.Printf("Read PersesDashboard %s/%s via %s: %v\n", analyticsNamespace, podDetailDashboardName, version, err)
			continue
		}
		spec, _, specErr := unstructured.NestedMap(dashboard.Object, "spec")
		config, _, configErr := unstructured.NestedMap(spec, "config")
		GinkgoWriter.Printf("PersesDashboard %s/%s via %s uid=%s resourceVersion=%s deletionTimestamp=%v\n  spec: %s (err=%v)\n  spec.config: %s (err=%v)\n",
			analyticsNamespace, podDetailDashboardName, version, dashboard.GetUID(), dashboard.GetResourceVersion(), dashboard.GetDeletionTimestamp(),
			summarizePersesDashboardSpec(spec), specErr, summarizePersesDashboardSpec(config), configErr)
	}
}

func summarizePersesDashboardSpec(spec map[string]any) string {
	variables, _, variablesErr := unstructured.NestedSlice(spec, "variables")
	names := make([]string, 0, len(variables))
	for _, variable := range variables {
		if v, ok := variable.(map[string]any); ok {
			name, _, _ := unstructured.NestedString(v, "spec", "name")
			names = append(names, name)
		}
	}
	panels, _, panelsErr := unstructured.NestedMap(spec, "panels")
	return fmt.Sprintf("fields=%v variables=%v (err=%v) panels=%d (err=%v)", slices.Sorted(maps.Keys(spec)), names, variablesErr, len(panels), panelsErr)
}

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
