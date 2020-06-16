// Copyright (c) 2020 Red Hat, Inc.

package util

import (
	ocpClientSet "github.com/openshift/client-go/config/clientset/versioned"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// CreateKubeClient creates kube client
func CreateKubeClient() (kubernetes.Interface, error) {
	// create the config from the path
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		log.Error(err, "Failed to create the config")
		return nil, err
	}

	// generate the client based off of the config
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Error(err, "Failed to create kube client")
		return nil, err
	}

	return kubeClient, err
}

// CreateOCPClient creates kocp client
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
