// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	"os"

	oav1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ocpClientSet "github.com/openshift/client-go/config/clientset/versioned"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
)

const (
	obAddonName = "observability-addon"
)

var (
	hubClient client.Client
	ocpClient ocpClientSet.Interface
)

var (
	log               = ctrl.Log.WithName("util")
	hubKubeConfigPath = os.Getenv("HUB_KUBECONFIG")
)

// GetOrCreateOCPClient get an existing hub client or create new one if it doesn't exist.
func GetOrCreateHubClient(renew bool, clientScheme *runtime.Scheme) (client.Client, error) {
	if os.Getenv("UNIT_TEST") == "true" {
		return hubClient, nil
	}

	if !renew && hubClient != nil {
		return hubClient, nil
	}
	// create the config from the path
	config, err := clientcmd.BuildConfigFromFlags("", hubKubeConfigPath)
	if err != nil {
		log.Error(err, "Failed to create the config")
		return nil, err
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
		log.Error(err, "Failed to create hub client")
		return nil, err
	}

	return hubClient, err
}

// GetOrCreateOCPClient get an existing ocp client or create new one if it doesn't exist.
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

func SetHubClient(c client.Client) {
	hubClient = c
}

func RenewAndRetry(ctx context.Context, scheme *runtime.Scheme) (client.Client, *oav1beta1.ObservabilityAddon, error) {
	// try to renew the hub client
	log.Info("renew hub client")
	hubClient, err := GetOrCreateHubClient(true, scheme)
	if err != nil {
		log.Error(err, "Failed to create the hub client")
		return nil, nil, err
	}

	hubObsAddon := &oav1beta1.ObservabilityAddon{}
	hubNamespace := os.Getenv("HUB_NAMESPACE")
	err = hubClient.Get(ctx, types.NamespacedName{Name: obAddonName, Namespace: hubNamespace}, hubObsAddon)
	if err != nil {
		log.Error(err, "Failed to get observabilityaddon in hub cluster", "namespace", hubNamespace)
		return nil, nil, err
	}

	return hubClient, hubObsAddon, nil
}
