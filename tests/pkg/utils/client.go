// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	ocpClientSet "github.com/openshift/client-go/config/clientset/versioned"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
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
		klog.V(1).Infof("New kubeclient for managedcluster <%v>", opt.ManagedClusters[0].Name)
	}
	return clientKube
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

func GetOCPClient(opt TestOptions) ocpClientSet.Interface {
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		klog.Errorf("Failed to create the config: %v", err)
		panic(err)
	}

	// generate the client based off of the config
	ocpClient, err := ocpClientSet.NewForConfig(config)
	if err != nil {
		klog.Errorf("Failed to create the ocp client: %v", err)
		panic(err)
	}

	return ocpClient
}
