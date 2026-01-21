// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/observabilityendpoint"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func main() {
	kubeClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{})
	if err != nil {
		log.Fatalf("unable to create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	klog.InitFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(klog.NewKlogr())

	if err := observabilityendpoint.RevertClusterMonitoringConfig(ctx, kubeClient, nil); err != nil {
		log.Printf("unable to revert cluster monitoring config: %v", err)
		return
	}
	log.Println("reverted cluster monitoring config")
	if err := observabilityendpoint.RevertUserWorkloadMonitoringConfig(ctx, kubeClient, nil); err != nil {
		log.Printf("unable to revert user workload monitoring config: %v", err)
		return
	}
	log.Println("reverted user workload monitoring config")
}
