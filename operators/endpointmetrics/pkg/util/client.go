// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"fmt"

	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	oav1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReloadableHubClient is a wrapper around the hub client that allows reloading the client.
// This is useful when the kubeconfig file is updated.
type ReloadableHubClient struct {
	client.Client

	reload func() (client.Client, error)
}

// NewReloadableHubClient creates a new hub client with a reload function.
func NewReloadableHubClient(filePath string, clientScheme *runtime.Scheme) (*ReloadableHubClient, error) {
	reload := func() (client.Client, error) {
		return newHubClient(filePath, clientScheme)
	}

	hubClient, err := reload()
	if err != nil {
		return nil, fmt.Errorf("failed to create the hub client: %w", err)
	}
	return &ReloadableHubClient{Client: hubClient, reload: reload}, nil
}

// NewReloadableHubClientWithReloadFunc creates a new hub client with a reload function.
// The reload function is called when the Reload method is called.
// This can be handy for testing purposes.
func NewReloadableHubClientWithReloadFunc(reload func() (client.Client, error)) (*ReloadableHubClient, error) {
	hubClient, err := reload()
	if err != nil {
		return nil, fmt.Errorf("failed to create the hub client: %w", err)
	}
	return &ReloadableHubClient{Client: hubClient, reload: reload}, nil
}

// Reload reloads the hub client and returns a new instance of HubClientWithReload.
// HubClientWithReload is immutable.
func (c *ReloadableHubClient) Reload() (*ReloadableHubClient, error) {
	hubClient, err := c.reload()
	if err != nil {
		return nil, fmt.Errorf("failed to reload the hub client: %w", err)
	}

	return &ReloadableHubClient{Client: hubClient, reload: c.reload}, nil
}

func newHubClient(filePath string, clientScheme *runtime.Scheme) (client.Client, error) {
	// create the config from the path
	config, err := clientcmd.BuildConfigFromFlags("", filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create the config: %w", err)
	}

	if clientScheme == nil {
		clientScheme := scheme.Scheme
		if err := oav1beta1.AddToScheme(clientScheme); err != nil {
			return nil, err
		}
		if err := oav1beta2.AddToScheme(clientScheme); err != nil {
			return nil, err
		}
	}

	// generate the client based off of the config
	hubClient, err := client.New(config, client.Options{Scheme: clientScheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create hub client: %w", err)
	}

	return hubClient, nil
}
