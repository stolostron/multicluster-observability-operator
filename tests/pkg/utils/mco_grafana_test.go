// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"os"
	"testing"
)

func TestGetGrafanaHost(t *testing.T) {
	// Ensure isolated test runs
	defer ResetCachedGrafanaHost()

	tests := []struct {
		name          string
		opt           TestOptions
		cloudProvider string
		cachedHost    string
		expected      string
	}{
		{
			name: "explicit GrafanaHost takes precedence",
			opt: TestOptions{
				HubCluster: Cluster{
					GrafanaHost: "explicit-grafana.apps.example.com",
					BaseDomain:  "example.com",
				},
			},
			expected: "explicit-grafana.apps.example.com",
		},
		{
			name: "extracts host from explicit GrafanaURL",
			opt: TestOptions{
				HubCluster: Cluster{
					GrafanaURL: "https://url-grafana.apps.example.com",
					BaseDomain: "example.com",
				},
			},
			expected: "url-grafana.apps.example.com",
		},
		{
			name: "uses cached host when available",
			opt: TestOptions{
				HubCluster: Cluster{
					BaseDomain: "different-domain.com",
				},
			},
			cachedHost: "cached-grafana.apps.example.com",
			expected:   "cached-grafana.apps.example.com",
		},
		{
			name: "fallback to BaseDomain heuristics standard openshift",
			opt: TestOptions{
				HubCluster: Cluster{
					BaseDomain: "my-cluster.example.com",
				},
			},
			expected: "grafana-open-cluster-management-observability.apps.my-cluster.example.com",
		},
		{
			name: "fallback to BaseDomain heuristics for rosa hcp",
			opt: TestOptions{
				HubCluster: Cluster{
					BaseDomain: "my-rosa.example.com",
				},
			},
			cloudProvider: "rosa,hcp",
			expected:      "grafana-open-cluster-management-observability.apps.rosa.my-rosa.example.com",
		},
		{
			name: "empty options returns empty string",
			opt: TestOptions{
				HubCluster: Cluster{},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ResetCachedGrafanaHost()
			if tt.cachedHost != "" {
				grafanaHostMutex.Lock()
				cachedGrafanaHost = tt.cachedHost
				grafanaHostMutex.Unlock()
			}

			if tt.cloudProvider != "" {
				orig := os.Getenv("CLOUD_PROVIDER")
				_ = os.Setenv("CLOUD_PROVIDER", tt.cloudProvider)
				defer func() { _ = os.Setenv("CLOUD_PROVIDER", orig) }()
			}

			// Ensure kind env is true so unit test doesn't attempt cluster route lookup
			origKind := os.Getenv("IS_KIND_ENV")
			_ = os.Setenv("IS_KIND_ENV", "true")
			defer func() { _ = os.Setenv("IS_KIND_ENV", origKind) }()

			got := GetGrafanaHost(tt.opt)
			if got != tt.expected {
				t.Errorf("GetGrafanaHost() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestGetGrafanaURL(t *testing.T) {
	defer ResetCachedGrafanaHost()

	tests := []struct {
		name     string
		opt      TestOptions
		expected string
	}{
		{
			name: "returns explicit GrafanaURL when configured",
			opt: TestOptions{
				HubCluster: Cluster{
					GrafanaURL:  "https://custom-grafana.apps.example.com:8443",
					GrafanaHost: "ignored-due-to-url.apps.example.com",
				},
			},
			expected: "https://custom-grafana.apps.example.com:8443",
		},
		{
			name: "constructs https URL from GrafanaHost",
			opt: TestOptions{
				HubCluster: Cluster{
					GrafanaHost: "explicit-grafana.apps.example.com",
				},
			},
			expected: "https://explicit-grafana.apps.example.com",
		},
		{
			name: "constructs https URL from BaseDomain",
			opt: TestOptions{
				HubCluster: Cluster{
					BaseDomain: "my-cluster.example.com",
				},
			},
			expected: "https://grafana-open-cluster-management-observability.apps.my-cluster.example.com",
		},
		{
			name: "returns empty string when no host can be resolved",
			opt: TestOptions{
				HubCluster: Cluster{},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ResetCachedGrafanaHost()
			origKind := os.Getenv("IS_KIND_ENV")
			_ = os.Setenv("IS_KIND_ENV", "true")
			defer func() { _ = os.Setenv("IS_KIND_ENV", origKind) }()

			got := GetGrafanaURL(tt.opt)
			if got != tt.expected {
				t.Errorf("GetGrafanaURL() = %q, want %q", got, tt.expected)
			}
		})
	}
}
