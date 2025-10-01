// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/observabilityendpoint"

	"go.uber.org/zap/zapcore"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func main() {
	//set env as CMO_SCRIPT_MODE to enable the script mode
	err := os.Setenv("CMO_SCRIPT_MODE", "true")
	if err != nil {
		return
	}
	kubeClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{})
	if err != nil {
		log.Fatalf("unable to create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	opts := zap.Options{Development: true, TimeEncoder: zapcore.ISO8601TimeEncoder}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if err := observabilityendpoint.RevertClusterMonitoringConfig(ctx, kubeClient, nil); err != nil {
		log.Fatalf("unable to revert cluster monitoring config: %v", err)
	}
	log.Println("reverted cluster monitoring config")
	if err := observabilityendpoint.RevertUserWorkloadMonitoringConfig(ctx, kubeClient, nil); err != nil {
		log.Fatalf("unable to revert user workload monitoring config: %v", err)
	}
	log.Println("reverted user workload monitoring config")

	//unset env after the script is done
	err = os.Unsetenv("CMO_SCRIPT_MODE")
	if err != nil {
		return
	}
}
