// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

var (
	cachedGrafanaHost string
	grafanaHostMutex  sync.Mutex
)

// ResetCachedGrafanaHost resets the cached Grafana host (useful in unit tests).
func ResetCachedGrafanaHost() {
	grafanaHostMutex.Lock()
	defer grafanaHostMutex.Unlock()
	cachedGrafanaHost = ""
}

// GetGrafanaHost resolves the hostname for the Grafana service.
// Precedence:
// 1. Explicit opt.HubCluster.GrafanaHost
// 2. Host extracted from opt.HubCluster.GrafanaURL (if set)
// 3. Cached host from previous discovery
// 4. Dynamic discovery from the OpenShift Route in MCO_NAMESPACE
// 5. Fallback heuristics using BaseDomain
func GetGrafanaHost(opt TestOptions) string {
	if opt.HubCluster.GrafanaHost != "" {
		return opt.HubCluster.GrafanaHost
	}

	if opt.HubCluster.GrafanaURL != "" {
		if u, err := url.Parse(opt.HubCluster.GrafanaURL); err == nil && u.Host != "" {
			return u.Host
		}
	}

	grafanaHostMutex.Lock()
	defer grafanaHostMutex.Unlock()

	if cachedGrafanaHost != "" {
		return cachedGrafanaHost
	}

	if os.Getenv("IS_KIND_ENV") != "true" {
		if config, err := LoadConfig(opt.HubCluster.ClusterServerURL, opt.KubeConfig, opt.HubCluster.KubeContext); err == nil && config != nil {
			if dynClient, err := dynamic.NewForConfig(config); err == nil && dynClient != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if route, err := dynClient.Resource(NewRouteGVR()).Namespace(MCO_NAMESPACE).Get(ctx, "grafana", metav1.GetOptions{}); err == nil && route != nil {
					if host, found, _ := unstructured.NestedString(route.Object, "spec", "host"); found && host != "" {
						cachedGrafanaHost = host
						return cachedGrafanaHost
					}
				}
			}
		}
	}

	cloudProvider := strings.ToLower(os.Getenv("CLOUD_PROVIDER"))
	if strings.Contains(cloudProvider, "rosa") && strings.Contains(cloudProvider, "hcp") {
		if opt.HubCluster.BaseDomain != "" {
			return "grafana-open-cluster-management-observability.apps.rosa." + opt.HubCluster.BaseDomain
		}
	} else if opt.HubCluster.BaseDomain != "" {
		return "grafana-open-cluster-management-observability.apps." + opt.HubCluster.BaseDomain
	}

	return ""
}

// GetGrafanaURL resolves the base URL for accessing Grafana.
// If opt.HubCluster.GrafanaURL is set, it is returned directly.
// Otherwise, it constructs an https URL from the resolved Grafana host.
func GetGrafanaURL(opt TestOptions) string {
	if opt.HubCluster.GrafanaURL != "" {
		return opt.HubCluster.GrafanaURL
	}
	host := GetGrafanaHost(opt)
	if host != "" {
		return "https://" + host
	}
	return ""
}
