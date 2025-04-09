package analytics

import (
	"context"
	"fmt"
	"reflect"

	"github.com/cloudflare/cfssl/log"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ensureRSNamespaceConfigMapExists ensures that the ConfigMap exists, creating it if necessary
func ensureRSNamespaceConfigMapExists(c client.Client) error {
	// Check if the ConfigMap already exists
	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rsConfigMapName,
			Namespace: config.GetDefaultNamespace(),
		},
	}

	// Fetch the ConfigMap
	err := c.Get(context.TODO(), types.NamespacedName{
		Name:      rsConfigMapName,
		Namespace: config.GetDefaultNamespace(),
	}, existingCM)
	log.Info("RS - fetch configmap completed2")

	// If the ConfigMap doesn't exist, create it
	if err != nil && errors.IsNotFound(err) {
		log.Info("RS - Creating a new test config", "Namespace", config.GetDefaultNamespace(), "Name", rsConfigMapName)
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "RS - Unable to fetch ConfigMap")
			return err
		}

		// Get configmap data
		existingCM.Data = getDefaultRSNamespaceConfig()

		// Create the ConfigMap
		err := c.Create(context.TODO(), existingCM)
		if err != nil {
			log.Error(err, "RS - Failed to create ConfigMap", "ConfigMap", rsConfigMapName)
			return err
		}
		log.Info("RS - Created configMap completed", "ConfigMap", rsConfigMapName)
	} else {
		log.Info("RS - ConfigMap already exists, skipping creation", "ConfigMap", rsConfigMapName, "namespace", rsNamespace)
	}

	return nil
}

func getDefaultRSNamespaceConfig() map[string]string {
	// get deafult config data with PrometheusRule config and placement config

	var ruleConfig RSPrometheusRuleConfig
	ruleConfig.NamespaceFilterCriteria.ExclusionCriteria = []string{"openshift.*"}
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

func GetNamespaceRSConfigMapPredicateFunc(c client.Client) predicate.Funcs {
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
