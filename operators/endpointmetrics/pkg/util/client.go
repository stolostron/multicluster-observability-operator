// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"fmt"

	oav1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
)

// HubClientWithReload is a wrapper around the hub client that allows reloading the client.
// This is useful when the kubeconfig file is updated.
type HubClientWithReload struct {
	client.Client
	reload func() (client.Client, error)
}

// NewHubClientWithFileReload creates a new hub client with a reload function.
func NewHubClientWithFileReload(filePath string, clientScheme *runtime.Scheme) (*HubClientWithReload, error) {
	reload := func() (client.Client, error) {
		return NewHubClient(filePath, clientScheme)
	}

	hubClient, err := reload()
	if err != nil {
		return nil, fmt.Errorf("failed to create the hub client: %w", err)
	}
	return &HubClientWithReload{Client: hubClient, reload: reload}, nil
}

// NewHubClientWithReloadFunc creates a new hub client with a reload function.
// The reload function is called when the Reload method is called.
// This can be handy for testing purposes.
func NewHubClientWithReloadFunc(reload func() (client.Client, error)) (*HubClientWithReload, error) {
	hubClient, err := reload()
	if err != nil {
		return nil, fmt.Errorf("failed to create the hub client: %w", err)
	}
	return &HubClientWithReload{Client: hubClient, reload: reload}, nil
}

// Reload reloads the hub client and returns a new instance of HubClientWithReload.
// HubClientWithReload is immutable.
func (c *HubClientWithReload) Reload() (*HubClientWithReload, error) {
	hubClient, err := c.reload()
	if err != nil {
		return nil, fmt.Errorf("failed to reload the hub client: %w", err)
	}

	return &HubClientWithReload{Client: hubClient, reload: c.reload}, nil
}

func NewHubClient(filePath string, clientScheme *runtime.Scheme) (client.Client, error) {
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
