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
	hyperv1 "github.com/openshift/hypershift/api/v1alpha1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	cmomanifests "github.com/stolostron/multicluster-observability-operator/pkg/cmo"
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
	log.Info("revertClusterMonitoringConfig called")

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

	// do not touch the configmap if are not already a manager
	touched := false
	log.Info("checking field.Manager")
	for _, field := range found.GetManagedFields() {
		log.Info("filed.Manager", "manager", field.Manager)
		if field.Manager == endpointMonitoringOperatorMgr {
			touched = true
			break
		}
	}

	if !touched {
		log.Info(
			"endpoint-monitoring-operator is not a manager of configmap, no action needed",
			"name",
			clusterMonitoringConfigName,
		)
		return nil
	}

	// revert the existing cluster-monitor-config configmap
	log.Info("configmap exists, check if it needs revert", "name", clusterMonitoringConfigName)
	foundClusterMonitoringConfigurationYAML, ok := found.Data[clusterMonitoringConfigDataKey]
	if !ok {
		log.Info(
			"configmap data doesn't contain key, no action need",
			"name",
			clusterMonitoringConfigName,
			"key",
			clusterMonitoringConfigDataKey,
		)
		return nil
	}
	foundClusterMonitoringConfigurationJSON, err := yaml.YAMLToJSON([]byte(foundClusterMonitoringConfigurationYAML))
	if err != nil {
		log.Error(err, "failed to transform YAML to JSON", "YAML", foundClusterMonitoringConfigurationYAML)
		return err
	}

	log.Info(
		"configmap exists and key config.yaml exists, check if the value needs revert",
		"name",
		clusterMonitoringConfigName,
		"key",
		clusterMonitoringConfigDataKey,
	)
	foundClusterMonitoringConfiguration := &cmomanifests.ClusterMonitoringConfiguration{}
	if err := json.Unmarshal([]byte(foundClusterMonitoringConfigurationJSON), foundClusterMonitoringConfiguration); err != nil {
		log.Error(err, "failed to marshal the cluster monitoring config")
		return err
	}

	if foundClusterMonitoringConfiguration.PrometheusK8sConfig == nil {
		log.Info(
			"configmap data doesn't key: prometheusK8s, no need action",
			"name",
			clusterMonitoringConfigName,
			"key",
			clusterMonitoringConfigDataKey,
		)
		return nil
	} else {
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
	updatedClusterMonitoringConfigurationYAMLBytes, err := yaml.JSONToYAML(
		updatedClusterMonitoringConfigurationJSONBytes,
	)
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
func DeleteHubMonitoringClusterRoleBinding(ctx context.Context, client client.Client) error {
	rb := &rbacv1.ClusterRoleBinding{}
	err := client.Get(ctx, types.NamespacedName{Name: clusterRoleBindingName,
		Namespace: ""}, rb)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("clusterrolebinding already deleted")
			return nil
		}
		log.Error(err, "Failed to check the clusterrolebinding")
		return err
	}
	err = client.Delete(ctx, rb)
	if err != nil {
		log.Error(err, "Error deleting clusterrolebinding")
		return err
	}
	log.Info("clusterrolebinding deleted")
	return nil
}

func DeleteHubCAConfigmap(ctx context.Context, client client.Client) error {
	cm := &corev1.ConfigMap{}
	err := client.Get(ctx, types.NamespacedName{Name: caConfigmapName,
		Namespace: hubMetricsCollectionNamespace}, cm)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("configmap already deleted")
			return nil
		}
		log.Error(err, "Failed to check the configmap")
		return err
	}
	err = client.Delete(ctx, cm)
	if err != nil {
		log.Error(err, "Error deleting configmap")
		return err
	}
	log.Info("configmap deleted")
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
