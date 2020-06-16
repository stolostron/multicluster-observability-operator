// Copyright (c) 2020 Red Hat, Inc.

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
