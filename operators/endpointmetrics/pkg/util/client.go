// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project.
package util

import (
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oav1beta1 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	ocpClientSet "github.com/openshift/client-go/config/clientset/versioned"
)

var (
	hubClient client.Client
	ocpClient ocpClientSet.Interface
)

var (
	log = ctrl.Log.WithName("util")
)

const (
	hubKubeConfigPath = "/spoke/hub-kubeconfig/kubeconfig"
)

// GetOrCreateOCPClient get an existing hub client or create new one if it doesn't exist
func GetOrCreateHubClient() (client.Client, error) {
	if hubClient != nil {
		return hubClient, nil
	}
	// create the config from the path
	config, err := clientcmd.BuildConfigFromFlags("", hubKubeConfigPath)
	if err != nil {
		log.Error(err, "Failed to create the config")
		return nil, err
	}

	s := scheme.Scheme
	if err := oav1beta1.AddToScheme(s); err != nil {
		return nil, err
	}

	// generate the client based off of the config
	hubClient, err := client.New(config, client.Options{Scheme: s})

	if err != nil {
		log.Error(err, "Failed to create hub client")
		return nil, err
	}

	return hubClient, err
}

// GetOrCreateOCPClient get an existing ocp client or create new one if it doesn't exist
func GetOrCreateOCPClient() (ocpClientSet.Interface, error) {
	if ocpClient != nil {
		return ocpClient, nil
	}
	// create the config from the path
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		log.Error(err, "Failed to create the config")
		return nil, err
	}

	// generate the client based off of the config
	ocpClient, err = ocpClientSet.NewForConfig(config)
	if err != nil {
		log.Error(err, "Failed to create ocp config client")
		return nil, err
	}

	return ocpClient, err
}
