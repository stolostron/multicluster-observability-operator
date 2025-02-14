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
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const ( // #nosec G101 -- Not a hardcoded credential.
	hubAmRouterCASecretName        = "hub-alertmanager-router-ca"
	clusterMonitoringConfigName    = "cluster-monitoring-config"
	clusterMonitoringConfigDataKey = "config.yaml"
	endpointMonitoringOperatorMgr  = "endpoint-monitoring-operator"
	promNamespace                  = "openshift-monitoring"
	clusterRoleBindingName         = "metrics-collector-view"
	caConfigmapName                = "metrics-collector-serving-certs-ca-bundle"
	hubMetricsCollectionNamespace  = "open-cluster-management-observability"
	etcdServiceMonitor             = "acm-etcd"
	kubeApiServiceMonitor          = "acm-kube-apiserver"
)

// revertClusterMonitoringConfig reverts the configmap cluster-monitoring-config and relevant resources
// (observability-alertmanager-accessor and hub-alertmanager-router-ca) for the openshift cluster monitoring stack.
func RevertHubClusterMonitoringConfig(ctx context.Context, client client.Client) error {
	// try to retrieve the current configmap in the cluster
	found := &corev1.ConfigMap{}
	if err := client.Get(ctx, types.NamespacedName{Name: clusterMonitoringConfigName,
		Namespace: promNamespace}, found); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get configmap %s: %w", clusterMonitoringConfigName, err)
	}

	// do not touch the configmap if are not already a manager
	touched := false
	for _, field := range found.GetManagedFields() {
		if field.Manager == endpointMonitoringOperatorMgr {
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
	if err := json.Unmarshal([]byte(foundClusterMonitoringConfigurationJSON), foundClusterMonitoringConfiguration); err != nil {
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

	// check if the foundClusterMonitoringConfiguration is empty
	if reflect.DeepEqual(*foundClusterMonitoringConfiguration, cmomanifests.ClusterMonitoringConfiguration{}) {
		log.Info("empty ClusterMonitoringConfiguration, deleting configmap", "name", clusterMonitoringConfigName)
		if err := client.Delete(ctx, found); err != nil {
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

func DeleteHubMonitoringClusterRoleBinding(ctx context.Context, client client.Client) error {
	rb := &rbacv1.ClusterRoleBinding{}
	if err := client.Get(ctx, types.NamespacedName{Name: clusterRoleBindingName}, rb); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get clusterrolebinding %s: %w", clusterRoleBindingName, err)
	}

	log.Info("deleting clusterrolebinding", "name", clusterRoleBindingName)
	if err := client.Delete(ctx, rb); err != nil {
		return fmt.Errorf("failed to delete clusterrolebinding %s: %w", clusterRoleBindingName, err)
	}

	return nil
}

func DeleteHubCAConfigmap(ctx context.Context, client client.Client) error {
	cm := &corev1.ConfigMap{}
	err := client.Get(ctx, types.NamespacedName{Name: caConfigmapName,
		Namespace: hubMetricsCollectionNamespace}, cm)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to check the configmap: %w", err)
	}
	log.Info("Deleting configmap")
	err = client.Delete(ctx, cm)
	if err != nil {
		return fmt.Errorf("failed to delete configmap: %w", err)
	}
	return nil
}

func DeleteServiceMonitors(ctx context.Context, c client.Client) error {
	hList := &hyperv1.HostedClusterList{}
	err := c.List(context.TODO(), hList, &client.ListOptions{})
	if err != nil {
		log.Error(err, "Failed to list HyperShiftDeployment")
		return err
	}
	for _, cluster := range hList.Items {
		namespace := fmt.Sprintf("%s-%s", cluster.ObjectMeta.Namespace, cluster.ObjectMeta.Name)
		err = deleteServiceMonitor(ctx, c, etcdServiceMonitor, namespace)
		if err != nil {
			return err
		}
		err = deleteServiceMonitor(ctx, c, kubeApiServiceMonitor, namespace)
		if err != nil {
			return err
		}
	}
	return nil
}

func deleteServiceMonitor(ctx context.Context, c client.Client, name, namespace string) error {
	sm := &promv1.ServiceMonitor{}
	err := c.Get(ctx, types.NamespacedName{Name: name,
		Namespace: namespace}, sm)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("ServiceMonitor already deleted", "namespace", namespace, "name", name)
			return nil
		}
		log.Error(err, "Failed to check the ServiceMonitor", "namespace", namespace, "name", name)
		return err
	}
	err = c.Delete(ctx, sm)
	if err != nil {
		log.Error(err, "Error deleting ServiceMonitor", namespace, "name", name)
		return err
	}
	log.Info("ServiceMonitor deleted", "namespace", namespace, "name", name)
	return nil
}
