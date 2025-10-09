// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"

	cooprometheusv1alpha1 "github.com/rhobs/obo-prometheus-operator/pkg/apis/monitoring/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
)

const (
	platformMetricsCollectorLabel = "platform-metrics-collector"
)

func CreateScrapeConfig(opt TestOptions, name, componentLabel string, matchParams []string) error {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	scrapeConfigTyped := &cooprometheusv1alpha1.ScrapeConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: cooprometheusv1alpha1.SchemeGroupVersion.String(),
			Kind:       cooprometheusv1alpha1.ScrapeConfigsKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: MCO_NAMESPACE,
			Labels: map[string]string{
				"app.kubernetes.io/component": componentLabel,
			},
		},
		Spec: cooprometheusv1alpha1.ScrapeConfigSpec{
			JobName:     ptr.To(name),
			MetricsPath: ptr.To("/federate"),
			Params: map[string][]string{
				"match[]": matchParams,
			},
		},
	}

	scrapeConfigMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(scrapeConfigTyped)
	if err != nil {
		return err
	}

	scrapeConfig := &unstructured.Unstructured{Object: scrapeConfigMap}

	_, err = clientDynamic.Resource(NewScrapeConfigGVR()).Namespace(MCO_NAMESPACE).Create(context.TODO(), scrapeConfig, metav1.CreateOptions{})
	return err
}

func DeleteScrapeConfig(opt TestOptions, name string) error {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	return clientDynamic.Resource(NewScrapeConfigGVR()).Namespace(MCO_NAMESPACE).Delete(context.TODO(), name, metav1.DeleteOptions{})
}
