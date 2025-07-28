// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsvirtualization

import (
	"context"

	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability/analytics/rs-utility"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateVirtualizationPlacementBinding creates the PlacementBinding resource for virtualization
func CreateVirtualizationPlacementBinding(ctx context.Context, c client.Client) error {
	return rsutility.CreateRSPlacementBinding(ctx, c, PlacementBindingName, Namespace, PlacementName, PrometheusRulePolicyName)
}
