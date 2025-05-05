// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/onsi/ginkgo/v2"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

const (
	availableManagedClusterCondition = "ManagedClusterConditionAvailable"
	idClusterClaim                   = "id.k8s.io"
)

var openshiftLabelSelector = labels.SelectorFromValidatedSet(map[string]string{
	"vendor": "OpenShift",
})

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
			labels, _ := metadata["labels"].(map[string]interface{})
			if labels["local-cluster"] == "true" {
				name = "local-cluster"
			}
			clusterNames = append(clusterNames, name)
		}
	}

	if len(clusterNames) == 0 {
		return clusterNames, errors.New("no managedcluster found")
	}

	return clusterNames, nil
}

func ListAvailableOCPManagedClusterIDs(opt TestOptions) ([]string, error) {
	managedClusters, err := GetManagedClusters(opt)
	if err != nil {
		return nil, err
	}

	// Filter out unavailable and non openshift clusters.
	// This is necessary for some e2e testing environments where some managed clusters might not be available.
	managedClusters = slices.DeleteFunc(managedClusters, func(e *clusterv1.ManagedCluster) bool {
		return !meta.IsStatusConditionTrue(e.Status.Conditions, availableManagedClusterCondition) || !isOpenshiftVendor(e)
	})

	ret := make([]string, 0, len(managedClusters))
	for _, mc := range managedClusters {
		ret = append(ret, getManagedClusterID(mc))
	}

	return ret, nil
}

func ListAvailableKSManagedClusterNames(opt TestOptions) ([]string, error) {
	managedClusters, err := GetManagedClusters(opt)
	if err != nil {
		return nil, err
	}

	// Filter out unavailable and non openshift clusters.
	// This is necessary for some e2e testing environments where some managed clusters might not be available.
	managedClusters = slices.DeleteFunc(managedClusters, func(e *clusterv1.ManagedCluster) bool {
		return !meta.IsStatusConditionTrue(e.Status.Conditions, availableManagedClusterCondition) || isOpenshiftVendor(e)
	})

	ret := make([]string, 0, len(managedClusters))
	for _, mc := range managedClusters {
		ret = append(ret, getManagedClusterID(mc))
	}

	return ret, nil
}

func isOpenshiftVendor(mc *clusterv1.ManagedCluster) bool {
	return openshiftLabelSelector.Matches(labels.Set(mc.GetLabels()))
}

func getManagedClusterID(mc *clusterv1.ManagedCluster) string {
	for _, cc := range mc.Status.ClusterClaims {
		if cc.Name == idClusterClaim {
			return cc.Value
		}
	}

	ginkgo.Fail(fmt.Sprintf("failed to get the managedCluster %q ID", mc.Name))

	return ""
}

func GetManagedClusters(opt TestOptions) ([]*clusterv1.ManagedCluster, error) {
	clientDynamic := GetKubeClientDynamic(opt, true)
	objs, err := clientDynamic.Resource(NewOCMManagedClustersGVR()).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ManagedClusters: %w", err)
	}

	ret := make([]*clusterv1.ManagedCluster, 0, len(objs.Items))
	for _, obj := range objs.Items {
		mc := &clusterv1.ManagedCluster{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, mc); err != nil {
			return nil, fmt.Errorf("failed to convert Unstructured to ManagedCluster: %w", err)
		}
		ret = append(ret, mc)
	}

	return ret, nil
}
