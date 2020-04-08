package util

import (
	monv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	manifests "github.com/openshift/cluster-monitoring-operator/pkg/manifests"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"strings"
)

var log = logf.Log.WithName("ocp_monitoring_config")

const (
	Name       = "cluster-monitoring-config"
	Namespace  = "openshift-monitoring"
	configKey  = "config.yaml"
	labelKey   = "cluster_name"
	labelValue = "hub_cluster"
	protocol   = "http://"
	urlSubPath = "/api/metrics/v1/write"
)

func CreateConfigMap(url string) (*v1.ConfigMap, error) {
	config := &manifests.Config{
		PrometheusK8sConfig: &manifests.PrometheusK8sConfig{
			RemoteWrite: []monv1.RemoteWriteSpec{
				*createRemoteWriteSpec(url),
			},
		},
	}
	configYaml, err := yaml.Marshal(config)
	if err != nil {
		return nil, err
	}
	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      Name,
			Namespace: Namespace,
		},
		Data: map[string]string{configKey: string(configYaml)},
	}, nil
}

func UpdateConfigMap(configmap *v1.ConfigMap, url string) error {
	configYaml := configmap.Data[configKey]
	config := &manifests.Config{}
	err := k8syaml.NewYAMLOrJSONDecoder(strings.NewReader(configYaml), 100).Decode(&config)
	if err != nil {
		return err
	}
	if config.PrometheusK8sConfig == nil {
		config.PrometheusK8sConfig = &manifests.PrometheusK8sConfig{}
	}
	if config.PrometheusK8sConfig.RemoteWrite == nil || len(config.PrometheusK8sConfig.RemoteWrite) == 0 {
		config.PrometheusK8sConfig.RemoteWrite = []monv1.RemoteWriteSpec{
			*createRemoteWriteSpec(url),
		}
	} else {
		flag := false
		for i, spec := range config.PrometheusK8sConfig.RemoteWrite {
			if strings.Contains(spec.URL, url) {
				flag = true
				config.PrometheusK8sConfig.RemoteWrite[i] = *createRemoteWriteSpec(url)
				break
			}
		}
		if !flag {
			config.PrometheusK8sConfig.RemoteWrite = append(config.PrometheusK8sConfig.RemoteWrite, *createRemoteWriteSpec(url))
		}
	}
	updateConfigYaml, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	configmap.Data[configKey] = string(updateConfigYaml)
	return nil
}

func createRemoteWriteSpec(url string) *monv1.RemoteWriteSpec {
	return &monv1.RemoteWriteSpec{
		URL: protocol + url + urlSubPath,
		WriteRelabelConfigs: []monv1.RelabelConfig{
			{
				SourceLabels: []string{"__name__"},
				TargetLabel:  labelKey,
				Replacement:  labelValue,
			},
		},
	}
}
