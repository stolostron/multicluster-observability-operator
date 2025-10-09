// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func NewPrometheusAgentGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "monitoring.rhobs",
		Version:  "v1alpha1",
		Resource: "prometheusagents",
	}
}

func UpdatePlatformPrometheusAgent(opt TestOptions, interval string) error {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	listOpt := metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/component=platform-metrics-collector",
	}
	paList, err := clientDynamic.Resource(NewPrometheusAgentGVR()).Namespace(MCO_NAMESPACE).List(context.TODO(), listOpt)
	if err != nil {
		return err
	}

	if len(paList.Items) != 1 {
		return fmt.Errorf("expected 1 PrometheusAgent with label app.kubernetes.io/component=platform-metrics-collector, but found %d", len(paList.Items))
	}

	pa := paList.Items[0]
	if err := unstructured.SetNestedField(pa.Object, interval, "spec", "scrapeInterval"); err != nil {
		return fmt.Errorf("failed to set scrapeInterval on PrometheusAgent: %w", err)
	}

	_, err = clientDynamic.Resource(NewPrometheusAgentGVR()).Namespace(MCO_NAMESPACE).Update(context.TODO(), &pa, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update PrometheusAgent %s: %w", pa.GetName(), err)
	}

	return nil
}
