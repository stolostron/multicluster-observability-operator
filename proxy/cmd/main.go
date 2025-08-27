// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"flag"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/spf13/pflag"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/cache"
	proxyconfig "github.com/stolostron/multicluster-observability-operator/proxy/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/informer"
	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/proxy"
	"github.com/stolostron/rbac-api-utils/pkg/rbac"
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

	//Kubeconfig flag
	flagset.StringVar(&cfg.kubeconfigLocation, "kubeconfig", "",
		"Path to a kubeconfig file. If unset, in-cluster configuration will be used")

	klog.Infof("proxy server will running on: %s", cfg.listenAddress)
	klog.Infof("metrics server is: %s", cfg.metricServer)
	klog.Infof("kubeconfig is: %s", cfg.kubeconfigLocation)

	kubeConfig := config.GetConfigOrDie()
	clusterClient, err := clusterclientset.NewForConfig(kubeConfig)
	if err != nil {
		klog.Fatalf("failed to initialize new cluster clientset: %v", err)
	}

	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		klog.Fatalf("failed to initialize new kubernetes client: %v", err)
	}

	_, err = proxyconfig.GetManagedClusterLabelAllowListConfigmap(kubeClient,
		proxyconfig.ManagedClusterLabelAllowListNamespace)

	if err != nil {
		if k8serrors.IsNotFound(err) {
			_ = proxyconfig.CreateManagedClusterLabelAllowListCM(
				proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(),
			)
		}
	}

	accessReviewer, err := rbac.NewAccessReviewer(kubeConfig, nil)
	if err != nil {
		klog.Fatalf("failed to create new access reviewer: %v", err)
	}

	// watch all managed clusters
	managedClusterInformer := informer.NewManagedClusterInformer(clusterClient, kubeClient)
	managedClusterInformer.Run()

	serverURL, err := url.Parse(cfg.metricServer)
	if err != nil {
		klog.Fatalf("failed to parse metrics server url: %v", err)
	}

	upi := cache.NewUserProjectInfo(24*60*60*time.Second, 5*60*time.Second)
	defer upi.Stop()

	tlsTransport, err := proxy.GetTLSTransport()
	if err != nil {
		klog.Fatalf("failed to set tls transport: %v", err)
	}
	p, err := proxy.NewProxy(serverURL, tlsTransport, kubeConfig.Host, upi, managedClusterInformer, accessReviewer)
	if err != nil {
		klog.Fatalf("failed to create proxy: %v", err)
	}

	handlers := http.NewServeMux()
	handlers.Handle("/", p)
	s := http.Server{
		Addr:              cfg.listenAddress,
		Handler:           handlers,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      12 * time.Minute,
	}

	if err := s.ListenAndServe(); err != nil {
		klog.Fatalf("failed to ListenAndServe: %v", err)
	}
}
