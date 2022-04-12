// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"context"

	"github.com/stolostron/multicluster-observability-operator/tests/pkg/kustomize"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

const (
	allowlistCMname = "observability-metrics-custom-allowlist"
	endpointSName   = "victoriametrics"
	deploymentName  = "victoriametrics"
	svcName         = "victoriametrics"
)

func CleanExportResources(opt TestOptions) error {
	hubClient := NewKubeClient(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	v1beta2KustomizationPath := "../../../examples/mco/e2e/v1beta2"
	yamlB, err := kustomize.Render(kustomize.Options{KustomizationPath: v1beta2KustomizationPath})
	if err != nil {
		return err
	}
	err = Apply(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext,
		yamlB,
	)
	if err != nil {
		return err
	}

	err = hubClient.CoreV1().ConfigMaps(MCO_NAMESPACE).
		Delete(context.TODO(), allowlistCMname, metav1.DeleteOptions{})
	if err != nil && errors.IsNotFound(err) {
		return err
	}

	err = hubClient.CoreV1().Secrets(MCO_NAMESPACE).
		Delete(context.TODO(), endpointSName, metav1.DeleteOptions{})
	if err != nil && errors.IsNotFound(err) {
		return err
	}

	err = hubClient.AppsV1().Deployments(MCO_NAMESPACE).
		Delete(context.TODO(), deploymentName, metav1.DeleteOptions{})
	if err != nil && errors.IsNotFound(err) {
		return err
	}

	err = hubClient.CoreV1().Services(MCO_NAMESPACE).
		Delete(context.TODO(), svcName, metav1.DeleteOptions{})
	if err != nil && errors.IsNotFound(err) {
		return err
	}

	klog.V(1).Infof("Clean up/reset all export related resources")
	return nil
}
