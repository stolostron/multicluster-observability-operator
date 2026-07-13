// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/observabilityendpoint"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func main() {
	var hubID string
	var clusterName string
	flag.StringVar(&hubID, "hub-id", "", "The ID of the Hub cluster to revert configuration for (optional).")
	flag.StringVar(&clusterName, "cluster-name", "", "The name of the managed cluster to revert configuration for (optional).")

	klog.InitFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(klog.NewKlogr())

	if err := run(hubID, clusterName); err != nil {
		log.Fatalf("revert offline cleanup failed: %v", err)
	}
	log.Println("revert offline cleanup completed successfully")
}

func run(hubID string, clusterName string) error {
	kubeClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{})
	if err != nil {
		return fmt.Errorf("unable to create client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	caSecret := observabilityendpoint.AppendHubClusterID(observabilityendpoint.HubAmRouterCASecretName, hubID)
	mtlsSecret := observabilityendpoint.AppendHubClusterID(observabilityendpoint.HubAmMtlsCASecretName, hubID)

	// Revert the legacy Router CA configurations if present
	if err := observabilityendpoint.RevertClusterMonitoringConfig(ctx, kubeClient, caSecret, clusterName); err != nil {
		return fmt.Errorf("unable to revert cluster monitoring config (Router CA): %w", err)
	}
	if err := observabilityendpoint.RevertUserWorkloadMonitoringConfig(ctx, kubeClient, caSecret); err != nil {
		return fmt.Errorf("unable to revert user workload monitoring config (Router CA): %w", err)
	}

	// Revert the new mTLS CA configurations if present
	if err := observabilityendpoint.RevertClusterMonitoringConfig(ctx, kubeClient, mtlsSecret, clusterName); err != nil {
		return fmt.Errorf("unable to revert cluster monitoring config (mTLS CA): %w", err)
	}
	if err := observabilityendpoint.RevertUserWorkloadMonitoringConfig(ctx, kubeClient, mtlsSecret); err != nil {
		return fmt.Errorf("unable to revert user workload monitoring config (mTLS CA): %w", err)
	}

	return nil
}
