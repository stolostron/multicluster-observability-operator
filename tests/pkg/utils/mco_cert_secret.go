// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

const (
	ServerCACerts = "observability-server-ca-certs"
	ClientCACerts = "observability-client-ca-certs"
	ServerCerts   = "observability-server-certs"
	GrafanaCerts  = "observability-grafana-certs"
)

func DeleteCertSecret(opt TestOptions) error {
	clientKube := NewKubeClient(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	klog.V(1).Infof("Delete certificate secret")
	propagationPolicy := metav1.DeletePropagationForeground
	err := clientKube.CoreV1().Secrets(MCO_NAMESPACE).Delete(context.TODO(), ServerCACerts, metav1.DeleteOptions{PropagationPolicy: &propagationPolicy})
	if err != nil {
		klog.Errorf("Failed to delete certificate secret %s due to %v", ServerCACerts, err)
		return err
	}

	err = clientKube.CoreV1().Secrets(MCO_NAMESPACE).Delete(context.TODO(), ClientCACerts, metav1.DeleteOptions{PropagationPolicy: &propagationPolicy})
	if err != nil {
		klog.Errorf("Failed to delete certificate secret %s due to %v", ClientCACerts, err)
		return err
	}

	err = clientKube.CoreV1().Secrets(MCO_NAMESPACE).Delete(context.TODO(), ServerCerts, metav1.DeleteOptions{PropagationPolicy: &propagationPolicy})
	if err != nil {
		klog.Errorf("Failed to delete certificate secret %s due to %v", ServerCerts, err)
		return err
	}

	err = clientKube.CoreV1().Secrets(MCO_NAMESPACE).Delete(context.TODO(), GrafanaCerts, metav1.DeleteOptions{PropagationPolicy: &propagationPolicy})
	if err != nil {
		klog.Errorf("Failed to delete certificate secret %s due to %v", GrafanaCerts, err)
		return err
	}
	return err
}
