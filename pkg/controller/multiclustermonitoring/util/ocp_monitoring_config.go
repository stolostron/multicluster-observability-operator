package util

import (
	"encoding/json"
	monv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	manifests "github.com/openshift/cluster-monitoring-operator/pkg/manifests"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateConfigMap(url string) (*v1.ConfigMap, error) {
	config := &manifests.Config{
		PrometheusK8sConfig: &manifests.PrometheusK8sConfig{
			RemoteWrite: []monv1.RemoteWriteSpec{
				{
					URL: "http://" + url + "/api/metrics/v1/write",
					WriteRelabelConfigs: []monv1.RelabelConfig{
						{
							SourceLabels: []string{"__name__"},
							TargetLabel:  "cluster_name",
							Replacement:  "hub_cluster",
						},
					},
				},
			},
		},
	}
	configJson, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-monitoring-config",
			Namespace: "openshift-monitoring",
		},
		Data: map[string]string{"config.yaml": string(configJson)},
	}, nil
}
