// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/mcoa"
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

		crdsToDelete := make(map[string]bool)
		for _, crd := range crds.Items {
			if crd.Spec.Group == "monitoring.rhobs" {
				crdsToDelete[crd.Name] = true
			}
		}
		// Also ensure all known MCOA managed CRDs are included
		for _, name := range mcoa.GetManagedCRDNames() {
			crdsToDelete[name] = true
		}

		crdNames := slices.Collect(maps.Keys(crdsToDelete))
		slices.Sort(crdNames)

		for _, crdName := range crdNames {
			crd, getErr := apiExtensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crdName, metav1.GetOptions{})
			if errors.IsNotFound(getErr) {
				delete(crdsToDelete, crdName)
				continue
			}
			if getErr != nil {
				return getErr
			}

			// Find the storage version to build a valid GVR.
			version := ""
			for _, v := range crd.Spec.Versions {
				if v.Storage {
					version = v.Name
					break
				}
			}
			if version != "" {
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
						// Clear finalizers if present so stuck instances don't block CRD deletion.
						if len(inst.GetFinalizers()) > 0 {
							inst.SetFinalizers(nil)
							_, _ = dynClient.Resource(gvr).Namespace(inst.GetNamespace()).Update(ctx, inst, metav1.UpdateOptions{})
						}
					}
				}
			}

			klog.InfoS("Deleting CRD", "crd", crd.Name, "cluster", cluster.Name)
			if err := apiExtensionsClient.ApiextensionsV1().CustomResourceDefinitions().Delete(ctx, crd.Name, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
				return err
			}
		}

		if len(crdsToDelete) == 0 {
			continue
		}

		// Wait for all deleted monitoring.rhobs CRDs to be removed using targeted Get calls.
		err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 3*time.Minute, true, func(_ context.Context) (bool, error) {
			for crdName := range crdsToDelete {
				reqCtx, reqCancel := context.WithTimeout(context.Background(), 10*time.Second)
				_, getErr := apiExtensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(reqCtx, crdName, metav1.GetOptions{})
				reqCancel()

				if getErr == nil {
					continue
				}
				if errors.IsNotFound(getErr) {
					// The CRD is deleted, remove it so we don't query it again on future iterations
					delete(crdsToDelete, crdName)
					continue
				}
				klog.Warningf("Error checking CRD %s on cluster %s: %v", crdName, cluster.Name, getErr)
			}
			return len(crdsToDelete) == 0, nil
		})
		if err != nil {
			remaining := slices.Collect(maps.Keys(crdsToDelete))
			slices.Sort(remaining)
			return fmt.Errorf("timed out waiting for monitoring.rhobs CRDs %v to be deleted on cluster %s: %w", remaining, cluster.Name, err)
		}
	}

	return nil
}
