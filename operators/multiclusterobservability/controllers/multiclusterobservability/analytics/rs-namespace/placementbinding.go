// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsnamespace

import (
	"context"

	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability/analytics/rs-utility"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreatePlacementBinding creates the PlacementBinding resource
func CreatePlacementBinding(ctx context.Context, c client.Client) error {
	return rsutility.CreateRSPlacementBinding(ctx, c, PlacementBindingName, Namespace, PlacementName, PrometheusRulePolicyName)
}
