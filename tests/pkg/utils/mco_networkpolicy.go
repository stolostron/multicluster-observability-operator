// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// spec.networkPolicies.enabled is true.
var spokeEndpointNetworkPolicyNames = []string{
	"endpoint-observability-operator",
	"metrics-collector",
	"uwl-metrics-collector",
}

// NetworkPolicies on the managed cluster addon namespace.
func CheckSpokeNetworkPolicies(opt TestOptions, expectPresent bool) error {
	spokes, err := GetAvailableManagedClustersAsClusters(opt)
	if err != nil {
		return err
	}
	if len(spokes) == 0 {
		return nil
	}
	for _, spoke := range spokes {
		client := GetKubeClientWithCluster(spoke)
		for _, name := range spokeEndpointNetworkPolicyNames {
			_, err := client.NetworkingV1().NetworkPolicies(MCO_ADDON_NAMESPACE).
				Get(context.TODO(), name, metav1.GetOptions{})
			if expectPresent {
				if err != nil {
					return fmt.Errorf("expected NetworkPolicy %s/%s on managed cluster: %w",
						MCO_ADDON_NAMESPACE, name, err)
				}
				continue
			}
			if err == nil {
				return fmt.Errorf("NetworkPolicy %s/%s still present on managed cluster",
					MCO_ADDON_NAMESPACE, name)
			}
			if !k8serrors.IsNotFound(err) {
				return fmt.Errorf("failed to get NetworkPolicy %s/%s: %w",
					MCO_ADDON_NAMESPACE, name, err)
			}
		}
	}
	return nil
}
