// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"net/url"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	operatorconfig "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/config"
)

// generateHubInfoSecret generates the secret that contains hubInfo.
// this function should only called when the watched resources are created/updated
func generateHubInfoSecret(client client.Client, obsNamespace string,
	namespace string, ingressCtlCrdExists bool) (*corev1.Secret, error) {

	obsApiRouteHost := ""
	alertmanagerEndpoint := ""
	alertmanagerRouterCA := ""

	if ingressCtlCrdExists {
		var err error
		obsApiRouteHost, err = config.GetObsAPIHost(client, obsNamespace)
		if err != nil {
			log.Error(err, "Failed to get the host for observatorium API route")
			return nil, err
		}

		alertmanagerEndpoint, err = config.GetAlertmanagerEndpoint(client, obsNamespace)
		if err != nil {
			log.Error(err, "Failed to get alertmanager endpoint")
			return nil, err
		}

		alertmanagerRouterCA, err = config.GetAlertmanagerRouterCA(client)
		if err != nil {
			log.Error(err, "Failed to CA of openshift Route")
			return nil, err
		}
	} else {
		// for KinD support, the managedcluster and hub cluster are assumed in the same cluster, the observatorium-api will be accessed through k8s service FQDN + port
		obsApiRouteHost = config.GetOperandNamePrefix() + "observatorium-api" + "." + config.GetDefaultNamespace() + ".svc.cluster.local:8080"
		alertmanagerEndpoint = config.AlertmanagerServiceName + "." + config.GetDefaultNamespace() + ".svc.cluster.local:9095"
		var err error
		alertmanagerRouterCA, err = config.GetAlertmanagerCA(client)
		if err != nil {
			log.Error(err, "Failed to CA of the Alertmanager")
			return nil, err
		}
	}

	obsApiURL := url.URL{
		Host: obsApiRouteHost,
		Path: operatorconfig.ObservatoriumAPIRemoteWritePath,
	}
	if !obsApiURL.IsAbs() {
		obsApiURL.Scheme = "https"
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
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconfig.HubInfoSecretName,
			Namespace: namespace,
		},
		Data: configYamlMap,
	}, nil
}
