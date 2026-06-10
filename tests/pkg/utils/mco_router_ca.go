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
