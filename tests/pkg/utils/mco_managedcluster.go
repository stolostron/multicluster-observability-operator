// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"errors"
	"os"

	goversion "github.com/hashicorp/go-version"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/klog"
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

func ListOCPManagedClusterIDs(opt TestOptions, minVersionStr string) ([]string, error) {
	minVersion, err := goversion.NewVersion(minVersionStr)
	if err != nil {
		return nil, err
	}
	clientDynamic := GetKubeClientDynamic(opt, true)
	objs, err := clientDynamic.Resource(NewOCMManagedClustersGVR()).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	clusterIDs := []string{}
	for _, obj := range objs.Items {
		metadata := obj.Object["metadata"].(map[string]interface{})
		labels := metadata["labels"].(map[string]interface{})
		if labels != nil {
			vendorStr := ""
			if vendor, ok := labels["vendor"]; ok {
				vendorStr = vendor.(string)
			}
			obsControllerStr := ""
			if obsController, ok := labels["feature.open-cluster-management.io/addon-observability-controller"]; ok {
				obsControllerStr = obsController.(string)
			}
			if vendorStr == "OpenShift" && obsControllerStr == "available" {
				clusterVersionStr := ""
				if clusterVersionVal, ok := labels["openshiftVersion"]; ok {
					clusterVersionStr = clusterVersionVal.(string)
				}
				clusterVersion, err := goversion.NewVersion(clusterVersionStr)
				if err != nil {
					return nil, err
				}
				if clusterVersion.GreaterThanOrEqual(minVersion) {
					clusterIDStr := ""
					if clusterID, ok := labels["clusterID"]; ok {
						clusterIDStr = clusterID.(string)
					}
					if len(clusterIDStr) > 0 {
						clusterIDs = append(clusterIDs, clusterIDStr)
					}
				}
			}
		}
	}

	return clusterIDs, nil
}

func ListLocalClusterIDs(opt TestOptions) ([]string, error) {
	clientDynamic := GetKubeClientDynamic(opt, true)
	objs, err := clientDynamic.Resource(NewOCMManagedClustersGVR()).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	clusterIDs := []string{}
	for _, obj := range objs.Items {
		metadata := obj.Object["metadata"].(map[string]interface{})
		labels := metadata["labels"].(map[string]interface{})
		if labels != nil {
			vendorStr := ""
			if vendor, ok := labels["vendor"]; ok {
				vendorStr = vendor.(string)
			}

			localClusterLabelStr := ""
			if localCluster, ok := labels["local-cluster"]; ok {
				localClusterLabelStr = localCluster.(string)
			}
			if vendorStr == "OpenShift" && localClusterLabelStr == "true" {
				clusterIDStr := ""
				if clusterID, ok := labels["clusterID"]; ok {
					clusterIDStr = clusterID.(string)
				}
				if len(clusterIDStr) > 0 {
					clusterIDs = append(clusterIDs, clusterIDStr)
				}
			}
		}
	}
	klog.V(3).Infof("clusterIDs is %s", clusterIDs)
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
		if labels != nil {
			vendorStr := ""
			if vendor, ok := labels["vendor"]; ok {
				vendorStr = vendor.(string)
			}
			obsControllerStr := ""
			if obsController, ok := labels["feature.open-cluster-management.io/addon-observability-controller"]; ok {
				obsControllerStr = obsController.(string)
			}
			if vendorStr != "OpenShift" && obsControllerStr == "available" {
				clusterNameStr := ""
				if clusterNameVal, ok := labels["name"]; ok {
					clusterNameStr = clusterNameVal.(string)
				}
				if len(clusterNameStr) > 0 {
					clusterNames = append(clusterNames, clusterNameStr)
				}
			}
		}
	}

	return clusterNames, nil
}
