// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
)

const (
	hubInfoName = "hub-info-secret"
	hubInfoKey  = "hubInfoKey"
	urlSubPath  = "/api/v1/receive"
)

// HubInfo is the struct for hub info
type HubInfo struct {
	ClusterName string `yaml:"cluster-name"`
	Endpoint    string `yaml:"endpoint"`
}

func createHubInfoSecret(client client.Client, obsNamespace string, namespace string, clusterName string) error {
	url, err := config.GetObsAPIUrl(client, obsNamespace)
	if err != nil {
		return err
	}
	hubInfo := &HubInfo{
		ClusterName: clusterName,
		Endpoint:    url + urlSubPath,
	}
	configYaml, err := yaml.Marshal(hubInfo)
	if err != nil {
		return err
	}
	hubInfoSec := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      hubInfoName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			hubInfoKey: configYaml,
		},
	}

	err = client.Create(context.TODO(), hubInfoSec)
	if err != nil {
		log.Error(err, "Failed to create hubInfo secret")
	}
	return err
}
