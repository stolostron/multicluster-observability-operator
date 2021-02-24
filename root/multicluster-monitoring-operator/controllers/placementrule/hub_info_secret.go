// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"context"
	"strings"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workv1 "github.com/open-cluster-management/api/work/v1"
	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/api/v1beta1"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/util"
)

const (
	hubInfoName = "hub-info-secret"
	hubInfoKey  = "hub-info.yaml"
	urlSubPath  = "/api/metrics/v1/default/api/v1/receive"
	protocol    = "https://"
)

// HubInfo is the struct for hub info
type HubInfo struct {
	ClusterName   string `yaml:"cluster-name"`
	Endpoint      string `yaml:"endpoint"`
	EnableMetrics bool   `yaml:"enable-metrics"`
	Interval      int32  `yaml:"internal"`
	DeleteFlag    bool   `yaml:"delete-flag"`
}

func newHubInfoSecret(client client.Client, obsNamespace string,
	namespace string, clusterName string, mco *mcov1beta1.MultiClusterObservability) (*corev1.Secret, error) {
	url, err := config.GetObsAPIUrl(client, obsNamespace)
	if err != nil {
		log.Error(err, "Failed to get api gateway")
		return nil, err
	}
	if !strings.HasPrefix(url, "http") {
		url = protocol + url
	}
	hubInfo := &HubInfo{
		ClusterName:   clusterName,
		Endpoint:      url + urlSubPath,
		EnableMetrics: mco.Spec.ObservabilityAddonSpec.EnableMetrics,
		Interval:      mco.Spec.ObservabilityAddonSpec.Interval,
		DeleteFlag:    false,
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

func updateDeleteFlag(client client.Client, namespace string) error {
	found := &workv1.ManifestWork{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: workName, Namespace: namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		log.Error(err, "Failed to check manifestwork", "namespace", namespace, "name", workName)
		return err
	}

	hubInfoObj, err := util.GetObject(found.Spec.Workload.Manifests[0].RawExtension)
	if err != nil {
		log.Error(err, "Failed to get hubInfo secret from manifestwork")
		return err
	}
	hubInfo := hubInfoObj.(*corev1.Secret)
	hubYaml := &HubInfo{}
	err = yaml.Unmarshal(hubInfo.Data[hubInfoKey], &hubYaml)
	if err != nil {
		log.Error(err, "Failed to unmarshall hubInfo")
		return err
	}
	hubYaml.DeleteFlag = true
	updateHubYaml, err := yaml.Marshal(hubYaml)
	if err != nil {
		log.Error(err, "Failed to marshall hubInfo")
		return err
	}
	hubInfo.Data[hubInfoKey] = updateHubYaml

	found.Spec.Workload.Manifests[0] = workv1.Manifest{
		RawExtension: runtime.RawExtension{Object: hubInfo},
	}

	err = client.Update(context.TODO(), found)
	if err != nil {
		log.Error(err, "Failed to update manifestwork", "namespace", namespace, "name", workName)
		return err
	}
	return nil
}
