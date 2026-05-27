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
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
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
	clusterMonitoringConfigName    = "cluster-monitoring-config"
	clusterMonitoringRevertedName  = "cluster-monitoring-reverted"
	ClusterMonitoringConfigDataKey = "config.yaml"
	EndpointMonitoringOperatorMgr  = "endpoint-monitoring-operator"
)

var (
	log                             = ctrl.Log.WithName("controllers").WithName("ObservabilityAddon")
	clusterMonitoringConfigReverted = false
	persistedRevertStateRead        = false
	AMSecretCleanupDone             = false
	AMSecretCleanupDoneUWL          = false
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

// createHubAmAccessorTokenSecret creates the secret that contains access token of the Hub's Alertmanager.
func createHubAmAccessorTokenSecret(ctx context.Context, client client.Client, namespace, targetNamespace string, hubInfo *operatorconfig.HubInfo) error {
	amAccessorToken, err := getAmAccessorToken(ctx, client, namespace)
	if err != nil {
		return fmt.Errorf("fail to get %s/%s secret: %w", namespace, HubAmAccessorSecretName, err)
	}

	hubAmAccessorSecret := AppendHubClusterID(HubAmAccessorSecretName, hubInfo)
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
	if err := client.Get(ctx, types.NamespacedName{
		Name:      HubAmAccessorSecretName,
		Namespace: ns,
	}, amAccessorSecret); err != nil {
		return "", err
	}

	amAccessorToken := amAccessorSecret.Data[hubAmAccessorSecretKey]
	if amAccessorToken == nil {
		return "", fmt.Errorf("no token in secret %s", HubAmAccessorSecretName)
	}

	return string(amAccessorToken), nil
}

func cleanUpOldAMSecrets(ctx context.Context, client client.Client, targetNamespace string, hubInfo *operatorconfig.HubInfo, uwlNsExists bool, installProm bool) error {
	var errs []string
	clusterDomain := ""
	if hubInfo != nil && hubInfo.ObservatoriumAPIEndpoint != "" {
		clusterDomain = config.GetClusterName(hubInfo.ObservatoriumAPIEndpoint)
	} else {
		log.Info("hubInfo or ObservatoriumAPIEndpoint missing; skipping cluster-domain-specific secret deletions")
	}

	deleteSecret := func(name string, namespace string) {
		sec := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		}
		if err := client.Delete(ctx, sec); err != nil {
			if errors.IsNotFound(err) {
				return
			}
			log.Error(err, fmt.Sprintf("failed to delete old secret %s/%s", targetNamespace, name))
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
		}
	}

	if !installProm {
		// we do not cleanup in *KS scenario as those secrets reside in the addon namespace and not openshift-monitoring
		// and are referenced for creating AM accessor token secret
		deleteSecret(HubAmAccessorSecretName, targetNamespace)
		deleteSecret(HubAmRouterCASecretName, targetNamespace)
	}
	if clusterDomain != "" {
		deleteSecret(HubAmAccessorSecretName+"-"+clusterDomain, targetNamespace)
		deleteSecret(HubAmRouterCASecretName+"-"+clusterDomain, targetNamespace)
	}
	if hubInfo != nil && hubInfo.HubClusterID != "" {
		deleteSecret(AppendHubClusterID(HubAmRouterCASecretName, hubInfo), targetNamespace)
	}

	if uwlNsExists {
		ns := operatorconfig.OCPUserWorkloadMonitoringNamespace
		deleteSecret(HubAmAccessorSecretName, ns)
		deleteSecret(HubAmRouterCASecretName, ns)
		if clusterDomain != "" {
			deleteSecret(HubAmAccessorSecretName+"-"+clusterDomain, ns)
			deleteSecret(HubAmRouterCASecretName+"-"+clusterDomain, ns)
		}
		if hubInfo != nil && hubInfo.HubClusterID != "" {
			deleteSecret(AppendHubClusterID(HubAmRouterCASecretName, hubInfo), ns)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to delete one or more old secrets: %s", strings.Join(errs, "; "))
	}
	return nil
}

func AppendHubClusterID(secretName string, hubInfo *operatorconfig.HubInfo) string {
	if hubInfo == nil || hubInfo.HubClusterID == "" {
		return secretName
	}
	return secretName + "-" + hubInfo.HubClusterID
}

func newAdditionalAlertmanagerConfig(hubInfo *operatorconfig.HubInfo) cmomanifests.AdditionalAlertmanagerConfig {
	amMtlsCARef := AppendHubClusterID(amMtlsCaName, hubInfo)
	amMtlsCertRef := AppendHubClusterID(amMtlsCertName, hubInfo)
	config := cmomanifests.AdditionalAlertmanagerConfig{
		Scheme:     "https",
		PathPrefix: "/",
		APIVersion: "v2",
		TLSConfig: cmomanifests.TLSConfig{
			CA: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: amMtlsCARef,
				},
				Key: "ca.crt",
			},
			Cert: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: amMtlsCertRef,
				},
				Key: "tls.crt",
			},
			Key: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: amMtlsCertRef,
				},
				Key: "tls.key",
			},
			InsecureSkipVerify: false,
		},
		BearerToken: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: AppendHubClusterID(HubAmAccessorSecretName, hubInfo),
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

	// Determine if user-workload-monitoring namespace exists
	// Check namespace existence regardless of current UWL enabled state to ensure proper cleanup
	nsExists := false
	if !installProm {
		// nsExists is true if namespace exists, regardless of current UWL enabled state
		nsExists = namespaceExists(ctx, client, operatorconfig.OCPUserWorkloadMonitoringNamespace)
	}

	if !AMSecretCleanupDone {
		// ACM-27841 - this is to clean up old alertmanager secrets created using cluster domain for Global Hu
		if err := cleanUpOldAMSecrets(ctx, client, targetNamespace, hubInfo, nsExists, installProm); err != nil {
			return false, fmt.Errorf("failed to clean up old alertmanager secrets: %w", err)
		}
	}

	// create the observability-alertmanager-accessor secret if it doesn't exist or update it if needed
	if err := createHubAmAccessorTokenSecret(ctx, client, namespace, targetNamespace, hubInfo); err != nil {
		return false, fmt.Errorf("failed to create or update the alertmanager accessor token secret: %w", err)
	}

	mtlsRename := map[string]string{mtlsCertName: amMtlsCertName, mtlsCaName: amMtlsCaName}
	for name, rename := range mtlsRename {
		if err := createMtlsSecretInNamespace(ctx, client, namespace, targetNamespace, name, rename, hubInfo); err != nil {
			return false, fmt.Errorf("failed to copy mTLS secret %s to %s: %w", name, targetNamespace, err)
		}
	}

	// Create secrets for user workload monitoring if namespace exists
	// Create Router CA and Accessor Token secrets in the UWM namespace even when alert forwarding is disabled,
	// so an external policy can configure UWM alert forwarding later if needed.
	if nsExists {
		if err := createHubAmAccessorTokenSecret(ctx, client, namespace, operatorconfig.OCPUserWorkloadMonitoringNamespace, hubInfo); err != nil {
			return false, fmt.Errorf("failed to create or update alertmanager accessor token in UWM namespace: %w", err)
		}
		for name, rename := range mtlsRename {
			if err := createMtlsSecretInNamespace(ctx, client, namespace, operatorconfig.OCPUserWorkloadMonitoringNamespace, name, rename, hubInfo); err != nil {
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

	updated, err := CreateOrUpdateCMOConfig(ctx, client, clusterID, hubInfo, namespace)
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
			if err := CreateOrUpdateUserWorkloadMonitoringConfig(ctx, client, hubInfo); err != nil {
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
	if !InManagedFields(found) {
		return nil
	}

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

		if len(foundClusterMonitoringConfiguration.PrometheusK8sConfig.ExternalLabels) == 0 {
			foundClusterMonitoringConfiguration.PrometheusK8sConfig.ExternalLabels = nil
		}
	}

	// check if alertmanagerConfigs exists
	if foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs != nil {
		copiedAlertmanagerConfigs := make([]cmomanifests.AdditionalAlertmanagerConfig, 0)
		for _, v := range foundClusterMonitoringConfiguration.PrometheusK8sConfig.AlertmanagerConfigs {
			if !IsManaged(v, hubInfo) {
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
		} else {
			updatedCMOCfg.PrometheusK8sConfig.ExternalLabels = newExternalLabels
		}

		existing := false
		var index int
		for i, cfg := range updatedCMOCfg.PrometheusK8sConfig.AlertmanagerConfigs {
			if IsManaged(cfg, hubInfo) {
				existing = true
				index = i
				break
			}
		}
		if existing {
			updatedCMOCfg.PrometheusK8sConfig.AlertmanagerConfigs[index] = newAdditionalAlertmanagerConfig(hubInfo)
		} else {
			updatedCMOCfg.PrometheusK8sConfig.AlertmanagerConfigs = append(updatedCMOCfg.PrometheusK8sConfig.AlertmanagerConfigs, newAdditionalAlertmanagerConfig(hubInfo))
		}

		// remove am configs from previous versions if any prior to Global Hub Changes (ACM 2.15.0)
		if !AMSecretCleanupDone {
			updatedCMOCfgTmp := make([]cmomanifests.AdditionalAlertmanagerConfig, 0)
			for i, cfg := range updatedCMOCfg.PrometheusK8sConfig.AlertmanagerConfigs {
				if !isOldManagedConfig(cfg, hubInfo) {
					updatedCMOCfgTmp = append(updatedCMOCfgTmp, updatedCMOCfg.PrometheusK8sConfig.AlertmanagerConfigs[i])
				}
			}
			AMSecretCleanupDone = true
			updatedCMOCfg.PrometheusK8sConfig.AlertmanagerConfigs = updatedCMOCfgTmp
		}
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
			if IsManaged(cfg, hubInfo) {
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

		// remove am configs from previous versions if any prior to Global Hub Changes (ACM 2.15.0)
		if !AMSecretCleanupDoneUWL {
			updatedCMOCfgTmp := make([]cmomanifests.AdditionalAlertmanagerConfig, 0)
			for i, cfg := range parsed.Prometheus.AlertmanagerConfigs {
				if !isOldManagedConfig(cfg, hubInfo) {
					updatedCMOCfgTmp = append(updatedCMOCfgTmp, parsed.Prometheus.AlertmanagerConfigs[i])
				}
			}
			AMSecretCleanupDoneUWL = true
			parsed.Prometheus.AlertmanagerConfigs = updatedCMOCfgTmp
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

// IsManaged checks if the additional alertmanager config is managed by ACM
func IsManaged(amc cmomanifests.AdditionalAlertmanagerConfig, hubInfo *operatorconfig.HubInfo) bool {
	if amc.TLSConfig.CA == nil {
		return false
	}
	caName := amc.TLSConfig.CA.Name
	if hubInfo != nil {
		switch caName {
		case HubAmRouterCASecretName + "-" + hubInfo.HubClusterID,
			amMtlsCaName + "-" + hubInfo.HubClusterID:
			return true
		default:
			return false
		}
	}
	return strings.Contains(caName, HubAmRouterCASecretName) ||
		strings.Contains(caName, amMtlsCaName)
}

// isOldManagedConfig checks if the additional alertmanager config is managed by ACM with old secret names prior to Global Hub changes
func isOldManagedConfig(amc cmomanifests.AdditionalAlertmanagerConfig, hubInfo *operatorconfig.HubInfo) bool {
	if hubInfo != nil && amc.TLSConfig.CA != nil {
		clusterDomainName := config.GetClusterName(hubInfo.ObservatoriumAPIEndpoint)
		switch amc.TLSConfig.CA.Name {
		case HubAmRouterCASecretName, HubAmRouterCASecretName + "-" + clusterDomainName:
			return true
		// check if managed by ACM with old secret prior to alertmanager fanout change
		case HubAmRouterCASecretName + "-" + hubInfo.HubClusterID:
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
	if !InManagedFields(found) {
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
			if !IsManaged(v, hubInfo) {
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

func createMtlsSecretInNamespace(ctx context.Context, c client.Client, sourceNamespace, targetNamespace, secretName string, secretRename string, hubInfo *operatorconfig.HubInfo) error {
	source := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{Name: secretName, Namespace: sourceNamespace}, source); err != nil {
		return fmt.Errorf("failed to get source secret %s/%s: %w", sourceNamespace, secretName, err)
	}

	targetName := AppendHubClusterID(secretRename, hubInfo)
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
