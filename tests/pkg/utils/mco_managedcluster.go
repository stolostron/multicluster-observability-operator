// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"errors"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func UpdateObservabilityFromManagedCluster(opt TestOptions, enableObservability bool) error {
	clusterName := GetManagedClusterName(opt)
	if clusterName != "" {
		clientDynamic := GetKubeClientDynamic(opt, true)
		cluster, err := clientDynamic.Resource(NewOCMManagedClustersGVR()).
			Get(context.TODO(), clusterName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		labels, ok := cluster.Object["metadata"].(map[string]interface{})["labels"].(map[string]interface{})
		if !ok {
			cluster.Object["metadata"].(map[string]interface{})["labels"] = map[string]interface{}{}
			labels = cluster.Object["metadata"].(map[string]interface{})["labels"].(map[string]interface{})
		}

		if !enableObservability {
			labels["observability"] = "disabled"
		} else {
			delete(labels, "observability")
		}
		_, updateErr := clientDynamic.Resource(NewOCMManagedClustersGVR()).
			Update(context.TODO(), cluster, metav1.UpdateOptions{})
		if updateErr != nil {
			return updateErr
		}
	}
	return nil
}

func ListManagedClusters(opt TestOptions) ([]string, error) {
	clientDynamic := GetKubeClientDynamic(opt, true)
	objs, err := clientDynamic.Resource(NewOCMManagedClustersGVR()).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	clusterNames := []string{}
	for _, obj := range objs.Items {
		metadata := obj.Object["metadata"].(map[string]interface{})
		name := metadata["name"].(string)

		if os.Getenv("IS_KIND_ENV") == "true" {
			// We do not have the obs add on label added in kind cluster
			clusterNames = append(clusterNames, name)
			continue
		}

		status, ok := obj.Object["status"].(map[string]interface{})
		if !ok {
			// No status found, skip this cluster
			continue
		}

		conditions, ok := status["conditions"].([]interface{})
		if !ok {
			// No conditions found, skip this cluster
			continue
		}

		available := false
		for _, condition := range conditions {
			conditionMap, ok := condition.(map[string]interface{})
			if !ok {
				continue
			}
			if conditionMap["type"] == "ManagedClusterConditionAvailable" && conditionMap["status"] == "True" {
				available = true
				break
			}
		}

		// Only add clusters with ManagedClusterConditionAvailable status == True
		if available {
			clusterNames = append(clusterNames, name)
		}
	}

	if len(clusterNames) == 0 {
		return clusterNames, errors.New("no managedcluster found")
	}

	return clusterNames, nil
}

func ListOCPManagedClusterIDs(opt TestOptions) ([]string, error) {
	clientDynamic := GetKubeClientDynamic(opt, true)
	objs, err := clientDynamic.Resource(NewOCMManagedClustersGVR()).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	clusterIDs := []string{}
	for _, obj := range objs.Items {
		metadata := obj.Object["metadata"].(map[string]interface{})
		labels := metadata["labels"].(map[string]interface{})
		if labels == nil {
			continue
		}

		if vendor, ok := labels["vendor"]; !ok || vendor != "OpenShift" {
			continue
		}

		if clusterID, ok := labels["clusterID"]; ok {
			clusterIDs = append(clusterIDs, clusterID.(string))
		}
	}

	return clusterIDs, nil
}

func ListKSManagedClusterNames(opt TestOptions) ([]string, error) {
	clientDynamic := GetKubeClientDynamic(opt, true)
	objs, err := clientDynamic.Resource(NewOCMManagedClustersGVR()).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	clusterNames := []string{}
	for _, obj := range objs.Items {
		metadata := obj.Object["metadata"].(map[string]interface{})
		labels := metadata["labels"].(map[string]interface{})
		if labels == nil {
			continue
		}

		if vendor, ok := labels["vendor"]; ok && vendor == "OpenShift" {
			continue
		}

		if clusterName, ok := labels["name"]; ok {
			clusterNames = append(clusterNames, clusterName.(string))
		}
	}

	return clusterNames, nil
}
