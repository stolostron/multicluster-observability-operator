// Copyright (c) 2020 Red Hat, Inc.

package util

import (
	"context"

	routev1 "github.com/openshift/api/route/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	obsAPIGateway = "observatorium-api"
)

// GetObsAPIUrl is used to get the URL for observartium api gateway
func GetObsAPIUrl(client runtimeclient.Client, namespace string) (string, error) {
	found := &routev1.Route{}

	err := client.Get(context.TODO(), types.NamespacedName{Name: obsAPIGateway, Namespace: namespace}, found)
	if err != nil {
		return "", err
	}
	return found.Spec.Host, nil
}
