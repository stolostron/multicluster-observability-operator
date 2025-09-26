// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"log"
)

func main() {
	//kubeClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{})
	//if err != nil {
	//	log.Fatalf("unable to create client: %v", err)
	//}
	//
	//ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	//defer cancel()
	//
	//opts := zap.Options{Development: true, TimeEncoder: zapcore.ISO8601TimeEncoder}
	//opts.BindFlags(flag.CommandLine)
	//flag.Parse()
	//ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	//if err := observabilityendpoint.RevertClusterMonitoringConfig(ctx, kubeClient); err != nil {
	//	log.Fatalf("unable to revert cluster monitoring config: %v", err)
	//}
	//log.Println("reverted cluster monitoring config")
	//if err := observabilityendpoint.RevertUserWorkloadMonitoringConfig(ctx, kubeClient); err != nil {
	//	log.Fatalf("unable to revert user workload monitoring config: %v", err)
	//}
	log.Println("reverted user workload monitoring config")

}
