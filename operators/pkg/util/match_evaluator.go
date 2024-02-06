// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"golang.org/x/exp/slices"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"regexp"
)

type evaluateFn func(metav1.LabelSelectorRequirement, ...interface{}) bool

var evaluateFns = map[string]evaluateFn{
	"clusterType": evaluateClusterType,
}

func EvaluateMatchExpression(expr metav1.LabelSelectorRequirement, params ...interface{}) bool {
	if _, ok := evaluateFns[expr.Key]; !ok {
		// return false if expr.key not defined
		return false
	}
	return evaluateFns[expr.Key](expr, params...)
}

func evaluateClusterType(expr metav1.LabelSelectorRequirement, params ...interface{}) bool {
	switch expr.Operator {
	case metav1.LabelSelectorOpIn:
		return slices.Contains(expr.Values, params[1].(string))
	case metav1.LabelSelectorOpNotIn:
		return !slices.Contains(expr.Values, params[1].(string))
	default:
		// return false for unsupported/invalid operator
		return false
	}
}

func GetNameInMatch(match string) string {
	r := regexp.MustCompile(`__name__="([^,]*)"`)
	m := r.FindAllStringSubmatch(match, -1)
	if m != nil {
		return m[0][1]
	}
	return ""
}
