// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsvirtualization

import (
	"context"

	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability/analytics/rs-utility"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateUpdateVirtualizationPlacement creates the Placement resource for virtualization
func CreateUpdateVirtualizationPlacement(ctx context.Context, c client.Client, placementConfig clusterv1beta1.Placement) error {
	return rsutility.CreateUpdateRSPlacement(ctx, c, PlacementName, Namespace, placementConfig)
}
