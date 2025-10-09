// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	cooSubscriptionName      = "cluster-observability-operator"
	cooSubscriptionNamespace = "openshift-cluster-observability-operator"
	cooDeploymentName        = "obo-prometheus-operator"
)

func NewSubscriptionGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1alpha1",
		Resource: "subscriptions",
	}
}

func CreateCOOSubscription(opt TestOptions) error {
	clusters := append([]Cluster{opt.HubCluster}, opt.ManagedClusters...)
	for _, cluster := range clusters {
		clientDynamic := NewKubeClientDynamic(
			cluster.ClusterServerURL,
			opt.KubeConfig,
			cluster.KubeContext)

		subUnstructured := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "operators.coreos.com/v1alpha1",
				"kind":       "Subscription",
				"metadata": map[string]interface{}{
					"name":      cooSubscriptionName,
					"namespace": cooSubscriptionNamespace,
				},
				"spec": map[string]interface{}{
					"channel":             "stable",
					"installPlanApproval": "Automatic",
					"name":                cooSubscriptionName,
					"source":              "redhat-operators",
					"sourceNamespace":     "openshift-marketplace",
				},
			},
		}

		_, err := clientDynamic.Resource(NewSubscriptionGVR()).Namespace(cooSubscriptionNamespace).Create(context.TODO(), subUnstructured, metav1.CreateOptions{})
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create COO subscription on cluster: %w", err)
		}
	}
	return nil
}

func DeleteCOOSubscription(opt TestOptions) error {
	clusters := append([]Cluster{opt.HubCluster}, opt.ManagedClusters...)
	for _, cluster := range clusters {
		clientDynamic := NewKubeClientDynamic(
			cluster.ClusterServerURL,
			opt.KubeConfig,
			cluster.KubeContext)

		err := clientDynamic.Resource(NewSubscriptionGVR()).Namespace(cooSubscriptionNamespace).Delete(context.TODO(), cooSubscriptionName, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete COO subscription on cluster: %w", err)
		}
	}
	return nil
}

func CheckCOODeployment(opt TestOptions) error {
	clusters := append([]Cluster{opt.HubCluster}, opt.ManagedClusters...)
	for _, cluster := range clusters {
		CheckDeploymentAvailability(cluster, cooDeploymentName, cooSubscriptionNamespace, true)
	}
	return nil
}
