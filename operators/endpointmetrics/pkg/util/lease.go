// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"

	"open-cluster-management.io/addon-framework/pkg/lease"
)

const (
	leaseName = "observability-controller" // #nosec G101 -- Not a hardcoded credential.
)

var (
	namespace         = os.Getenv("WATCH_NAMESPACE")
	clusterName       = os.Getenv("HUB_NAMESPACE")
	log               = ctrl.Log.WithName("util")
	hubKubeConfigPath = os.Getenv("HUB_KUBECONFIG")
)

func StartLease() {
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Error(err, "Failed to create incluster config")
		panic(err.Error())
	}
	// creates the clientset
	c, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Error(err, "Failed to create kube client")
		panic(err.Error())
	}

	// create the config from the path
	hubConfig, err := clientcmd.BuildConfigFromFlags("", hubKubeConfigPath)
	if err != nil {
		log.Error(err, "Failed to create the hub config")
		panic(err.Error())
	}

	actual := lease.CheckAddonPodFunc(c.CoreV1(), namespace, "name=endpoint-observability-operator")
	leaseController := lease.NewLeaseUpdater(c, leaseName, namespace, actual).
		WithHubLeaseConfig(hubConfig, clusterName)

	go leaseController.Start(context.TODO())
}
