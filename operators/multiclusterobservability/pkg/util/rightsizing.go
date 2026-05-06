// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	obv1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
)

const (
	// RightSizingCapableAnnotation on the MCO CR indicates MCOA can handle right-sizing.
	RightSizingCapableAnnotation = "observability.open-cluster-management.io/right-sizing-capable"

	// Right-sizing ADC key names — must match KeyPlatform* constants in MCOA repo.
	ADCKeyPlatformNamespaceRightSizing      = "platformNamespaceRightSizing"
	ADCKeyPlatformVirtualizationRightSizing = "platformVirtualizationRightSizing"
)

// IsRightSizingDelegated checks if the MCO CR has the right-sizing delegation
// annotation. When true, MCOA handles right-sizing via ManifestWork instead of
// MCO's Policy-based approach.
func IsRightSizingDelegated(cr *obv1beta2.MultiClusterObservability) bool {
	if cr.Annotations == nil {
		return false
	}
	_, exists := cr.Annotations[RightSizingCapableAnnotation]
	return exists
}
