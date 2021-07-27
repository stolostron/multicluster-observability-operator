// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"net/url"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	operatorconfig "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/config"
)

func newHubInfoSecret(client client.Client, obsNamespace string,
	namespace string, mco *mcov1beta2.MultiClusterObservability) (*corev1.Secret, error) {
	obsApiRouteHost, err := config.GetObsAPIHost(client, obsNamespace)
	if err != nil {
		log.Error(err, "Failed to get the host for observatorium API route")
		return nil, err
	}
	obsApiURL := url.URL{
		Host: obsApiRouteHost,
		Path: operatorconfig.ObservatoriumAPIRemoteWritePath,
	}
	if !obsApiURL.IsAbs() {
		obsApiURL.Scheme = "https"
	}
	alertmanagerEndpoint, err := config.GetAlertmanagerEndpoint(client, obsNamespace)
	if err != nil {
		log.Error(err, "Failed to get alertmanager endpoint")
		return nil, err
	}
	alertmanagerRouterCA, err := config.GetAlertmanagerRouterCA(client)
	if err != nil {
		log.Error(err, "Failed to CA of openshift Route")
		return nil, err
	}
	hubInfo := &operatorconfig.HubInfo{
		ObservatoriumAPIEndpoint: obsApiURL.String(),
		AlertmanagerEndpoint:     alertmanagerEndpoint,
		AlertmanagerRouterCA:     alertmanagerRouterCA,
	}
	configYaml, err := yaml.Marshal(hubInfo)
	if err != nil {
		return nil, err
	}
	configYamlMap := map[string][]byte{}
	configYamlMap[operatorconfig.HubInfoSecretKey] = configYaml
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconfig.HubInfoSecretName,
			Namespace: namespace,
		},
		Data: configYamlMap,
	}, nil
}
