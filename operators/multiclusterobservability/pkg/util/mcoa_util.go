// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
)

// IsMCOAEnabled returns true if any MCOA capability is enabled.
func IsMCOAEnabled(mco *mcov1beta2.MultiClusterObservability) bool {
	if mco == nil || mco.Spec.Capabilities == nil {
		return false
	}

	if mco.Spec.Capabilities.Platform != nil {
		if mco.Spec.Capabilities.Platform.Logs.Collection.Enabled ||
			mco.Spec.Capabilities.Platform.Metrics.Default.Enabled ||
			mco.Spec.Capabilities.Platform.Analytics.IncidentDetection.Enabled {
			return true
		}
	}

	if mco.Spec.Capabilities.UserWorkloads != nil {
		if mco.Spec.Capabilities.UserWorkloads.Logs.Collection.ClusterLogForwarder.Enabled ||
			mco.Spec.Capabilities.UserWorkloads.Metrics.Default.Enabled {
			return true
		}
	}

	return false
}
