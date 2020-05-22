// Copyright (c) 2020 Red Hat, Inc.

package util

import (
	"strings"

	monv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	manifests "github.com/openshift/cluster-monitoring-operator/pkg/manifests"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"
)

const (
	// ClusterNameLabelKey is the key for the injected label
	ClusterNameLabelKey = "cluster"
	clusterIDLabelKey   = "cluster_id"
	collectorType       = "OCP_PROMETHEUS"
	cmName              = "cluster-monitoring-config"
	cmNamespace         = "openshift-monitoring"
	configKey           = "config.yaml"
	labelValue          = "hub_cluster"
	protocol            = "http://"
	urlSubPath          = "/api/metrics/v1/write"
)

var log = logf.Log.WithName("util")

func getConfigMap(client kubernetes.Interface) (*v1.ConfigMap, error) {
	cm, err := client.CoreV1().ConfigMaps(cmNamespace).Get(cmName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return cm, err
}

func createRemoteWriteSpec(url string, labelConfigs *[]monv1.RelabelConfig) (*monv1.RemoteWriteSpec, error) {
	clusterID, err := GetClusterID()
	if err != nil {
		return nil, err
	}
	relabelConfig := monv1.RelabelConfig{
		SourceLabels: []string{"__name__"},
		TargetLabel:  clusterIDLabelKey,
		Replacement:  clusterID,
	}
	newlabelConfigs := append(*labelConfigs, relabelConfig)
	if !strings.HasPrefix(url, "http") {
		url = protocol + url
	}
	if !strings.HasSuffix(url, urlSubPath) {
		url = url + urlSubPath
	}

	return &monv1.RemoteWriteSpec{
		URL:                 url,
		WriteRelabelConfigs: newlabelConfigs,
	}, nil
}

func createConfigMap(client kubernetes.Interface, url string, labelConfigs *[]monv1.RelabelConfig) error {
	rwSpec, err := createRemoteWriteSpec(url, labelConfigs)
	if err != nil {
		return err
	}
	config := &manifests.Config{
		PrometheusK8sConfig: &manifests.PrometheusK8sConfig{
			RemoteWrite: []monv1.RemoteWriteSpec{
				*rwSpec,
			},
		},
	}
	configYaml, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cmNamespace,
		},
		Data: map[string]string{configKey: string(configYaml)},
	}
	_, err = client.CoreV1().ConfigMaps(cmNamespace).Create(cm)
	if err == nil {
		log.Info("Configmap created")
	}
	return err
}

func updateConfigMap(client kubernetes.Interface, configmap *v1.ConfigMap, url string, labelConfigs *[]monv1.RelabelConfig) error {
	configYaml := configmap.Data[configKey]
	config := &manifests.Config{}
	err := k8syaml.NewYAMLOrJSONDecoder(strings.NewReader(configYaml), 100).Decode(&config)
	if err != nil {
		return err
	}

	rwSpec, err := createRemoteWriteSpec(url, labelConfigs)
	if err != nil {
		return err
	}
	if config.PrometheusK8sConfig == nil {
		config.PrometheusK8sConfig = &manifests.PrometheusK8sConfig{}
	}
	if config.PrometheusK8sConfig.RemoteWrite == nil || len(config.PrometheusK8sConfig.RemoteWrite) == 0 {
		config.PrometheusK8sConfig.RemoteWrite = []monv1.RemoteWriteSpec{
			*rwSpec,
		}
	} else {
		flag := false
		for i, spec := range config.PrometheusK8sConfig.RemoteWrite {
			if strings.Contains(spec.URL, url) {
				flag = true
				config.PrometheusK8sConfig.RemoteWrite[i] = *rwSpec
				break
			}
		}
		if !flag {
			config.PrometheusK8sConfig.RemoteWrite = append(config.PrometheusK8sConfig.RemoteWrite, *rwSpec)
		}
	}
	updateConfigYaml, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	configmap.Data[configKey] = string(updateConfigYaml)
	_, err = client.CoreV1().ConfigMaps(cmNamespace).Update(configmap)
	if err == nil {
		log.Info("Configmap updated")
	}
	return err
}

// UpdateClusterMonitoringConfig is used to update cluster-monitoring-config configmap on spoke clusters
func UpdateClusterMonitoringConfig(url string, labelConfigs *[]monv1.RelabelConfig) error {
	client, err := createKubeClient()
	if err != nil {
		return err
	}
	cm, err := getConfigMap(client)
	if err != nil {
		if errors.IsNotFound(err) {
			err = createConfigMap(client, url, labelConfigs)
			return err
		}
		return err
	}
	err = updateConfigMap(client, cm, url, labelConfigs)
	return err
}

// UpdateHubClusterMonitoringConfig is used to cluster-monitoring-config configmap on hub clusters
func UpdateHubClusterMonitoringConfig(client client.Client, namespace string) (*reconcile.Result, error) {
	url, err := GetObsAPIUrl(client, namespace)
	if err != nil {
		return &reconcile.Result{}, err
	}

	labelConfigs := []monv1.RelabelConfig{
		{
			SourceLabels: []string{"__name__"},
			TargetLabel:  ClusterNameLabelKey,
			Replacement:  labelValue,
		},
	}
	return &reconcile.Result{}, UpdateClusterMonitoringConfig(url, &labelConfigs)
}
