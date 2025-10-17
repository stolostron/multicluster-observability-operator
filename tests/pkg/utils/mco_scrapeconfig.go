// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func CreateScrapeConfig(opt TestOptions, name, componentLabel string, matchParams []string) error {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	scrapeConfig := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "monitoring.rhobs/v1alpha1",
			"kind":       "ScrapeConfig",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": MCO_NAMESPACE,
				"labels": map[string]string{
					"app.kubernetes.io/component": componentLabel,
				},
			},
			"spec": map[string]interface{}{
				"jobName":     name,
				"metricsPath": "/federate",
				"params": map[string][]string{
					"match[]": matchParams,
				},
			},
		},
	}

	_, err := clientDynamic.Resource(NewScrapeConfigGVR()).Namespace(MCO_NAMESPACE).Create(context.TODO(), scrapeConfig, metav1.CreateOptions{})
	return err
}

func DeleteScrapeConfig(opt TestOptions, name string) error {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	return clientDynamic.Resource(NewScrapeConfigGVR()).Namespace(MCO_NAMESPACE).Delete(context.TODO(), name, metav1.DeleteOptions{})
}
