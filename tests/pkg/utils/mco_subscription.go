// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	cooSubscriptionName      = "cluster-observability-operator"
	cooSubscriptionNamespace = "openshift-cluster-observability-operator"
	cooOperatorGroupName     = "cluster-observability-operator-group"
	cooDeploymentName        = "obo-prometheus-operator"
	packageName              = "cluster-observability-operator"
	consolePluginName        = "monitoring-console-plugin"
)

func NewSubscriptionGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1alpha1",
		Resource: "subscriptions",
	}
}

func NewPackageManifestGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "packages.operators.coreos.com",
		Version:  "v1",
		Resource: "packagemanifests",
	}
}

func NewClusterServiceVersionGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1alpha1",
		Resource: "clusterserviceversions",
	}
}

func NewOperatorGroupGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1",
		Resource: "operatorgroups",
	}
}

func NewConsolePluginGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "console.openshift.io",
		Version:  "v1",
		Resource: "consoleplugins",
	}
}

func NewClusterOperatorGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "config.openshift.io",
		Version:  "v1",
		Resource: "clusteroperators",
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

func GetOCPClustersWithAPIAccess(opt TestOptions) ([]Cluster, error) {
	ocpClusters, err := GetOCPClusters(opt)
	if err != nil {
		return nil, err
	}

	accessibleOCPClusters := []Cluster{}
	for _, cluster := range ocpClusters {
		clientKube := NewKubeClient(
			cluster.ClusterServerURL,
			cluster.KubeConfig,
			cluster.KubeContext)
		_, err := clientKube.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			klog.Warningf("failed to access cluster %s: %v", cluster.Name, err)
		} else {
			accessibleOCPClusters = append(accessibleOCPClusters, cluster)
		}
	}

	return accessibleOCPClusters, nil
}

func CreateCOOSubscription(clusters []Cluster) error {
	for _, cluster := range clusters {
		clientKube := NewKubeClient(
			cluster.ClusterServerURL,
			cluster.KubeConfig,
			cluster.KubeContext)

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: cooSubscriptionNamespace,
			},
		}
		_, err := clientKube.CoreV1().Namespaces().Get(context.TODO(), cooSubscriptionNamespace, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				_, createErr := clientKube.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
				if createErr != nil {
					return fmt.Errorf("failed to create namespace %s on cluster %s: %w", cooSubscriptionNamespace, cluster.Name, createErr)
				}
			} else {
				return fmt.Errorf("failed to get namespace %s on cluster %s: %w", cooSubscriptionNamespace, cluster.Name, err)
			}
		}

		clientDynamic := NewKubeClientDynamic(
			cluster.ClusterServerURL,
			cluster.KubeConfig,
			cluster.KubeContext)

		// Create OperatorGroup
		og := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "operators.coreos.com/v1",
				"kind":       "OperatorGroup",
				"metadata": map[string]any{
					"name":      cooOperatorGroupName,
					"namespace": cooSubscriptionNamespace,
				},
				"spec": map[string]any{},
			},
		}
		_, err = clientDynamic.Resource(NewOperatorGroupGVR()).Namespace(cooSubscriptionNamespace).Create(context.TODO(), og, metav1.CreateOptions{})
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create OperatorGroup on cluster %s: %w", cluster.Name, err)
		}

		pkg, err := clientDynamic.Resource(NewPackageManifestGVR()).Namespace("openshift-marketplace").Get(context.TODO(), packageName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get packagemanifest for %s: %w", packageName, err)
		}

		status, ok := pkg.Object["status"].(map[string]any)
		if !ok {
			return fmt.Errorf("failed to parse status of packagemanifest for %s", packageName)
		}

		channels, ok := status["channels"].([]any)
		if !ok {
			return fmt.Errorf("failed to parse channels of packagemanifest for %s", packageName)
		}

		var latestCSV string
		for _, channel := range channels {
			ch, ok := channel.(map[string]any)
			if !ok {
				continue
			}
			if ch["name"] == "stable" {
				currentCSV, ok := ch["currentCSV"].(string)
				if ok {
					latestCSV = currentCSV
					break
				}
			}
		}

		if latestCSV == "" {
			return fmt.Errorf("failed to find latest CSV for %s in stable channel", packageName)
		}

		// Clean up existing CSV if it exists
		err = clientDynamic.Resource(NewClusterServiceVersionGVR()).Namespace(cooSubscriptionNamespace).Delete(context.TODO(), latestCSV, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete existing CSV %s: %w", latestCSV, err)
		}

		// Wait for CSV to be deleted
		err = wait.PollUntilContextTimeout(context.Background(), 1*time.Second, 1*time.Minute, true, func(ctx context.Context) (bool, error) {
			_, errGet := clientDynamic.Resource(NewClusterServiceVersionGVR()).Namespace(cooSubscriptionNamespace).Get(ctx, latestCSV, metav1.GetOptions{})
			if errors.IsNotFound(errGet) {
				klog.Infof("Orphaned CSV %s is deleted on cluster %s", latestCSV, cluster.Name)
				return true, nil
			}
			return false, nil
		})
		if err != nil {
			return fmt.Errorf("failed to wait for orphaned CSV %s to be deleted on cluster %s: %w", latestCSV, cluster.Name, err)
		}

		subUnstructured := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "operators.coreos.com/v1alpha1",
				"kind":       "Subscription",
				"metadata": map[string]any{
					"name":      cooSubscriptionName,
					"namespace": cooSubscriptionNamespace,
				},
				"spec": map[string]any{
					"channel":             "stable",
					"installPlanApproval": "Automatic",
					"name":                cooSubscriptionName,
					"source":              "redhat-operators",
					"sourceNamespace":     "openshift-marketplace",
					"startingCSV":         latestCSV,
				},
			},
		}

		_, err = clientDynamic.Resource(NewSubscriptionGVR()).Namespace(cooSubscriptionNamespace).Create(context.TODO(), subUnstructured, metav1.CreateOptions{})
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create COO subscription on cluster: %w", err)
		}

		klog.Infof("Waiting for InstallPlan to be created for subscription %s on cluster %s", cooSubscriptionName, cluster.Name)
		err = wait.PollUntilContextTimeout(context.Background(), 1*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
			sub, err := clientDynamic.Resource(NewSubscriptionGVR()).Namespace(cooSubscriptionNamespace).Get(ctx, cooSubscriptionName, metav1.GetOptions{})
			if err != nil {
				klog.Warningf("Error getting subscription %s on cluster %s: %v", cooSubscriptionName, cluster.Name, err)
				return false, nil
			}
			if sub.Object["status"] != nil {
				status := sub.Object["status"].(map[string]any)
				if status["installPlanRef"] != nil {
					klog.Infof("InstallPlan created for subscription %s on cluster %s", cooSubscriptionName, cluster.Name)
					return true, nil
				}
			}
			return false, nil
		})
		if err != nil {
			return fmt.Errorf("failed to wait for InstallPlan to be created for subscription %s on cluster %s: %w", cooSubscriptionName, cluster.Name, err)
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

		// Get the subscription to find the installed CSV
		sub, err := clientDynamic.Resource(NewSubscriptionGVR()).Namespace(cooSubscriptionNamespace).Get(context.TODO(), cooSubscriptionName, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				klog.Infof("Subscription %s not found on cluster %s, skipping deletion", cooSubscriptionName, cluster.Name)
				continue // Nothing to do
			}
			return fmt.Errorf("failed to get COO subscription on cluster %s: %w", cluster.Name, err)
		}

		// Remove owner references to allow deletion
		if len(sub.GetOwnerReferences()) > 0 {
			klog.Infof("Removing owner references from subscription %s on cluster %s", cooSubscriptionName, cluster.Name)
			sub.SetOwnerReferences(nil)
			_, err = clientDynamic.Resource(NewSubscriptionGVR()).Namespace(cooSubscriptionNamespace).Update(context.TODO(), sub, metav1.UpdateOptions{})
			if err != nil && !errors.IsNotFound(err) { // Can be not found if it's already being deleted
				return fmt.Errorf("failed to remove owner references from subscription on cluster %s: %w", cluster.Name, err)
			}
		}

		// Delete the subscription
		klog.Infof("Deleting subscription %s on cluster %s", cooSubscriptionName, cluster.Name)
		err = clientDynamic.Resource(NewSubscriptionGVR()).Namespace(cooSubscriptionNamespace).Delete(context.TODO(), cooSubscriptionName, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete COO subscription on cluster %s: %w", cluster.Name, err)
		}

		// Wait for subscription to be deleted
		err = wait.PollUntilContextTimeout(context.Background(), 1*time.Second, 1*time.Minute, true, func(ctx context.Context) (bool, error) {
			_, err := clientDynamic.Resource(NewSubscriptionGVR()).Namespace(cooSubscriptionNamespace).Get(ctx, cooSubscriptionName, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				klog.Infof("Subscription %s is deleted on cluster %s", cooSubscriptionName, cluster.Name)
				return true, nil
			}
			return false, nil
		})
		if err != nil {
			return fmt.Errorf("failed to wait for subscription %s to be deleted on cluster %s: %w", cooSubscriptionName, cluster.Name, err)
		}

		// Delete the installed CSV
		if sub.Object["status"] != nil {
			status, ok := sub.Object["status"].(map[string]any)
			if ok && status["installedCSV"] != nil {
				installedCSV, ok := status["installedCSV"].(string)
				if ok && installedCSV != "" {
					klog.Infof("Deleting CSV %s on cluster %s", installedCSV, cluster.Name)
					err := clientDynamic.Resource(NewClusterServiceVersionGVR()).Namespace(cooSubscriptionNamespace).Delete(context.TODO(), installedCSV, metav1.DeleteOptions{})
					if err != nil && !errors.IsNotFound(err) {
						return fmt.Errorf("failed to delete CSV %s on cluster %s: %w", installedCSV, cluster.Name, err)
					}
					// Wait for CSV to be deleted
					err = wait.PollUntilContextTimeout(context.Background(), 1*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
						_, err := clientDynamic.Resource(NewClusterServiceVersionGVR()).Namespace(cooSubscriptionNamespace).Get(ctx, installedCSV, metav1.GetOptions{})
						if errors.IsNotFound(err) {
							klog.Infof("CSV %s is deleted on cluster %s", installedCSV, cluster.Name)
							return true, nil
						}
						return false, nil
					})
					if err != nil {
						return fmt.Errorf("failed to wait for CSV %s to be deleted on cluster %s: %w", installedCSV, cluster.Name, err)
					}
				}
			}
		}

		// Delete the ConsolePlugin
		klog.Infof("Deleting ConsolePlugin %s on cluster %s", consolePluginName, cluster.Name)
		err = clientDynamic.Resource(NewConsolePluginGVR()).Delete(context.TODO(), consolePluginName, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete ConsolePlugin on cluster %s: %w", cluster.Name, err)
		}

		// Wait for ConsolePlugin to be deleted
		err = wait.PollUntilContextTimeout(context.Background(), 1*time.Second, 1*time.Minute, true, func(ctx context.Context) (bool, error) {
			_, errGet := clientDynamic.Resource(NewConsolePluginGVR()).Get(ctx, consolePluginName, metav1.GetOptions{})
			if errors.IsNotFound(errGet) {
				klog.Infof("ConsolePlugin %s is deleted on cluster %s", consolePluginName, cluster.Name)
				return true, nil
			}
			return false, nil
		})
		if err != nil {
			return fmt.Errorf("failed to wait for ConsolePlugin %s to be deleted on cluster %s: %w", consolePluginName, cluster.Name, err)
		}

		// Wait for console operator to stabilize
		klog.Infof("Waiting for console operator to stabilize on cluster %s", cluster.Name)
		err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
			co, err := clientDynamic.Resource(NewClusterOperatorGVR()).Get(ctx, "console", metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			conditions, found, err := unstructured.NestedSlice(co.Object, "status", "conditions")
			if err != nil || !found {
				return false, err
			}
			for _, cond := range conditions {
				condition, ok := cond.(map[string]any)
				if !ok {
					continue
				}
				if condition["type"] == "Progressing" && condition["status"] == "False" {
					klog.Infof("Console operator is stable on cluster %s", cluster.Name)
					return true, nil
				}
			}
			return false, nil
		})
		if err != nil {
			return fmt.Errorf("failed to wait for console operator to stabilize on cluster %s: %w", cluster.Name, err)
		}

		// Delete the OperatorGroup
		klog.Infof("Deleting OperatorGroup %s on cluster %s", cooOperatorGroupName, cluster.Name)
		err = clientDynamic.Resource(NewOperatorGroupGVR()).Namespace(cooSubscriptionNamespace).Delete(context.TODO(), cooOperatorGroupName, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete OperatorGroup on cluster %s: %w", cluster.Name, err)
		}

		// Wait for OperatorGroup to be deleted
		err = wait.PollUntilContextTimeout(context.Background(), 1*time.Second, 1*time.Minute, true, func(ctx context.Context) (bool, error) {
			_, errGet := clientDynamic.Resource(NewOperatorGroupGVR()).Namespace(cooSubscriptionNamespace).Get(ctx, cooOperatorGroupName, metav1.GetOptions{})
			if errors.IsNotFound(errGet) {
				klog.Infof("OperatorGroup %s is deleted on cluster %s", cooOperatorGroupName, cluster.Name)
				return true, nil
			}
			return false, nil
		})
		if err != nil {
			return fmt.Errorf("failed to wait for OperatorGroup %s to be deleted on cluster %s: %w", cooOperatorGroupName, cluster.Name, err)
		}

		// Delete the namespace for COO
		clientKube := NewKubeClient(
			cluster.ClusterServerURL,
			cluster.KubeConfig,
			cluster.KubeContext)
		klog.Infof("Deleting namespace %s on cluster %s", cooSubscriptionNamespace, cluster.Name)
		err = clientKube.CoreV1().Namespaces().Delete(context.TODO(), cooSubscriptionNamespace, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete namespace %s on cluster %s: %w", cooSubscriptionNamespace, cluster.Name, err)
		}

		// Wait for namespace to be deleted
		klog.Infof("Waiting for namespace %s to be deleted on cluster %s", cooSubscriptionNamespace, cluster.Name)
		err = wait.PollUntilContextTimeout(context.Background(), 1*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
			_, errGet := clientKube.CoreV1().Namespaces().Get(ctx, cooSubscriptionNamespace, metav1.GetOptions{})
			if errors.IsNotFound(errGet) {
				klog.Infof("Namespace %s is deleted on cluster %s", cooSubscriptionNamespace, cluster.Name)
				return true, nil
			}
			return false, errGet
		})
		if err != nil {
			return fmt.Errorf("failed to wait for namespace %s to be deleted on cluster %s: %w", cooSubscriptionNamespace, cluster.Name, err)
		}
	}
	return nil
}

func CheckCOODeployment(clusters []Cluster) {
	for _, cluster := range clusters {
		CheckDeploymentAvailability(cluster, cooDeploymentName, cooSubscriptionNamespace, true)
	}
}
