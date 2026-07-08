// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rightsizing

import (
	"context"
	"errors"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/analytics/rightsizing/rs-utility"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("analytics")

// CleanupRightSizingResources removes all right-sizing resources during MCO CR deletion.
// Uses a two-pass hybrid approach:
//  1. Label-based: catches all labeled resources across any namespace.
//  2. Name-based: catches unlabeled resources from pre-2.17 (2.16) installations.
func CleanupRightSizingResources(ctx context.Context, c client.Client, mco *mcov1beta2.MultiClusterObservability) error {
	log.Info("rs - cleaning up all right-sizing resources")

	// Error only occurs for unknown component types, which can't happen with compile-time constants.
	_, nsNS, _ := rsutility.GetComponentConfig(mco, rsutility.ComponentTypeNamespace)
	if nsNS == "" {
		nsNS = rsutility.DefaultNamespace
	}
	_, virtNS, _ := rsutility.GetComponentConfig(mco, rsutility.ComponentTypeVirtualization)
	if virtNS == "" {
		virtNS = rsutility.DefaultNamespace
	}

	return errors.Join(
		rsutility.CleanupRSResourcesByLabel(ctx, c),
		rsutility.CleanupLegacyPolicyResourcesByName(ctx, c, nsNS, virtNS),
	)
}

// CleanupLegacyPolicyResources removes Policy-based right-sizing resources left from pre-GA installations.
// ConfigMaps are PRESERVED because MCOA reuses them for per-cluster configuration.
// Uses a two-pass approach: label-based (2.17 resources) + name-based (potentially unlabeled 2.16 resources).
func CleanupLegacyPolicyResources(ctx context.Context, c client.Client, mco *mcov1beta2.MultiClusterObservability) error {
	// Error only occurs for unknown component types, which can't happen with compile-time constants.
	_, nsNS, _ := rsutility.GetComponentConfig(mco, rsutility.ComponentTypeNamespace)
	if nsNS == "" {
		nsNS = rsutility.DefaultNamespace
	}
	_, virtNS, _ := rsutility.GetComponentConfig(mco, rsutility.ComponentTypeVirtualization)
	if virtNS == "" {
		virtNS = rsutility.DefaultNamespace
	}

	log.Info("rs - cleaning up legacy Policy resources for upgrade migration (preserving ConfigMaps)",
		"nsNamespace", nsNS, "virtNamespace", virtNS)

	return errors.Join(
		rsutility.CleanupLegacyPolicyResourcesByLabel(ctx, c),
		rsutility.CleanupLegacyPolicyResourcesByName(ctx, c, nsNS, virtNS),
	)
}
