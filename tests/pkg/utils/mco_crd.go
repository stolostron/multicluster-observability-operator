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

// Must stay in sync with operators/endpointmetrics/controllers/mcoa.ManagedByLabel*.
// The endpoint operator self-heals CRDs that carry this label while it is running
// (see DeployCRDs / CRD Delete watch). E2E cleanup must tolerate that race.
const (
	mcoaEndpointManagedByLabelKey   = "app.kubernetes.io/managed-by"
	mcoaEndpointManagedByLabelValue = "mcoa-endpoint-operator"
)

func DeleteMonitoringCRDs(opt TestOptions, clusters []Cluster) error {
	for _, cluster := range clusters {
		apiExtensionsClient := NewKubeClientAPIExtension(cluster.ClusterServerURL, cluster.KubeConfig, cluster.KubeContext)
		dynClient := GetKubeClientDynamicWithCluster(cluster)

		crds, err := apiExtensionsClient.ApiextensionsV1().CustomResourceDefinitions().List(context.TODO(), metav1.ListOptions{})
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
			instances, listErr := dynClient.Resource(gvr).Namespace(metav1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
			if listErr != nil && !errors.IsNotFound(listErr) {
				klog.Warningf("Failed to list instances of %s on cluster %s: %v", crd.Name, cluster.Name, listErr)
			} else if instances != nil {
				for i := range instances.Items {
					inst := &instances.Items[i]
					klog.Infof("Deleting %s/%s on cluster %s", crd.Name, inst.GetName(), cluster.Name)
					delErr := dynClient.Resource(gvr).Namespace(inst.GetNamespace()).Delete(context.TODO(), inst.GetName(), metav1.DeleteOptions{})
					if delErr != nil && !errors.IsNotFound(delErr) {
						klog.Warningf("Failed to delete %s/%s on cluster %s: %v", crd.Name, inst.GetName(), cluster.Name, delErr)
					}
				}
			}

			klog.Infof("Deleting CRD %s on cluster %s", crd.Name, cluster.Name)
			if err := apiExtensionsClient.ApiextensionsV1().CustomResourceDefinitions().Delete(context.TODO(), crd.Name, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
				return err
			}
		}

		for _, crdName := range crdsToDelete {
			klog.Infof("Waiting for CRD %s to be deleted on cluster %s", crdName, cluster.Name)
			err := wait.PollUntilContextTimeout(context.Background(), 1*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
				crd, err := apiExtensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crdName, metav1.GetOptions{})
				if errors.IsNotFound(err) {
					klog.Infof("CRD %s is deleted on cluster %s", crdName, cluster.Name)
					return true, nil
				}
				if err != nil {
					klog.Warningf("Error getting CRD %s on cluster %s: %v", crdName, cluster.Name, err)
					return false, nil
				}
				// Endpoint operator CRD self-heal (PR #2566) recreates OBO CRDs while the
				// operator pod is still running. That is expected on the hub / local-cluster
				// and must not fail e2e setup/teardown. Only accept a fully restored CRD
				// (not one still terminating from the delete we just issued).
				if crd.DeletionTimestamp == nil &&
					crd.Labels[mcoaEndpointManagedByLabelKey] == mcoaEndpointManagedByLabelValue {
					klog.Infof("CRD %s was restored by endpoint operator on cluster %s; treating cleanup as successful", crdName, cluster.Name)
					return true, nil
				}
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
