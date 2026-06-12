// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"crypto/tls"
	"fmt"

	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	goyaml "gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

const (
	RouterCertsSecretName = "router-certs-default"
	amMtlsCertPrefix      = "obs-alertmanager-mtls-cert-"
	amMtlsCAPrefix        = "obs-alertmanager-mtls-ca-"
	promNamespace         = "openshift-monitoring"
)

func GetRouterCA(cli kubernetes.Interface) ([]byte, error) {
	var caCrt []byte
	caSecret, err := cli.CoreV1().
		Secrets("openshift-ingress").
		Get(context.TODO(), RouterCertsSecretName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to get router certificate secret %s due to %v", RouterCertsSecretName, err)
		return caCrt, err
	}
	caCrt, ok := caSecret.Data["tls.crt"]
	if ok {
		return caCrt, nil
	}
	return caCrt, fmt.Errorf("failed to get tls.crt from %s secret", RouterCertsSecretName)
}

func GetHubClusterID(cli kubernetes.Interface) (string, error) {
	secret, err := cli.CoreV1().
		Secrets(MCO_NAMESPACE).
		Get(context.TODO(), operatorconfig.HubInfoSecretName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get %s secret: %w", operatorconfig.HubInfoSecretName, err)
	}
	hubInfo := &operatorconfig.HubInfo{}
	payload, ok := secret.Data[operatorconfig.HubInfoSecretKey]
	if !ok {
		return "", fmt.Errorf("key %q not found in %s", operatorconfig.HubInfoSecretKey, operatorconfig.HubInfoSecretName)
	}
	if err := goyaml.Unmarshal(payload, hubInfo); err != nil {
		return "", fmt.Errorf("failed to unmarshal hub info: %w", err)
	}
	if hubInfo.HubClusterID == "" {
		return "", fmt.Errorf("hub-cluster-id is empty in %s", operatorconfig.HubInfoSecretName)
	}
	return hubInfo.HubClusterID, nil
}

func GetObsAPIServerCA(cli kubernetes.Interface) ([]byte, error) {
	clusterID, err := GetHubClusterID(cli)
	if err != nil {
		return nil, err
	}
	secretName := amMtlsCAPrefix + clusterID
	secret, err := cli.CoreV1().
		Secrets(promNamespace).
		Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get server CA secret %s/%s: %w", promNamespace, secretName, err)
	}
	caCrt, ok := secret.Data["ca.crt"]
	if !ok {
		return nil, fmt.Errorf("ca.crt not found in %s/%s secret", promNamespace, secretName)
	}
	return caCrt, nil
}

func GetObsAPIClientCert(cli kubernetes.Interface) (tls.Certificate, error) {
	clusterID, err := GetHubClusterID(cli)
	if err != nil {
		return tls.Certificate{}, err
	}
	secretName := amMtlsCertPrefix + clusterID
	secret, err := cli.CoreV1().
		Secrets(promNamespace).
		Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to get client cert secret %s/%s: %w", promNamespace, secretName, err)
	}
	cert, err := tls.X509KeyPair(secret.Data["tls.crt"], secret.Data["tls.key"])
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to parse client cert from %s/%s: %w", promNamespace, secretName, err)
	}
	return cert, nil
}
