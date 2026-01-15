// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/ghodss/yaml"
	cmomanifests "github.com/openshift/cluster-monitoring-operator/pkg/manifests"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	goyaml "gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const ( // #nosec G101 -- Not a hardcoded credential.
	hubAmRouterCASecretName        = "hub-alertmanager-router-ca"
	clusterMonitoringConfigName    = "cluster-monitoring-config"
	clusterMonitoringConfigDataKey = "config.yaml"
	endpointMonitoringOperatorMgr  = "endpoint-monitoring-operator"
	mcoManager                     = "mco-operator"
	promNamespace                  = "openshift-monitoring"
	clusterRoleBindingName         = "hub-metrics-collector-view"
)

// revertClusterMonitoringConfig reverts the configmap cluster-monitoring-config and relevant resources
// (observability-alertmanager-accessor and hub-alertmanager-router-ca) for the openshift cluster monitoring stack.
func RevertHubClusterMonitoringConfig(ctx context.Context, client client.Client) error {
	// try to retrieve the current configmap in the cluster

	hubInfoSecret := &corev1.Secret{}
	err := client.Get(ctx, types.NamespacedName{
		Name:      operatorconfig.HubInfoSecretName,
		Namespace: config.GetDefaultNamespace(),
	}, hubInfoSecret)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get secret %s: %w", operatorconfig.HubInfoSecretName, err)
	}

	hubInfo := &operatorconfig.HubInfo{}
	// We use goyaml (v2) here because HubInfo has yaml tags but no json tags.
	// github.com/ghodss/yaml would fail to unmarshal correctly as it relies on JSON tags.
	// Specifically, kebab-case keys like 'hub-cluster-id' are not automatically mapped to
	// CamelCase struct fields by the standard JSON decoder.
	err = goyaml.Unmarshal(hubInfoSecret.Data[operatorconfig.HubInfoSecretKey], &hubInfo)
	if err != nil {
		return fmt.Errorf("failed to unmarshal hub info: %w", err)
	}

	found := &corev1.ConfigMap{}
	if err := client.Get(ctx, types.NamespacedName{
		Name:      clusterMonitoringConfigName,
		Namespace: promNamespace,
	}, found); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get configmap %s: %w", clusterMonitoringConfigName, err)
	}

	// do not touch the configmap if are not already a manager
	touched := false
	for _, field := range found.GetManagedFields() {
		if field.Manager == endpointMonitoringOperatorMgr || field.Manager == mcoManager {
			touched = true
			break
		}
	}

	if !touched {
		return nil
	}

	foundClusterMonitoringConfigurationYAML, ok := found.Data[clusterMonitoringConfigDataKey]
	if !ok {
		return nil
	}

	foundClusterMonitoringConfigurationJSON, err := yaml.YAMLToJSON([]byte(foundClusterMonitoringConfigurationYAML))
	if err != nil {
		return fmt.Errorf("failed to transform YAML to JSON for configmap %s: %w", clusterMonitoringConfigName, err)
	}

	foundClusterMonitoringConfiguration := &cmomanifests.ClusterMonitoringConfiguration{}
	if err := json.Unmarshal(foundClusterMonitoringConfigurationJSON, foundClusterMonitoringConfiguration); err != nil {
		return fmt.Errorf("failed to unmarshal cluster monitoring config: %w", err)
	}

	if foundClusterMonitoringConfiguration.PrometheusK8sConfig == nil {
		return nil
	}

	// check if externalLabels exists
	if foundClusterMonitoringConfiguration.PrometheusK8sConfig.ExternalLabels != nil {
		delete(foundClusterMonitoringConfiguration.PrometheusK8sConfig.ExternalLabels, operatorconfig.ClusterLabelKeyForAlerts)
		if len(foundClusterMonitoringConfiguration.PrometheusK8sConfig.ExternalLabels) == 0 {
			foundClusterMonitoringConfiguration.PrometheusK8sConfig.ExternalLabels = nil
		}
	}

	// check if alertmanagerConfigs exists
	if foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs != nil {
		copiedAlertmanagerConfigs := make([]cmomanifests.AdditionalAlertmanagerConfig, 0)
		for _, v := range foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs {
			if v.TLSConfig == (cmomanifests.TLSConfig{}) ||
				(v.TLSConfig.CA != nil && v.TLSConfig.CA.Name != hubAmRouterCASecretName+"-"+hubInfo.HubClusterID) {
				copiedAlertmanagerConfigs = append(copiedAlertmanagerConfigs, v)
			}
		}
		if len(copiedAlertmanagerConfigs) == 0 {
			foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs = nil
			if equality.Semantic.DeepEqual(*foundClusterMonitoringConfiguration.PrometheusK8sConfig, cmomanifests.PrometheusK8sConfig{}) {
				foundClusterMonitoringConfiguration.PrometheusK8sConfig = nil
			}
		} else {
			foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs = copiedAlertmanagerConfigs
		}
	}

	// check if the foundClusterMonitoringConfiguration is empty
	if reflect.DeepEqual(*foundClusterMonitoringConfiguration, cmomanifests.ClusterMonitoringConfiguration{}) {
		log.Info("empty ClusterMonitoringConfiguration, deleting configmap", "name", clusterMonitoringConfigName)
		err = client.Delete(ctx, found)
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete configmap %s: %w", clusterMonitoringConfigName, err)
		}
		return nil
	}

	// prepare to write back the cluster monitoring configuration
	updatedClusterMonitoringConfigurationJSONBytes, err := json.Marshal(foundClusterMonitoringConfiguration)
	if err != nil {
		return fmt.Errorf("failed to marshal updated cluster monitoring config: %w", err)
	}

	updatedClusterMonitoringConfigurationYAMLBytes, err := yaml.JSONToYAML(updatedClusterMonitoringConfigurationJSONBytes)
	if err != nil {
		return fmt.Errorf("failed to transform JSON to YAML for updated config: %w", err)
	}

	log.Info("updating configmap", "name", clusterMonitoringConfigName)
	found.Data[clusterMonitoringConfigDataKey] = string(updatedClusterMonitoringConfigurationYAMLBytes)
	if err := client.Update(ctx, found); err != nil {
		return fmt.Errorf("failed to update configmap %s: %w", clusterMonitoringConfigName, err)
	}

	return nil
}
