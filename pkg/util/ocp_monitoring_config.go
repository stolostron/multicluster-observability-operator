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
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"
)

var log = logf.Log.WithName("ocp_monitoring_config")

const (
	Name       = "cluster-monitoring-config"
	Namespace  = "openshift-monitoring"
	configKey  = "config.yaml"
	labelKey   = "cluster"
	labelValue = "hub_cluster"
	protocol   = "http://"
	urlSubPath = "/api/metrics/v1/write"
)

func getConfigMap(client kubernetes.Interface) (*v1.ConfigMap, error) {
	cm, err := client.CoreV1().ConfigMaps(Namespace).Get(Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	} else {
		return cm, err
	}
}

func createRemoteWriteSpec(url string, labelConfigs *[]monv1.RelabelConfig) *monv1.RemoteWriteSpec {
	if !strings.HasPrefix(url, "http") {
		url = protocol + url
	}
	if !strings.HasSuffix(url, urlSubPath) {
		url = url + urlSubPath
	}
	return &monv1.RemoteWriteSpec{
		URL:                 url,
		WriteRelabelConfigs: *labelConfigs,
	}
}

func createConfigMap(client kubernetes.Interface, url string, labelConfigs *[]monv1.RelabelConfig) error {
	config := &manifests.Config{
		PrometheusK8sConfig: &manifests.PrometheusK8sConfig{
			RemoteWrite: []monv1.RemoteWriteSpec{
				*createRemoteWriteSpec(url, labelConfigs),
			},
		},
	}
	configYaml, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      Name,
			Namespace: Namespace,
		},
		Data: map[string]string{configKey: string(configYaml)},
	}
	_, err = client.CoreV1().ConfigMaps(Namespace).Create(cm)
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
	if config.PrometheusK8sConfig == nil {
		config.PrometheusK8sConfig = &manifests.PrometheusK8sConfig{}
	}
	if config.PrometheusK8sConfig.RemoteWrite == nil || len(config.PrometheusK8sConfig.RemoteWrite) == 0 {
		config.PrometheusK8sConfig.RemoteWrite = []monv1.RemoteWriteSpec{
			*createRemoteWriteSpec(url, labelConfigs),
		}
	} else {
		flag := false
		for i, spec := range config.PrometheusK8sConfig.RemoteWrite {
			if strings.Contains(spec.URL, url) {
				flag = true
				config.PrometheusK8sConfig.RemoteWrite[i] = *createRemoteWriteSpec(url, labelConfigs)
				break
			}
		}
		if !flag {
			config.PrometheusK8sConfig.RemoteWrite = append(config.PrometheusK8sConfig.RemoteWrite, *createRemoteWriteSpec(url, labelConfigs))
		}
	}
	updateConfigYaml, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	configmap.Data[configKey] = string(updateConfigYaml)
	_, err = client.CoreV1().ConfigMaps(Namespace).Update(configmap)
	if err == nil {
		log.Info("Configmap updated")
	}
	return err
}

func UpdateClusterMonitoringConfig(url string, labelConfigs *[]monv1.RelabelConfig) error {
	client, err := CreateKubeClient()
	if err != nil {
		return err
	}
	cm, err := getConfigMap(client)
	if err != nil {
		if errors.IsNotFound(err) {
			err = createConfigMap(client, url, labelConfigs)
			return err
		} else {
			return err
		}
	} else {
		err = updateConfigMap(client, cm, url, labelConfigs)
		return err
	}
}

func UpdateHubClusterMonitoringConfig(url string) error {
	labelConfigs := []monv1.RelabelConfig{
		{
			SourceLabels: []string{"__name__"},
			TargetLabel:  labelKey,
			Replacement:  labelValue,
		},
	}
	return UpdateClusterMonitoringConfig(url, &labelConfigs)
}
