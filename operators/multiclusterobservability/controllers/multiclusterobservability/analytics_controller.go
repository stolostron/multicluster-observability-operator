package multiclusterobservability

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"

	configpolicyv1 "open-cluster-management.io/config-policy-controller/api/v1"

	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	rsPolicySetName                   = "rs-policyset"
	rsPlacementName                   = "rs-placement"
	rsPlacementBindingName            = "rs-policyset-binding"
	rsPrometheusRulePolicyName        = "rs-prom-rules-policy"
	rsPrometheusRulePolicyConfigName  = "rs-prometheus-rules-policy-config"
	rsPrometheusRuleName              = "rs-namespace-prometheus-rules"
	rsConfigMapName                   = "rs-namespace-config"
	rsDefaultNamespace                = "open-cluster-management-global-set"
	rsMonitoringNamespace             = "openshift-monitoring"
	rsDefaultRecommendationPercentage = 110
)

var (
	rsNamespace = rsDefaultNamespace
)

type RSLabelFilter struct {
	LabelName         string   `yaml:"labelName"`
	InclusionCriteria []string `yaml:"inclusionCriteria,omitempty"`
	ExclusionCriteria []string `yaml:"exclusionCriteria,omitempty"`
}

type RSPrometheusRuleConfig struct {
	NamespaceFilterCriteria struct {
		InclusionCriteria []string `yaml:"inclusionCriteria"`
		ExclusionCriteria []string `yaml:"exclusionCriteria"`
	} `yaml:"namespaceFilterCriteria"`
	LabelFilterCriteria      []RSLabelFilter `yaml:"labelFilterCriteria"`
	RecommendationPercentage int             `yaml:"recommendationPercentage"`
}

type RSNamespaceConfigMapData struct {
	PrometheusRuleConfig   RSPrometheusRuleConfig   `yaml:"prometheusRuleConfig"`
	PlacementConfiguration clusterv1beta1.Placement `yaml:"placementConfiguration"`
}

// CreateAnalyticsComponent is used to enable the analytics component like right-sizing
func CreateAnalyticsComponent(
	c client.Client,
	scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability,
	mgr ctrl.Manager) (*ctrl.Result, error) {

	log.Info("inside CreateAnalyticsComponent9")

	// Check if the analytics right-sizing namespace recommendation configuration is enabled
	if isRightSizingNamespaceEnabled(mco) {

		if mco.Spec.Capabilities.Platform.Analytics.NamespaceRightSizingRecommendation.NamespaceBinding != "" {
			rsNamespace = mco.Spec.Capabilities.Platform.Analytics.NamespaceRightSizingRecommendation.NamespaceBinding
		}

		log.Info("Analytics.NamespaceRightSizing spec presents8")

		// Check if the ConfigMap already exists
		existingCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rsConfigMapName,
				Namespace: config.GetDefaultNamespace(), // Specify the namespace for the ConfigMap
			},
		}
		//  TODO - check if we can get actual instance rather than context.TODO()
		err := c.Get(context.TODO(), types.NamespacedName{
			Name:      rsConfigMapName,
			Namespace: config.GetDefaultNamespace(),
		}, existingCM)
		log.Info("RS - fetch configmap completed2")

		// If the ConfigMap doesn't exist, create it
		if err != nil && errors.IsNotFound(err) {

			log.Info("RS - Creating a new test config",
				"Namespace", config.GetDefaultNamespace(),
				"Name", rsConfigMapName,
			)
			if client.IgnoreNotFound(err) != nil {
				log.Error(err, "RS - Unable to fetch ConfigMap")
				return &ctrl.Result{}, err
			}

			// Get configmap data
			existingCM.Data = getDefaultRSNamespaceConfig()

			// Create the ConfigMap
			err := c.Create(context.TODO(), existingCM)
			if err != nil {
				log.Error(err, "RS - Failed to create ConfigMap", "ConfigMap", rsConfigMapName)
				return &ctrl.Result{}, err
			}
			log.Info("RS - Created configMap completed", "ConfigMap", rsConfigMapName)
		} else {
			log.Info("RS - ConfigMap already exists, skipping creation", "ConfigMap", rsConfigMapName, "namespace", rsNamespace)
		}

		log.Info("RS - Analytics.NamespaceRightSizing resource creation completed")
	} else {

		// Change ComplianceType to "MustNotHave" for PrometheusRule deletion
		// As deleting Policy doesn't explicitly delete related PrometheusRule
		modifyComplianceTypeIfPolicyExists(c)

		// Cleanup created resources if available
		cleanupRSNamespaceResources(c, rsNamespace)
	}

	log.Info("RS - CreateAnalyticsComponent task completed6")
	return nil, nil
}

func modifyComplianceTypeIfPolicyExists(c client.Client) error {
	policy := &policyv1.Policy{}
	err := c.Get(context.TODO(), types.NamespacedName{
		Name:      rsPrometheusRulePolicyName,
		Namespace: rsNamespace,
	}, policy)

	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Policy does not exist. Skipping update.")
			return nil
		}
		log.Error(err, "Error retrieving the policy")
		return err
	}

	// Unmarshal the inner ConfigurationPolicy
	for _, pt := range policy.Spec.PolicyTemplates {
		var configPolicy configpolicyv1.ConfigurationPolicy
		err := json.Unmarshal(pt.ObjectDefinition.Raw, &configPolicy)
		if err != nil {
			log.Error(err, "Failed to unmarshal ConfigurationPolicy from PolicyTemplate")
			return err
		}

		// Change ComplianceType if it's "MustOnlyHave"
		changed := false
		for _, objTemplate := range configPolicy.Spec.ObjectTemplates {
			if objTemplate.ComplianceType == "MustOnlyHave" {
				objTemplate.ComplianceType = "MustNotHave"
				changed = true
			}
		}

		if changed {
			// Marshal the modified ConfigurationPolicy back into JSON
			modifiedRaw, err := json.Marshal(configPolicy)
			if err != nil {
				log.Error(err, "Failed to marshal modified ConfigurationPolicy")
				return err
			}

			pt.ObjectDefinition = runtime.RawExtension{Raw: modifiedRaw}
		}
	}

	// Update the modified policy
	err = c.Update(context.TODO(), policy)
	if err != nil {
		log.Error(err, "Failed to update the modified policy")
		return err
	}

	log.Info("Successfully updated ComplianceType in policy")

	// Wait for 5 seconds
	time.Sleep(5 * time.Second)

	return nil
}

func cleanupRSNamespaceResources(c client.Client, namespace string) {
	log.Info("RS - Cleaning up NamespaceRightSizing resources")

	// Define all objects to delete
	resourcesToDelete := []client.Object{
		&policyv1.PlacementBinding{ObjectMeta: metav1.ObjectMeta{Name: rsPlacementBindingName, Namespace: namespace}},
		&clusterv1beta1.Placement{ObjectMeta: metav1.ObjectMeta{Name: rsPlacementName, Namespace: namespace}},
		&policyv1.Policy{ObjectMeta: metav1.ObjectMeta{Name: rsPrometheusRulePolicyName, Namespace: namespace}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: rsConfigMapName, Namespace: config.GetDefaultNamespace()}},
	}

	// Iterate and delete each resource if it exists
	for _, resource := range resourcesToDelete {
		err := c.Delete(context.TODO(), resource)
		if err != nil {
			if errors.IsNotFound(err) {
				// Do nothing if resource is not found
				continue
			}
			log.Error(err, "Failed to delete resource", "Resource", resource.GetObjectKind().GroupVersionKind().Kind, "Name", resource.GetName())
		} else {
			log.Info("Deleted resource successfully", "Resource", resource.GetObjectKind().GroupVersionKind().Kind, "Name", resource.GetName())
		}
	}

	// Set default namespace again
	rsNamespace = rsDefaultNamespace
}

// isRightSizingNamespaceEnabled checks if the right-sizing namespace analytics feature is enabled in the provided MCO configuration.
func isRightSizingNamespaceEnabled(mco *mcov1beta2.MultiClusterObservability) bool {
	return mco.Spec.Capabilities != nil &&
		mco.Spec.Capabilities.Platform != nil &&
		mco.Spec.Capabilities.Platform.Analytics != nil &&
		mco.Spec.Capabilities.Platform.Analytics.NamespaceRightSizingRecommendation != nil &&
		mco.Spec.Capabilities.Platform.Analytics.NamespaceRightSizingRecommendation.Enabled
}

func getDefaultRSNamespaceConfig() map[string]string {
	// get deafult config data - namespaceFilterCriteria, labelFilterCriteria, placement definition

	var ruleConfig RSPrometheusRuleConfig
	ruleConfig.NamespaceFilterCriteria.InclusionCriteria = []string{"prod.*"}
	ruleConfig.NamespaceFilterCriteria.ExclusionCriteria = []string{"openshift.*"}
	ruleConfig.LabelFilterCriteria = []RSLabelFilter{
		{
			LabelName:         "label_kubernetes_io_metadata_name",
			InclusionCriteria: []string{"prod", "staging"},
			ExclusionCriteria: []string{"kube.*"},
		},
	}
	ruleConfig.RecommendationPercentage = rsDefaultRecommendationPercentage

	placement := &clusterv1beta1.Placement{
		Spec: clusterv1beta1.PlacementSpec{
			Predicates: []clusterv1beta1.ClusterPredicate{},
			Tolerations: []clusterv1beta1.Toleration{
				{
					Key:      "cluster.open-cluster-management.io/unreachable",
					Operator: clusterv1beta1.TolerationOpExists,
				},
				{
					Key:      "cluster.open-cluster-management.io/unavailable",
					Operator: clusterv1beta1.TolerationOpExists,
				},
			},
		},
	}

	return map[string]string{
		"prometheusRuleConfig":   formatYAML(ruleConfig),
		"placementConfiguration": formatYAML(placement),
	}
}

// formatYAML converts a Go data structure to a YAML-formatted string
func formatYAML(data interface{}) string {
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		log.Error(err, "Error marshaling data to YAML: %v")
	}
	return string(yamlData)
}

// getRightSizingConfigData extracts and unmarshals the data from the ConfigMap into RightSizingConfigData
func getRightSizingConfigData(cm *corev1.ConfigMap) (RSNamespaceConfigMapData, error) {
	log.Info("RS - inside getRightSizingConfigData")
	var configData RSNamespaceConfigMapData

	// Unmarshal namespaceFilterCriteria
	if err := yaml.Unmarshal([]byte(cm.Data["prometheusRuleConfig"]), &configData.PrometheusRuleConfig); err != nil {
		log.Error(err, "failed to unmarshal prometheusRuleConfig")
		return configData, fmt.Errorf("failed to unmarshal prometheusRuleConfig: %v", err)
	}

	// Unmarshal placementConfiguration
	if cm.Data["placementConfiguration"] != "" {
		if err := yaml.Unmarshal([]byte(cm.Data["placementConfiguration"]), &configData.PlacementConfiguration); err != nil {
			log.Error(err, "failed to unmarshal placementConfiguration")
			return configData, fmt.Errorf("failed to unmarshal placementConfiguration: %v", err)

		}
	}

	// Log or process the `configData` as needed
	log.Info("ConfigMap Data successfully unmarshalled")

	return configData, nil
}

func generatePrometheusRule(configData RSNamespaceConfigMapData) (monitoringv1.PrometheusRule, error) {
	ns := configData.PrometheusRuleConfig.NamespaceFilterCriteria
	recommendationPercentage := configData.PrometheusRuleConfig.RecommendationPercentage

	// Enforce only one of inclusion/exclusion for namespaces
	if len(ns.InclusionCriteria) > 0 && len(ns.ExclusionCriteria) > 0 {
		return monitoringv1.PrometheusRule{}, fmt.Errorf("only one of inclusion or exclusion allowed for namespaceFilterCriteria")
	}

	// Build namespace filter
	var nsFilter string
	if len(ns.InclusionCriteria) > 0 {
		nsFilter = fmt.Sprintf(`namespace=~"%s"`, strings.Join(ns.InclusionCriteria, "|"))
	} else if len(ns.ExclusionCriteria) > 0 {
		nsFilter = fmt.Sprintf(`namespace!~"%s"`, strings.Join(ns.ExclusionCriteria, "|"))
	} else {
		nsFilter = `namespace!=""`
	}

	// Build label_env filter only if label filter is provided
	var labelJoin string
	for _, l := range configData.PrometheusRuleConfig.LabelFilterCriteria {
		if l.LabelName != "label_env" {
			continue
		}
		if len(l.InclusionCriteria) > 0 && len(l.ExclusionCriteria) > 0 {
			return monitoringv1.PrometheusRule{}, fmt.Errorf("only one of inclusion or exclusion allowed for label_env")
		}
		if len(l.InclusionCriteria) > 0 {
			selector := fmt.Sprintf(`kube_namespace_labels{label_env=~"%s"}`, strings.Join(l.InclusionCriteria, "|"))
			labelJoin = fmt.Sprintf(`* on (namespace) group_left() (%s or kube_namespace_labels{label_env=""})`, selector)
		} else if len(l.ExclusionCriteria) > 0 {
			selector := fmt.Sprintf(`kube_namespace_labels{label_env!~"%s"}`, strings.Join(l.ExclusionCriteria, "|"))
			labelJoin = fmt.Sprintf(`* on (namespace) group_left() (%s or kube_namespace_labels{label_env=""})`, selector)
		}
		break
	}

	// Define durations
	duration5m := monitoringv1.Duration("5m")
	duration1d := monitoringv1.Duration("15m")

	// Helper for rules
	rule := func(record, metricExpr string) monitoringv1.Rule {
		expr := metricExpr
		if labelJoin != "" {
			expr = fmt.Sprintf("%s %s", metricExpr, labelJoin)
		}
		return monitoringv1.Rule{
			Record: record,
			Expr:   intstr.FromString(expr),
		}
	}
	ruleWithLabels := func(record, expr string) monitoringv1.Rule {
		return monitoringv1.Rule{
			Record: record,
			Expr:   intstr.FromString(expr),
			Labels: map[string]string{
				"profile":     "Max OverAll",
				"aggregation": "1d",
			},
		}
	}

	// Group: namespace 5m
	nsRules5m := []monitoringv1.Rule{
		rule("acm_rs:namespace:cpu_request_hard:5m", fmt.Sprintf(`max_over_time(sum(kube_resourcequota{resource=~"requests.cpu", type="hard", %s}) by (namespace)[5m:])`, nsFilter)),
		rule("acm_rs:namespace:cpu_request:5m", fmt.Sprintf(`max_over_time(sum(kube_pod_container_resource_requests{%s, resource="cpu", container!=""}) by (namespace)[5m:])`, nsFilter)),
		rule("acm_rs:namespace:cpu_usage:5m", fmt.Sprintf(`max_over_time(sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{%s, container!=""}) by (namespace)[5m:])`, nsFilter)),
		rule("acm_rs:namespace:memory_request_hard:5m", fmt.Sprintf(`max_over_time(sum(kube_resourcequota{resource=~"requests.memory", type="hard", %s}) by (namespace)[5m:])`, nsFilter)),
		rule("acm_rs:namespace:memory_request:5m", fmt.Sprintf(`max_over_time(sum(kube_pod_container_resource_requests{%s, resource="memory", container!=""}) by (namespace)[5m:])`, nsFilter)),
		rule("acm_rs:namespace:memory_usage:5m", fmt.Sprintf(`max_over_time(sum(container_memory_working_set_bytes{%s, container!=""}) by (namespace)[5m:])`, nsFilter)),
	}

	// Group: namespace 1d
	nsRules1d := []monitoringv1.Rule{
		ruleWithLabels("acm_rs:namespace:cpu_request_hard", `max_over_time(acm_rs:namespace:cpu_request_hard:5m[1d])`),
		ruleWithLabels("acm_rs:namespace:cpu_request", `max_over_time(acm_rs:namespace:cpu_request:5m[1d])`),
		ruleWithLabels("acm_rs:namespace:cpu_usage", `max_over_time(acm_rs:namespace:cpu_usage:5m[1d])`),
		ruleWithLabels("acm_rs:namespace:cpu_recommendation", fmt.Sprintf(`max_over_time(acm_rs:namespace:cpu_usage{profile="Max OverAll"}[1d]) * (1 + (%d/100))`, recommendationPercentage)),
		ruleWithLabels("acm_rs:namespace:memory_request_hard", `max_over_time(acm_rs:namespace:memory_request_hard:5m[1d])`),
		ruleWithLabels("acm_rs:namespace:memory_request", `max_over_time(acm_rs:namespace:memory_request:5m[1d])`),
		ruleWithLabels("acm_rs:namespace:memory_usage", `max_over_time(acm_rs:namespace:memory_usage:5m[1d])`),
		ruleWithLabels("acm_rs:namespace:memory_recommendation", fmt.Sprintf(`max_over_time(acm_rs:namespace:memory_usage{profile="Max OverAll"}[1d]) * (1 + (%d/100))`, recommendationPercentage)),
	}

	// Group: cluster 5m
	clusterRules5m := []monitoringv1.Rule{
		rule("acm_rs:cluster:cpu_request_hard:5m", fmt.Sprintf(`max_over_time(sum(kube_resourcequota{resource=~"requests.cpu", type="hard", %s}) by (cluster)[5m:])`, nsFilter)),
		rule("acm_rs:cluster:cpu_request:5m", fmt.Sprintf(`max_over_time(sum(kube_pod_container_resource_requests{%s, resource="cpu", container!=""}) by (cluster)[5m:])`, nsFilter)),
		rule("acm_rs:cluster:cpu_usage:5m", fmt.Sprintf(`max_over_time(sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{%s, container!=""}) by (cluster)[5m:])`, nsFilter)),
		rule("acm_rs:cluster:memory_request_hard:5m", fmt.Sprintf(`max_over_time(sum(kube_resourcequota{resource=~"requests.memory", type="hard", %s}) by (cluster)[5m:])`, nsFilter)),
		rule("acm_rs:cluster:memory_request:5m", fmt.Sprintf(`max_over_time(sum(kube_pod_container_resource_requests{%s, resource="memory", container!=""}) by (cluster)[5m:])`, nsFilter)),
		rule("acm_rs:cluster:memory_usage:5m", fmt.Sprintf(`max_over_time(sum(container_memory_working_set_bytes{%s, container!=""}) by (cluster)[5m:])`, nsFilter)),
	}

	// Group: cluster 1d
	clusterRules1d := []monitoringv1.Rule{
		ruleWithLabels("acm_rs:cluster:cpu_request_hard", `max_over_time(acm_rs:cluster:cpu_request_hard:5m[1d])`),
		ruleWithLabels("acm_rs:cluster:cpu_request", `max_over_time(acm_rs:cluster:cpu_request:5m[1d])`),
		ruleWithLabels("acm_rs:cluster:cpu_usage", `max_over_time(acm_rs:cluster:cpu_usage:5m[1d])`),
		ruleWithLabels("acm_rs:cluster:cpu_recommendation", fmt.Sprintf(`max_over_time(acm_rs:cluster:cpu_usage{profile="Max OverAll"}[1d]) * (1 + (%d/100))`, recommendationPercentage)),
		ruleWithLabels("acm_rs:cluster:memory_request_hard", `max_over_time(acm_rs:cluster:memory_request_hard:5m[1d])`),
		ruleWithLabels("acm_rs:cluster:memory_request", `max_over_time(acm_rs:cluster:memory_request:5m[1d])`),
		ruleWithLabels("acm_rs:cluster:memory_usage", `max_over_time(acm_rs:cluster:memory_usage:5m[1d])`),
		ruleWithLabels("acm_rs:cluster:memory_recommendation", fmt.Sprintf(`max_over_time(acm_rs:cluster:memory_usage{profile="Max OverAll"}[1d]) * (1 + (%d/100))`, recommendationPercentage)),
	}

	return monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rsPrometheusRuleName,
			Namespace: rsNamespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "PrometheusRule",
			APIVersion: "monitoring.coreos.com/v1",
		},
		Spec: monitoringv1.PrometheusRuleSpec{
			Groups: []monitoringv1.RuleGroup{
				{Name: "acm-right-sizing-namespace-5m.rule", Interval: &duration5m, Rules: nsRules5m},
				{Name: "acm-right-sizing-namespace-1d.rules", Interval: &duration1d, Rules: nsRules1d},
				{Name: "acm-right-sizing-cluster-5m.rule", Interval: &duration5m, Rules: clusterRules5m},
				{Name: "acm-right-sizing-cluster-1d.rule", Interval: &duration1d, Rules: clusterRules1d},
			},
		},
	}, nil
}

func applyRSNamespaceConfigMapChanges(c client.Client, configData RSNamespaceConfigMapData) error {

	log.Info("inside applyRSNamespaceConfigMapChanges")

	prometheusRule, err := generatePrometheusRule(configData)
	if err != nil {
		log.Error(err, "Error while calling generatePrometheusRule")
		return err
	}

	err = createOrUpdatePrometheusRulePolicy(c, prometheusRule)
	if err != nil {
		log.Error(err, "Error while calling createOrUpdatePrometheusRulePolicy")
		return err
	}

	err = createUpdatePlacement(c, configData.PlacementConfiguration)
	if err != nil {
		log.Error(err, "Error while calling createUpdatePlacement")
		return err
	}

	err = createPlacementBinding(c)
	if err != nil {
		log.Error(err, "Error while calling createPlacementBinding")
		return err
	}
	log.Info("applyRSNamespaceConfigMapChanges completed")

	return nil
}

// Function to add "apiVersion" and "kind" to a Kubernetes object
func addAPIVersionAndKind(obj interface{}, apiVersion, kind string) ([]byte, error) {

	// Step 1: Marshal object to JSON
	objJSON, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("error marshalling object: %w", err)
	}

	// Step 2: Convert JSON into a map to modify fields
	var objMap map[string]interface{}
	if err := json.Unmarshal(objJSON, &objMap); err != nil {
		return nil, fmt.Errorf("error unmarshalling JSON: %w", err)
	}

	// Step 3: Inject "apiVersion" and "kind"
	objMap["apiVersion"] = apiVersion
	objMap["kind"] = kind

	// Step 4: Convert back to JSON
	finalJSON, err := json.Marshal(objMap)
	if err != nil {
		return nil, fmt.Errorf("error re-marshalling JSON: %w", err)
	}

	return finalJSON, nil
}

func createOrUpdatePrometheusRulePolicy(c client.Client, prometheusRule monitoringv1.PrometheusRule) error {

	policy := &policyv1.Policy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy.open-cluster-management.io/v1",
			Kind:       "Policy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      rsPrometheusRulePolicyName,
			Namespace: rsNamespace,
		},
	}
	log.Info("RS - Policy object created")

	errPolicy := c.Get(context.TODO(), types.NamespacedName{
		Name:      policy.Name,
		Namespace: policy.Namespace,
	}, policy)

	if errPolicy != nil && !errors.IsNotFound(errPolicy) {
		fmt.Println("Error retriving Policy:", errPolicy)
		return errPolicy
	}

	// Marshal the PrometheusRule object into JSON
	promRuleJSON, err := addAPIVersionAndKind(prometheusRule, "monitoring.coreos.com/v1", "PrometheusRule")
	if err != nil {
		fmt.Println("Error marshaling ConfigurationPolicy:", err)
	}

	// Define the ConfigurationPolicy object
	configPolicy := configpolicyv1.ConfigurationPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy.open-cluster-management.io/v1",
			Kind:       "ConfigurationPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: rsPrometheusRulePolicyConfigName,
		},
		Spec: &configpolicyv1.ConfigurationPolicySpec{
			RemediationAction: "inform",
			Severity:          "low",
			NamespaceSelector: configpolicyv1.Target{
				Include: []configpolicyv1.NonEmptyString{
					configpolicyv1.NonEmptyString(rsMonitoringNamespace),
				},
			},
			ObjectTemplates: []*configpolicyv1.ObjectTemplate{
				{
					ComplianceType: "MustOnlyHave",
					ObjectDefinition: runtime.RawExtension{
						Raw: promRuleJSON,
					},
				},
			},
		},
	}

	// Marshal the ConfigurationPolicy object into JSON
	configPolicyJSON, err := json.Marshal(configPolicy)
	if err != nil {
		fmt.Println("Error marshaling ConfigurationPolicy:", err)
		return err
	}

	policy.Spec = policyv1.PolicySpec{
		RemediationAction: "enforce",
		Disabled:          false,
		PolicyTemplates: []*policyv1.PolicyTemplate{
			{
				ObjectDefinition: runtime.RawExtension{
					Raw: configPolicyJSON,
				},
				// ObjectDefinition: runtime.RawExtension{
				// 	Object: &prometheusRule, // Use marshaled YAML of PrometheusRule
				// },
			},
		},
	}

	if errors.IsNotFound(errPolicy) {

		log.Info("RS - PrometheusRulePolicy not found, creating a new one",
			"Namespace", rsNamespace,
			"Name", rsPrometheusRulePolicyName,
		)
		if client.IgnoreNotFound(errPolicy) != nil {
			log.Error(errPolicy, "RS - Unable to fetch PrometheusRulePolicy")
			return errPolicy
		}

		if err = c.Create(context.TODO(), policy); err != nil {
			log.Error(err, "Failed to create PrometheusRulePolicy")
			return err
		}
		log.Info("RS - Created PrometheusRulePolicy completed", "Policy", rsPrometheusRulePolicyName)
	} else {
		log.Info("RS - PrometheusRulePolicy already exists, updating data",
			"Namespace", rsNamespace,
			"Name", rsPrometheusRulePolicyName,
		)

		if err = c.Update(context.TODO(), policy); err != nil {
			log.Error(err, "Failed to update PrometheusRulePolicy")
			return err
		}
		log.Info("RS - PrometheusRulePolicy updated successfully", "Policy", rsPrometheusRulePolicyName)

	}

	return nil
}

// createUpdatePlacement creates the Placement resource
func createUpdatePlacement(c client.Client, placementConfig clusterv1beta1.Placement) error {
	log.Info("RS - inside createUpdatePlacement")

	// Convert the Policy object to YAML
	placementYAML, err := yaml.Marshal(placementConfig)
	if err != nil {
		log.Error(err, "RS - Unable to marshal placement to YAML")
		return err
	}
	fmt.Println("RS - palcementYaml content before updating(from configmap):")
	fmt.Println(string(placementYAML))

	placement := &clusterv1beta1.Placement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rsPlacementName,
			Namespace: rsNamespace,
		},
	}
	key := types.NamespacedName{
		Namespace: rsNamespace,
		Name:      rsPlacementName,
	}

	if err := c.Get(context.TODO(), key, placement); errors.IsNotFound(err) {
		log.Info("RS - Placement not found, creating a new one", "Namespace", placement.Namespace, "Name", placement.Name)

		placement.Spec = placementConfig.Spec
		log.Info("RS - Updated Placement Spec")

		if err := c.Create(context.TODO(), placement); err != nil {
			log.Error(err, "Failed to create Placement")
			return err
		}

		log.Info("RS - Placement created", "Placement", placement.Name)
		return nil
	}

	if err != nil {
		log.Error(err, "RS - Unable to fetch Placement")
		return err
	}

	log.Info("RS - Placement exists, updating", "Namespace", placement.Namespace, "Name", placement.Name)

	placement.Spec = placementConfig.Spec
	log.Info("RS - Updated Placement Spec")

	if err := c.Update(context.TODO(), placement); err != nil {
		log.Error(err, "Failed to update Placement")
		return err
	}

	log.Info("RS - Placement updated", "Placement", placement.Name)
	return nil
}

// createPlacementBinding creates the PlacementBinding resource
func createPlacementBinding(c client.Client) error {
	log.Info("RS - PlacementBinding creation started")
	placementBinding := &policyv1.PlacementBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rsPlacementBindingName,
			Namespace: rsNamespace,
		},
	}
	err := c.Get(context.TODO(), types.NamespacedName{
		Namespace: placementBinding.Namespace,
		Name:      placementBinding.Name,
	}, placementBinding)
	log.Info("RS - fetch PlacementBinding completed2")

	if err != nil && errors.IsNotFound(err) {

		log.Info("RS - PlacementBinding not found, creating a new one",
			"Namespace", placementBinding.Namespace,
			"Name", placementBinding.Name,
		)
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "RS - Unable to fetch PlacementBinding")
			return err
		}

		placementBinding.PlacementRef = policyv1.PlacementSubject{
			Name:     rsPlacementName,
			APIGroup: "cluster.open-cluster-management.io",
			Kind:     "Placement",
		}
		placementBinding.Subjects = []policyv1.Subject{
			{
				Name:     rsPrometheusRulePolicyName,
				APIGroup: "policy.open-cluster-management.io",
				Kind:     "Policy",
			},
		}

		if err = c.Create(context.TODO(), placementBinding); err != nil {
			log.Error(err, "Failed to create Placement")
			return err
		}
		log.Info("RS - Create PlacementBinding completed", "PlacementBinding", rsPlacementBindingName)
	}
	log.Info("RS - PlacementBinding creation completed")
	return nil
}

func getNamespaceRSConfigMapPredicateFunc(c client.Client) predicate.Funcs {
	log.Info("RS - Watch for ConfigMap events set up started8")

	processConfigMap := func(cm *corev1.ConfigMap) bool {
		configData, err := getRightSizingConfigData(cm)
		if err != nil {
			log.Error(err, "Failed to extract RightSizingConfigData")
			return false
		}
		log.Info("Successfully unmarshalled ConfigMap data", "configData", configData)

		// Apply changes based on the config map
		if err := applyRSNamespaceConfigMapChanges(c, configData); err != nil {
			log.Error(err, "Failed to apply RS Namespace ConfigMap Changes")
			return false
		}
		return true
	}

	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			log.Info("inside create configmap - rs8", "name", e.Object.GetName(), "Namespace", e.Object.GetNamespace())
			if e.Object.GetName() == rsConfigMapName && e.Object.GetNamespace() == config.GetDefaultNamespace() {
				log.Info("Found matching configmap name - rs8")
				if cm, ok := e.Object.(*corev1.ConfigMap); ok {
					return processConfigMap(cm)
				}
			}
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			log.Info("inside update configmap - rs8", "name", e.ObjectNew.GetName(), "Namespace", e.ObjectNew.GetNamespace())
			if e.ObjectNew.GetName() != rsConfigMapName || e.ObjectNew.GetNamespace() != config.GetDefaultNamespace() {
				return true
			}

			// Check if the ConfigMap `Data` has changed before proceeding
			oldCM, oldOK := e.ObjectOld.(*corev1.ConfigMap)
			newCM, newOK := e.ObjectNew.(*corev1.ConfigMap)

			if oldOK && newOK && reflect.DeepEqual(oldCM.Data, newCM.Data) {
				log.Info("No changes detected in ConfigMap data, skipping update")
				return true
			}

			log.Info("ConfigMap data has changed, processing update")
			return processConfigMap(newCM)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			log.Info("inside delete configmap - rs8", "name", e.Object.GetName(), "Namespace", e.Object.GetNamespace())
			if e.Object.GetName() == rsConfigMapName && e.Object.GetNamespace() == config.GetDefaultNamespace() {
				log.Info("found matching configmap name")
			}

			// TODO - Question - if user deletes configmap what to do ? I belive reconsiler logic will create configmap is not exist and then again configmap predicate functiona will be called to craete PrometheusRulePolicy etc
			return true
		},
	}
}
