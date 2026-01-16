// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"errors"
	"net"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// IsTransientClientErr checks if the error is a transient error
// This suggests that a retry (without any change) might be successful
func IsTransientClientErr(err error) bool {
	var netError net.Error
	if errors.As(err, &netError) {
		return true
	}

	statusErr := &apierrors.StatusError{}
	if errors.As(err, &statusErr) {
		code := statusErr.Status().Code
		if code >= 500 && code < 600 && code != 501 {
			return true
		}
	}

	return apierrors.IsTimeout(err) || apierrors.IsServerTimeout(err) || apierrors.IsTooManyRequests(err)
}
