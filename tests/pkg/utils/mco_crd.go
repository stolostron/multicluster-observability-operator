// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

func DeleteMonitoringCRDs(ctx context.Context, clusters []Cluster) error {
	for _, cluster := range clusters {
		apiExtensionsClient := NewKubeClientAPIExtension(cluster.ClusterServerURL, cluster.KubeConfig, cluster.KubeContext)
		dynClient := GetKubeClientDynamicWithCluster(cluster)

		crds, err := apiExtensionsClient.ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{})
		if err != nil {
			return err
		}

		for _, crd := range crds.Items {
			if crd.Spec.Group != "monitoring.rhobs" {
				continue
			}

			// Find the storage version to build a valid GVR.
			version := ""
			for _, v := range crd.Spec.Versions {
				if v.Storage {
					version = v.Name
					break
				}
			}
			if version == "" {
				continue
			}

			gvr := schema.GroupVersionResource{
				Group:    crd.Spec.Group,
				Version:  version,
				Resource: crd.Spec.Names.Plural,
			}

			// Delete all instances explicitly so that GC does not block CRD deletion.
			// metav1.NamespaceAll ("") works for both namespaced and cluster-scoped resources.
			instances, listErr := dynClient.Resource(gvr).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
			if listErr != nil {
				if !errors.IsNotFound(listErr) {
					klog.Warningf("Failed to list instances of %s on cluster %s: %v", crd.Name, cluster.Name, listErr)
				}
			} else if instances != nil {
				for i := range instances.Items {
					inst := &instances.Items[i]
					klog.InfoS("Deleting CRD instance", "crd", crd.Name, "instance", inst.GetName(), "cluster", cluster.Name)
					delErr := dynClient.Resource(gvr).Namespace(inst.GetNamespace()).Delete(ctx, inst.GetName(), metav1.DeleteOptions{})
					if delErr != nil && !errors.IsNotFound(delErr) {
						klog.Warningf("Failed to delete %s/%s on cluster %s: %v", crd.Name, inst.GetName(), cluster.Name, delErr)
					}
				}
			}

			klog.InfoS("Deleting CRD", "crd", crd.Name, "cluster", cluster.Name)
			if err := apiExtensionsClient.ApiextensionsV1().CustomResourceDefinitions().Delete(ctx, crd.Name, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
				return err
			}
		}

		// Wait for all monitoring.rhobs CRDs to be removed.
		err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 3*time.Minute, true, func(pollCtx context.Context) (bool, error) {
			remaining, listErr := apiExtensionsClient.ApiextensionsV1().CustomResourceDefinitions().List(pollCtx, metav1.ListOptions{})
			if listErr != nil {
				klog.Warningf("Error listing CRDs on cluster %s: %v", cluster.Name, listErr)
				return false, nil
			}
			for _, crd := range remaining.Items {
				if crd.Spec.Group == "monitoring.rhobs" {
					return false, nil
				}
			}
			return true, nil
		})
		if err != nil {
			return fmt.Errorf("timed out waiting for monitoring.rhobs CRDs to be deleted on cluster %s: %w", cluster.Name, err)
		}
	}

	return nil
}
