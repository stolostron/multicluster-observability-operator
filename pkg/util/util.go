// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	logf "sigs.k8s.io/controller-runtime/pkg/log"
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

// Contains is used to check whether a list contains string s
func Contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// GetAnnotation returns the annotation value for a given key, or an empty string if not set
func GetAnnotation(annotations map[string]string, key string) string {
	if annotations == nil {
		return ""
	}
	return annotations[key]
}

// GetReplicaCount returns replicas value.
// return 2 for deployment and 3 for statefulset
func GetReplicaCount(resourceType string) *int32 {
	var replicas2 int32 = 2
	var replicas3 int32 = 3
	if resourceType == "Deployment" {
		return &replicas2
	} else if resourceType == "StatefulSet" {
		return &replicas3
	}
	return &replicas2
}
