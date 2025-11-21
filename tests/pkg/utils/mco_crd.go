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
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
)

func DeleteMonitoringCRDs(opt TestOptions, clusters []Cluster) error {
	for _, cluster := range clusters {
		apiExtensionsClient := NewKubeClientAPIExtension(cluster.ClusterServerURL, cluster.KubeConfig, cluster.KubeContext)

		crds, err := apiExtensionsClient.ApiextensionsV1().CustomResourceDefinitions().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}

		var crdsToDelete []string
		for _, crd := range crds.Items {
			if crd.Spec.Group == "monitoring.rhobs" {
				err := apiExtensionsClient.ApiextensionsV1().CustomResourceDefinitions().Delete(context.TODO(), crd.Name, metav1.DeleteOptions{})
				if err != nil && !errors.IsNotFound(err) {
					return err
				}
				crdsToDelete = append(crdsToDelete, crd.Name)
			}
		}

		for _, crdName := range crdsToDelete {
			klog.Infof("Waiting for CRD %s to be deleted on cluster %s", crdName, cluster.Name)
			err := wait.PollUntilContextTimeout(context.Background(), 1*time.Second, 1*time.Minute, true, func(ctx context.Context) (bool, error) {
				_, err := apiExtensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crdName, metav1.GetOptions{})
				if errors.IsNotFound(err) {
					klog.Infof("CRD %s is deleted on cluster %s", crdName, cluster.Name)
					return true, nil
				}
				if err != nil {
					klog.Warningf("Error getting CRD %s on cluster %s: %v", crdName, cluster.Name, err)
				}
				return false, nil
			})
			if err != nil {
				return fmt.Errorf("failed to wait for CRD %s to be deleted on cluster %s: %w", crdName, cluster.Name, err)
			}
		}
	}

	return nil
}
