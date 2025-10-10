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

func GetOCPClusters(opt TestOptions) ([]Cluster, error) {
	availableManagedClusters, err := GetAvailableManagedClusters(opt)
	if err != nil {
		return nil, fmt.Errorf("failed to get available managed clusters: %w", err)
	}

	managedClusterMap := make(map[string]Cluster, len(opt.ManagedClusters))
	for _, c := range opt.ManagedClusters {
		managedClusterMap[c.Name] = c
	}

	var ocpClusters []Cluster
	for _, mc := range availableManagedClusters {
		if !isOpenshiftVendor(mc) {
			continue
		}

		if IsHubCluster(mc) {
			continue
		}

		if c, ok := managedClusterMap[mc.Name]; ok {
			ocpClusters = append(ocpClusters, c)
			delete(managedClusterMap, mc.Name)
		}
	}

	return ocpClusters, nil
}

func CreateCOOSubscription(clusters []Cluster) error {
	for _, cluster := range clusters {
		clientDynamic := NewKubeClientDynamic(
			cluster.ClusterServerURL,
			cluster.KubeConfig,
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

func DeleteCOOSubscription(clusters []Cluster) error {
	for _, cluster := range clusters {
		clientDynamic := NewKubeClientDynamic(
			cluster.ClusterServerURL,
			cluster.KubeConfig,
			cluster.KubeContext)

		err := clientDynamic.Resource(NewSubscriptionGVR()).Namespace(cooSubscriptionNamespace).Delete(context.TODO(), cooSubscriptionName, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete COO subscription on cluster: %w", err)
		}
	}
	return nil
}

func CheckCOODeployment(clusters []Cluster) {
	for _, cluster := range clusters {
		CheckDeploymentAvailability(cluster, cooDeploymentName, cooSubscriptionNamespace, true)
	}
}
