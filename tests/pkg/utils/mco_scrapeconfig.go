// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func CreateScrapeConfig(opt TestOptions, name, componentLabel string, matchParams []string) error {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	scrapeConfig := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "monitoring.rhobs/v1alpha1",
			"kind":       "ScrapeConfig",
			"metadata": map[string]any{
				"name":      name,
				"namespace": MCO_NAMESPACE,
				"labels": map[string]string{
					"app.kubernetes.io/component": componentLabel,
				},
			},
			"spec": map[string]any{
				"jobName":     name,
				"metricsPath": "/federate",
				"params": map[string][]string{
					"match[]": matchParams,
				},
			},
		},
	}

	_, err := clientDynamic.Resource(NewScrapeConfigGVR()).Namespace(MCO_NAMESPACE).Create(context.TODO(), scrapeConfig, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			existing, err := clientDynamic.Resource(NewScrapeConfigGVR()).Namespace(MCO_NAMESPACE).Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			scrapeConfig.SetResourceVersion(existing.GetResourceVersion())
			_, err = clientDynamic.Resource(NewScrapeConfigGVR()).Namespace(MCO_NAMESPACE).Update(context.TODO(), scrapeConfig, metav1.UpdateOptions{})
			return err
		}
	}
	return err
}

func DeleteScrapeConfig(opt TestOptions, name string) error {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	return clientDynamic.Resource(NewScrapeConfigGVR()).Namespace(MCO_NAMESPACE).Delete(context.TODO(), name, metav1.DeleteOptions{})
}
