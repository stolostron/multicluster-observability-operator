// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

func getKubeClient(opt TestOptions, isHub bool) kubernetes.Interface {
	clientKube := NewKubeClient(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)
	if !isHub && len(opt.ManagedClusters) > 0 {
		clientKube = NewKubeClient(
			opt.ManagedClusters[0].ClusterServerURL,
			opt.ManagedClusters[0].KubeConfig,
			opt.ManagedClusters[0].KubeContext)
	}
	return clientKube
}

func getKubeClientForCluster(clusterConfig Cluster, isHub bool) kubernetes.Interface {
	return NewKubeClient(
		clusterConfig.ClusterServerURL,
		clusterConfig.KubeConfig,
		clusterConfig.KubeContext)
}

func GetKubeClientDynamic(opt TestOptions, isHub bool) dynamic.Interface {
	url := opt.HubCluster.ClusterServerURL
	kubeConfig := opt.KubeConfig
	kubeContext := opt.HubCluster.KubeContext
	if !isHub && len(opt.ManagedClusters) > 0 {
		url = opt.ManagedClusters[0].ClusterServerURL
		kubeConfig = opt.ManagedClusters[0].KubeConfig
		kubeContext = opt.ManagedClusters[0].KubeContext
	}

	config, err := LoadConfig(url, kubeConfig, kubeContext)
	if err != nil {
		panic(err)
	}

	clientset, err := dynamic.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	return clientset
}

func GetManagedClusterName(opt TestOptions) string {
	if len(opt.ManagedClusters) > 0 {
		return opt.ManagedClusters[0].Name
	}
	return ""
}
