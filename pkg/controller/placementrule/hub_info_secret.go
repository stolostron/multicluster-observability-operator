// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"context"
	"strings"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
)

const (
	hubInfoName = "hub-info-secret"
	hubInfoKey  = "hub-info.yaml"
	urlSubPath  = "/api/v1/receive"
	protocol    = "http://"
)

// HubInfo is the struct for hub info
type HubInfo struct {
	ClusterName string `yaml:"cluster-name"`
	Endpoint    string `yaml:"endpoint"`
}

func newHubInfoSecret(client client.Client, obsNamespace string,
	namespace string, clusterName string) (*corev1.Secret, error) {
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

func createHubInfoSecret(client client.Client, obsNamespace string, namespace string, clusterName string) error {

	found := &corev1.Secret{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: hubInfoName, Namespace: namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating hubinfo secret", "namespace", namespace)
		hubInfoSec, err := newHubInfoSecret(client, obsNamespace, namespace, clusterName)
		if err != nil {
			return err
		}
		err = client.Create(context.TODO(), hubInfoSec)
		if err != nil {
			log.Error(err, "Failed to create hubInfo secret")
		}
		return err
	} else if err != nil {
		log.Error(err, "Failed to check hubinfo secret")
		return err
	}
	log.Info("hubinfo secret already existed", "namespace", namespace)
	return nil
}
