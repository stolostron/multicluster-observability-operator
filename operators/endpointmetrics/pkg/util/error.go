// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"net"

	"k8s.io/apimachinery/pkg/api/errors"
)

// IsTransientClientErr checks if the error is a transient error
// This suggests that a retry (without any change) might be successful
func IsTransientClientErr(err error) bool {
	if _, ok := err.(net.Error); ok {
		return true
	}

	if statusErr, ok := err.(*errors.StatusError); ok {
		code := statusErr.Status().Code
		if code >= 500 && code < 600 && code != 501 {
			return true
		}
	}

	return errors.IsTimeout(err) || errors.IsServerTimeout(err) || errors.IsTooManyRequests(err)
}
