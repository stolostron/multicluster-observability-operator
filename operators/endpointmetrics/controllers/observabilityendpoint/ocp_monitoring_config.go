// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package observabilityendpoint

import (
	"context"
	"fmt"
	"net/url"
	"reflect"
	"strings"

	cmomanifests "github.com/openshift/cluster-monitoring-operator/pkg/manifests"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"
)

const (
	HubAmAccessorSecretName        = "observability-alertmanager-accessor" // #nosec G101 -- Not a hardcoded credential.
	hubAmAccessorSecretKey         = "token"                               // #nosec G101 -- Not a hardcoded credential.
	HubAmRouterCASecretName        = "hub-alertmanager-router-ca"
	HubAmMtlsCASecretName          = "obs-alertmanager-mtls-ca"
	clusterMonitoringConfigName    = "cluster-monitoring-config"
	clusterMonitoringRevertedName  = "cluster-monitoring-reverted"
	ClusterMonitoringConfigDataKey = "config.yaml"
	EndpointMonitoringOperatorMgr  = "endpoint-monitoring-operator"
)

var (
	log                             = ctrl.Log.WithName("controllers").WithName("ObservabilityAddon")
	clusterMonitoringConfigReverted = false
	persistedRevertStateRead        = false
)

// initializes clusterMonitoringConfigReverted based on the presence of clusterMonitoringRevertedName
// configmap in openshift-monitoring namespace.
// Note: In MCOA, cleanup/reverting is handled dynamically via the controller loop rather than a static
// cleanup command or persistent revert-state configmaps. We set the namespace param to empty ("")
// in MCOA mode to skip this legacy state tracker logic.
func initPersistedRevertState(ctx context.Context, client client.Client, ns string) error {
	if ns == "" {
		return nil
	}
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
	if ns == "" {
		return false, nil
	}
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
	if ns == "" {
		return nil
	}
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

// CreateHubAmAccessorTokenSecret creates the secret that contains access token of the Hub's Alertmanager.
func CreateHubAmAccessorTokenSecret(ctx context.Context, client client.Client, sourceName, namespace, targetNamespace string, hubInfo *operatorconfig.HubInfo) error {
	hubAmAccessorSecret := AppendHubClusterID(HubAmAccessorSecretName, hubInfo.HubClusterID)
	found := &corev1.Secret{}
	targetErr := client.Get(ctx, types.NamespacedName{Name: hubAmAccessorSecret, Namespace: targetNamespace}, found)

	amAccessorToken, err := getAmAccessorToken(ctx, client, sourceName, namespace)
	if err != nil {
		// If the source secret is missing but the target already exists, we skip the sync.
		// This avoids reconcile failures during MCOA transition where legacy source secrets
		// might be removed before or during the bridge agent's operation.
		if targetErr == nil {
			log.Info("target accessor secret already exists and source secret is missing, skipping sync", "target", targetNamespace+"/"+hubAmAccessorSecret, "source_ns", namespace)
			return nil
		}
		return fmt.Errorf("fail to get %s/%s secret: %w", namespace, sourceName, err)
	}

	dataMap := map[string][]byte{hubAmAccessorSecretKey: []byte(amAccessorToken)}
	hubAmAccessorTokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hubAmAccessorSecret,
			Namespace: targetNamespace,
		},
		Data: dataMap,
	}

	if targetErr != nil {
		if errors.IsNotFound(targetErr) {
			err = client.Create(ctx, hubAmAccessorTokenSecret)
			if err != nil {
				log.Error(err, "failed to create the observability-alertmanager-accessor secret")
				return err
			}
			log.Info("the observability-alertmanager-accessor secret is created")
			return nil
		} else {
			log.Error(targetErr, "failed to check the observability-alertmanager-accessor secret")
			return targetErr
		}
	}

	if equality.Semantic.DeepEqual(found.Data, dataMap) {
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

// getAmAccessorToken retrieves the alertmanager access token from source secret.
func getAmAccessorToken(ctx context.Context, client client.Client, sourceName, ns string) (string, error) {
	amAccessorSecret := &corev1.Secret{}
	if err := client.Get(ctx, types.NamespacedName{
		Name:      sourceName,
		Namespace: ns,
	}, amAccessorSecret); err != nil {
		return "", err
	}

	amAccessorToken := amAccessorSecret.Data[hubAmAccessorSecretKey]
	if amAccessorToken == nil {
		return "", fmt.Errorf("no token in secret %s", sourceName)
	}

	return string(amAccessorToken), nil
}

func AppendHubClusterID(secretName string, hubClusterID string) string {
	if hubClusterID == "" {
		return secretName
	}
	return secretName + "-" + hubClusterID
}

func newAdditionalAlertmanagerConfig(alertmanagerEndpoint string, caSecret string, certSecret string, accessorSecret string) cmomanifests.AdditionalAlertmanagerConfig {
	bearerToken := &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: accessorSecret,
		},
		Key: hubAmAccessorSecretKey,
	}

	config := cmomanifests.AdditionalAlertmanagerConfig{
		Scheme:     "https",
		PathPrefix: "/",
		APIVersion: "v2",
		TLSConfig: cmomanifests.TLSConfig{
			CA: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: caSecret,
				},
				Key: "ca.crt",
			},
			Cert: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: certSecret,
				},
				Key: "tls.crt",
			},
			Key: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: certSecret,
				},
				Key: "tls.key",
			},
			InsecureSkipVerify: false,
		},
		BearerToken:   bearerToken,
		StaticConfigs: []string{},
	}
	amURL, err := url.Parse(alertmanagerEndpoint)
	if err != nil {
		log.Error(err, "failed to parse alertmanager endpoint. ignoring it", "endpoint", alertmanagerEndpoint)
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

	// Determine if user-workload-monitoring namespace exists
	// Check namespace existence regardless of current UWL enabled state to ensure proper cleanup
	nsExists := false
	if !installProm {
		// nsExists is true if namespace exists, regardless of current UWL enabled state
		nsExists = namespaceExists(ctx, client, operatorconfig.OCPUserWorkloadMonitoringNamespace)
	}

	// create the observability-alertmanager-accessor secret if it doesn't exist or update it if needed
	if err := CreateHubAmAccessorTokenSecret(ctx, client, HubAmAccessorSecretName, namespace, targetNamespace, hubInfo); err != nil {
		return false, fmt.Errorf("failed to create or update the alertmanager accessor token secret: %w", err)
	}

	mtlsRename := map[string]string{mtlsCertName: amMtlsCertName, mtlsCaName: amMtlsCaName}
	for name, rename := range mtlsRename {
		if err := CreateMtlsSecretInNamespace(ctx, client, namespace, targetNamespace, name, rename, hubInfo); err != nil {
			return false, fmt.Errorf("failed to copy mTLS secret %s to %s: %w", name, targetNamespace, err)
		}
	}

	// Create secrets for user workload monitoring if namespace exists
	// Create Router CA and Accessor Token secrets in the UWM namespace even when alert forwarding is disabled,
	// so an external policy can configure UWM alert forwarding later if needed.
	if nsExists {
		if err := CreateHubAmAccessorTokenSecret(ctx, client, HubAmAccessorSecretName, namespace, operatorconfig.OCPUserWorkloadMonitoringNamespace, hubInfo); err != nil {
			return false, fmt.Errorf("failed to create or update alertmanager accessor token in UWM namespace: %w", err)
		}
		for name, rename := range mtlsRename {
			if err := CreateMtlsSecretInNamespace(ctx, client, namespace, operatorconfig.OCPUserWorkloadMonitoringNamespace, name, rename, hubInfo); err != nil {
				return false, fmt.Errorf("failed to copy mTLS secret %s to UWM namespace: %w", name, err)
			}
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
			caSecret := AppendHubClusterID(amMtlsCaName, hubInfo.HubClusterID)
			if err = RevertClusterMonitoringConfig(ctx, client, caSecret, ""); err != nil {
				return false, err
			}
			if nsExists {
				if err = RevertUserWorkloadMonitoringConfig(ctx, client, caSecret); err != nil {
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

	caSecret := AppendHubClusterID(amMtlsCaName, hubInfo.HubClusterID)
	certSecret := AppendHubClusterID(amMtlsCertName, hubInfo.HubClusterID)
	accessorSecret := AppendHubClusterID(HubAmAccessorSecretName, hubInfo.HubClusterID)

	updated, err := CreateOrUpdateCMOConfig(
		ctx,
		client,
		clusterID,
		"", // Pass empty string to avoid setting managed_cluster_name for legacy collector to prevent issues with global hub
		hubInfo.AlertmanagerEndpoint,
		caSecret,
		certSecret,
		accessorSecret,
		namespace,
	)
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
			if err := CreateOrUpdateUserWorkloadMonitoringConfig(
				ctx,
				client,
				hubInfo.AlertmanagerEndpoint,
				hubInfo.UWMAlertingDisabled,
				caSecret,
				certSecret,
				accessorSecret,
			); err != nil {
				return updated, fmt.Errorf("failed to create or update user workload monitoring config: %w", err)
			}
		} else {
			// UWL is disabled, clean up the configmap
			if err := RevertUserWorkloadMonitoringConfig(ctx, client, caSecret); err != nil {
				return updated, fmt.Errorf("failed to revert user workload monitoring config: %w", err)
			}
		}
	}

	return updated, nil
}

// RevertClusterMonitoringConfig reverts the configmap cluster-monitoring-config and relevant resources
// (observability-alertmanager-accessor and hub-alertmanager-router-ca) for the openshift cluster monitoring stack.
func RevertClusterMonitoringConfig(ctx context.Context, client client.Client, caSecret string, clusterName string) error {
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

	foundClusterMonitoringConfigurationYAML, ok := HasClusterMonitoringConfigData(found)
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
		if clusterName != "" {
			delete(foundClusterMonitoringConfiguration.PrometheusK8sConfig.ExternalLabels, operatorconfig.ClusterNameLabelKeyForAlerts)
		}

		if len(foundClusterMonitoringConfiguration.PrometheusK8sConfig.ExternalLabels) == 0 {
			foundClusterMonitoringConfiguration.PrometheusK8sConfig.ExternalLabels = nil
		}
	}

	// check if alertmanagerConfigs exists
	if foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs != nil {
		copiedAlertmanagerConfigs := make([]cmomanifests.AdditionalAlertmanagerConfig, 0)
		for _, v := range foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs {
			if !IsManaged(v, caSecret) {
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

	found.Data[ClusterMonitoringConfigDataKey] = string(updatedClusterMonitoringConfigurationYAMLBytes)
	return updateClusterMonitoringConfig(ctx, client, found)
}

// CreateOrUpdateCMOConfig encapsulates logic to create or update the cluster-monitoring-config configmap.
func CreateOrUpdateCMOConfig(
	ctx context.Context,
	client client.Client,
	clusterID string,
	clusterName string,
	alertmanagerEndpoint string,
	caSecret string,
	certSecret string,
	accessorSecret string,
	namespace string,
) (bool, error) {
	newExternalLabels := map[string]string{
		operatorconfig.ClusterLabelKeyForAlerts: clusterID,
	}
	if clusterName != "" {
		newExternalLabels[operatorconfig.ClusterNameLabelKeyForAlerts] = clusterName
	}
	newAlertmanagerConfigs := []cmomanifests.AdditionalAlertmanagerConfig{newAdditionalAlertmanagerConfig(alertmanagerEndpoint, caSecret, certSecret, accessorSecret)}
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
				Data: map[string]string{ClusterMonitoringConfigDataKey: string(yamlBytes)},
			}
			log.Info("cluster monitoring configmap not found, trying to create it", "name", clusterMonitoringConfigName)
			return true, createCMOConfigMapAndUnset(ctx, client, newCM, namespace)
		}
		return false, fmt.Errorf("failed to check configmap %s: %w", clusterMonitoringConfigName, err)
	}

	currentYAML, ok := HasClusterMonitoringConfigData(found)
	if !ok {
		found.Data[ClusterMonitoringConfigDataKey] = string(yamlBytes)
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
			if clusterName != "" {
				updatedCMOCfg.PrometheusK8sConfig.ExternalLabels[operatorconfig.ClusterNameLabelKeyForAlerts] = clusterName
			} else {
				delete(updatedCMOCfg.PrometheusK8sConfig.ExternalLabels, operatorconfig.ClusterNameLabelKeyForAlerts)
			}
		} else {
			updatedCMOCfg.PrometheusK8sConfig.ExternalLabels = newExternalLabels
		}

		// Filter out any of our pre-existing managed Alertmanager configurations (including legacy ones).
		// This guarantees we only ever have exactly one active ACM/MCOA alertmanager configuration,
		// and handles the upgrade path (Router CA -> mTLS CA) cleanly.
		cleanAlertmanagerConfigs := make([]cmomanifests.AdditionalAlertmanagerConfig, 0)
		for _, cfg := range updatedCMOCfg.PrometheusK8sConfig.AlertmanagerConfigs {
			if !IsManaged(cfg, caSecret) {
				cleanAlertmanagerConfigs = append(cleanAlertmanagerConfigs, cfg)
			}
		}
		// Append exactly one fresh configured stanza
		cleanAlertmanagerConfigs = append(
			cleanAlertmanagerConfigs,
			newAdditionalAlertmanagerConfig(alertmanagerEndpoint, caSecret, certSecret, accessorSecret),
		)
		updatedCMOCfg.PrometheusK8sConfig.AlertmanagerConfigs = cleanAlertmanagerConfigs
	} else {
		updatedCMOCfg.PrometheusK8sConfig = newPmK8sConfig
	}

	updatedYAML, err := yaml.Marshal(updatedCMOCfg)
	if err != nil {
		return false, err
	}

	if equality.Semantic.DeepEqual(*existingCfg, *updatedCMOCfg) {
		return false, nil
	}

	found.Data[ClusterMonitoringConfigDataKey] = string(updatedYAML)
	return true, updateClusterMonitoringConfigAndUnset(ctx, client, found, namespace)
}

// CreateOrUpdateUserWorkloadMonitoringConfig creates/updates the user-workload-monitoring-config configmap
func CreateOrUpdateUserWorkloadMonitoringConfig(
	ctx context.Context,
	c client.Client,
	alertmanagerEndpoint string,
	uwmAlertingDisabled bool,
	caSecret string,
	certSecret string,
	accessorSecret string,
) error {
	// handle the case when alert forwarding is disabled globally or UWM alerting is disabled specifically
	if alertmanagerEndpoint == "" || uwmAlertingDisabled {
		log.Info("request to disable alert forwarding")
		return RevertUserWorkloadMonitoringConfig(ctx, c, caSecret)
	}

	alertCfg := cmomanifests.PrometheusRestrictedConfig{
		AlertmanagerConfigs: []cmomanifests.AdditionalAlertmanagerConfig{newAdditionalAlertmanagerConfig(alertmanagerEndpoint, caSecret, certSecret, accessorSecret)},
	}
	newCfg := cmomanifests.UserWorkloadConfiguration{
		Prometheus: &alertCfg,
	}
	yamlBytes, err := yaml.Marshal(newCfg)
	if err != nil {
		return fmt.Errorf("failed to marshal user workload monitoring config: %w", err)
	}

	existing := &corev1.ConfigMap{}
	err = c.Get(ctx, types.NamespacedName{
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
		return c.Create(ctx, newCM)
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
		return c.Update(ctx, existing)
	}

	parsed := &cmomanifests.UserWorkloadConfiguration{}
	if err := yaml.Unmarshal([]byte(existingYAML), parsed); err != nil {
		return fmt.Errorf("failed to unmarshal existing user workload monitoring config: %w", err)
	}

	if parsed.Prometheus != nil {
		// Filter out any of our pre-existing managed Alertmanager configurations (including legacy ones).
		cleanAlertmanagerConfigs := make([]cmomanifests.AdditionalAlertmanagerConfig, 0)
		for _, cfg := range parsed.Prometheus.AlertmanagerConfigs {
			if !IsManaged(cfg, caSecret) {
				cleanAlertmanagerConfigs = append(cleanAlertmanagerConfigs, cfg)
			}
		}
		// Append exactly one fresh configured stanza
		cleanAlertmanagerConfigs = append(
			cleanAlertmanagerConfigs,
			newAdditionalAlertmanagerConfig(alertmanagerEndpoint, caSecret, certSecret, accessorSecret),
		)
		parsed.Prometheus.AlertmanagerConfigs = cleanAlertmanagerConfigs
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
	return c.Update(ctx, existing, client.FieldOwner(EndpointMonitoringOperatorMgr))
}

// createCMOConfigMapAndUnset creates the configmap cluster-monitoring-config in the openshift-monitoring namespace.
// the namespace parameter is used to call unset function.
func createCMOConfigMapAndUnset(ctx context.Context, c client.Client, obj client.Object, namespace string) error {
	err := c.Create(ctx, obj, client.FieldOwner(EndpointMonitoringOperatorMgr))
	if err != nil {
		log.Error(err, "failed to create configmap", "name", clusterMonitoringConfigName)
		return err
	}
	log.Info("configmap created", "name", clusterMonitoringConfigName)
	return unset(ctx, c, namespace)
}

func updateClusterMonitoringConfig(ctx context.Context, c client.Client, obj client.Object) error {
	err := c.Update(ctx, obj, client.FieldOwner(EndpointMonitoringOperatorMgr))
	if err != nil {
		return fmt.Errorf("failed to update configmap %s: %w", clusterMonitoringConfigName, err)
	}
	log.Info("configmap updated", "name", clusterMonitoringConfigName)
	return nil
}

// updateClusterMonitoringConfigAndUnset updates the configmap cluster-monitoring-config in the openshift-monitoring namespace.
// the namespace parameter is used to call unset function.
func updateClusterMonitoringConfigAndUnset(ctx context.Context, c client.Client, obj client.Object, namespace string) error {
	err := updateClusterMonitoringConfig(ctx, c, obj)
	if err != nil {
		return err
	}
	return unset(ctx, c, namespace)
}

// unset config reverted flag after successfully updating cluster-monitoring-config
func unset(ctx context.Context, c client.Client, ns string) error {
	if ns == "" {
		return nil
	}
	// if reverted before, reset so we can revert again
	revertedAlready, err := isRevertedAlready(ctx, c, ns)
	if err == nil && revertedAlready {
		err = unsetConfigReverted(ctx, c, ns)
	}
	return err
}

// InManagedFields checks if the configmap has had a CRUD operation by endpoint-monitoring-operator
func InManagedFields(cm *corev1.ConfigMap) bool {
	for _, field := range cm.GetManagedFields() {
		if field.Manager == EndpointMonitoringOperatorMgr {
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

// IsManaged checks if the additional alertmanager config is managed by ACM.
//
// This checks both the exact caSecret name and any matching legacy names with the same suffix.
// Suffix matching is necessary when migrating a managed cluster from the legacy flow to the new mTLS flow:
// - Legacy: We targeted the hub Alertmanager directly using the router CA (e.g. hub-alertmanager-router-ca-<hub-id>).
//   - New: We target the Observatorium API instead, which handles fanning out alerts to all instances,
//     using the new mTLS CA (e.g. obs-alertmanager-mtls-ca-<hub-id>).
//
// Extracting the suffix and comparing CA names ensures that the legacy configuration stanza is correctly
// identified as managed and replaced in-place, rather than being treated as unmanaged and duplicated.
func IsManaged(amc cmomanifests.AdditionalAlertmanagerConfig, caSecret string) bool {
	if amc.TLSConfig.CA == nil {
		return false
	}
	caName := amc.TLSConfig.CA.Name

	if caSecret == "" {
		return false
	}

	if caName == caSecret {
		return true
	}

	// Suffix extraction and legacy upgrade matching:
	// Extract the suffix (e.g. "-<hub-id>") from the expected caSecret.
	var suffix string
	if strings.HasPrefix(caSecret, amMtlsCaName+"-") {
		suffix = strings.TrimPrefix(caSecret, amMtlsCaName)
	} else if strings.HasPrefix(caSecret, HubAmRouterCASecretName+"-") {
		suffix = strings.TrimPrefix(caSecret, HubAmRouterCASecretName)
	}

	// Ensure we found a valid, non-empty suffix (e.g. "-12345", meaning length > 1) before matching.
	if len(suffix) > 1 {
		isOurBase := strings.HasPrefix(caName, HubAmRouterCASecretName+"-") || strings.HasPrefix(caName, amMtlsCaName+"-")
		if isOurBase && strings.HasSuffix(caName, suffix) {
			return true
		}
	}

	return false
}

// HasClusterMonitoringConfigData checks if the configmap has the required key and logs if not
func HasClusterMonitoringConfigData(cm *corev1.ConfigMap) (string, bool) {
	data, ok := cm.Data[ClusterMonitoringConfigDataKey]
	if !ok {
		log.Info(
			"configmap doesn't contain required key",
			"name",
			clusterMonitoringConfigName,
			"key",
			ClusterMonitoringConfigDataKey,
		)
	}
	return data, ok
}

// RevertUserWorkloadMonitoringConfig reverts the configmap user-workload-monitoring-config
// in the openshift-user-workload-monitoring namespace.
func RevertUserWorkloadMonitoringConfig(ctx context.Context, client client.Client, caSecret string) error {
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
			if !IsManaged(v, caSecret) {
				copiedAlertmanagerConfigs = append(copiedAlertmanagerConfigs, v)
			}
		}

		parsed.Prometheus.AlertmanagerConfigs = copiedAlertmanagerConfigs
		if len(copiedAlertmanagerConfigs) == 0 {
			parsed.Prometheus.AlertmanagerConfigs = nil
			if equality.Semantic.DeepEqual(*parsed.Prometheus, cmomanifests.PrometheusRestrictedConfig{}) {
				parsed.Prometheus = nil
			}
		}
	}

	// check if the parsed is empty UserWorkloadConfiguration
	if equality.Semantic.DeepEqual(*parsed, cmomanifests.UserWorkloadConfiguration{}) {
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

func CreateMtlsSecretInNamespace(ctx context.Context, c client.Client, sourceNamespace, targetNamespace, secretName string, secretRename string, hubInfo *operatorconfig.HubInfo) error {
	source := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{Name: secretName, Namespace: sourceNamespace}, source); err != nil {
		return fmt.Errorf("failed to get source secret %s/%s: %w", sourceNamespace, secretName, err)
	}

	targetName := AppendHubClusterID(secretRename, hubInfo.HubClusterID)
	target := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      targetName,
			Namespace: targetNamespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, c, target, func() error {
		target.Type = source.Type
		target.Data = source.Data
		target.Labels = source.Labels
		target.Annotations = source.Annotations
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update secret %s/%s: %w", targetNamespace, targetName, err)
	}

	if op != controllerutil.OperationResultNone {
		log.Info("mTLS secret in target namespace", "operation", op, "secret", targetName, "namespace", targetNamespace)
	} else {
		log.V(1).Info("mTLS secret already up to date", "secret", targetName, "namespace", targetNamespace)
	}

	return nil
}
