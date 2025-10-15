// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package observabilityendpoint

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ghodss/yaml"
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
	log                             = ctrl.Log.WithName("controllers").WithName("ObservabilityAddon")
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

	hubAmRouterSecret := hubAmRouterCASecretName + "-" + hubInfo.HubClusterDomain

	hubAmRouterCA := hubInfo.AlertmanagerRouterCA
	dataMap := map[string][]byte{hubAmRouterCASecretKey: []byte(hubAmRouterCA)}
	hubAmRouterCASecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hubAmRouterSecret,
			Namespace: targetNamespace,
		},
		Data: dataMap,
	}

	found := &corev1.Secret{}
	err := client.Get(ctx, types.NamespacedName{Name: hubAmRouterSecret, Namespace: targetNamespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info(fmt.Sprintf("creating %s/%s secret", targetNamespace, hubAmRouterSecret))
			err = client.Create(ctx, hubAmRouterCASecret)
			if err != nil {
				return fmt.Errorf("failed to create %s/%s secret: %w", targetNamespace, hubAmRouterSecret, err)
			}
			return nil
		} else {
			return fmt.Errorf("failed to check the %s/%s secret: %w", targetNamespace, hubAmRouterSecret, err)
		}
	}

	if equality.Semantic.DeepEqual(found.Data, dataMap) {
		return nil
	}

	log.Info(fmt.Sprintf("updating %s/%s secret", targetNamespace, hubAmRouterSecret))
	err = client.Update(ctx, hubAmRouterCASecret)
	if err != nil {
		return fmt.Errorf("failed to update the %s/%s secret: %w", targetNamespace, hubAmRouterSecret, err)
	}

	return err

}

// createHubAmAccessorTokenSecret creates the secret that contains access token of the Hub's Alertmanager.
func createHubAmAccessorTokenSecret(ctx context.Context, client client.Client, namespace, targetNamespace string, hubInfo *operatorconfig.HubInfo) error {
	amAccessorToken, err := getAmAccessorToken(ctx, client, namespace)
	if err != nil {
		return fmt.Errorf("fail to get %s/%s secret: %w", namespace, hubAmAccessorSecretName, err)
	}

	hubAmAccessorSecret := hubAmAccessorSecretName + "-" + hubInfo.HubClusterDomain
	dataMap := map[string][]byte{hubAmAccessorSecretKey: []byte(amAccessorToken)}
	hubAmAccessorTokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hubAmAccessorSecret,
			Namespace: targetNamespace,
		},
		Data: dataMap,
	}

	found := &corev1.Secret{}
	err = client.Get(ctx, types.NamespacedName{Name: hubAmAccessorSecret, Namespace: targetNamespace}, found)
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
					Name: hubAmRouterCASecretName + "-" + hubInfo.HubClusterDomain,
				},
				Key: hubAmRouterCASecretKey,
			},
			InsecureSkipVerify: false,
		},
		BearerToken: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: hubAmAccessorSecretName + "-" + hubInfo.HubClusterDomain,
			},
			Key: hubAmAccessorSecretKey,
		},
		StaticConfigs: []string{},
	}
	amURL, err := url.Parse(hubInfo.AlertmanagerEndpoint)
	if err != nil {
		log.Error(err, "failed to parse alertmanager endpoint. ignoring it", "endpoint", hubInfo.AlertmanagerEndpoint)
		return config
	}

	config.PathPrefix = amURL.Path
	config.StaticConfigs = append(config.StaticConfigs, amURL.Host)
	return config
}

// namespaceExists checks whether the given namespace exists in the cluster
func namespaceExists(ctx context.Context, client client.Client, ns string) bool {
	namespace := &corev1.Namespace{}
	if err := client.Get(ctx, types.NamespacedName{Name: ns}, namespace); err != nil {
		return false
	}
	return true
}

// isUserWorkloadMonitoringEnabled checks if user workload monitoring is enabled in cluster-monitoring-config
func isUserWorkloadMonitoringEnabled(ctx context.Context, client client.Client) (bool, error) {
	cm := &corev1.ConfigMap{}
	err := client.Get(ctx, types.NamespacedName{
		Name:      clusterMonitoringConfigName,
		Namespace: promNamespace,
	}, cm)
	if err != nil {
		if errors.IsNotFound(err) {
			// If configmap doesn't exist, assume UWM is not enabled
			return false, nil
		}
		return false, err
	}

	configYAML, ok := cm.Data["config.yaml"]
	if !ok {
		return false, nil
	}

	parsed := &cmomanifests.ClusterMonitoringConfiguration{}
	if err := yaml.Unmarshal([]byte(configYAML), parsed); err != nil {
		return false, err
	}

	return parsed.UserWorkloadEnabled != nil && *parsed.UserWorkloadEnabled, nil
}

// createOrUpdateClusterMonitoringConfig creates or updates the configmaps
// cluster-monitoring-config and user-workload-monitoring-config (when needed),
// and relevant resources (observability-alertmanager-accessor
// and hub-alertmanager-router-ca) for the openshift cluster monitoring stack.
// Returns a boolean indicating wether the configmap was effectively updated.
func createOrUpdateClusterMonitoringConfig(
	ctx context.Context,
	hubInfo *operatorconfig.HubInfo,
	clusterID string,
	client client.Client,
	installProm bool,
	namespace string,
) (bool, error) {
	targetNamespace := promNamespace
	if installProm {
		// for *KS, the hub CA and alertmanager access token should be created
		// in namespace: open-cluster-management-addon-observability
		targetNamespace = namespace
	}

	// create the hub-alertmanager-router-ca secret if it doesn't exist or update it if needed
	if err := createHubAmRouterCASecret(ctx, hubInfo, client, targetNamespace); err != nil {
		return false, fmt.Errorf("failed to create or update the hub-alertmanager-router-ca secret: %w", err)
	}

	// create the observability-alertmanager-accessor secret if it doesn't exist or update it if needed
	if err := createHubAmAccessorTokenSecret(ctx, client, namespace, targetNamespace, hubInfo); err != nil {
		return false, fmt.Errorf("failed to create or update the alertmanager accessor token secret: %w", err)
	}

	// Determine if user-workload-monitoring namespace exists
	// Check namespace existence regardless of current UWL enabled state to ensure proper cleanup
	nsExists := false
	if !installProm {
		// nsExists is true if namespace exists, regardless of current UWL enabled state
		nsExists = namespaceExists(ctx, client, operatorconfig.OCPUserWorkloadMonitoringNamespace)
	}

	// Create secrets for user workload monitoring if namespace exists
	// Create Router CA and Accessor Token secrets in the UWM namespace even when alert forwarding is disabled,
	// so an external policy can configure UWM alert forwarding later if needed.
	if nsExists {
		if err := createHubAmRouterCASecret(ctx, hubInfo, client, operatorconfig.OCPUserWorkloadMonitoringNamespace); err != nil {
			return false, fmt.Errorf("failed to create or update hub-alertmanager-router-ca in UWM namespace: %w", err)
		}
		if err := createHubAmAccessorTokenSecret(ctx, client, namespace, operatorconfig.OCPUserWorkloadMonitoringNamespace, hubInfo); err != nil {
			return false, fmt.Errorf("failed to create or update alertmanager accessor token in UWM namespace: %w", err)
		}
	}

	// handle the case when alert forwarding is disabled
	if hubInfo.AlertmanagerEndpoint == "" {
		log.Info("request to disable alert forwarding")
		// only revert (once) if not done already and remember state
		revertedAlready, err := isRevertedAlready(ctx, client, namespace)
		if err != nil {
			return false, err
		}
		if !revertedAlready {
			if err = RevertClusterMonitoringConfig(ctx, client, hubInfo); err != nil {
				return false, err
			}
			if nsExists {
				if err = RevertUserWorkloadMonitoringConfig(ctx, client, hubInfo); err != nil {
					return false, err
				}
			}
			if err = setConfigReverted(ctx, client, namespace); err != nil {
				return false, err
			}
		} else {
			log.Info("configuration reverted, nothing to do")
		}
		return false, nil
	}

	if installProm {
		// *KS scenario. No need to create configmaps
		return false, unset(ctx, client, namespace)
	}

	updated, err := createOrUpdateCMOConfig(ctx, client, clusterID, hubInfo, namespace)
	if err != nil {
		return false, err
	}

	// Check UWL enabled state after CMO config is updated
	uwmEnabled := false
	if !installProm {
		var err error
		uwmEnabled, err = isUserWorkloadMonitoringEnabled(ctx, client)
		if err != nil {
			return false, fmt.Errorf("failed to determine if UWM is enabled: %w", err)
		}
	}

	if nsExists {
		if uwmEnabled {
			// UWL is enabled, create or update the config
			if err := createOrUpdateUserWorkloadMonitoringConfig(ctx, client, hubInfo); err != nil {
				return updated, fmt.Errorf("failed to create or update user workload monitoring config: %w", err)
			}
		} else {
			// UWL is disabled, clean up the configmap
			if err := RevertUserWorkloadMonitoringConfig(ctx, client, hubInfo); err != nil {
				return updated, fmt.Errorf("failed to revert user workload monitoring config: %w", err)
			}
		}
	}

	return updated, nil
}

// RevertClusterMonitoringConfig reverts the configmap cluster-monitoring-config and relevant resources
// (observability-alertmanager-accessor and hub-alertmanager-router-ca) for the openshift cluster monitoring stack.
func RevertClusterMonitoringConfig(ctx context.Context, client client.Client, hubInfo *operatorconfig.HubInfo) error {
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
			if !isManaged(v, hubInfo) {
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

// createOrUpdateCMOConfig encapsulates logic to create or update the cluster-monitoring-config configmap.
func createOrUpdateCMOConfig(
	ctx context.Context,
	client client.Client,
	clusterID string,
	hubInfo *operatorconfig.HubInfo,
	namespace string,
) (bool, error) {
	newExternalLabels := map[string]string{operatorconfig.ClusterLabelKeyForAlerts: clusterID}
	newAlertmanagerConfigs := []cmomanifests.AdditionalAlertmanagerConfig{newAdditionalAlertmanagerConfig(hubInfo)}
	newPmK8sConfig := &cmomanifests.PrometheusK8sConfig{
		ExternalLabels:      newExternalLabels,
		AlertmanagerConfigs: newAlertmanagerConfigs,
	}

	newClusterMonitoringConfiguration := cmomanifests.ClusterMonitoringConfiguration{
		PrometheusK8sConfig: newPmK8sConfig,
	}

	yamlBytes, err := yaml.Marshal(newClusterMonitoringConfiguration)
	if err != nil {
		return false, fmt.Errorf("failed to marshal cluster monitoring config to YAML: %w", err)
	}

	found := &corev1.ConfigMap{}
	err = client.Get(ctx, types.NamespacedName{
		Name:      clusterMonitoringConfigName,
		Namespace: promNamespace,
	}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			newCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterMonitoringConfigName,
					Namespace: promNamespace,
				},
				Data: map[string]string{clusterMonitoringConfigDataKey: string(yamlBytes)},
			}
			log.Info("cluster monitoring configmap not found, trying to create it", "name", clusterMonitoringConfigName)
			return true, createCMOConfigMapAndUnset(ctx, client, newCM, namespace)
		}
		return false, fmt.Errorf("failed to check configmap %s: %w", clusterMonitoringConfigName, err)
	}

	currentYAML, ok := hasClusterMonitoringConfigData(found)
	if !ok {
		found.Data[clusterMonitoringConfigDataKey] = string(yamlBytes)
		return true, updateClusterMonitoringConfigAndUnset(ctx, client, found, namespace)
	}

	existingCfg := &cmomanifests.ClusterMonitoringConfiguration{}
	if err := yaml.Unmarshal([]byte(currentYAML), existingCfg); err != nil {
		return false, fmt.Errorf("failed to unmarshal existing CMO config: %w", err)
	}

	updatedCMOCfg := &cmomanifests.ClusterMonitoringConfiguration{}
	if err := yaml.Unmarshal([]byte(currentYAML), updatedCMOCfg); err != nil {
		return false, fmt.Errorf("failed to unmarshal updated CMO config: %w", err)
	}

	if updatedCMOCfg.PrometheusK8sConfig != nil {
		// check and set externalLabels
		if updatedCMOCfg.PrometheusK8sConfig.ExternalLabels != nil {
			updatedCMOCfg.PrometheusK8sConfig.ExternalLabels[operatorconfig.ClusterLabelKeyForAlerts] = clusterID
		} else {
			updatedCMOCfg.PrometheusK8sConfig.ExternalLabels = newExternalLabels
		}

		existing := false
		var index int
		for i, cfg := range updatedCMOCfg.PrometheusK8sConfig.AlertmanagerConfigs {
			if isManaged(cfg, hubInfo) {
				existing = true
				index = i
				break
			}
		}
		if existing {
			updatedCMOCfg.PrometheusK8sConfig.AlertmanagerConfigs[index] = newAdditionalAlertmanagerConfig(hubInfo)
		} else {
			updatedCMOCfg.PrometheusK8sConfig.AlertmanagerConfigs = append(existingCfg.PrometheusK8sConfig.AlertmanagerConfigs, newAdditionalAlertmanagerConfig(hubInfo))
		}
	} else {
		updatedCMOCfg.PrometheusK8sConfig = newPmK8sConfig
	}

	updatedYAML, err := yaml.Marshal(updatedCMOCfg)
	if err != nil {
		return false, err
	}

	if reflect.DeepEqual(*existingCfg, *updatedCMOCfg) {
		return false, nil
	}

	found.Data[clusterMonitoringConfigDataKey] = string(updatedYAML)
	return true, updateClusterMonitoringConfigAndUnset(ctx, client, found, namespace)
}

// createOrUpdateUserWorkloadMonitoringConfig creates/updates the user-workload-monitoring-config configmap
func createOrUpdateUserWorkloadMonitoringConfig(
	ctx context.Context,
	client client.Client,
	hubInfo *operatorconfig.HubInfo,
) error {

	// handle the case when alert forwarding is disabled globally or UWM alerting is disabled specifically
	if hubInfo.AlertmanagerEndpoint == "" || hubInfo.UWMAlertingDisabled {
		log.Info("request to disable alert forwarding")
		return RevertUserWorkloadMonitoringConfig(ctx, client, hubInfo)
	}

	alertCfg := cmomanifests.PrometheusRestrictedConfig{
		AlertmanagerConfigs: []cmomanifests.AdditionalAlertmanagerConfig{newAdditionalAlertmanagerConfig(hubInfo)},
	}
	newCfg := cmomanifests.UserWorkloadConfiguration{
		Prometheus: &alertCfg,
	}
	yamlBytes, err := yaml.Marshal(newCfg)
	if err != nil {
		return fmt.Errorf("failed to marshal user workload monitoring config: %w", err)
	}

	existing := &corev1.ConfigMap{}
	err = client.Get(ctx, types.NamespacedName{
		Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
		Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
	}, existing)
	if err != nil && errors.IsNotFound(err) {
		newCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
				Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
			},
			Data: map[string]string{"config.yaml": string(yamlBytes)},
		}
		log.Info("user workload monitoring configmap not found, creating it", "name", operatorconfig.OCPUserWorkloadMonitoringConfigMap)
		return client.Create(ctx, newCM)
	} else if err != nil {
		return fmt.Errorf("failed to retrieve user workload monitoring configmap: %w", err)
	}

	existingYAML, ok := existing.Data["config.yaml"]
	if !ok {
		if existing.Data == nil {
			existing.Data = map[string]string{"config.yaml": string(yamlBytes)}
		} else {
			existing.Data["config.yaml"] = string(yamlBytes)
		}
		log.Info("user workload monitoring configmap missing config.yaml, updating")
		return client.Update(ctx, existing)
	}

	parsed := &cmomanifests.UserWorkloadConfiguration{}
	if err := yaml.Unmarshal([]byte(existingYAML), parsed); err != nil {
		return fmt.Errorf("failed to unmarshal existing user workload monitoring config: %w", err)
	}

	if parsed.Prometheus != nil {
		exists := false
		var index int
		for i, cfg := range parsed.Prometheus.AlertmanagerConfigs {
			if isManaged(cfg, hubInfo) {
				exists = true
				index = i
				break
			}
		}
		if !exists {
			parsed.Prometheus.AlertmanagerConfigs = append(parsed.Prometheus.AlertmanagerConfigs, newAdditionalAlertmanagerConfig(hubInfo))
		} else {
			parsed.Prometheus.AlertmanagerConfigs[index] = newAdditionalAlertmanagerConfig(hubInfo)
		}
	} else {
		parsed.Prometheus = &alertCfg
	}

	updatedYAMLBytes, err := yaml.Marshal(parsed)
	if err != nil {
		return fmt.Errorf("failed to marshal updated user workload monitoring config: %w", err)
	}

	if string(updatedYAMLBytes) == existingYAML {
		return nil
	}

	existing.Data["config.yaml"] = string(updatedYAMLBytes)
	log.Info("user workload monitoring configmap changed, updating")
	return client.Update(ctx, existing)
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
func isManaged(amc cmomanifests.AdditionalAlertmanagerConfig, hubInfo *operatorconfig.HubInfo) bool {
	if hubInfo != nil && amc.TLSConfig.CA != nil && amc.TLSConfig.CA.LocalObjectReference.Name == hubAmRouterCASecretName+"-"+hubInfo.HubClusterDomain {
		return true
	} else if os.Getenv("CMO_SCRIPT_MODE") == "true" && amc.TLSConfig.CA != nil && strings.Contains(amc.TLSConfig.CA.LocalObjectReference.Name, hubAmRouterCASecretName) {
		//This is only for the CMO cleanup script to clean up old configs
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

// RevertUserWorkloadMonitoringConfig reverts the configmap user-workload-monitoring-config
// in the openshift-user-workload-monitoring namespace.
func RevertUserWorkloadMonitoringConfig(ctx context.Context, client client.Client, hubInfo *operatorconfig.HubInfo) error {
	log.Info("RevertUserWorkloadMonitoringConfig called")

	found := &corev1.ConfigMap{}
	err := client.Get(ctx, types.NamespacedName{
		Name:      operatorconfig.OCPUserWorkloadMonitoringConfigMap,
		Namespace: operatorconfig.OCPUserWorkloadMonitoringNamespace,
	}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("configmap not found, no need action", "name", operatorconfig.OCPUserWorkloadMonitoringConfigMap)
			return nil
		}
		return fmt.Errorf("failed to check configmap %s: %w", operatorconfig.OCPUserWorkloadMonitoringConfigMap, err)
	}

	log.Info("checking if user workload monitoring config needs revert", "name", operatorconfig.OCPUserWorkloadMonitoringConfigMap)
	found = found.DeepCopy()
	if !inManagedFields(found) {
		return nil
	}

	existingYAML, ok := found.Data["config.yaml"]
	if !ok {
		return nil
	}

	parsed := &cmomanifests.UserWorkloadConfiguration{}
	if err := yaml.Unmarshal([]byte(existingYAML), parsed); err != nil {
		return fmt.Errorf("failed to unmarshal existing user workload monitoring config: %w", err)
	}

	if parsed.Prometheus == nil {
		log.Info("configmap data doesn't contain 'prometheus', no need action", "name", operatorconfig.OCPUserWorkloadMonitoringConfigMap)
		return nil
	}

	// check if alertmanagerConfigs exists
	if parsed.Prometheus.AlertmanagerConfigs != nil {
		copiedAlertmanagerConfigs := make([]cmomanifests.AdditionalAlertmanagerConfig, 0)
		for _, v := range parsed.Prometheus.AlertmanagerConfigs {
			if !isManaged(v, hubInfo) {
				copiedAlertmanagerConfigs = append(copiedAlertmanagerConfigs, v)
			}
		}

		parsed.Prometheus.AlertmanagerConfigs = copiedAlertmanagerConfigs
		if len(copiedAlertmanagerConfigs) == 0 {
			parsed.Prometheus.AlertmanagerConfigs = nil
			if reflect.DeepEqual(*parsed.Prometheus, cmomanifests.PrometheusRestrictedConfig{}) {
				parsed.Prometheus = nil
			}
		}
	}

	// check if the parsed is empty UserWorkloadConfiguration
	if reflect.DeepEqual(*parsed, cmomanifests.UserWorkloadConfiguration{}) {
		log.Info("empty UserWorkloadConfiguration, deleting configmap if it still exists", "name", operatorconfig.OCPUserWorkloadMonitoringConfigMap)
		err = client.Delete(ctx, found)
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete configmap %s: %w", operatorconfig.OCPUserWorkloadMonitoringConfigMap, err)
		}
		return nil
	}

	updatedYAMLBytes, err := yaml.Marshal(parsed)
	if err != nil {
		return fmt.Errorf("failed to marshal the user workload monitoring config: %w", err)
	}

	found.Data["config.yaml"] = string(updatedYAMLBytes)
	return client.Update(ctx, found)
}
