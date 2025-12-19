// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"context"
	"fmt"
	"net/url"
	"os"
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
	namespace string, crdMap map[string]bool, isUWMAlertingDisabled bool) (*corev1.Secret, error) {

	obsAPIHost := ""
	alertmanagerEndpoint := ""
	alertmanagerRouterCA := ""

	if crdMap[config.IngressControllerCRD] {
		var err error
		obsAPIURL, err := config.GetObsAPIExternalURL(context.TODO(), client, obsNamespace)
		if err != nil {
			log.Error(err, "Failed to get the host for Observatorium API host URL")
			return nil, err
		}
		obsAPIHost = obsAPIURL.Host

		// if alerting is disabled, do not set alertmanagerEndpoint
		if !config.IsAlertingDisabled() {
			alertmanagerURL, err := config.GetAlertmanagerURL(context.TODO(), client, obsNamespace)
			if err != nil {
				log.Error(err, "Failed to get alertmanager endpoint")
				return nil, err
			}
			alertmanagerEndpoint = alertmanagerURL.String()
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

	// Due to ambiguities in URL parsing when the scheme is not present, we prepend it here.
	if !strings.HasPrefix(obsAPIHost, "https://") {
		obsAPIHost = "https://" + obsAPIHost
	}

	obsApiURL, err := url.Parse(obsAPIHost)
	if err != nil {
		return nil, err
	}

	// We have to *append* the Remote Write path, and not hardcode it, because it could include
	// a custom sub-path required by intermediate components running between spokes and the hub (i.e. reverse proxies
	// or load balancers).
	if !strings.HasSuffix(obsApiURL.Path, operatorconfig.ObservatoriumAPIRemoteWritePath) {
		obsApiURL = obsApiURL.JoinPath(operatorconfig.ObservatoriumAPIRemoteWritePath)
	}

	// get the trimmed cluster id for the cluster
	trimmedClusterID := ""
	if os.Getenv("UNIT_TEST") != "true" {
		trimmedClusterID, err = config.GetTrimmedClusterID(client)
		if err != nil {
			// TODO: include better info
			return nil, fmt.Errorf("Unable to get hub ClusterID for hub-info-secret: %w", err)
		}
	} else {
		// there is no clusterID to get in unit tests.
		trimmedClusterID = "1a9af6dc0801433cb28a200af81"
	}

	hubInfo := &operatorconfig.HubInfo{
		ObservatoriumAPIEndpoint: obsApiURL.String(),
		AlertmanagerEndpoint:     alertmanagerEndpoint,
		AlertmanagerRouterCA:     alertmanagerRouterCA,
		UWMAlertingDisabled:      isUWMAlertingDisabled,
		HubClusterID:             trimmedClusterID,
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
