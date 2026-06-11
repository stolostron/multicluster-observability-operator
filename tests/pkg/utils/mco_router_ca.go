// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

const (
	RouterCertsSecretName = "router-certs-default"
	DefaultIngressCertName = "default-ingress-cert"
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
