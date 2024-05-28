// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"context"
	"net/url"
	"strings"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
)

// generateHubInfoSecret generates the secret that contains hubInfo.
// this function should only called when the watched resources are created/updated.
func generateHubInfoSecret(client client.Client, obsNamespace string,
	namespace string, ingressCtlCrdExists bool) (*corev1.Secret, error) {

	obsAPIHost := ""
	alertmanagerEndpoint := ""
	alertmanagerRouterCA := ""

	if ingressCtlCrdExists {
		var err error
		obsAPIHost, err = config.GetObsAPIExternalHost(context.TODO(), client, obsNamespace)
		if err != nil {
			log.Error(err, "Failed to get the host for Observatorium API host URL")
			return nil, err
		}

		// if alerting is disabled, do not set alertmanagerEndpoint
		if !config.IsAlertingDisabled() {
			alertmanagerEndpoint, err = config.GetAlertmanagerEndpoint(context.TODO(), client, obsNamespace)
			if !strings.HasPrefix(alertmanagerEndpoint, "https://") {
				alertmanagerEndpoint = "https://" + alertmanagerEndpoint
			}

			if err != nil {
				log.Error(err, "Failed to get alertmanager endpoint")
				return nil, err
			}
		}

		alertmanagerRouterCA, err = config.GetAlertmanagerRouterCA(client)
		if err != nil {
			log.Error(err, "Failed to CA of openshift Route")
			return nil, err
		}
	} else {
		// for KinD support, the managedcluster and hub cluster are assumed in the same cluster, the observatorium-api
		// will be accessed through k8s service FQDN + port
		obsAPIHost = config.GetOperandNamePrefix() + "observatorium-api" + "." + config.GetDefaultNamespace() + ".svc.cluster.local:8080"
		// if alerting is disabled, do not set alertmanagerEndpoint
		if !config.IsAlertingDisabled() {
			alertmanagerEndpoint = config.AlertmanagerServiceName + "." + config.GetDefaultNamespace() + ".svc.cluster.local:9095"
		}
		var err error
		alertmanagerRouterCA, err = config.GetAlertmanagerCA(client)
		if err != nil {
			log.Error(err, "Failed to CA of the Alertmanager")
			return nil, err
		}
	}

	if !strings.HasSuffix(obsAPIHost, operatorconfig.ObservatoriumAPIRemoteWritePath) {
		obsAPIHost += operatorconfig.ObservatoriumAPIRemoteWritePath
	}

	obsApiURL, err := url.Parse(obsAPIHost)
	if err != nil {
		return nil, err
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
