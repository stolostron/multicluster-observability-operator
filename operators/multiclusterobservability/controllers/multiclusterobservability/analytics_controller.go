package multiclusterobservability

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

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
	rsPolicySetName        = "rs-policyset"
	rsPlacementName        = "rs-placement"
	rsPlacementBindingName = "rs-policyset-binding"
	// TODO - need to add validation for  Policy name +namespace should not exceed 62 char
	rsPrometheusRulePolicyName       = "rs-prom-rules-policy"
	rsPrometheusRulePolicyConfigName = "rs-prometheus-rules-policy-config"
	rsPrometheusRuleName             = "rs-namespace-prometheus-rules"
	rsConfigMapName                  = "rs-namespace-config"
	rsDefaultNamespace               = "open-cluster-management-global-set"
)

var (
	rsNamespace = rsDefaultNamespace
)

// PrometheusRuleSpec structure with intstr.IntOrString handling for expr

type PrometheusRuleSpecWithExpr struct {
	Groups []monitoringv1.RuleGroup `json:"groups"`
}

type RightSizingConfigMapData struct {
	NamespaceFilterCriteria struct {
		InclusionCriteria []string `yaml:"inclusionCriteria"`
		ExclusionCriteria []string `yaml:"exclusionCriteria"`
	} `yaml:"namespaceFilterCriteria"`
	LabelFilterCriteria []struct {
		LabelName         string   `yaml:"labelName"`
		InclusionCriteria []string `yaml:"inclusionCriteria,omitempty"`
		ExclusionCriteria []string `yaml:"exclusionCriteria,omitempty"`
	} `yaml:"labelFilterCriteria"`
	RecommendationPercentage int                `yaml:"recommendationPercentage"`
	PlacementConfiguration   policyv1.Placement `yaml:"placementConfiguration"`
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
			// log.Info("Existing ConfigMap", "ConfigMap", fmt.Sprintf("%+v", existingCM))
			log.Info("existingCM Values:")
			for key, value := range existingCM.Data {
				log.Info("Key: %s, Value: %s", key, value)
			}

			// existingCMYaml, err := yaml.Marshal(existingCM)
			// if err != nil {
			// 	log.Error(err, "RS - Unable to marshal existingCMYaml to YAML2")
			// 	return &ctrl.Result{}, err
			// }
			// fmt.Println("RS - existingCMYaml YAML content:")
			// fmt.Println(string(existingCMYaml))
		}

		log.Info("RS - Analytics.NamespaceRightSizing resource creation completed")
	}

	log.Info("RS - CreateAnalyticsComponent task completed6")
	return nil, nil
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

	// Define deafult namespaceFilterCriteria, labelFilterCriteria, placement definition
	namespaceFilterCriteria := map[string]interface{}{
		"exclusionCriteria": []string{"openshift.*"},
	}
	labelFilterCriteria := []map[string]interface{}{
		{
			"labelName":         "label_kubernetes_io_metadata_name",
			"exclusionCriteria": []string{"kube.*"},
		},
	}
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
		"namespaceFilterCriteria":  formatYAML(namespaceFilterCriteria),
		"labelFilterCriteria":      formatYAML(labelFilterCriteria),
		"recommendationPercentage": "110",
		"placementConfiguration":   formatYAML(placement), // Embed the serialized Placement YAML here
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

// unmarshalYAML unmarshals a YAML string into a Go data structure.
func unmarshalYAML(data string, target interface{}) error {
	if len(data) > 0 {
		return yaml.Unmarshal([]byte(data), target)
	}
	return fmt.Errorf("empty data string")
}

// getRightSizingConfigData extracts and unmarshals the data from the ConfigMap into RightSizingConfigData
func getRightSizingConfigData(cm *corev1.ConfigMap) (RightSizingConfigMapData, error) {
	log.Info("RS - inside getRightSizingConfigData")
	var configData RightSizingConfigMapData
	// Print the configMap object for debugging
	configMapJson, err := json.Marshal(cm)
	if err != nil {
		log.Error(err, "Failed to marshal ConfigMap to JSON")
		return configData, err
	}

	// Print the ConfigMap in JSON format
	fmt.Println("RS - ConfigMap content in JSON format:")
	fmt.Println(string(configMapJson))

	// Unmarshal namespaceFilterCriteria
	if err := yaml.Unmarshal([]byte(cm.Data["namespaceFilterCriteria"]), &configData.NamespaceFilterCriteria); err != nil {
		log.Error(err, "failed to unmarshal namespaceFilterCriteria")
		return configData, fmt.Errorf("failed to unmarshal namespaceFilterCriteria: %v", err)
	}

	// Unmarshal labelFilterCriteria
	if err := yaml.Unmarshal([]byte(cm.Data["labelFilterCriteria"]), &configData.LabelFilterCriteria); err != nil {
		log.Error(err, "failed to unmarshal labelFilterCriteria")
		return configData, fmt.Errorf("failed to unmarshal labelFilterCriteria: %v", err)
	}

	// Unmarshal recommendationPercentage
	if err := yaml.Unmarshal([]byte(cm.Data["recommendationPercentage"]), &configData.RecommendationPercentage); err != nil {
		log.Error(err, "failed to unmarshal recommendationPercentage")
		return configData, fmt.Errorf("failed to unmarshal recommendationPercentage: %v", err)
	}

	// Unmarshal placementConfiguration
	if cm.Data["placementConfiguration"] != "" {
		if err := yaml.Unmarshal([]byte(cm.Data["placementConfiguration"]), &configData.PlacementConfiguration); err != nil {
			log.Error(err, "failed to unmarshal placementConfiguration")
			return configData, fmt.Errorf("failed to unmarshal placementConfiguration: %v", err)
		}
	}

	// Log or process the `configData` as needed
	log.Info("ConfigMap Data successfully unmarshalled", "ConfigData", configData)

	configDataYaml, err := yaml.Marshal(configData)
	if err != nil {
		log.Error(err, "RS - Unable to marshal configDataYaml to YAML2")
		return configData, err
	}
	fmt.Println("RS - configDataYaml YAML content:")
	fmt.Println(string(configDataYaml))

	return configData, nil
}

func generatePrometheusRule(configData RightSizingConfigMapData) (monitoringv1.PrometheusRule, error) {

	// TODO - write logic to get PrometheusRule based on RightSizingConfigMapData
	duration := monitoringv1.Duration("5m")
	pr := &monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rsPrometheusRuleName,
			Namespace: rsNamespace,
		},
		Spec: monitoringv1.PrometheusRuleSpec{
			Groups: []monitoringv1.RuleGroup{
				{
					Name:     "acm-right-sizing-namespace-5m.rule",
					Interval: &duration,
					Rules: []monitoringv1.Rule{
						{
							Record: "acm_rs:namespace:cpu_request:5m",
							Expr:   intstr.FromString("max_over_time( sum( kube_pod_container_resource_requests{ namespace!~'openshift.*|xyz.*', container!='', resource='cpu'}) by (namespace)[5m:])"),
						},
					},
				},
			},
		},
	}
	return *pr, nil
}

func applyRSNamespaceConfigMapChanges(c client.Client, configData RightSizingConfigMapData) error {

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

	err = createUpdatePlacement(c)
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
					configpolicyv1.NonEmptyString(rsNamespace), // Convert string to NonEmptyString
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

		// Convert the Policy object to YAML
		policyYaml, err := yaml.Marshal(policy)
		if err != nil {
			log.Error(err, "RS - Unable to marshal policy to YAML")
			return err
		}
		fmt.Println("RS - Policy YAML content before creation:")
		fmt.Println(string(policyYaml))

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
		// Convert the Policy object to YAML
		policyYaml, err := yaml.Marshal(policy)
		if err != nil {
			log.Error(err, "RS - Unable to marshal policy to YAML")
			return err
		}
		fmt.Println("RS - Policy YAML content before updating:")
		fmt.Println(string(policyYaml))

		if err = c.Update(context.TODO(), policy); err != nil {
			log.Error(err, "Failed to update PrometheusRulePolicy")
			return err
		}
		log.Info("RS - PrometheusRulePolicy updated successfully", "Policy", rsPrometheusRulePolicyName)

	}

	return nil
}

// createUpdatePlacement creates the Placement resource
func createUpdatePlacement(c client.Client) error {
	log.Info("RS - Placement creation started")
	placement := &clusterv1beta1.Placement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rsPlacementName,
			Namespace: rsNamespace,
		},
	}

	err := c.Get(context.TODO(), types.NamespacedName{
		Namespace: rsNamespace,
		Name:      rsPlacementName,
	}, placement)
	log.Info("RS - fetch Placement completed2")

	placement.Spec = clusterv1beta1.PlacementSpec{
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
	}

	if err != nil && errors.IsNotFound(err) {

		log.Info("RS - Placement not found, creating a new one",
			"Namespace", placement.Namespace,
			"Name", placement.Name,
		)
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "RS - Unable to fetch Placement")
			return err
		}

		if err = c.Create(context.TODO(), placement); err != nil {
			log.Error(err, "Failed to create Placement")
			return err
		}
		log.Info("RS - Create Placement completed", "Placement", rsPlacementName)
	} else {
		log.Info("RS - Placement already exists, updating data",
			"Namespace", placement.Namespace,
			"Name", placement.Name,
		)
		if err = c.Update(context.TODO(), placement); err != nil {
			log.Error(err, "Failed to update Placement")
			return err
		}
		log.Info("RS - Placement updated successfully", "Placement", rsPlacementName)
	}

	log.Info("RS - Placement creation completed")
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
