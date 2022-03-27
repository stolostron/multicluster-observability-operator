// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project.
package observabilityendpoint

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
)

type evaluateFn func(metav1.LabelSelectorRequirement, ...interface{}) bool

var evaluateFns = map[string]evaluateFn{
	"clusterType": evaluateClusterType,
}

func evluateMatchExpression(expr metav1.LabelSelectorRequirement, params ...interface{}) bool {
	if _, ok := evaluateFns[expr.Key]; !ok {
		// return false if expr.key not defined
		return false
	}
	return evaluateFns[expr.Key](expr, params...)
}

func evaluateClusterType(expr metav1.LabelSelectorRequirement, params ...interface{}) bool {
	switch expr.Operator {
	case metav1.LabelSelectorOpIn:
		return util.Contains(expr.Values, params[1].(string))
	case metav1.LabelSelectorOpNotIn:
		return !util.Contains(expr.Values, params[1].(string))
	default:
		// return false for unsupported/invalid operator
		return false
	}
}
