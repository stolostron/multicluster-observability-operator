// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package main

import (
	"flag"
	"net/http"
	"os"

	"github.com/spf13/pflag"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/open-cluster-management/multicluster-observability-operator/proxy/pkg/proxy"
	"github.com/open-cluster-management/multicluster-observability-operator/proxy/pkg/util"
	clusterclientset "open-cluster-management.io/api/client/cluster/clientset/versioned"
)

const (
	defaultListenAddress = "0.0.0.0:3002"
)

type proxyConf struct {
	listenAddress      string
	metricServer       string
	kubeconfigLocation string
}

func main() {

	cfg := proxyConf{}

	klogFlags := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	klog.InitFlags(klogFlags)
	flagset := pflag.NewFlagSet(os.Args[0], pflag.ExitOnError)
	flagset.AddGoFlagSet(klogFlags)

	flagset.StringVar(&cfg.listenAddress, "listen-address",
		defaultListenAddress, "The address HTTP server should listen on.")
	flagset.StringVar(&cfg.metricServer, "metrics-server", "",
		"The address the metrics server should run on.")

	_ = flagset.Parse(os.Args[1:])
	if err := os.Setenv("METRICS_SERVER", cfg.metricServer); err != nil {
		klog.Fatalf("failed to Setenv: %v", err)
	}

	//Kubeconfig flag
	flagset.StringVar(&cfg.kubeconfigLocation, "kubeconfig", "",
		"Path to a kubeconfig file. If unset, in-cluster configuration will be used")

	klog.Infof("proxy server will running on: %s", cfg.listenAddress)
	klog.Infof("metrics server is: %s", cfg.metricServer)
	klog.Infof("kubeconfig is: %s", cfg.kubeconfigLocation)

	clusterClient, err := clusterclientset.NewForConfig(config.GetConfigOrDie())
	if err != nil {
		klog.Fatalf("failed to new cluster clientset: %v", err)
	}

	// watch all managed clusters
	go util.WatchManagedCluster(clusterClient)
	go util.CleanExpiredProjectInfo(24 * 60 * 60)

	http.HandleFunc("/", proxy.HandleRequestAndRedirect)
	if err := http.ListenAndServe(cfg.listenAddress, nil); err != nil {
		klog.Fatalf("failed to ListenAndServe: %v", err)
	}
}
