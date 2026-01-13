// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/pflag"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/cache"
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
	tlsCaFile          string
	tlsCertFile        string
	tlsKeyFile         string
}

func main() {
	if err := run(); err != nil {
		klog.Fatalf("failed to run proxy: %v", err)
	}
}

func run() error {
	log.SetLogger(klog.NewKlogr())
	cfg := proxyConf{}

	klogFlags := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	klog.InitFlags(klogFlags)
	flagset := pflag.NewFlagSet(os.Args[0], pflag.ExitOnError)
	flagset.AddGoFlagSet(klogFlags)

	flagset.StringVar(&cfg.listenAddress, "listen-address",
		defaultListenAddress, "The address HTTP server should listen on.")
	flagset.StringVar(&cfg.metricServer, "metrics-server", "",
		"The address the metrics server should run on.")
	flagset.StringVar(&cfg.tlsCaFile, "tls-ca-file", "/var/rbac_proxy/ca/ca.crt", "The path to the CA certificate file for connecting to the downstream server.")
	flagset.StringVar(&cfg.tlsCertFile, "tls-cert-file", "/var/rbac_proxy/certs/tls.crt", "The path to the client certificate file for connecting to the downstream server.")
	flagset.StringVar(&cfg.tlsKeyFile, "tls-key-file", "/var/rbac_proxy/certs/tls.key", "The path to the client key file for connecting to the downstream server.")

	_ = flagset.Parse(os.Args[1:])

	// Kubeconfig flag
	flagset.StringVar(&cfg.kubeconfigLocation, "kubeconfig", "",
		"Path to a kubeconfig file. If unset, in-cluster configuration will be used")

	klog.Infof("proxy server will running on: %s", cfg.listenAddress)
	klog.Infof("metrics server is: %s", cfg.metricServer)
	klog.Infof("kubeconfig is: %s", cfg.kubeconfigLocation)

	// create a context that is canceled on SIGINT/SIGTERM
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	kubeConfig := config.GetConfigOrDie()
	clusterClient, err := clusterclientset.NewForConfig(kubeConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize new cluster clientset: %w", err)
	}

	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize new kubernetes client: %w", err)
	}

	accessReviewer, err := rbac.NewAccessReviewer(kubeConfig, nil)
	if err != nil {
		return fmt.Errorf("failed to create new access reviewer: %w", err)
	}

	// watch all managed clusters
	managedClusterInformer := informer.NewManagedClusterInformer(ctx, clusterClient, kubeClient)
	managedClusterInformer.Run()

	serverURL, err := url.Parse(cfg.metricServer)
	if err != nil {
		return fmt.Errorf("failed to parse metrics server url: %w", err)
	}

	upi := cache.NewUserProjectInfo(ctx, 24*60*60*time.Second, 5*60*time.Second)

	tlsTransport, err := proxy.NewTransport(&proxy.TLSOptions{
		CaFile:          cfg.tlsCaFile,
		KeyFile:         cfg.tlsKeyFile,
		CertFile:        cfg.tlsCertFile,
		PollingInterval: 15 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed to set tls transport: %w", err)
	}
	defer tlsTransport.Close()
	p, err := proxy.NewProxy(kubeConfig, serverURL, tlsTransport, upi, managedClusterInformer, accessReviewer)
	if err != nil {
		return fmt.Errorf("failed to create proxy: %w", err)
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

	// start the server in a goroutine
	go func() {
		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			klog.Fatalf("failed to ListenAndServe: %v", err)
		}
	}()

	// wait for the context to be canceled
	<-ctx.Done()

	// shutdown the server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := s.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	return nil
}
