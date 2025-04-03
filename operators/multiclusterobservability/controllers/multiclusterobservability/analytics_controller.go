package multiclusterobservability

import (
	"context"
	"fmt"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	globalPolicyName       = "default"
	clusterSetName         = "right-sizing"
	rsPolicySetName        = "rs-policyset"
	rsPlacementName        = "rs-placement"
	rsPlacementBindingName = "rs-policyset-binding"
	// TODO - rethink policy name, for time being sorterning it as Policy name +namespace should not exceed 62 char (Original name - rs-prometheus-rules-policy)
	rsPrometheusRulePolicyName       = "rs-prom-rules-policy"
	rsPrometheusRulePolicyConfigName = "rs-prometheus-rules-policy-config"
	rsPrometheusRuleName             = "rs-namespace-prometheus-rules"
	rsConfigMapName                  = "rs-namespace-config"
	// rsConfigFilePath                   = "/usr/local/manifests/base/right-sizing/namespace-prometheus-rule-config.yaml"
)

var (
	rsNamespace = "open-cluster-management-observability"
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

	log.Info("inside CreateAnalyticsComponent8")

	if mco.Spec.Capabilities != nil && mco.Spec.Capabilities.Platform != nil && mco.Spec.Capabilities.Platform.Analytics != nil && mco.Spec.Capabilities.Platform.Analytics.NamespaceRightSizingRecommendation != nil && mco.Spec.Capabilities.Platform.Analytics.NamespaceRightSizingRecommendation.Enabled {

		if mco.Spec.Capabilities.Platform.Analytics.NamespaceRightSizingRecommendation.NamespaceBinding != "" {
			rsNamespace = mco.Spec.Capabilities.Platform.Analytics.NamespaceRightSizingRecommendation.NamespaceBinding
		}

		log.Info("Analytics.NamespaceRightSizing spec presents8")

		// Check if the ConfigMap already exists
		existingCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rsConfigMapName,
				Namespace: rsNamespace, // Specify the namespace for the ConfigMap
			},
		}
		err := c.Get(context.TODO(), types.NamespacedName{
			Namespace: rsNamespace,
			Name:      rsConfigMapName,
		}, existingCM)
		log.Info("RS - fetch configmap completed2")

		// If the ConfigMap doesn't exist, create it
		if err != nil && errors.IsNotFound(err) {

			log.Info("RS - Creating a new test config",
				"Namespace", rsNamespace,
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
