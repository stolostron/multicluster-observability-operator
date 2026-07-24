// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestLoadConfigErrorHandling(t *testing.T) {
	// 1. If we provide an invalid kubeconfig path, BuildConfigFromFlags should fail.
	// The error returned should contain "failed to build config from flags" as context.
	_, err := LoadConfig("", "/non/existent/path/to/kubeconfig", "")
	if err == nil {
		t.Fatal("expected an error when providing a non-existent kubeconfig path, got nil")
	}

	if !strings.Contains(err.Error(), "failed to build config from flags") {
		t.Errorf("expected error to contain operation context, got: %v", err)
	}

	// 2. If kubeconfig and KUBECONFIG env var are empty, and InClusterConfig fails,
	// it will fall back to user.Current() and try building from default config.
	origKubeconfig := os.Getenv("KUBECONFIG")
	os.Setenv("KUBECONFIG", "")
	defer os.Setenv("KUBECONFIG", origKubeconfig)

	_, err2 := LoadConfig("", "", "")
	if err2 == nil {
		// It might succeed if the default ~/.kube/config is valid, which is fine.
	} else {
		if !strings.Contains(err2.Error(), "failed to build config from flags") && !strings.Contains(err2.Error(), "failed to get current user") {
			t.Errorf("expected error to contain fallback context, got: %v", err2)
		}
	}
}

func TestIsTransientAddonError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error is not transient",
			err:      nil,
			expected: false,
		},
		{
			name:     "standard non-transient error",
			err:      errors.New("some regular error"),
			expected: false,
		},
		{
			name:     "context.DeadlineExceeded is transient",
			err:      context.DeadlineExceeded,
			expected: true,
		},
		{
			name:     "wrapped context.DeadlineExceeded is transient",
			err:      fmt.Errorf("failed during execution: %w", context.DeadlineExceeded),
			expected: true,
		},
		{
			name:     "apierrors.IsTooManyRequests is transient",
			err:      apierrors.NewTooManyRequests("too many requests", 10),
			expected: true,
		},
		{
			name:     "apierrors.IsNotFound is transient",
			err:      apierrors.NewNotFound(schema.GroupResource{}, "addon"),
			expected: true,
		},
		{
			name:     "apierrors.IsTimeout is transient",
			err:      apierrors.NewTimeoutError("timeout", 10),
			expected: true,
		},
		{
			name:     "apierrors.IsServerTimeout is transient",
			err:      apierrors.NewServerTimeout(schema.GroupResource{}, "server timeout", 10),
			expected: true,
		},
		{
			name:     "apierrors.IsBadRequest is not transient",
			err:      apierrors.NewBadRequest("bad request"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTransientAddonError(tt.err)
			if got != tt.expected {
				t.Errorf("isTransientAddonError() = %v, expected %v", got, tt.expected)
			}
		})
	}
}
