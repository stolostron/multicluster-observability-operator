// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func DeleteMonitoringCRDs(opt TestOptions) error {
	clusters := append([]Cluster{opt.HubCluster}, opt.ManagedClusters...)
	for _, cluster := range clusters {
		apiExtensionsClient := NewKubeClientAPIExtension(cluster.ClusterServerURL, opt.KubeConfig, cluster.KubeContext)

		crds, err := apiExtensionsClient.ApiextensionsV1().CustomResourceDefinitions().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}

		for _, crd := range crds.Items {
			if crd.Spec.Group == "monitoring.rhobs" {
				err := apiExtensionsClient.ApiextensionsV1().CustomResourceDefinitions().Delete(context.TODO(), crd.Name, metav1.DeleteOptions{})
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}
