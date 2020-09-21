// Copyright (c) 2020 Red Hat, Inc.

package util

import (
	"fmt"

	ocpClientSet "github.com/openshift/client-go/config/clientset/versioned"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateOCPClient creates ocp client
func CreateOCPClient() (ocpClientSet.Interface, error) {
	// create the config from the path
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		log.Error(err, "Failed to create the config")
		return nil, err
	}

	// generate the client based off of the config
	ocpClient, err := ocpClientSet.NewForConfig(config)
	if err != nil {
		log.Error(err, "Failed to create ocp config client")
		return nil, err
	}

	return ocpClient, err
}

func CreateNewK8s(conf *rest.Config) (client.Client, error) {
	kubeClient, err := client.New(conf, client.Options{})
	if err != nil {
		log.Info("Failed to initialize a client connection to the cluster", "error", err.Error())
		return nil, fmt.Errorf("Failed to initialize a client connection to the cluster")
	}
	return kubeClient, nil
}
