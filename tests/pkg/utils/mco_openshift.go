// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
)

const (
	clusterMonitoringConfigMapName = "cluster-monitoring-config"
	openshiftMonitoringNamespace   = "openshift-monitoring"
	configKey                      = "config.yaml"
)

// EnableUWLMonitoringOnManagedClusters enables user workload monitoring on all managed OpenShift clusters.
func EnableUWLMonitoringOnManagedClusters(opt TestOptions, ocpClusters []Cluster) error {
	for _, cluster := range ocpClusters {
		kubeClient := NewKubeClient(
			cluster.ClusterServerURL,
			cluster.KubeConfig,
			cluster.KubeContext)

		err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			cm, err := kubeClient.CoreV1().ConfigMaps(openshiftMonitoringNamespace).Get(context.TODO(), clusterMonitoringConfigMapName, metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					config := map[string]any{
						"enableUserWorkload": true,
					}
					yamlData, err := yaml.Marshal(config)
					if err != nil {
						return err
					}
					newCM := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      clusterMonitoringConfigMapName,
							Namespace: openshiftMonitoringNamespace,
						},
						Data: map[string]string{
							configKey: string(yamlData),
						},
					}
					_, createErr := kubeClient.CoreV1().ConfigMaps(openshiftMonitoringNamespace).Create(context.TODO(), newCM, metav1.CreateOptions{})
					return createErr
				}
				return err
			}

			config := make(map[string]any)
			if cm.Data != nil && cm.Data[configKey] != "" {
				if err = yaml.Unmarshal([]byte(cm.Data[configKey]), &config); err != nil {
					return err
				}
			}
			config["enableUserWorkload"] = true
			yamlData, err := yaml.Marshal(config)
			if err != nil {
				return err
			}
			if cm.Data == nil {
				cm.Data = make(map[string]string)
			}
			cm.Data[configKey] = string(yamlData)
			_, updateErr := kubeClient.CoreV1().ConfigMaps(openshiftMonitoringNamespace).Update(context.TODO(), cm, metav1.UpdateOptions{})
			return updateErr
		})
		if err != nil {
			return err
		}
	}
	return nil
}
