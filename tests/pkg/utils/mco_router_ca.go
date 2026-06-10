// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"crypto/tls"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

const (
	RouterCertsSecretName   = "router-certs-default"
	DefaultIngressCertName  = "default-ingress-cert"
	IngressSecretsNamespace = "openshift-ingress"
)

func GetRouterCA(cli kubernetes.Interface) ([]byte, error) {
	caSecret, err := cli.CoreV1().
		Secrets(IngressSecretsNamespace).
		Get(context.TODO(), RouterCertsSecretName, metav1.GetOptions{})
	if err == nil {
		if caCrt, ok := caSecret.Data["tls.crt"]; ok {
			return caCrt, nil
		}
	}
	klog.V(1).Infof("%s not found in %s, trying %s", RouterCertsSecretName, IngressSecretsNamespace, DefaultIngressCertName)

	caSecret, err = cli.CoreV1().
		Secrets(IngressSecretsNamespace).
		Get(context.TODO(), DefaultIngressCertName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ingress CA from both %s and %s in %s: %w",
			RouterCertsSecretName, DefaultIngressCertName, IngressSecretsNamespace, err)
	}
	if caCrt, ok := caSecret.Data["tls.crt"]; ok {
		return caCrt, nil
	}
	return nil, fmt.Errorf("tls.crt not found in %s secret", DefaultIngressCertName)
}

func GetObsAPIServerCA(cli kubernetes.Interface) ([]byte, error) {
	secret, err := cli.CoreV1().
		Secrets(MCO_NAMESPACE).
		Get(context.TODO(), ServerCACerts, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get observability server CA secret %s: %w", ServerCACerts, err)
	}
	caCrt, ok := secret.Data["tls.crt"]
	if !ok {
		return nil, fmt.Errorf("tls.crt not found in %s secret", ServerCACerts)
	}
	return caCrt, nil
}

func GetObsAPIClientCert(cli kubernetes.Interface) (tls.Certificate, error) {
	secret, err := cli.CoreV1().
		Secrets(MCO_NAMESPACE).
		Get(context.TODO(), GrafanaCerts, metav1.GetOptions{})
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to get client cert secret %s: %w", GrafanaCerts, err)
	}
	cert, err := tls.X509KeyPair(secret.Data["tls.crt"], secret.Data["tls.key"])
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to parse client cert from %s: %w", GrafanaCerts, err)
	}
	return cert, nil
}
