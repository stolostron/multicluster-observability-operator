// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"strings"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
)

const (
	hubInfoName = "hub-info-secret"
	hubInfoKey  = "hub-info.yaml"
	urlSubPath  = "/api/metrics/v1/default/api/v1/receive"
	protocol    = "https://"
)

// HubInfo is the struct that contains the observatorium api gateway URL, hub alertmanager URl and hub router CA
type HubInfo struct {
	ClusterName              string `yaml:"cluster-name"`
	Endpoint                 string `yaml:"endpoint"` // deprecated, keep it for back compatibility, use 'ObservatoriumAPIEndpoint' to replace 'Endpoint'
	ObservatoriumAPIEndpoint string `yaml:"observatorium-api-endpoint"`
	AlertmanagerEndpoint     string `yaml:"alertmanager-endpoint"`
	AlertmanagerRouterCA     string `yaml:"alertmanager-router-ca"`
}

func newHubInfoSecret(client client.Client, obsNamespace string,
	namespace string, mco *mcov1beta2.MultiClusterObservability) (*corev1.Secret, error) {
	obsApiEp, err := config.GetObsAPIUrl(client, obsNamespace)
	if err != nil {
		log.Error(err, "Failed to get api gateway")
		return nil, err
	}
	if !strings.HasPrefix(obsApiEp, "http") {
		obsApiEp = protocol + obsApiEp
	}
	alertmanagerEndpoint, err := config.GetAlertmanagerEndpoint(client, obsNamespace)
	if err != nil {
		log.Error(err, "Failed to get alertmanager endpoint")
		return nil, err
	}
	if !strings.HasPrefix(alertmanagerEndpoint, "http") {
		alertmanagerEndpoint = protocol + alertmanagerEndpoint
	}
	alertmanagerRouterCA, err := config.GetAlertmanagerRouterCA(client)
	if err != nil {
		log.Error(err, "Failed to CA of openshift Route")
		return nil, err
	}
	hubInfo := &HubInfo{
		Endpoint:                 obsApiEp + urlSubPath,
		ObservatoriumAPIEndpoint: obsApiEp + urlSubPath,
		AlertmanagerEndpoint:     alertmanagerEndpoint,
		AlertmanagerRouterCA:     alertmanagerRouterCA,
	}
	configYaml, err := yaml.Marshal(hubInfo)
	if err != nil {
		return nil, err
	}
	configYamlMap := map[string][]byte{}
	configYamlMap[hubInfoKey] = configYaml
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      hubInfoName,
			Namespace: namespace,
		},
		Data: configYamlMap,
	}, nil
}
