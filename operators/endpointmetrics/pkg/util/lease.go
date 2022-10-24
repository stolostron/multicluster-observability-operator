// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project.

package util

import (
	"context"
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/lease"
)

const (
	leaseName = "observability-controller"
)

var (
	namespace   = os.Getenv("WATCH_NAMESPACE")
	clusterName = os.Getenv("HUB_NAMESPACE")
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

	actual := lease.CheckAddonPodFunc(c.CoreV1(), namespace, "name=endpoint-observability-operator")
	leaseController := lease.NewLeaseUpdater(c, leaseName, namespace, actual).
		WithHubLeaseConfig(hubKubeConfigPath, clusterName)

	go leaseController.Start(context.TODO())
}
