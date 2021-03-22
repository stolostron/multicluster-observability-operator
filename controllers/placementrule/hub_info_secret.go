// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"strings"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta2"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
)

const (
	hubInfoName = "hub-info-secret"
	hubInfoKey  = "hub-info.yaml"
	urlSubPath  = "/api/metrics/v1/default/api/v1/receive"
	protocol    = "https://"
)

// HubInfo is the struct for hub info
type HubInfo struct {
	ClusterName string `yaml:"cluster-name"`
	Endpoint    string `yaml:"endpoint"`
}

func newHubInfoSecret(client client.Client, obsNamespace string,
	namespace string, clusterName string, mco *mcov1beta2.MultiClusterObservability) (*corev1.Secret, error) {
	url, err := config.GetObsAPIUrl(client, obsNamespace)
	if err != nil {
		log.Error(err, "Failed to get api gateway")
		return nil, err
	}
	if !strings.HasPrefix(url, "http") {
		url = protocol + url
	}
	hubInfo := &HubInfo{
		ClusterName: clusterName,
		Endpoint:    url + urlSubPath,
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
