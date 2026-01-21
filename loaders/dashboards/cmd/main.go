// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/pflag"
	"github.com/stolostron/multicluster-observability-operator/loaders/dashboards/pkg/controller"
	"k8s.io/klog/v2"
)

func main() {
	klogFlags := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	klog.InitFlags(klogFlags)
	flagset := pflag.NewFlagSet(os.Args[0], pflag.ExitOnError)
	flagset.AddGoFlagSet(klogFlags)

	// use a channel to synchronize the finalization for a graceful shutdown
	stop := make(chan struct{})
	defer close(stop)

	controller.RunGrafanaDashboardController(stop)

	// use a channel to handle OS signals to terminate and gracefully shut
	// down processing
	sigTerm := make(chan os.Signal, 1)
	signal.Notify(sigTerm, syscall.SIGTERM)
	signal.Notify(sigTerm, syscall.SIGINT)
	<-sigTerm
}
