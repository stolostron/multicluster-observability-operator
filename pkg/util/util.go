// Copyright (c) 2020 Red Hat, Inc.

package util

import (
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
)

var log = logf.Log.WithName("util")

// Remove is used to remove string from a string array
func Remove(list []string, s string) []string {
	result := []string{}
	for _, v := range list {
		if v != s {
			result = append(result, v)
		}
	}
	return result
}

// GetAnnotation returns the annotation value for a given key, or an empty string if not set
func GetAnnotation(instance *mcov1beta1.MultiClusterObservability, key string) string {
	a := instance.GetAnnotations()
	if a == nil {
		return ""
	}
	return a[key]
}
