// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics

import (
	"context"
	"fmt"
	"reflect"

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
func EnsureRSNamespaceConfigMapExists(ctx context.Context, c client.Client) error {
	// Check if the ConfigMap already exists
	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rsConfigMapName,
			Namespace: config.GetDefaultNamespace(),
		},
	}

	// Declare name, namespace in common log context and use it later everywhere
	logCtx := []any{"namespace: ", existingCM.Namespace, ", name:", existingCM.Name}

	// Get the configmap
	err := c.Get(ctx, types.NamespacedName{
		Name:      existingCM.Name,
		Namespace: existingCM.Namespace,
	}, existingCM)

	if err != nil {
		if errors.IsNotFound(err) {

			existingCM.Data = GetDefaultRSNamespaceConfig()

			// Create the Configmap
			if err := c.Create(ctx, existingCM); err != nil {
				return fmt.Errorf("rs - failed to create configmap: %w", err)
			}

			log.Info("rs - configmap created successfully", logCtx...)
			return nil
		}
		return fmt.Errorf("rs - failed to fetch configmap: %w", err)
	}

	log.V(1).Info("rs - configmap already exists, skipping creation", logCtx...)
	return nil
}

func GetDefaultRSNamespaceConfig() map[string]string {
	// get default config data with PrometheusRule config and placement config

	var ruleConfig RSPrometheusRuleConfig
	ruleConfig.NamespaceFilterCriteria.ExclusionCriteria = []string{"openshift.*"}
	ruleConfig.RecommendationPercentage = rsDefaultRecommendationPercentage

	placement := clusterv1beta1.Placement{
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
		"prometheusRuleConfig":   FormatYAML(ruleConfig),
		"placementConfiguration": FormatYAML(placement),
	}
}

// GetRightSizingConfigData extracts and unmarshals the data from the ConfigMap into RightSizingConfigData
func GetRightSizingConfigData(cm *corev1.ConfigMap) (RSNamespaceConfigMapData, error) {
	var configData RSNamespaceConfigMapData

	// Unmarshal namespaceFilterCriteria
	if err := yaml.Unmarshal([]byte(cm.Data["prometheusRuleConfig"]), &configData.PrometheusRuleConfig); err != nil {
		return configData, fmt.Errorf("failed to unmarshal prometheusRuleConfig: %v", err)
	}

	// Unmarshal placementConfiguration
	if cm.Data["placementConfiguration"] != "" {
		if err := yaml.Unmarshal([]byte(cm.Data["placementConfiguration"]), &configData.PlacementConfiguration); err != nil {
			return configData, fmt.Errorf("failed to unmarshal placementConfiguration: %v", err)
		}
	}

	log.V(1).Info("rs - configmap data successfully unmarshalled")

	return configData, nil
}

func GetNamespaceRSConfigMapPredicateFunc(ctx context.Context, c client.Client) predicate.Funcs {
	log.Info("rs - watch for ConfigMap events set up started")

	processConfigMap := func(cm *corev1.ConfigMap) bool {
		configData, err := GetRightSizingConfigData(cm)
		if err != nil {
			log.Error(err, "rs - failed to extract config data")
			return false
		}

		// Apply changes based on the config map
		if err := applyRSNamespaceConfigMapChanges(ctx, c, configData); err != nil {
			log.Error(err, "rs - failed to apply configmap changes")
			return false
		}
		return true
	}

	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetName() == rsConfigMapName && e.Object.GetNamespace() == config.GetDefaultNamespace() {
				if cm, ok := e.Object.(*corev1.ConfigMap); ok {
					return processConfigMap(cm)
				}
			}
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetName() != rsConfigMapName || e.ObjectNew.GetNamespace() != config.GetDefaultNamespace() {
				return true
			}

			// Check if the ConfigMap `Data` has changed before proceeding
			oldCM, oldOK := e.ObjectOld.(*corev1.ConfigMap)
			newCM, newOK := e.ObjectNew.(*corev1.ConfigMap)

			if oldOK && newOK && reflect.DeepEqual(oldCM.Data, newCM.Data) {
				log.V(1).Info("rs - no changes detected in configmap data, skipping update")
				return true
			}

			log.V(1).Info("rs - configmap data has changed, processing update")
			return processConfigMap(newCM)
		},
	}
}

func applyRSNamespaceConfigMapChanges(ctx context.Context, c client.Client, configData RSNamespaceConfigMapData) error {

	prometheusRule, err := generatePrometheusRule(configData)
	if err != nil {
		return err
	}

	err = createOrUpdatePrometheusRulePolicy(ctx, c, prometheusRule)
	if err != nil {
		return err
	}

	err = createUpdatePlacement(ctx, c, configData.PlacementConfiguration)
	if err != nil {
		return err
	}

	err = createPlacementBinding(ctx, c)
	if err != nil {
		return err
	}
	log.Info("rs - namespace configmap changes applied")

	return nil
}
