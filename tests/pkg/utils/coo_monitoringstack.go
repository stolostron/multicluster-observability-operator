// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func NewMonitoringStackGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "monitoring.rhobs",
		Version:  "v1alpha1",
		Resource: "monitoringstacks",
	}
}

func CreateMonitoringStack(ctx context.Context, opt TestOptions, cluster Cluster, name, namespace string) error {
	clientDynamic := NewKubeClientDynamic(
		cluster.ClusterServerURL,
		opt.KubeConfig,
		cluster.KubeContext)

	ms := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "monitoring.rhobs/v1alpha1",
			"kind":       "MonitoringStack",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]any{
				"prometheusConfig": map[string]any{
					"replicas": 1,
				},
				"resources": map[string]any{
					"limits": map[string]any{
						"cpu":    "100m",
						"memory": "128Mi",
					},
					"requests": map[string]any{
						"cpu":    "50m",
						"memory": "64Mi",
					},
				},
				"resourceSelector": map[string]any{},
				"retention":        "120h",
			},
		},
	}

	_, err := clientDynamic.Resource(NewMonitoringStackGVR()).Namespace(namespace).Create(ctx, ms, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			existing, err := clientDynamic.Resource(NewMonitoringStackGVR()).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			existing.Object["spec"] = ms.Object["spec"]
			_, err = clientDynamic.Resource(NewMonitoringStackGVR()).Namespace(namespace).Update(ctx, existing, metav1.UpdateOptions{})
			return err
		}
	}
	return err
}

func DeleteMonitoringStack(ctx context.Context, opt TestOptions, cluster Cluster, name, namespace string) error {
	clientDynamic := NewKubeClientDynamic(
		cluster.ClusterServerURL,
		opt.KubeConfig,
		cluster.KubeContext)

	err := clientDynamic.Resource(NewMonitoringStackGVR()).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && errors.IsNotFound(err) {
		return nil
	}
	return err
}
