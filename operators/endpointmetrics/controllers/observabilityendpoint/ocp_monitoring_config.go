// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package observabilityendpoint

import (
	"context"
	"fmt"
	"net/url"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cmomanifests "github.com/openshift/cluster-monitoring-operator/pkg/manifests"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
)

const (
	hubAmAccessorSecretName        = "observability-alertmanager-accessor" // #nosec G101 -- Not a hardcoded credential.
	hubAmAccessorSecretKey         = "token"                               // #nosec G101 -- Not a hardcoded credential.
	hubAmRouterCASecretName        = "hub-alertmanager-router-ca"
	hubAmRouterCASecretKey         = "service-ca.crt"
	clusterMonitoringConfigName    = "cluster-monitoring-config"
	clusterMonitoringRevertedName  = "cluster-monitoring-reverted"
	clusterMonitoringConfigDataKey = "config.yaml"
	endpointMonitoringOperatorMgr  = "endpoint-monitoring-operator"
)

var (
	clusterMonitoringConfigReverted = false
	persistedRevertStateRead        = false
)

// initializes clusterMonitoringConfigReverted based on the presence of clusterMonitoringRevertedName
// configmap in openshift-monitoring namespace.
func initPersistedRevertState(ctx context.Context, client client.Client, ns string) error {
	if !persistedRevertStateRead {
		// check if reverted configmap is present
		found := &corev1.ConfigMap{}
		err := client.Get(ctx, types.NamespacedName{Name: clusterMonitoringRevertedName, Namespace: ns}, found)
		if err != nil {
			// treat this as non-fatal error
			if errors.IsNotFound(err) {
				// configmap does not exist. Not reverted before
				clusterMonitoringConfigReverted = false
				persistedRevertStateRead = true
			} else {
				// treat the condition as transient error for retry
				log.Error(err, "failed to lookup cluster-monitoring-reverted configmap")
				return err
			}
		} else {
			// marker configmap present
			persistedRevertStateRead = true
			clusterMonitoringConfigReverted = true
		}
	}

	return nil
}

func isRevertedAlready(ctx context.Context, client client.Client, ns string) (bool, error) {
	log.Info("in isRevertedAlready")
	err := initPersistedRevertState(ctx, client, ns)
	if err != nil {
		log.Info("isRevertedAlready: error from initPersistedRevertState", "error:", err.Error())
		return clusterMonitoringConfigReverted, err
	} else {
		log.Info("isRevertedAlready", "clusterMonitoringConfigReverted:", clusterMonitoringConfigReverted)
		return clusterMonitoringConfigReverted, nil
	}
}

func setConfigReverted(ctx context.Context, client client.Client, ns string) error {
	err := initPersistedRevertState(ctx, client, ns)
	if err != nil {
		return err
	}

	// if not created already, set persistent state by creating configmap
	c := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterMonitoringRevertedName,
			Namespace: ns,
		},
	}
	err = client.Create(ctx, c)
	log.Info("cluster-monitoring-reverted comfigmap created")
	if err != nil {
		if errors.IsAlreadyExists(err) {
			log.Info("persistent state already set. configmap cluster-monitoring-reverted already exists")
		} else {
			log.Error(err, "failed to set persistent state. could not cluster-monitoring-reverted configmap")
			return err
		}
	}
	clusterMonitoringConfigReverted = true
	return nil
}

func unsetConfigReverted(ctx context.Context, client client.Client, ns string) error {
	err := initPersistedRevertState(ctx, client, ns)
	if err != nil {
		return err
	}

	// delete any persistent state if present
	c := &corev1.ConfigMap{}
	err = client.Get(ctx, types.NamespacedName{Name: clusterMonitoringRevertedName, Namespace: ns}, c)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("persistent state already set. cluster-monitoring-reverted configmap does not exist")
		} else {
			log.Error(err, "failed to set persistent state. error looking up cluster-monitoring-reverted configmap")
			return nil
		}
	} else {
		// delete configmap
		err = client.Delete(ctx, c)
		log.Info("cluster-monitoring-reverted configmap deleted")
		if err != nil {
			log.Error(err, "failed to delete persistent state. error deleting cluster-monitoring-reverted configmap")
			return nil
		}
	}

	clusterMonitoringConfigReverted = false
	return nil
}

// createHubAmRouterCASecret creates the secret that contains CA of the Hub's Alertmanager Route.
func createHubAmRouterCASecret(
	ctx context.Context,
	hubInfo *operatorconfig.HubInfo,
	client client.Client,
	targetNamespace string) error {

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
	err := client.Get(ctx, types.NamespacedName{Name: hubAmRouterCASecretName, Namespace: targetNamespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info(fmt.Sprintf("creating %s/%s secret", targetNamespace, hubAmRouterCASecretName))
			err = client.Create(ctx, hubAmRouterCASecret)
			if err != nil {
				return fmt.Errorf("failed to create %s/%s secret: %w", targetNamespace, hubAmRouterCASecretName, err)
			}
			return nil
		} else {
			return fmt.Errorf("failed to check the %s/%s secret: %w", targetNamespace, hubAmRouterCASecretName, err)
		}
	}

	if reflect.DeepEqual(found.Data, dataMap) {
		return nil
	}

	log.Info(fmt.Sprintf("updating %s/%s secret", targetNamespace, hubAmRouterCASecretName))
	err = client.Update(ctx, hubAmRouterCASecret)
	if err != nil {
		return fmt.Errorf("failed to update the %s/%s secret: %w", targetNamespace, hubAmRouterCASecretName, err)
	}

	return err

}

// createHubAmAccessorTokenSecret creates the secret that contains access token of the Hub's Alertmanager.
func createHubAmAccessorTokenSecret(ctx context.Context, client client.Client, namespace, targetNamespace string) error {
	amAccessorToken, err := getAmAccessorToken(ctx, client, namespace)
	if err != nil {
		return fmt.Errorf("fail to get %s/%s secret: %w", namespace, hubAmAccessorSecretName, err)
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
	err = client.Get(ctx, types.NamespacedName{Name: hubAmAccessorSecretName, Namespace: targetNamespace}, found)
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

// getAmAccessorToken retrieves the alertmanager access token from observability-alertmanager-accessor secret.
func getAmAccessorToken(ctx context.Context, client client.Client, ns string) (string, error) {
	amAccessorSecret := &corev1.Secret{}
	if err := client.Get(ctx, types.NamespacedName{Name: hubAmAccessorSecretName,
		Namespace: ns}, amAccessorSecret); err != nil {
		return "", err
	}

	amAccessorToken := amAccessorSecret.Data[hubAmAccessorSecretKey]
	if amAccessorToken == nil {
		return "", fmt.Errorf("no token in secret %s", hubAmAccessorSecretName)
	}

	return string(amAccessorToken), nil
}

func newAdditionalAlertmanagerConfig(hubInfo *operatorconfig.HubInfo) cmomanifests.AdditionalAlertmanagerConfig {
	config := cmomanifests.AdditionalAlertmanagerConfig{
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
		StaticConfigs: []string{},
	}
	amURL, err := url.Parse(hubInfo.AlertmanagerEndpoint)
	if err != nil {
		return config
	}

	config.PathPrefix = amURL.Path
	config.StaticConfigs = append(config.StaticConfigs, amURL.Host)
	return config
}

// createOrUpdateClusterMonitoringConfig creates or updates the configmap
// cluster-monitoring-config and relevant resources (observability-alertmanager-accessor
// and hub-alertmanager-router-ca) for the openshift cluster monitoring stack.
func createOrUpdateClusterMonitoringConfig(
	ctx context.Context,
	hubInfo *operatorconfig.HubInfo,
	clusterID string,
	client client.Client,
	installProm bool,
	namespace string,
) error {
	targetNamespace := promNamespace
	if installProm {
		// for *KS, the hub CA and alertmanager access token should be created
		// in namespace: open-cluster-management-addon-observability
		targetNamespace = namespace
	}

	// create the hub-alertmanager-router-ca secret if it doesn't exist or update it if needed
	if err := createHubAmRouterCASecret(ctx, hubInfo, client, targetNamespace); err != nil {
		return fmt.Errorf("failed to create or update the hub-alertmanager-router-ca secret: %w", err)
	}

	// create the observability-alertmanager-accessor secret if it doesn't exist or update it if needed
	if err := createHubAmAccessorTokenSecret(ctx, client, namespace, targetNamespace); err != nil {
		return fmt.Errorf("failed to create or update the alertmanager accessor token secret: %w", err)
	}

	// create or update the cluster-monitoring-config configmap and relevant resources
	if hubInfo.AlertmanagerEndpoint == "" {
		log.Info("request to disable alert forwarding")
		// only revert (once) if not done already and remember state
		revertedAlready, err := isRevertedAlready(ctx, client, namespace)
		if err != nil {
			return err
		}
		if !revertedAlready {
			if err = RevertClusterMonitoringConfig(ctx, client); err != nil {
				return err
			}
			if err = setConfigReverted(ctx, client, namespace); err != nil {
				return err
			}
		} else {
			log.Info("configuration reverted, nothing to do")
		}
		return nil
	}

	if installProm {
		// no need to create configmap cluster-monitoring-config for *KS
		return unset(ctx, client, namespace)
	}

	// init the prometheus k8s config
	newExternalLabels := map[string]string{operatorconfig.ClusterLabelKeyForAlerts: clusterID}
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

	newClusterMonitoringConfigurationYAMLBytes, err := yaml.Marshal(newClusterMonitoringConfiguration)
	if err != nil {
		return fmt.Errorf("failed to marshal cluster monitoring config to YAML: %w", err)
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
	err = client.Get(ctx, types.NamespacedName{Name: clusterMonitoringConfigName, Namespace: promNamespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("cluster monitoring configmap not found, trying to create it", "name", clusterMonitoringConfigName)
			return createCMOConfigMapAndUnset(ctx, client, newCusterMonitoringConfigMap, namespace)
		}
		return fmt.Errorf("failed to check configmap %s: %w", clusterMonitoringConfigName, err)
	}

	log.Info("cluster monitoring configmap exists", "name", clusterMonitoringConfigName)
	foundClusterMonitoringConfigurationYAMLString, ok := hasClusterMonitoringConfigData(found)
	if !ok {
		// replace config.yaml in configmap
		found.Data[clusterMonitoringConfigDataKey] = string(newClusterMonitoringConfigurationYAMLBytes)
		return updateClusterMonitoringConfigAndUnset(ctx, client, found, namespace)
	}

	foundClusterMonitoringConfiguration := &cmomanifests.ClusterMonitoringConfiguration{}
	if err := yaml.Unmarshal([]byte(foundClusterMonitoringConfigurationYAMLString), foundClusterMonitoringConfiguration); err != nil {
		return fmt.Errorf("failed to unmarshal the cluster monitoring config: %w", err)
	}

	newCMOCfg := &cmomanifests.ClusterMonitoringConfiguration{}
	if err := yaml.Unmarshal([]byte(foundClusterMonitoringConfigurationYAMLString), newCMOCfg); err != nil {
		return fmt.Errorf("failed to unmarshal the cluster monitoring config: %w", err)
	}

	if newCMOCfg.PrometheusK8sConfig != nil {

		// check if externalLabels exists
		if newCMOCfg.PrometheusK8sConfig.ExternalLabels != nil {
			newCMOCfg.PrometheusK8sConfig.ExternalLabels[operatorconfig.ClusterLabelKeyForAlerts] = clusterID
		} else {
			newCMOCfg.PrometheusK8sConfig.ExternalLabels = newExternalLabels
		}

		// check if alertmanagerConfigs exists
		if newCMOCfg.PrometheusK8sConfig.AlertmanagerConfigs != nil {
			additionalAlertmanagerConfigExists := false
			var atIndex int
			for i, v := range newCMOCfg.PrometheusK8sConfig.AlertmanagerConfigs {
				if isManaged(v) {
					additionalAlertmanagerConfigExists = true
					atIndex = i
					break
				}
			}
			if !additionalAlertmanagerConfigExists {
				newCMOCfg.PrometheusK8sConfig.AlertmanagerConfigs = append(
					newCMOCfg.PrometheusK8sConfig.AlertmanagerConfigs,
					newAdditionalAlertmanagerConfig(hubInfo))
			} else {
				newCMOCfg.PrometheusK8sConfig.AlertmanagerConfigs[atIndex] = newAdditionalAlertmanagerConfig(hubInfo)
			}
		} else {
			newCMOCfg.PrometheusK8sConfig.AlertmanagerConfigs = newAlertmanagerConfigs
		}

	} else {
		newCMOCfg.PrometheusK8sConfig = newPmK8sConfig
	}

	if equality.Semantic.DeepEqual(*foundClusterMonitoringConfiguration, *newCMOCfg) {
		return nil
	}

	updatedClusterMonitoringConfigurationYAMLBytes, err := yaml.Marshal(newCMOCfg)
	if err != nil {
		return fmt.Errorf("failed to marshal the cluster monitoring config to yaml: %w", err)
	}

	found.Data[clusterMonitoringConfigDataKey] = string(updatedClusterMonitoringConfigurationYAMLBytes)
	return updateClusterMonitoringConfigAndUnset(ctx, client, found, namespace)
}

// RevertClusterMonitoringConfig reverts the configmap cluster-monitoring-config and relevant resources
// (observability-alertmanager-accessor and hub-alertmanager-router-ca) for the openshift cluster monitoring stack.
func RevertClusterMonitoringConfig(ctx context.Context, client client.Client) error {
	log.Info("RevertClusterMonitoringConfig called")

	found := &corev1.ConfigMap{}
	err := client.Get(ctx, types.NamespacedName{Name: clusterMonitoringConfigName, Namespace: promNamespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("configmap not found, no need action", "name", clusterMonitoringConfigName)
			return nil
		}
		return fmt.Errorf("failed to check configmap %s: %w", clusterMonitoringConfigName, err)
	}

	log.Info("checking if cluster monitoring config needs revert", "name", clusterMonitoringConfigName)
	found = found.DeepCopy()
	if !inManagedFields(found) {
		return nil
	}

	foundClusterMonitoringConfigurationYAML, ok := hasClusterMonitoringConfigData(found)
	if !ok {
		return nil
	}

	foundClusterMonitoringConfiguration := &cmomanifests.ClusterMonitoringConfiguration{}
	err = yaml.Unmarshal([]byte(foundClusterMonitoringConfigurationYAML), foundClusterMonitoringConfiguration)
	if err != nil {
		return fmt.Errorf("failed to unmarshal the cluster monitoring config: %w", err)
	}

	if foundClusterMonitoringConfiguration.PrometheusK8sConfig == nil {
		log.Info("configmap data doesn't contain 'prometheusK8s', no need action", "name", clusterMonitoringConfigName)
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
			if !isManaged(v) {
				copiedAlertmanagerConfigs = append(copiedAlertmanagerConfigs, v)
			}
		}

		foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs = copiedAlertmanagerConfigs
		if len(copiedAlertmanagerConfigs) == 0 {
			foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs = nil
			if reflect.DeepEqual(*foundClusterMonitoringConfiguration.PrometheusK8sConfig, cmomanifests.PrometheusK8sConfig{}) {
				foundClusterMonitoringConfiguration.PrometheusK8sConfig = nil
			}
		}
	}

	// check if the foundClusterMonitoringConfiguration is empty ClusterMonitoringConfiguration
	if reflect.DeepEqual(*foundClusterMonitoringConfiguration, cmomanifests.ClusterMonitoringConfiguration{}) {
		log.Info("empty ClusterMonitoringConfiguration, deleting configmap if it still exists", "name", clusterMonitoringConfigName)
		err = client.Delete(ctx, found)
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete configmap %s: %w", clusterMonitoringConfigName, err)
		}
		return nil
	}

	updatedClusterMonitoringConfigurationYAMLBytes, err := yaml.Marshal(foundClusterMonitoringConfiguration)
	if err != nil {
		return fmt.Errorf("failed to marshal the cluster monitoring config: %w", err)
	}

	found.Data[clusterMonitoringConfigDataKey] = string(updatedClusterMonitoringConfigurationYAMLBytes)
	return updateClusterMonitoringConfig(ctx, client, found)
}

// createCMOConfigMapAndUnset creates the configmap cluster-monitoring-config in the openshift-monitoring namespace.
// the namespace parameter is used to call unset function.
func createCMOConfigMapAndUnset(ctx context.Context, client client.Client, obj client.Object, namespace string) error {
	err := client.Create(ctx, obj)
	if err != nil {
		log.Error(err, "failed to create configmap", "name", clusterMonitoringConfigName)
		return err
	}
	log.Info("configmap created", "name", clusterMonitoringConfigName)
	return unset(ctx, client, namespace)
}

func updateClusterMonitoringConfig(ctx context.Context, client client.Client, obj client.Object) error {
	err := client.Update(ctx, obj)
	if err != nil {
		return fmt.Errorf("failed to update configmap %s: %w", clusterMonitoringConfigName, err)
	}
	log.Info("configmap updated", "name", clusterMonitoringConfigName)
	return nil
}

// updateClusterMonitoringConfigAndUnset updates the configmap cluster-monitoring-config in the openshift-monitoring namespace.
// the namespace parameter is used to call unset function.
func updateClusterMonitoringConfigAndUnset(ctx context.Context, client client.Client, obj client.Object, namespace string) error {
	err := updateClusterMonitoringConfig(ctx, client, obj)
	if err != nil {
		return err
	}
	return unset(ctx, client, namespace)
}

// unset config reverted flag after successfully updating cluster-monitoring-config
func unset(ctx context.Context, client client.Client, ns string) error {
	// if reverted before, reset so we can revert again
	revertedAlready, err := isRevertedAlready(ctx, client, ns)
	if err == nil && revertedAlready {
		err = unsetConfigReverted(ctx, client, ns)
	}
	return err
}

// inManagedFields checks if the configmap has had a CRUD operation by endpoint-monitoring-operator
func inManagedFields(cm *corev1.ConfigMap) bool {
	for _, field := range cm.GetManagedFields() {
		if field.Manager == endpointMonitoringOperatorMgr {
			return true
		}
	}
	log.Info(
		"endpoint-monitoring-operator is not a manager of configmap",
		"name",
		clusterMonitoringConfigName,
	)
	return false
}

// isManaged checks if the additional alertmanager config is managed by ACM
func isManaged(amc cmomanifests.AdditionalAlertmanagerConfig) bool {
	if amc.TLSConfig.CA != nil && amc.TLSConfig.CA.LocalObjectReference.Name == hubAmRouterCASecretName {
		return true
	}
	return false
}

// hasClusterMonitoringConfigData checks if the configmap has the required key and logs if not
func hasClusterMonitoringConfigData(cm *corev1.ConfigMap) (string, bool) {
	data, ok := cm.Data[clusterMonitoringConfigDataKey]
	if !ok {
		log.Info(
			"configmap doesn't contain required key",
			"name",
			clusterMonitoringConfigName,
			"key",
			clusterMonitoringConfigDataKey,
		)
	}
	return data, ok
}
