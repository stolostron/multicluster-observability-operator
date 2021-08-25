// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project.
package observabilityendpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ghodss/yaml"
	operatorconfig "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/config"
	cmomanifests "github.com/openshift/cluster-monitoring-operator/pkg/manifests"
)

const (
	hubAmAccessorSecretName        = "observability-alertmanager-accessor" // #nosec
	hubAmAccessorSecretKey         = "token"
	hubAmRouterCASecretName        = "hub-alertmanager-router-ca"
	hubAmRouterCASecretKey         = "service-ca.crt"
	clusterMonitoringConfigName    = "cluster-monitoring-config"
	clusterMonitoringConfigDataKey = "config.yaml"
	clusterLabelKeyForAlerts       = "cluster"
)

// createHubAmRouterCASecret creates the secret that contains CA of the Hub's Alertmanager Route
func createHubAmRouterCASecret(ctx context.Context, hubInfo *operatorconfig.HubInfo, client client.Client, targetNamespace string) error {
	hubAmRouterCA := hubInfo.AlertmanagerRouterCA
	dataMap := map[string][]byte{hubAmRouterCASecretKey: []byte(hubAmRouterCA)}
	hubAmRouterCASecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hubAmRouterCASecretName,
			Namespace: targetNamespace,
		},
		Data: dataMap,
	}

	found := &corev1.Secret{}
	err := client.Get(ctx, types.NamespacedName{Name: hubAmRouterCASecretName,
		Namespace: targetNamespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			err = client.Create(ctx, hubAmRouterCASecret)
			if err != nil {
				log.Error(err, "failed to create the hub-alertmanager-router-ca secret")
				return err
			}
			log.Info("the hub-alertmanager-router-ca secret is created")
			return nil
		} else {
			log.Error(err, "failed to check the hub-alertmanager-router-ca secret")
			return err
		}
	}

	log.Info("the hub-alertmanager-router-ca secret already exists, check if it needs to be updated")
	if reflect.DeepEqual(found.Data, dataMap) {
		log.Info("no change for the hub-alertmanager-router-ca secret")
		return nil
	} else {
		err = client.Update(ctx, hubAmRouterCASecret)
		if err != nil {
			log.Error(err, "failed to update the hub-alertmanager-router-ca secret")
			return nil
		}
		log.Info("the hub-alertmanager-router-ca secret is updated")
		return err
	}
}

// deleteHubAmRouterCASecret deletes the secret that contains CA of the Hub's Alertmanager Route
func deleteHubAmRouterCASecret(ctx context.Context, client client.Client, targetNamespace string) error {
	found := &corev1.Secret{}
	err := client.Get(ctx, types.NamespacedName{Name: hubAmRouterCASecretName,
		Namespace: targetNamespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("the hub-alertmanager-router-ca secret is already deleted")
			return nil
		}
		log.Error(err, "failed to check the hub-alertmanager-router-ca secret")
		return err
	}
	err = client.Delete(ctx, found)
	if err != nil {
		log.Error(err, "error deleting the hub-alertmanager-router-ca secret")
		return err
	}
	log.Info("the hub-alertmanager-router-ca secret is deleted")
	return nil
}

// createHubAmAccessorTokenSecret creates the secret that contains access token of the Hub's Alertmanager
func createHubAmAccessorTokenSecret(ctx context.Context, client client.Client, targetNamespace string) error {
	amAccessorToken, err := getAmAccessorToken(ctx, client)
	if err != nil {
		return fmt.Errorf("fail to get the alertmanager accessor token %v", err)
	}

	dataMap := map[string][]byte{hubAmAccessorSecretKey: []byte(amAccessorToken)}
	hubAmAccessorTokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hubAmAccessorSecretName,
			Namespace: targetNamespace,
		},
		Data: dataMap,
	}

	found := &corev1.Secret{}
	err = client.Get(ctx, types.NamespacedName{Name: hubAmAccessorSecretName,
		Namespace: targetNamespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			err = client.Create(ctx, hubAmAccessorTokenSecret)
			if err != nil {
				log.Error(err, "failed to create the observability-alertmanager-accessor secret")
				return err
			}
			log.Info("the observability-alertmanager-accessor secret is created")
			return nil
		} else {
			log.Error(err, "failed to check the observability-alertmanager-accessor secret")
			return err
		}
	}

	log.Info("the observability-alertmanager-accessor secret already exists, check if it needs to be updated")
	if reflect.DeepEqual(found.Data, dataMap) {
		log.Info("no change for the observability-alertmanager-accessor secret")
		return nil
	} else {
		err = client.Update(ctx, hubAmAccessorTokenSecret)
		if err != nil {
			log.Error(err, "failed to update the observability-alertmanager-accessor secret")
			return nil
		}
		log.Info("the observability-alertmanager-accessor secret is updated")
		return err
	}
}

// deleteHubAmAccessorTokenSecret deletes the secret that contains access token of the Hub's Alertmanager
func deleteHubAmAccessorTokenSecret(ctx context.Context, client client.Client, targetNamespace string) error {
	found := &corev1.Secret{}
	err := client.Get(ctx, types.NamespacedName{Name: hubAmAccessorSecretName,
		Namespace: targetNamespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("the observability-alertmanager-accessor secret is already deleted")
			return nil
		}
		log.Error(err, "failed to check the observability-alertmanager-accessor secret")
		return err
	}
	err = client.Delete(ctx, found)
	if err != nil {
		log.Error(err, "error deleting the observability-alertmanager-accessor secret")
		return err
	}
	log.Info("the observability-alertmanager-accessor secret is deleted")
	return nil
}

// getAmAccessorToken retrieves the alertmanager access token from observability-alertmanager-accessor secret
func getAmAccessorToken(ctx context.Context, client client.Client) (string, error) {
	amAccessorSecret := &corev1.Secret{}
	if err := client.Get(ctx, types.NamespacedName{Name: hubAmAccessorSecretName,
		Namespace: namespace}, amAccessorSecret); err != nil {
		return "", err
	}

	amAccessorToken := amAccessorSecret.Data[hubAmAccessorSecretKey]
	if amAccessorToken == nil {
		return "", fmt.Errorf("no token in secret %s", hubAmAccessorSecretName)
	}

	return string(amAccessorToken), nil
}

func newAdditionalAlertmanagerConfig(hubInfo *operatorconfig.HubInfo) cmomanifests.AdditionalAlertmanagerConfig {
	return cmomanifests.AdditionalAlertmanagerConfig{
		Scheme:     "https",
		PathPrefix: "/",
		APIVersion: "v2",
		TLSConfig: cmomanifests.TLSConfig{
			CA: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: hubAmRouterCASecretName,
				},
				Key: hubAmRouterCASecretKey,
			},
			InsecureSkipVerify: false,
		},
		BearerToken: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: hubAmAccessorSecretName,
			},
			Key: hubAmAccessorSecretKey,
		},
		StaticConfigs: []string{strings.TrimLeft(hubInfo.AlertmanagerEndpoint, "https://")},
	}
}

// createOrUpdateClusterMonitoringConfig creates or updates the configmap
// cluster-monitoring-config and relevant resources (observability-alertmanager-accessor
// and hub-alertmanager-router-ca) for the openshift cluster monitoring stack
func createOrUpdateClusterMonitoringConfig(
	ctx context.Context,
	hubInfo *operatorconfig.HubInfo,
	clusterID string,
	client client.Client,
	installProm bool) error {
	targetNamespace := promNamespace
	if installProm {
		// for *KS, the hub CA and alertmanager access token should be created in namespace: open-cluster-management-addon-observability
		targetNamespace = namespace
	}

	// create the hub-alertmanager-router-ca secret if it doesn't exist or update it if needed
	if err := createHubAmRouterCASecret(ctx, hubInfo, client, targetNamespace); err != nil {
		log.Error(err, "failed to create or update the hub-alertmanager-router-ca secret")
		return err
	}

	// create the observability-alertmanager-accessor secret if it doesn't exist or update it if needed
	if err := createHubAmAccessorTokenSecret(ctx, client, targetNamespace); err != nil {
		log.Error(err, "failed to create or update the observability-alertmanager-accessor secret")
		return err
	}

	if installProm {
		// no need to create configmap cluster-monitoring-config for *KS
		return nil
	}

	// init the prometheus k8s config
	newExternalLabels := map[string]string{clusterLabelKeyForAlerts: clusterID}
	newAlertmanagerConfigs := []cmomanifests.AdditionalAlertmanagerConfig{newAdditionalAlertmanagerConfig(hubInfo)}
	newPmK8sConfig := &cmomanifests.PrometheusK8sConfig{
		// add cluster label for alerts from managed cluster
		ExternalLabels: newExternalLabels,
		// add alertmanager configs
		AlertmanagerConfigs: newAlertmanagerConfigs,
	}

	// root for CMO configuration
	newClusterMonitoringConfiguration := cmomanifests.ClusterMonitoringConfiguration{
		PrometheusK8sConfig: newPmK8sConfig,
	}

	// marshal new CMO configuration to json then to yaml
	newClusterMonitoringConfigurationJSONBytes, err := json.Marshal(newClusterMonitoringConfiguration)
	if err != nil {
		log.Error(err, "failed to marshal the cluster monitoring config")
		return err
	}
	newClusterMonitoringConfigurationYAMLBytes, err := yaml.JSONToYAML(newClusterMonitoringConfigurationJSONBytes)
	if err != nil {
		log.Error(err, "failed to transform JSON to YAML", "JSON", newClusterMonitoringConfigurationJSONBytes)
		return err
	}

	newCusterMonitoringConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterMonitoringConfigName,
			Namespace: promNamespace,
		},
		Data: map[string]string{clusterMonitoringConfigDataKey: string(newClusterMonitoringConfigurationYAMLBytes)},
	}

	// try to retrieve the current configmap in the cluster
	found := &corev1.ConfigMap{}
	err = client.Get(ctx, types.NamespacedName{Name: clusterMonitoringConfigName,
		Namespace: promNamespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("configmap not found, try to create it", "name", clusterMonitoringConfigName)
			err = client.Create(ctx, newCusterMonitoringConfigMap)
			if err != nil {
				log.Error(err, "failed to create configmap", "name", clusterMonitoringConfigName)
				return err
			}
			log.Info("configmap created", "name", clusterMonitoringConfigName)
			return nil
		} else {
			log.Error(err, "failed to check configmap", "name", clusterMonitoringConfigName)
			return err
		}
	}

	log.Info("configmap already exists, check if it needs update", "name", clusterMonitoringConfigName)
	foundClusterMonitoringConfigurationYAMLString, ok := found.Data[clusterMonitoringConfigDataKey]
	if !ok {
		log.Info("configmap data doesn't contain key, try to update it", "name", clusterMonitoringConfigName, "key", clusterMonitoringConfigDataKey)
		// replace config.yaml in configmap
		found.Data[clusterMonitoringConfigDataKey] = string(newClusterMonitoringConfigurationYAMLBytes)
		err = client.Update(ctx, found)
		if err != nil {
			log.Error(err, "failed to update configmap", "name", clusterMonitoringConfigName)
			return err
		}
		log.Info("configmap updated", "name", clusterMonitoringConfigName)
		return nil
	}

	log.Info("configmap already exists and key config.yaml exists, check if the value needs update",
		"name", clusterMonitoringConfigName,
		"key", clusterMonitoringConfigDataKey)
	foundClusterMonitoringConfigurationJSONBytes, err := yaml.YAMLToJSON([]byte(foundClusterMonitoringConfigurationYAMLString))
	if err != nil {
		log.Error(err, "failed to transform YAML to JSON", "YAML", foundClusterMonitoringConfigurationYAMLString)
		return err
	}
	foundClusterMonitoringConfiguration := &cmomanifests.ClusterMonitoringConfiguration{}
	if err := json.Unmarshal([]byte(foundClusterMonitoringConfigurationJSONBytes), foundClusterMonitoringConfiguration); err != nil {
		log.Error(err, "failed to marshal the cluster monitoring config")
		return err
	}

	if foundClusterMonitoringConfiguration.PrometheusK8sConfig == nil {
		foundClusterMonitoringConfiguration.PrometheusK8sConfig = newPmK8sConfig
	} else {
		// check if externalLabels exists
		if foundClusterMonitoringConfiguration.PrometheusK8sConfig.ExternalLabels == nil {
			foundClusterMonitoringConfiguration.PrometheusK8sConfig.ExternalLabels = newExternalLabels
		} else {
			foundClusterMonitoringConfiguration.PrometheusK8sConfig.ExternalLabels[clusterLabelKeyForAlerts] = clusterID
		}

		// check if alertmanagerConfigs exists
		if foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs == nil {
			foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs = newAlertmanagerConfigs
		} else {
			additionalAlertmanagerConfigExists := false
			for _, v := range foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs {
				if v.TLSConfig != (cmomanifests.TLSConfig{}) &&
					v.TLSConfig.CA != nil &&
					v.TLSConfig.CA.LocalObjectReference != (corev1.LocalObjectReference{}) &&
					v.TLSConfig.CA.LocalObjectReference.Name == hubAmRouterCASecretName {
					additionalAlertmanagerConfigExists = true
					break
				}
			}
			if !additionalAlertmanagerConfigExists {
				foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs = append(
					foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs,
					newAdditionalAlertmanagerConfig(hubInfo))
			}
		}
	}

	// prepare to write back the cluster monitoring configuration
	updatedClusterMonitoringConfigurationJSONBytes, err := json.Marshal(foundClusterMonitoringConfiguration)
	if err != nil {
		log.Error(err, "failed to marshal the cluster monitoring config")
		return err
	}
	updatedclusterMonitoringConfigurationYAMLBytes, err := yaml.JSONToYAML(updatedClusterMonitoringConfigurationJSONBytes)
	if err != nil {
		log.Error(err, "failed to transform JSON to YAML", "JSON", updatedClusterMonitoringConfigurationJSONBytes)
		return err
	}
	found.Data[clusterMonitoringConfigDataKey] = string(updatedclusterMonitoringConfigurationYAMLBytes)
	err = client.Update(ctx, found)
	if err != nil {
		log.Error(err, "failed to update configmap", "name", clusterMonitoringConfigName)
		return err
	}
	log.Info("configmap updated", "name", clusterMonitoringConfigName)
	return nil
}

// revertClusterMonitoringConfig reverts the configmap cluster-monitoring-config and relevant resources
// (observability-alertmanager-accessor and hub-alertmanager-router-ca) for the openshift cluster monitoring stack
func revertClusterMonitoringConfig(ctx context.Context, client client.Client, installProm bool) error {
	targetNamespace := promNamespace
	if installProm {
		// for *KS, the hub CA and alertmanager access token are not created in namespace: open-cluster-management-addon-observability
		targetNamespace = namespace
	}

	// delete the hub-alertmanager-router-ca secret
	if err := deleteHubAmRouterCASecret(ctx, client, targetNamespace); err != nil {
		log.Error(err, "failed to delete the hub-alertmanager-router-ca secret")
		return err
	}

	// delete the observability-alertmanager-accessor secret
	if err := deleteHubAmAccessorTokenSecret(ctx, client, targetNamespace); err != nil {
		log.Error(err, "failed to delete the observability-alertmanager-accessor secret")
		return err
	}

	// try to retrieve the current configmap in the cluster
	found := &corev1.ConfigMap{}
	err := client.Get(ctx, types.NamespacedName{Name: clusterMonitoringConfigName,
		Namespace: promNamespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("configmap not found, no need action", "name", clusterMonitoringConfigName)
			return nil
		} else {
			log.Error(err, "failed to check configmap", "name", clusterMonitoringConfigName)
			return err
		}
	}

	// revert the existing cluster-monitor-config configmap
	log.Info("configmap exists, check if it needs revert", "name", clusterMonitoringConfigName)
	foundClusterMonitoringConfigurationYAML, ok := found.Data[clusterMonitoringConfigDataKey]
	if !ok {
		log.Info("configmap data doesn't contain key, no need action", "name", clusterMonitoringConfigName, "key", clusterMonitoringConfigDataKey)
		return nil
	}
	foundClusterMonitoringConfigurationJSON, err := yaml.YAMLToJSON([]byte(foundClusterMonitoringConfigurationYAML))
	if err != nil {
		log.Error(err, "failed to transform YAML to JSON", "YAML", foundClusterMonitoringConfigurationYAML)
		return err
	}

	log.Info("configmap exists and key config.yaml exists, check if the value needs revert", "name", clusterMonitoringConfigName, "key", clusterMonitoringConfigDataKey)
	foundClusterMonitoringConfiguration := &cmomanifests.ClusterMonitoringConfiguration{}
	if err := json.Unmarshal([]byte(foundClusterMonitoringConfigurationJSON), foundClusterMonitoringConfiguration); err != nil {
		log.Error(err, "failed to marshal the cluster monitoring config")
		return err
	}

	if foundClusterMonitoringConfiguration.PrometheusK8sConfig == nil {
		log.Info("configmap data doesn't key: prometheusK8s, no need action", "name", clusterMonitoringConfigName, "key", clusterMonitoringConfigDataKey)
		return nil
	} else {
		// check if externalLabels exists
		if foundClusterMonitoringConfiguration.PrometheusK8sConfig.ExternalLabels != nil {
			if _, ok := foundClusterMonitoringConfiguration.PrometheusK8sConfig.ExternalLabels[clusterLabelKeyForAlerts]; ok {
				delete(foundClusterMonitoringConfiguration.PrometheusK8sConfig.ExternalLabels, clusterLabelKeyForAlerts)
			}
			if len(foundClusterMonitoringConfiguration.PrometheusK8sConfig.ExternalLabels) == 0 {
				foundClusterMonitoringConfiguration.PrometheusK8sConfig.ExternalLabels = nil
			}
		}

		// check if alertmanagerConfigs exists
		if foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs != nil {
			copiedAlertmanagerConfigs := make([]cmomanifests.AdditionalAlertmanagerConfig, 0)
			for _, v := range foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs {
				if v.TLSConfig == (cmomanifests.TLSConfig{}) ||
					v.TLSConfig.CA == nil ||
					v.TLSConfig.CA.LocalObjectReference == (corev1.LocalObjectReference{}) ||
					v.TLSConfig.CA.LocalObjectReference.Name != hubAmRouterCASecretName {
					copiedAlertmanagerConfigs = append(copiedAlertmanagerConfigs, v)
				}
			}
			if len(copiedAlertmanagerConfigs) == 0 {
				foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs = nil
				if reflect.DeepEqual(*foundClusterMonitoringConfiguration.PrometheusK8sConfig, cmomanifests.PrometheusK8sConfig{}) {
					foundClusterMonitoringConfiguration.PrometheusK8sConfig = nil
				}
			} else {
				foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs = copiedAlertmanagerConfigs
			}
		}
	}

	// check if the foundClusterMonitoringConfiguration is empty ClusterMonitoringConfiguration
	if reflect.DeepEqual(*foundClusterMonitoringConfiguration, cmomanifests.ClusterMonitoringConfiguration{}) {
		log.Info("empty ClusterMonitoringConfiguration, should delete configmap", "name", clusterMonitoringConfigName)
		err = client.Delete(ctx, found)
		if err != nil {
			log.Error(err, "failed to delete configmap", "name", clusterMonitoringConfigName)
			return err
		}
		log.Info("configmap delete", "name", clusterMonitoringConfigName)
		return nil
	}

	// prepare to write back the cluster monitoring configuration
	updatedClusterMonitoringConfigurationJSONBytes, err := json.Marshal(foundClusterMonitoringConfiguration)
	if err != nil {
		log.Error(err, "failed to marshal the cluster monitoring config")
		return err
	}
	updatedClusterMonitoringConfigurationYAMLBytes, err := yaml.JSONToYAML(updatedClusterMonitoringConfigurationJSONBytes)
	if err != nil {
		log.Error(err, "failed to transform JSON to YAML", "JSON", updatedClusterMonitoringConfigurationJSONBytes)
		return err
	}
	found.Data[clusterMonitoringConfigDataKey] = string(updatedClusterMonitoringConfigurationYAMLBytes)
	err = client.Update(ctx, found)
	if err != nil {
		log.Error(err, "failed to update configmap", "name", clusterMonitoringConfigName)
		return err
	}
	log.Info("configmap updated", "name", clusterMonitoringConfigName)
	return nil
}
