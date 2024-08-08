// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package collector

import (
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func evaluateMatchExpression(expr metav1.LabelSelectorRequirement, key, value string) bool {
	if expr.Key != key {
		return false
	}

	switch expr.Operator {
	case metav1.LabelSelectorOpIn:
		return slices.Contains(expr.Values, value)
	case metav1.LabelSelectorOpNotIn:
		return !slices.Contains(expr.Values, value)
	default:
		return false
	}
}
