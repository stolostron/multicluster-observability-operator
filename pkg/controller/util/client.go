// Copyright (c) 2020 Red Hat, Inc.

package util

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func CreateKubeClient() (kubernetes.Interface, error) {
	// create the config from the path
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		return nil, err
	}

	// generate the client based off of the config
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return kubeClient, err
}
