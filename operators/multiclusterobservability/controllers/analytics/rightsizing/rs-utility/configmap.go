// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsutility

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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// EnsureRSConfigMapExists ensures that the ConfigMap exists, creating it if necessary
func EnsureRSConfigMapExists(ctx context.Context, c client.Client, configMapName string, getDefaultDataFunc func() map[string]string) error {
	// Check if the ConfigMap already exists
	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: config.GetDefaultNamespace(),
			Labels:    RSLabels(),
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
			existingCM.Data = getDefaultDataFunc()

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

// GetRSConfigData extracts and unmarshals the data from the ConfigMap into RSNamespaceConfigMapData
func GetRSConfigData(cm *corev1.ConfigMap) (RSNamespaceConfigMapData, error) {
	var configData RSNamespaceConfigMapData

	// Unmarshal prometheusRuleConfig
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

// GetRSConfigMapPredicateFunc returns a predicate that filters ConfigMap watch
// events to only the named RS ConfigMap with actual data changes.
func GetRSConfigMapPredicateFunc(configMapName string) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return e.Object.GetName() == configMapName && e.Object.GetNamespace() == config.GetDefaultNamespace()
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetName() != configMapName || e.ObjectNew.GetNamespace() != config.GetDefaultNamespace() {
				return false
			}

			oldCM, oldOK := e.ObjectOld.(*corev1.ConfigMap)
			newCM, newOK := e.ObjectNew.(*corev1.ConfigMap)
			if oldOK && newOK && reflect.DeepEqual(oldCM.Data, newCM.Data) {
				return false
			}
			return true
		},
		DeleteFunc:  func(e event.DeleteEvent) bool { return false },
		GenericFunc: func(e event.GenericEvent) bool { return false },
	}
}
