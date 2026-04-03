// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"slices"
	"testing"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestFindContextForURLs(t *testing.T) {
	makeConfig := func(clusterServer, contextCluster string) *clientcmdapi.Config {
		cfg := clientcmdapi.NewConfig()
		cfg.Clusters["my-cluster"] = &clientcmdapi.Cluster{Server: clusterServer}
		cfg.Contexts["my-context"] = &clientcmdapi.Context{Cluster: contextCluster}
		return cfg
	}

	tests := []struct {
		name          string
		config        *clientcmdapi.Config
		serverURLs    []string
		wantContext   string
		wantServerURL string
		wantOK        bool
	}{
		{
			name:          "exact match",
			config:        makeConfig("https://api.cluster.example.com:6443", "my-cluster"),
			serverURLs:    []string{"https://api.cluster.example.com:6443"},
			wantContext:   "my-context",
			wantServerURL: "https://api.cluster.example.com:6443",
			wantOK:        true,
		},
		{
			name:          "trailing slash on kubeconfig side",
			config:        makeConfig("https://api.cluster.example.com:6443/", "my-cluster"),
			serverURLs:    []string{"https://api.cluster.example.com:6443"},
			wantContext:   "my-context",
			wantServerURL: "https://api.cluster.example.com:6443",
			wantOK:        true,
		},
		{
			name:          "trailing slash on managed cluster side",
			config:        makeConfig("https://api.cluster.example.com:6443", "my-cluster"),
			serverURLs:    []string{"https://api.cluster.example.com:6443/"},
			wantContext:   "my-context",
			wantServerURL: "https://api.cluster.example.com:6443/",
			wantOK:        true,
		},
		{
			name:          "case insensitive",
			config:        makeConfig("HTTPS://API.CLUSTER.EXAMPLE.COM:6443", "my-cluster"),
			serverURLs:    []string{"https://api.cluster.example.com:6443"},
			wantContext:   "my-context",
			wantServerURL: "https://api.cluster.example.com:6443",
			wantOK:        true,
		},
		{
			name:       "no match",
			config:     makeConfig("https://api.other.example.com:6443", "my-cluster"),
			serverURLs: []string{"https://api.cluster.example.com:6443"},
			wantOK:     false,
		},
		{
			name:       "empty URL list",
			config:     makeConfig("https://api.cluster.example.com:6443", "my-cluster"),
			serverURLs: []string{},
			wantOK:     false,
		},
		{
			name:       "context cluster name mismatch",
			config:     makeConfig("https://api.cluster.example.com:6443", "different-cluster"),
			serverURLs: []string{"https://api.cluster.example.com:6443"},
			wantOK:     false,
		},
		{
			name:   "second URL in list matches",
			config: makeConfig("https://api.cluster.example.com:6443", "my-cluster"),
			serverURLs: []string{
				"https://api.other.example.com:6443",
				"https://api.cluster.example.com:6443",
			},
			wantContext:   "my-context",
			wantServerURL: "https://api.cluster.example.com:6443",
			wantOK:        true,
		},
		{
			// Two contexts point at the same cluster server; the lexicographically
			// smaller context name must always be returned regardless of map
			// iteration order.
			name: "two matching contexts — deterministic selection",
			config: func() *clientcmdapi.Config {
				cfg := clientcmdapi.NewConfig()
				cfg.Clusters["my-cluster"] = &clientcmdapi.Cluster{Server: "https://api.cluster.example.com:6443"}
				cfg.Contexts["zzz-context"] = &clientcmdapi.Context{Cluster: "my-cluster"}
				cfg.Contexts["aaa-context"] = &clientcmdapi.Context{Cluster: "my-cluster"}
				return cfg
			}(),
			serverURLs:    []string{"https://api.cluster.example.com:6443"},
			wantContext:   "aaa-context",
			wantServerURL: "https://api.cluster.example.com:6443",
			wantOK:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotContext, gotURL, gotOK := findContextForURLs(tt.config, tt.serverURLs)
			if gotOK != tt.wantOK {
				t.Fatalf("ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotOK {
				if gotContext != tt.wantContext {
					t.Errorf("context = %q, want %q", gotContext, tt.wantContext)
				}
				if gotURL != tt.wantServerURL {
					t.Errorf("serverURL = %q, want %q", gotURL, tt.wantServerURL)
				}
			}
		})
	}
}

func TestBaseDomainFromURL(t *testing.T) {
	tests := []struct {
		name      string
		serverURL string
		want      string
	}{
		{
			name:      "standard OCP API URL",
			serverURL: "https://api.cluster.example.com:6443",
			want:      "cluster.example.com",
		},
		{
			name:      "no port",
			serverURL: "https://api.cluster.example.com",
			want:      "cluster.example.com",
		},
		{
			name:      "http scheme",
			serverURL: "http://api.cluster.example.com:6443",
			want:      "cluster.example.com",
		},
		{
			name:      "no api prefix",
			serverURL: "https://cluster.example.com:6443",
			want:      "cluster.example.com",
		},
		{
			name:      "deep subdomain",
			serverURL: "https://api.sno-cluster.dev07.red-chesterfield.com:6443",
			want:      "sno-cluster.dev07.red-chesterfield.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := baseDomainFromURL(tt.serverURL)
			if got != tt.want {
				t.Errorf("baseDomainFromURL(%q) = %q, want %q", tt.serverURL, got, tt.want)
			}
		})
	}
}

func TestManagedClusterURLs(t *testing.T) {
	tests := []struct {
		name string
		obj  map[string]any
		want []string
	}{
		{
			name: "single URL",
			obj: map[string]any{
				"spec": map[string]any{
					"managedClusterClientConfigs": []any{
						map[string]any{"url": "https://api.cluster.example.com:6443"},
					},
				},
			},
			want: []string{"https://api.cluster.example.com:6443"},
		},
		{
			name: "multiple URLs",
			obj: map[string]any{
				"spec": map[string]any{
					"managedClusterClientConfigs": []any{
						map[string]any{"url": "https://api.cluster1.example.com:6443"},
						map[string]any{"url": "https://api.cluster2.example.com:6443"},
					},
				},
			},
			want: []string{
				"https://api.cluster1.example.com:6443",
				"https://api.cluster2.example.com:6443",
			},
		},
		{
			name: "empty URL filtered out",
			obj: map[string]any{
				"spec": map[string]any{
					"managedClusterClientConfigs": []any{
						map[string]any{"url": ""},
						map[string]any{"url": "https://api.cluster.example.com:6443"},
					},
				},
			},
			want: []string{"https://api.cluster.example.com:6443"},
		},
		{
			name: "missing spec",
			obj:  map[string]any{},
			want: nil,
		},
		{
			name: "missing managedClusterClientConfigs",
			obj: map[string]any{
				"spec": map[string]any{},
			},
			want: nil,
		},
		{
			name: "entry missing url key",
			obj: map[string]any{
				"spec": map[string]any{
					"managedClusterClientConfigs": []any{
						map[string]any{"caBundle": "abc"},
					},
				},
			},
			want: nil,
		},
		{
			name: "non-map entry in configs slice",
			obj: map[string]any{
				"spec": map[string]any{
					"managedClusterClientConfigs": []any{
						"unexpected-string-entry",
						map[string]any{"url": "https://api.cluster.example.com:6443"},
					},
				},
			},
			want: []string{"https://api.cluster.example.com:6443"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := managedClusterURLs("test-cluster", tt.obj)
			if !slices.Equal(got, tt.want) {
				t.Errorf("managedClusterURLs() = %v, want %v", got, tt.want)
			}
		})
	}
}
