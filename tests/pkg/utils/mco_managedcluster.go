// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/onsi/ginkgo/v2"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

const (
	availableManagedClusterCondition = "ManagedClusterConditionAvailable"
	idClusterClaim                   = "id.k8s.io"
	selfManagedClusterLabelKey       = "local-cluster"
)

var openshiftLabelSelector = labels.SelectorFromValidatedSet(map[string]string{
	"vendor": "OpenShift",
})

type ClustersInfo struct {
	Name           string
	IsLocalCluster bool
	Vendor         string
}

func UpdateObservabilityFromManagedCluster(opt TestOptions, enableObservability bool) error {
	clusterName := GetManagedClusterName(opt)
	if clusterName == "" {
		return errors.New("no managed cluster found")
	}
	if !enableObservability {
		return SetManagedClusterLabel(context.TODO(), opt, clusterName, "observability", "disabled")
	}
	return RemoveManagedClusterLabel(context.TODO(), opt, clusterName, "observability")
}

func ListManagedClusters(opt TestOptions) ([]ClustersInfo, error) {
	clientDynamic := GetKubeClientDynamic(opt, true)
	objs, err := clientDynamic.Resource(NewOCMManagedClustersGVR()).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	clusters := make([]ClustersInfo, 0, len(objs.Items))
	for _, obj := range objs.Items {
		metadata := obj.Object["metadata"].(map[string]any)
		name := metadata["name"].(string)

		status, ok := obj.Object["status"].(map[string]any)
		if !ok {
			klog.Infof("Skip cluster %s: no status found", name)
			continue
		}

		conditions, ok := status["conditions"].([]any)
		if !ok {
			klog.Infof("Skip cluster %s: no conditions found", name)
			continue
		}

		available := false
		for _, condition := range conditions {
			conditionMap, ok := condition.(map[string]any)
			if !ok {
				continue
			}
			if conditionMap["type"] == "ManagedClusterConditionAvailable" && conditionMap["status"] == "True" {
				available = true
				break
			}
		}

		labelsMap, _ := metadata["labels"].(map[string]any)
		vendor, _ := labelsMap["vendor"].(string)
		if vendor == "" {
			vendor = statusUnknown
		}

		// Only add clusters with ManagedClusterConditionAvailable status == True
		if available {
			klog.Infof("Add cluster %s (vendor: %s) to the list", name, vendor)
			clusters = append(clusters, ClustersInfo{
				Name:           name,
				IsLocalCluster: labelsMap["local-cluster"] == "true",
				Vendor:         vendor,
			})
		} else {
			klog.Infof("Skip cluster %s (vendor: %s): ManagedClusterConditionAvailable is not True", name, vendor)
		}
	}

	if len(clusters) == 0 {
		return clusters, errors.New("no managedcluster found")
	}

	return clusters, nil
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

func IsHubCluster(cluster *clusterv1.ManagedCluster) bool {
	return cluster.Labels[selfManagedClusterLabelKey] == "true"
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

// GetManagedCluster retrieves a specific ManagedCluster by name.
func GetManagedCluster(ctx context.Context, opt TestOptions, clusterName string) (*clusterv1.ManagedCluster, error) {
	clientDynamic := GetKubeClientDynamic(opt, true)
	obj, err := clientDynamic.Resource(NewOCMManagedClustersGVR()).Get(ctx, clusterName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ManagedCluster %s: %w", clusterName, err)
	}

	mc := &clusterv1.ManagedCluster{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, mc); err != nil {
		return nil, fmt.Errorf("failed to convert Unstructured to ManagedCluster: %w", err)
	}
	return mc, nil
}

// GetHubClusterName queries the ManagedCluster API and returns the name of the hub cluster
// (identified by the "local-cluster=true" label). Falls back to "local-cluster" if not found.
func GetHubClusterName(opt TestOptions) (string, error) {
	clusters, err := ListManagedClusters(opt)
	if err != nil {
		return "", fmt.Errorf("failed to list managed clusters: %w", err)
	}
	for _, c := range clusters {
		if c.IsLocalCluster {
			return c.Name, nil
		}
	}
	return "local-cluster", nil
}

func GetAvailableManagedClusters(opt TestOptions) ([]*clusterv1.ManagedCluster, error) {
	clusters, err := GetManagedClusters(opt)
	if err != nil {
		return nil, err
	}

	return slices.DeleteFunc(clusters, func(e *clusterv1.ManagedCluster) bool {
		return !meta.IsStatusConditionTrue(e.Status.Conditions, "ManagedClusterConditionAvailable")
	}), nil
}

// check spoke exists (excluding local-cluster)
func HasManagedClusters(opt TestOptions) bool {
	clusters, err := ListManagedClusters(opt)
	if err != nil {
		return false
	}
	for _, c := range clusters {
		if c.Name != "" && !c.IsLocalCluster {
			return true
		}
	}
	return false
}

// GetManagedClusterIDToNameMap returns a mapping of cluster ID (from id.k8s.io cluster claim)
// to cluster name for all managed clusters.
func GetManagedClusterIDToNameMap(opt TestOptions) (map[string]string, error) {
	managedClusters, err := GetManagedClusters(opt)
	if err != nil {
		return nil, err
	}
	idToName := make(map[string]string, len(managedClusters))
	for _, mc := range managedClusters {
		for _, cc := range mc.Status.ClusterClaims {
			if cc.Name == idClusterClaim {
				idToName[cc.Value] = mc.Name
				break
			}
		}
	}
	return idToName, nil
}

// mutateManagedCluster modifies a ManagedCluster using a mutator function and retry.RetryOnConflict.
// If mutate returns false, the object was not changed and the API update is skipped.
func mutateManagedCluster(ctx context.Context, opt TestOptions, clusterName string, mutate func(cluster metav1.Object) bool) error {
	clientDynamic := GetKubeClientDynamic(opt, true)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cluster, err := clientDynamic.Resource(NewOCMManagedClustersGVR()).Get(ctx, clusterName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get ManagedCluster %s: %w", clusterName, err)
		}
		if !mutate(cluster) {
			return nil
		}
		_, err = clientDynamic.Resource(NewOCMManagedClustersGVR()).Update(ctx, cluster, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update ManagedCluster %s: %w", clusterName, err)
		}
		return nil
	})
}

func setAnnotation(obj metav1.Object, key, value string) bool {
	annotations := obj.GetAnnotations()
	if annotations != nil && annotations[key] == value {
		return false
	}
	newAnnotations := maps.Clone(annotations)
	if newAnnotations == nil {
		newAnnotations = make(map[string]string)
	}
	newAnnotations[key] = value
	obj.SetAnnotations(newAnnotations)
	return true
}

func removeAnnotation(obj metav1.Object, key string) bool {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false
	}
	if _, exists := annotations[key]; !exists {
		return false
	}
	newAnnotations := maps.Clone(annotations)
	delete(newAnnotations, key)
	obj.SetAnnotations(newAnnotations)
	return true
}

func setLabel(obj metav1.Object, key, value string) bool {
	labelsMap := obj.GetLabels()
	if labelsMap != nil && labelsMap[key] == value {
		return false
	}
	newLabels := maps.Clone(labelsMap)
	if newLabels == nil {
		newLabels = make(map[string]string)
	}
	newLabels[key] = value
	obj.SetLabels(newLabels)
	return true
}

func removeLabel(obj metav1.Object, key string) bool {
	labelsMap := obj.GetLabels()
	if labelsMap == nil {
		return false
	}
	if _, exists := labelsMap[key]; !exists {
		return false
	}
	newLabels := maps.Clone(labelsMap)
	delete(newLabels, key)
	obj.SetLabels(newLabels)
	return true
}

// SetManagedClusterAnnotation sets an annotation on the specified ManagedCluster with retry on conflict.
func SetManagedClusterAnnotation(ctx context.Context, opt TestOptions, clusterName, key, value string) error {
	return mutateManagedCluster(ctx, opt, clusterName, func(cluster metav1.Object) bool {
		return setAnnotation(cluster, key, value)
	})
}

// RemoveManagedClusterAnnotation removes an annotation from the specified ManagedCluster with retry on conflict.
func RemoveManagedClusterAnnotation(ctx context.Context, opt TestOptions, clusterName, key string) error {
	return mutateManagedCluster(ctx, opt, clusterName, func(cluster metav1.Object) bool {
		return removeAnnotation(cluster, key)
	})
}

// SetManagedClusterLabel sets a label on the specified ManagedCluster with retry on conflict.
func SetManagedClusterLabel(ctx context.Context, opt TestOptions, clusterName, key, value string) error {
	return mutateManagedCluster(ctx, opt, clusterName, func(cluster metav1.Object) bool {
		return setLabel(cluster, key, value)
	})
}

// RemoveManagedClusterLabel removes a label from the specified ManagedCluster with retry on conflict.
func RemoveManagedClusterLabel(ctx context.Context, opt TestOptions, clusterName, key string) error {
	return mutateManagedCluster(ctx, opt, clusterName, func(cluster metav1.Object) bool {
		return removeLabel(cluster, key)
	})
}
