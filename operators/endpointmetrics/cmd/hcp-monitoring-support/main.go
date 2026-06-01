// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/hypershift"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func main() {
	setup := flag.Bool("setup", false, "Create/Update ServiceMonitors for Hosted Clusters")
	cleanup := flag.Bool("cleanup", false, "Delete ServiceMonitors for Hosted Clusters")
	autoApprove := flag.Bool("auto-approve", false, "Automatically approve all actions")
	dryRun := flag.Bool("dry-run", false, "Display actions without executing them")
	flag.Parse()

	if !*setup && !*cleanup {
		fmt.Fprintln(os.Stderr, "Error: Either --setup or --cleanup must be specified.")
		flag.Usage()
		os.Exit(1)
	}

	if *setup && *cleanup {
		fmt.Fprintln(os.Stderr, "Error: --setup and --cleanup cannot be used together.")
		flag.Usage()
		os.Exit(1)
	}

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(promv1.AddToScheme(scheme))
	utilruntime.Must(hyperv1.AddToScheme(scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))

	ctx := context.Background()
	cfg, err := config.GetConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get kubeconfig: %v\n", err)
		os.Exit(1)
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create client: %v\n", err)
		os.Exit(1)
	}

	// Check for HyperShift CRD
	crd := &apiextensionsv1.CustomResourceDefinition{}
	err = c.Get(ctx, client.ObjectKey{Name: hypershift.HostedClusterCRDName}, crd)
	if err != nil {
		if errors.IsNotFound(err) {
			fmt.Fprintln(os.Stderr, "Error: This cluster does not appear to be a HyperShift cluster (HostedCluster CRD not found).")
		} else {
			fmt.Fprintf(os.Stderr, "Failed to check for HyperShift CRD: %v\n", err)
		}
		os.Exit(1)
	}

	if *dryRun {
		c = client.NewDryRunClient(c)
		fmt.Println("--- DRY RUN MODE ENABLED ---")
	}

	if *setup {
		if !confirmAction("This will create/update ACM ServiceMonitors for all HostedClusters. Proceed?", *autoApprove, *dryRun) {
			return
		}
		fmt.Println("Scanning for HostedClusters...")
		count, err := hypershift.ReconcileHostedClustersServiceMonitors(ctx, c)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Setup failed: %v\n", err)
			os.Exit(1)
		}
		if *dryRun {
			fmt.Printf("[Dry-run] Would have reconciled ServiceMonitors for %d HostedClusters.\n", count)
		} else {
			fmt.Printf("Successfully reconciled ServiceMonitors for %d HostedClusters.\n", count)
		}
	}

	if *cleanup {
		if !confirmAction("This will delete ACM ServiceMonitors for all HostedClusters. Proceed?", *autoApprove, *dryRun) {
			return
		}
		err := hypershift.DeleteServiceMonitors(ctx, c)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cleanup failed: %v\n", err)
			os.Exit(1)
		}
		if *dryRun {
			fmt.Println("[Dry-run] Would have deleted ACM ServiceMonitors.")
		} else {
			fmt.Println("Successfully deleted ACM ServiceMonitors.")
		}
	}
}

func confirmAction(message string, autoApprove, dryRun bool) bool {
	if autoApprove || dryRun {
		return true
	}

	fmt.Printf("%s (y/n): ", message)
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	response = strings.ToLower(strings.TrimSpace(response))
	if response == "y" || response == "yes" {
		return true
	}

	fmt.Println("Aborted.")
	return false
}
