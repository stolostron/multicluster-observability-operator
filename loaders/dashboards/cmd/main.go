// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/pflag"
	"github.com/stolostron/multicluster-observability-operator/loaders/dashboards/pkg/controller"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

func main() {
	// Initialize klog flags using a separate FlagSet to avoid polluting the global one.
	klogFlags := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	klog.InitFlags(klogFlags)

	// Set up pflag and bridge the klog flags into it.
	flagset := pflag.NewFlagSet(os.Args[0], pflag.ExitOnError)
	flagset.AddGoFlagSet(klogFlags)

	var grafanaURI string
	flagset.StringVar(&grafanaURI, "grafana-uri", "http://127.0.0.1:3001", "The URI of the Grafana server.")

	// Parse flags
	if err := flagset.Parse(os.Args[1:]); err != nil {
		klog.Fatalf("failed to parse flags: %v", err)
	}

	// Set up context that is canceled on OS signals
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Build kubeconfig and client
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		klog.Fatalf("failed to get cluster config: %v", err)
	}
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("failed to build kubeclient: %v", err)
	}

	// Initialize and run the controller
	c, err := controller.NewGrafanaDashboardController(kubeClient.CoreV1(), grafanaURI)
	if err != nil {
		klog.Fatalf("failed to create controller: %v", err)
	}
	if err := c.Run(ctx); err != nil {
		klog.Fatalf("controller failed: %v", err)
	}
}
